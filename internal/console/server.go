package console

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	routerserver "github.com/bupt20220407/llm-slo-orchestrator/internal/router"
)

const (
	maxChatBodyBytes = 2 << 20
	maxJSONBodyBytes = 8 << 10
	maxEvidenceBytes = 2 << 20
)

type Config struct {
	RouterURL          string
	DashboardURL       string
	RepositoryRoot     string
	Model              string
	DemoMode           bool
	ControlToken       string
	RouterControlToken string
	HTTPClient         *http.Client
	Runner             CommandRunner
}

type Server struct {
	routerURL          *url.URL
	dashboardURL       string
	repositoryRoot     string
	model              string
	demoMode           bool
	controlToken       string
	routerControlToken string
	client             *http.Client
	jobs               *JobManager
	ui                 fs.FS
}

type statusResponse struct {
	Status        string              `json:"status"`
	CheckedAt     time.Time           `json:"checkedAt"`
	Router        routerserver.Status `json:"router"`
	RouterError   string              `json:"routerError,omitempty"`
	Console       consoleCapabilities `json:"console"`
	EvidenceCount int                 `json:"evidenceCount"`
}

type consoleCapabilities struct {
	DemoMode         bool   `json:"demoMode"`
	ControlsEnabled  bool   `json:"controlsEnabled"`
	StrategyControl  bool   `json:"strategyControl"`
	ScenarioControl  bool   `json:"scenarioControl"`
	DashboardURL     string `json:"dashboardUrl"`
	EvidenceBoundary string `json:"evidenceBoundary"`
}

type evidenceItem struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Kind          string `json:"kind"`
	Status        string `json:"status,omitempty"`
	EvidenceLevel string `json:"evidenceLevel,omitempty"`
	UpdatedAt     string `json:"updatedAt"`
	Size          int64  `json:"size"`
}

func New(config Config) (*Server, error) {
	routerURL, err := url.Parse(config.RouterURL)
	if err != nil || (routerURL.Scheme != "http" && routerURL.Scheme != "https") || routerURL.Host == "" {
		return nil, fmt.Errorf("invalid router URL %q", config.RouterURL)
	}
	repositoryRoot, err := filepath.Abs(config.RepositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	if config.DemoMode && strings.TrimSpace(config.ControlToken) == "" {
		return nil, errors.New("demo mode requires a control token")
	}
	if config.DashboardURL != "" {
		dashboardURL, dashboardErr := url.Parse(config.DashboardURL)
		if dashboardErr != nil || (dashboardURL.Scheme != "http" && dashboardURL.Scheme != "https") || dashboardURL.Host == "" {
			return nil, fmt.Errorf("invalid dashboard URL %q", config.DashboardURL)
		}
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Transport: http.DefaultTransport}
	}
	ui, err := uiFileSystem()
	if err != nil {
		return nil, err
	}
	server := &Server{
		routerURL:          routerURL,
		dashboardURL:       config.DashboardURL,
		repositoryRoot:     repositoryRoot,
		model:              config.Model,
		demoMode:           config.DemoMode,
		controlToken:       config.ControlToken,
		routerControlToken: config.RouterControlToken,
		client:             client,
		ui:                 ui,
	}
	server.jobs = NewJobManager(repositoryRoot, config.Runner)
	return server, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/chat/completions", s.handleChat)
	mux.HandleFunc("/api/v1/evidence", s.handleEvidence)
	mux.HandleFunc("/api/v1/evidence/file", s.handleEvidenceFile)
	mux.HandleFunc("/api/v1/control/strategy", s.handleStrategy)
	mux.HandleFunc("/api/v1/scenarios", s.handleScenarios)
	mux.HandleFunc("/api/v1/scenarios/", s.handleScenario)
	mux.HandleFunc("/api/v1/jobs/", s.handleJob)
	mux.Handle("/", spaHandler(s.ui))
	return securityHeaders(mux)
}

func (s *Server) handleStatus(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()
	status, routerErr := s.fetchRouterStatus(ctx)
	evidence, evidenceErr := s.listEvidence()
	response := statusResponse{
		Status:    "ok",
		CheckedAt: time.Now().UTC(),
		Router:    status,
		Console: consoleCapabilities{
			DemoMode:         s.demoMode,
			ControlsEnabled:  s.demoMode && s.controlToken != "",
			StrategyControl:  s.demoMode && s.routerControlToken != "",
			ScenarioControl:  s.demoMode,
			DashboardURL:     s.dashboardURL,
			EvidenceBoundary: "H1-H3 policy benefits require two independent real accelerator endpoints (L3); H4 controlled mixed-priority evidence remains separate.",
		},
	}
	if routerErr != nil {
		response.Status = "degraded"
		response.RouterError = routerErr.Error()
		response.Router.Status = "unreachable"
		response.Router.Model = s.model
	}
	if evidenceErr == nil {
		response.EvidenceCount = len(evidence)
	}
	writeJSON(writer, http.StatusOK, response)
}

