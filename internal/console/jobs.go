package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const maxJobLogLines = 2000

type Scenario struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Command     []string `json:"-"`
	Risk        string   `json:"risk"`
}

type Job struct {
	ID         string             `json:"id"`
	ScenarioID string             `json:"scenarioId"`
	Status     string             `json:"status"`
	CreatedAt  time.Time          `json:"createdAt"`
	StartedAt  *time.Time         `json:"startedAt,omitempty"`
	EndedAt    *time.Time         `json:"endedAt,omitempty"`
	Error      string             `json:"error,omitempty"`
	Logs       []string           `json:"logs"`
	Cancel     context.CancelFunc `json:"-"`
}

type JobEvent struct {
	Type string `json:"type"`
	Line string `json:"line,omitempty"`
	Job  *Job   `json:"job,omitempty"`
}

type CommandRunner func(ctx context.Context, directory string, command []string, output io.Writer) error

type JobManager struct {
	mu          sync.Mutex
	directory   string
	runner      CommandRunner
	scenarios   map[string]Scenario
	jobs        map[string]*Job
	order       []string
	active      string
	subscribers map[string]map[chan JobEvent]struct{}
	sequence    uint64
}

func NewJobManager(directory string, runner CommandRunner) *JobManager {
	if runner == nil {
		runner = runCommand
	}
	scenarios := []Scenario{
		{ID: "verify", Name: "Repository verification", Description: "Run formatting, tests, race checks, builds, repository gates, and report validation.", Command: []string{"make", "verify"}, Risk: "read-mostly"},
		{ID: "fault-smoke", Name: "Failure and recovery", Description: "Run the deterministic fake-backend failure, survivor traffic, and recovery campaign.", Command: []string{"make", "fault-smoke"}, Risk: "local-processes"},
		{ID: "core-demo", Name: "Core demonstration", Description: "Run the complete local L0/L1 demonstration without a real accelerator endpoint.", Command: []string{"python3", "hack/demo.py"}, Risk: "local-processes"},
	}
	indexed := make(map[string]Scenario, len(scenarios))
	for _, scenario := range scenarios {
		indexed[scenario.ID] = scenario
	}
	return &JobManager{directory: directory, runner: runner, scenarios: indexed, jobs: make(map[string]*Job), subscribers: make(map[string]map[chan JobEvent]struct{})}
}

func runCommand(ctx context.Context, directory string, command []string, output io.Writer) error {
	if len(command) == 0 {
		return errors.New("empty command")
	}
	process := exec.CommandContext(ctx, command[0], command[1:]...)
	process.Dir = directory
	process.Stdout = output
	process.Stderr = output
	return process.Run()
}

func (manager *JobManager) Scenarios() []Scenario {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	result := make([]Scenario, 0, len(manager.scenarios))
	for _, id := range []string{"verify", "fault-smoke", "core-demo"} {
		result = append(result, manager.scenarios[id])
	}
	return result
}

func (manager *JobManager) Start(scenarioID string) (Job, error) {
	manager.mu.Lock()
	scenario, ok := manager.scenarios[scenarioID]
	if !ok {
		manager.mu.Unlock()
		return Job{}, fmt.Errorf("unknown scenario %q", scenarioID)
	}
	if manager.active != "" {
		active := manager.active
		manager.mu.Unlock()
		return Job{}, fmt.Errorf("job %s is already running", active)
	}
	manager.sequence++
	jobID := fmt.Sprintf("job-%d-%03d", time.Now().UTC().Unix(), manager.sequence)
	ctx, cancel := context.WithCancel(context.Background())
	job := &Job{ID: jobID, ScenarioID: scenarioID, Status: "queued", CreatedAt: time.Now().UTC(), Logs: []string{}, Cancel: cancel}
	manager.jobs[jobID] = job
	manager.order = append(manager.order, jobID)
	manager.active = jobID
	manager.trimLocked()
	snapshot := cloneJob(job)
	manager.mu.Unlock()
	go manager.execute(ctx, scenario, jobID)
	return snapshot, nil
}

func (manager *JobManager) execute(ctx context.Context, scenario Scenario, jobID string) {
	started := time.Now().UTC()
	manager.update(jobID, func(job *Job) {
		job.Status = "running"
		job.StartedAt = &started
	})
	manager.appendLog(jobID, "$ "+strings.Join(scenario.Command, " "))
	writer := newLineWriter(func(line string) { manager.appendLog(jobID, line) })
	err := manager.runner(ctx, manager.directory, append([]string(nil), scenario.Command...), writer)
	writer.Flush()
	ended := time.Now().UTC()
	manager.update(jobID, func(job *Job) {
		job.EndedAt = &ended
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			job.Status = "cancelled"
			job.Error = "cancelled by operator"
		case err != nil:
			job.Status = "failed"
			job.Error = err.Error()
		default:
			job.Status = "passed"
		}
		manager.active = ""
	})
}

func (manager *JobManager) Get(id string) (Job, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	job, ok := manager.jobs[id]
	if !ok {
		return Job{}, false
	}
	return cloneJob(job), true
}

func (manager *JobManager) Cancel(id string) (Job, error) {
	manager.mu.Lock()
	job, ok := manager.jobs[id]
	if !ok {
		manager.mu.Unlock()
		return Job{}, errors.New("job not found")
	}
	if job.Status != "queued" && job.Status != "running" {
		snapshot := cloneJob(job)
		manager.mu.Unlock()
		return snapshot, errors.New("job is not running")
	}
	cancel := job.Cancel
	snapshot := cloneJob(job)
	manager.mu.Unlock()
	cancel()
	return snapshot, nil
}