func (s *Server) fetchRouterStatus(ctx context.Context) (routerserver.Status, error) {
	endpoint := *s.routerURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v1/router/status"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return routerserver.Status{}, err
	}
	response, err := s.client.Do(request)
	if err != nil {
		return routerserver.Status{}, fmt.Errorf("router status request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return routerserver.Status{}, fmt.Errorf("router status returned HTTP %d", response.StatusCode)
	}
	var status routerserver.Status
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxJSONBodyBytes))
	if err := decoder.Decode(&status); err != nil {
		return routerserver.Status{}, fmt.Errorf("decode router status: %w", err)
	}
	return status, nil
}

func (s *Server) handleChat(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maxChatBodyBytes))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "request body is invalid or too large")
		return
	}
	var payload struct {
		Model    string            `json:"model"`
		Stream   bool              `json:"stream"`
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || len(payload.Messages) == 0 {
		writeError(writer, http.StatusBadRequest, "a valid chat completion request with messages is required")
		return
	}
	if payload.Model == "" {
		writeError(writer, http.StatusBadRequest, "model is required")
		return
	}
	priority := request.Header.Get("X-Request-Priority")
	if priority != "" && priority != "online-high" && priority != "online-normal" && priority != "batch-low" {
		writeError(writer, http.StatusBadRequest, "unsupported request priority")
		return
	}
	tenant := request.Header.Get("X-Tenant-ID")
	if len(tenant) > 128 {
		writeError(writer, http.StatusBadRequest, "tenant ID is too long")
		return
	}
	endpoint := *s.routerURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v1/chat/completions"
	outgoing, err := http.NewRequestWithContext(request.Context(), http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not prepare router request")
		return
	}
	outgoing.Header.Set("Content-Type", "application/json")
	copyRequestHeader(outgoing.Header, request.Header, "X-Tenant-ID")
	copyRequestHeader(outgoing.Header, request.Header, "X-Request-Priority")
	copyRequestHeader(outgoing.Header, request.Header, "X-Prefix-Key")
	copyRequestHeader(outgoing.Header, request.Header, "X-Request-ID")
	response, err := s.client.Do(outgoing)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "router is unavailable")
		return
	}
	defer response.Body.Close()
	copyResponseHeader(writer.Header(), response.Header, "Content-Type")
	copyResponseHeader(writer.Header(), response.Header, "Retry-After")
	copyResponseHeader(writer.Header(), response.Header, "X-LLM-Endpoint")
	copyResponseHeader(writer.Header(), response.Header, "X-LLM-Request-ID")
	copyResponseHeader(writer.Header(), response.Header, "X-LLM-Decision-Reason")
	copyResponseHeader(writer.Header(), response.Header, "X-LLM-Policy-Generation")
	copyResponseHeader(writer.Header(), response.Header, "X-LLM-Retry-Count")
	writer.WriteHeader(response.StatusCode)
	buffer := make([]byte, 32*1024)
	for {
		count, readErr := response.Body.Read(buffer)
		if count > 0 {
			if _, writeErr := writer.Write(buffer[:count]); writeErr != nil {
				return
			}
			if payload.Stream {
				if flusher, ok := writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if readErr != nil {
			return
		}
	}
}

func (s *Server) handleEvidence(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	items, err := s.listEvidence()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not read evidence index")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) listEvidence() ([]evidenceItem, error) {
	var items []evidenceItem
	reportRoot := filepath.Join(s.repositoryRoot, "docs", "reports")
	reports, err := filepath.Glob(filepath.Join(reportRoot, "*.md"))
	if err != nil {
		return nil, err
	}
	for _, path := range reports {
		item, itemErr := s.evidenceMetadata(path, "report")
		if itemErr == nil {
			items = append(items, item)
		}
	}
	evidenceRoot := filepath.Join(s.repositoryRoot, "evidence")
	err = filepath.WalkDir(evidenceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "summary.json" {
			return nil
		}
		item, itemErr := s.evidenceMetadata(path, "evidence")
		if itemErr == nil {
			items = append(items, item)
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind == items[j].Kind {
			return items[i].ID < items[j].ID
		}
		return items[i].Kind < items[j].Kind
	})
	return items, nil
}

func (s *Server) evidenceMetadata(path, kind string) (evidenceItem, error) {
	info, err := os.Stat(path)
	if err != nil {
		return evidenceItem{}, err
	}
	relative, err := filepath.Rel(s.repositoryRoot, path)
	if err != nil {
		return evidenceItem{}, err
	}
	id := filepath.ToSlash(relative)
	item := evidenceItem{ID: id, Title: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), Kind: kind, UpdatedAt: info.ModTime().UTC().Format(time.RFC3339), Size: info.Size()}
	content, err := os.ReadFile(path)
	if err != nil {
		return evidenceItem{}, err
	}
	if kind == "report" {
		for _, line := range strings.Split(string(content), "\n") {
			if strings.HasPrefix(line, "# ") {
				item.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
				break
			}
		}
	} else {
		var summary struct {
			Status        string `json:"status"`
			EvidenceLevel string `json:"evidence_level"`
		}
		if json.Unmarshal(content, &summary) == nil {
			item.Status = summary.Status
			item.EvidenceLevel = summary.EvidenceLevel
		}
		item.Title = filepath.Base(filepath.Dir(path))
	}
	return item, nil
}

func (s *Server) handleEvidenceFile(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path, err := s.resolveEvidence(request.URL.Query().Get("id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid evidence ID")
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeError(writer, http.StatusNotFound, "evidence file not found")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() > maxEvidenceBytes {
		writeError(writer, http.StatusRequestEntityTooLarge, "evidence file is too large")
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "text/plain; charset=utf-8"
	}
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(writer, io.LimitReader(file, maxEvidenceBytes))
}

func (s *Server) resolveEvidence(id string) (string, error) {
	if id == "" || strings.ContainsRune(id, '\x00') {
		return "", errors.New("empty evidence ID")
	}
	clean := filepath.Clean(filepath.FromSlash(id))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("evidence path escapes repository")
	}
	reportRoot := filepath.Join("docs", "reports") + string(filepath.Separator)
	evidenceRoot := "evidence" + string(filepath.Separator)
	allowedReport := strings.HasPrefix(clean, reportRoot) && filepath.Dir(clean) == filepath.Join("docs", "reports") && filepath.Ext(clean) == ".md"
	allowedSummary := strings.HasPrefix(clean, evidenceRoot) && filepath.Base(clean) == "summary.json"
	allowed := allowedReport || allowedSummary
	if !allowed {
		return "", errors.New("evidence path is outside allowed roots")
	}
	return filepath.Join(s.repositoryRoot, clean), nil
}

func (s *Server) handleStrategy(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.authorizeControl(writer, request) {
		return
	}
	if s.routerControlToken == "" {
		writeError(writer, http.StatusConflict, "router strategy control is not configured")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maxJSONBodyBytes))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid strategy request")
		return
	}
	var payload struct {
		Strategy string `json:"strategy"`
	}
	if json.Unmarshal(body, &payload) != nil || !supportedStrategy(payload.Strategy) {
		writeError(writer, http.StatusBadRequest, "unsupported routing strategy")
		return
	}
	endpoint := *s.routerURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/admin/v1/strategy"
	outgoing, err := http.NewRequestWithContext(request.Context(), http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not prepare control request")
		return
	}
	outgoing.Header.Set("Authorization", "Bearer "+s.routerControlToken)
	outgoing.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(outgoing)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "router control API is unavailable")
		return
	}
	defer response.Body.Close()
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(response.StatusCode)
	_, _ = io.Copy(writer, io.LimitReader(response.Body, maxJSONBodyBytes))
}

func supportedStrategy(strategy string) bool {
	switch strategy {
	case "round-robin", "random", "least-outstanding", "least-queue", "slo-prefix-affinity":
		return true
	default:
		return false
	}
}

func (s *Server) authorizeControl(writer http.ResponseWriter, request *http.Request) bool {
	if !s.demoMode {
		writeError(writer, http.StatusNotFound, "demo controls are disabled")
		return false
	}
	if !sameOrigin(request) {
		writeError(writer, http.StatusForbidden, "cross-origin control request rejected")
		return false
	}
	value, ok := strings.CutPrefix(request.Header.Get("Authorization"), "Bearer ")
	if !ok || !secureEqual(value, s.controlToken) {
		writeError(writer, http.StatusUnauthorized, "invalid control token")
		return false
	}
	return true
}

func sameOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host == request.Host && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func secureEqual(actual, expected string) bool {
	actualHash := sha256.Sum256([]byte(actual))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(actualHash[:], expectedHash[:]) == 1
}

func copyRequestHeader(destination, source http.Header, key string) {
	if value := source.Get(key); value != "" {
		destination.Set(key, value)
	}
}

func copyResponseHeader(destination, source http.Header, key string) {
	if value := source.Get(key); value != "" {
		destination.Set(key, value)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data:; script-src 'self'; style-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(writer, request)
	})
}

func spaHandler(files fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		path := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(request.URL.Path)), "/")
		if path == "" || path == "." {
			path = "index.html"
		}
		if _, err := fs.Stat(files, path); err != nil {
			request.URL.Path = "/"
		}
		fileServer.ServeHTTP(writer, request)
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]any{"error": map[string]string{"message": message, "type": "console_error"}})
}