func (manager *JobManager) Subscribe(id string) (Job, <-chan JobEvent, func(), bool) {
	manager.mu.Lock()
	job, ok := manager.jobs[id]
	if !ok {
		manager.mu.Unlock()
		return Job{}, nil, nil, false
	}
	channel := make(chan JobEvent, 32)
	if manager.subscribers[id] == nil {
		manager.subscribers[id] = make(map[chan JobEvent]struct{})
	}
	manager.subscribers[id][channel] = struct{}{}
	snapshot := cloneJob(job)
	manager.mu.Unlock()
	unsubscribe := func() {
		manager.mu.Lock()
		if subscribers := manager.subscribers[id]; subscribers != nil {
			delete(subscribers, channel)
		}
		manager.mu.Unlock()
	}
	return snapshot, channel, unsubscribe, true
}

func (manager *JobManager) appendLog(id, line string) {
	manager.mu.Lock()
	job, ok := manager.jobs[id]
	if !ok {
		manager.mu.Unlock()
		return
	}
	line = strings.TrimSuffix(line, "\r")
	job.Logs = append(job.Logs, line)
	if len(job.Logs) > maxJobLogLines {
		job.Logs = append([]string(nil), job.Logs[len(job.Logs)-maxJobLogLines:]...)
	}
	manager.broadcastLocked(id, JobEvent{Type: "log", Line: line})
	manager.mu.Unlock()
}

func (manager *JobManager) update(id string, update func(*Job)) {
	manager.mu.Lock()
	job, ok := manager.jobs[id]
	if ok {
		update(job)
		snapshot := cloneJob(job)
		manager.broadcastLocked(id, JobEvent{Type: "state", Job: &snapshot})
	}
	manager.mu.Unlock()
}

func (manager *JobManager) broadcastLocked(id string, event JobEvent) {
	for subscriber := range manager.subscribers[id] {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func (manager *JobManager) trimLocked() {
	for len(manager.order) > 32 {
		id := manager.order[0]
		if id == manager.active {
			return
		}
		manager.order = manager.order[1:]
		delete(manager.jobs, id)
		delete(manager.subscribers, id)
	}
}

func cloneJob(job *Job) Job {
	result := *job
	result.Logs = append([]string(nil), job.Logs...)
	result.Cancel = nil
	return result
}

type lineWriter struct {
	mu     sync.Mutex
	buffer string
	onLine func(string)
}

func newLineWriter(onLine func(string)) *lineWriter {
	return &lineWriter{onLine: onLine}
}

func (writer *lineWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	writer.buffer += string(data)
	for {
		index := strings.IndexByte(writer.buffer, '\n')
		if index < 0 {
			break
		}
		writer.onLine(strings.TrimSuffix(writer.buffer[:index], "\r"))
		writer.buffer = writer.buffer[index+1:]
	}
	return len(data), nil
}

func (writer *lineWriter) Flush() {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.buffer != "" {
		writer.onLine(strings.TrimSuffix(writer.buffer, "\r"))
		writer.buffer = ""
	}
}

func (s *Server) handleScenarios(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": s.jobs.Scenarios(), "enabled": s.demoMode})
}

func (s *Server) handleScenario(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/run") {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.authorizeControl(writer, request) {
		return
	}
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/scenarios/")
	id := strings.TrimSuffix(path, "/run")
	if id == "" || strings.Contains(id, "/") {
		writeError(writer, http.StatusNotFound, "scenario not found")
		return
	}
	job, err := s.jobs.Start(id)
	if err != nil {
		if strings.Contains(err.Error(), "already running") {
			writeError(writer, http.StatusConflict, err.Error())
			return
		}
		writeError(writer, http.StatusNotFound, "scenario not found")
		return
	}
	writeJSON(writer, http.StatusAccepted, job)
}

func (s *Server) handleJob(writer http.ResponseWriter, request *http.Request) {
	if !s.authorizeControl(writer, request) {
		return
	}
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/jobs/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(writer, http.StatusNotFound, "job not found")
		return
	}
	id := parts[0]
	if len(parts) == 1 && request.Method == http.MethodGet {
		job, ok := s.jobs.Get(id)
		if !ok {
			writeError(writer, http.StatusNotFound, "job not found")
			return
		}
		writeJSON(writer, http.StatusOK, job)
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" && request.Method == http.MethodPost {
		job, err := s.jobs.Cancel(id)
		if err != nil {
			writeError(writer, http.StatusConflict, err.Error())
			return
		}
		writeJSON(writer, http.StatusAccepted, job)
		return
	}
	if len(parts) == 2 && parts[1] == "events" && request.Method == http.MethodGet {
		s.serveJobEvents(writer, request, id)
		return
	}
	writeError(writer, http.StatusNotFound, "job route not found")
}

func (s *Server) serveJobEvents(writer http.ResponseWriter, request *http.Request, id string) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		writeError(writer, http.StatusInternalServerError, "streaming is unavailable")
		return
	}
	job, events, unsubscribe, ok := s.jobs.Subscribe(id)
	if !ok {
		writeError(writer, http.StatusNotFound, "job not found")
		return
	}
	defer unsubscribe()
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Accel-Buffering", "no")
	writeSSE(writer, JobEvent{Type: "state", Job: &job})
	flusher.Flush()
	if job.Status == "passed" || job.Status == "failed" || job.Status == "cancelled" {
		return
	}
	for {
		select {
		case <-request.Context().Done():
			return
		case event := <-events:
			writeSSE(writer, event)
			flusher.Flush()
			if event.Job != nil && (event.Job.Status == "passed" || event.Job.Status == "failed" || event.Job.Status == "cancelled") {
				return
			}
		}
	}
}

func writeSSE(writer io.Writer, event JobEvent) {
	data, _ := json.Marshal(event)
	_, _ = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event.Type, data)
}
