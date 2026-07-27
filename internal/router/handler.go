package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"inference.bupt.dev/llm-slo-orchestrator/internal/admission"
	"inference.bupt.dev/llm-slo-orchestrator/internal/api"
	"inference.bupt.dev/llm-slo-orchestrator/internal/clock"
	"inference.bupt.dev/llm-slo-orchestrator/internal/picker"
)

const maxRequestBodyBytes = 8 << 20

type TargetConfig struct {
	ID             string
	Model          string
	BaseURL        string
	Healthy        bool
	Ready          bool
	Running        int
	Waiting        int
	KVUsage        float64
	TTFTEWMA       time.Duration
	PrefixAffinity float64
}

type Handler struct {
	policy   api.Policy
	picker   picker.Picker
	targets  map[string]*target
	ordered  []*target
	logger   *log.Logger
	sequence atomic.Uint64
	metrics  metrics
	limiter  *admission.Limiter
	client   *http.Client
}

type target struct {
	config      TargetConfig
	baseURL     *url.URL
	outstanding atomic.Int64
	healthy     atomic.Bool
	ready       atomic.Bool
	checkedAt   atomic.Int64
	running     atomic.Int64
	waiting     atomic.Int64
	kvUsage     atomic.Uint64
	ttftEWMA    atomic.Int64
	metricsAt   atomic.Int64
}

type metrics struct {
	mu         sync.Mutex
	requests   map[metricKey]uint64
	admissions map[admissionMetricKey]uint64
}

type metricKey struct {
	Endpoint string
	Code     int
}

type admissionMetricKey struct {
	Priority string
	Reason   string
}

func New(policy api.Policy, selectedPicker picker.Picker, configs []TargetConfig, logger *log.Logger) (*Handler, error) {
	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("invalid policy: %w", err)
	}
	if selectedPicker == nil {
		return nil, errors.New("picker is required")
	}
	if len(configs) == 0 {
		return nil, errors.New("at least one backend target is required")
	}

	handler := &Handler{
		policy:  policy,
		picker:  selectedPicker,
		targets: make(map[string]*target, len(configs)),
		logger:  logger,
		metrics: metrics{requests: make(map[metricKey]uint64), admissions: make(map[admissionMetricKey]uint64)},
		client: &http.Client{Transport: http.DefaultTransport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}
	limiter, err := admission.New(policy.Fairness.TenantRatePerSecond, policy.Fairness.TenantBurst, clock.Real{})
	if err != nil {
		return nil, fmt.Errorf("create tenant limiter: %w", err)
	}
	handler.limiter = limiter
	for _, config := range configs {
		if config.ID == "" {
			return nil, errors.New("backend target ID is required")
		}
		if _, exists := handler.targets[config.ID]; exists {
			return nil, fmt.Errorf("duplicate backend target ID %q", config.ID)
		}
		if config.Model == "" {
			config.Model = policy.Model
		}
		baseURL, err := url.Parse(config.BaseURL)
		if err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
			return nil, fmt.Errorf("backend %q has invalid URL %q", config.ID, config.BaseURL)
		}
		item := &target{config: config, baseURL: baseURL}
		item.healthy.Store(config.Healthy)
		item.ready.Store(config.Ready)
		item.running.Store(int64(config.Running))
		item.waiting.Store(int64(config.Waiting))
		item.kvUsage.Store(math.Float64bits(config.KVUsage))
		item.ttftEWMA.Store(int64(config.TTFTEWMA))
		item.metricsAt.Store(time.Now().UnixNano())
		handler.targets[config.ID] = item
		handler.ordered = append(handler.ordered, item)
	}
	sort.Slice(handler.ordered, func(i, j int) bool {
		return handler.ordered[i].config.ID < handler.ordered[j].config.ID
	})
	return handler, nil
}

type EndpointMetrics struct {
	Running, Waiting int
	KVUsage          float64
	TTFTEWMA         time.Duration
	CollectedAt      time.Time
}

func (h *Handler) SetEndpointMetrics(endpointID string, update EndpointMetrics) error {
	selected, exists := h.targets[endpointID]
	if !exists {
		return fmt.Errorf("unknown backend target %q", endpointID)
	}
	if update.Running < 0 || update.Waiting < 0 || update.KVUsage < 0 || update.KVUsage > 1 {
		return errors.New("endpoint metrics are invalid")
	}
	if update.CollectedAt.IsZero() {
		update.CollectedAt = time.Now()
	}
	selected.running.Store(int64(update.Running))
	selected.waiting.Store(int64(update.Waiting))
	selected.kvUsage.Store(math.Float64bits(update.KVUsage))
	if update.TTFTEWMA > 0 {
		selected.ttftEWMA.Store(int64(update.TTFTEWMA))
	}
	selected.metricsAt.Store(update.CollectedAt.UnixNano())
	return nil
}

func (h *Handler) SetEndpointHealth(endpointID string, healthy bool, checkedAt time.Time) error {
	selected, exists := h.targets[endpointID]
	if !exists {
		return fmt.Errorf("unknown backend target %q", endpointID)
	}
	if checkedAt.IsZero() {
		checkedAt = time.Now()
	}
	previous := selected.healthy.Swap(healthy)
	selected.checkedAt.Store(checkedAt.UnixNano())
	if previous != healthy && h.logger != nil {
		h.logger.Printf("endpoint=%s healthy=%t", endpointID, healthy)
	}
	return nil
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/health":
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("ok\n"))
		return
	case "/metrics":
		h.serveMetrics(writer)
		return
	}
	if !strings.HasPrefix(request.URL.Path, "/v1/") {
		writeJSONError(writer, http.StatusNotFound, "route not found")
		return
	}
	if request.Method != http.MethodPost && request.Method != http.MethodGet {
		writeJSONError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sequence := h.sequence.Add(1)
	routingRequest, err := h.routingRequest(writer, request, sequence)
	if err != nil {
		writeJSONError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if allowed, retryAfter := h.limiter.Allow(routingRequest.Tenant); !allowed {
		writer.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
		h.metrics.recordAdmission(string(routingRequest.Priority), "tenant-rate-limit")
		writeJSONError(writer, http.StatusTooManyRequests, "tenant rate limit exceeded")
		return
	}
	snapshot := h.snapshot(sequence - 1)
	decision, err := h.picker.Pick(routingRequest, snapshot, h.policy)
	if err != nil {
		if errors.Is(err, picker.ErrNoEligibleEndpoint) {
			writeJSONError(writer, http.StatusServiceUnavailable, "no eligible backend")
			return
		}
		writeJSONError(writer, http.StatusInternalServerError, "routing decision failed")
		return
	}
	if decision.Admission == "reject" {
		writer.Header().Set("Retry-After", strconv.Itoa(int(h.policy.Admission.RetryAfter.Seconds())))
		h.metrics.recordAdmission(string(routingRequest.Priority), "predicted-slo-violation")
		writeJSONError(writer, http.StatusTooManyRequests, "request rejected by admission policy")
		return
	}

	request.Header.Set("X-Request-ID", routingRequest.ID)
	h.serveDecision(writer, request, routingRequest, snapshot, decision)
}

func (h *Handler) serveDecision(writer http.ResponseWriter, request *http.Request, routingRequest api.Request, snapshot api.EndpointSnapshot, decision api.Decision) {
	for attempt := 0; attempt < 2; attempt++ {
		selected := h.targets[decision.SelectedEndpoint]
		selected.outstanding.Add(1)
		status, started, err := h.forward(writer, request, selected, routingRequest, decision, attempt)
		selected.outstanding.Add(-1)
		if err == nil || started {
			h.metrics.record(decision.SelectedEndpoint, status)
			h.logAttempt(routingRequest.ID, decision, status, attempt, err)
			return
		}
		h.metrics.record(decision.SelectedEndpoint, http.StatusBadGateway)
		h.logAttempt(routingRequest.ID, decision, http.StatusBadGateway, attempt, err)
		if attempt == 1 {
			break
		}
		retrySnapshot := snapshot
		retrySnapshot.Generation++
		retrySnapshot.Endpoints = append([]api.Endpoint(nil), snapshot.Endpoints...)
		for index := range retrySnapshot.Endpoints {
			if retrySnapshot.Endpoints[index].ID == decision.SelectedEndpoint {
				retrySnapshot.Endpoints[index].Healthy = false
			}
		}
		retryDecision, pickErr := h.picker.Pick(routingRequest, retrySnapshot, h.policy)
		if pickErr != nil || retryDecision.Admission == "reject" {
			break
		}
		decision = retryDecision
	}
	writeJSONError(writer, http.StatusBadGateway, "backend request failed before response headers")
}

func (h *Handler) forward(writer http.ResponseWriter, request *http.Request, selected *target, routingRequest api.Request, decision api.Decision, attempt int) (int, bool, error) {
	outgoing := request.Clone(request.Context())
	outgoing.URL.Scheme = selected.baseURL.Scheme
	outgoing.URL.Host = selected.baseURL.Host
	outgoing.URL.Path = strings.TrimRight(selected.baseURL.Path, "/") + request.URL.Path
	outgoing.RequestURI = ""
	outgoing.Host = selected.baseURL.Host
	outgoing.Header = request.Header.Clone()
	removeHopHeaders(outgoing.Header)
	if request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return 0, false, err
		}
		outgoing.Body = body
	}
	response, err := h.client.Do(outgoing)
	if err != nil {
		return 0, false, err
	}
	defer response.Body.Close()
	removeHopHeaders(response.Header)
	for key, values := range response.Header {
		for _, value := range values {
			writer.Header().Add(key, value)
		}
	}
	writer.Header().Set("X-LLM-Endpoint", decision.SelectedEndpoint)
	writer.Header().Set("X-LLM-Request-ID", routingRequest.ID)
	writer.Header().Set("X-LLM-Decision-Reason", decision.Reason)
	writer.Header().Set("X-LLM-Policy-Generation", strconv.FormatUint(decision.PolicyGeneration, 10))
	writer.Header().Set("X-LLM-Retry-Count", strconv.Itoa(attempt))
	writer.WriteHeader(response.StatusCode)
	err = copyResponse(writer, response.Body, strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream"))
	return response.StatusCode, true, err
}

func (h *Handler) logAttempt(requestID string, decision api.Decision, status, attempt int, err error) {
	if h.logger != nil {
		h.logger.Printf("request_id=%s endpoint=%s status=%d reason=%s candidates=%d retry=%d error=%v", requestID, decision.SelectedEndpoint, status, decision.Reason, decision.CandidateCount, attempt, err)
	}
}

func (h *Handler) routingRequest(
	writer http.ResponseWriter,
	request *http.Request,
	sequence uint64,
) (api.Request, error) {
	requestID := request.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = fmt.Sprintf("router-%012d", sequence)
	}
	priority := api.Priority(request.Header.Get("X-Request-Priority"))
	if priority == "" {
		priority = api.PriorityOnlineNormal
	}
	if priority != api.PriorityOnlineHigh && priority != api.PriorityOnlineNormal && priority != api.PriorityBatchLow {
		return api.Request{}, fmt.Errorf("unsupported request priority %q", priority)
	}
	tenant := request.Header.Get("X-Tenant-ID")
	if tenant == "" {
		tenant = "default"
	}
	if len(tenant) > 128 {
		return api.Request{}, errors.New("tenant ID is too long")
	}

	result := api.Request{ID: requestID, Tenant: tenant, Priority: priority}
	if deadline, ok := request.Context().Deadline(); ok {
		result.Deadline = deadline
	}
	if request.Method == http.MethodGet {
		result.Model = h.policy.Model
		return result, nil
	}

	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maxRequestBodyBytes))
	if err != nil {
		return api.Request{}, errors.New("request body is invalid or too large")
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	request.ContentLength = int64(len(body))
	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return api.Request{}, errors.New("request body must be valid JSON")
	}
	result.Model = payload.Model
	result.PromptTokens = len(body) / 4
	result.PrefixKey = request.Header.Get("X-Prefix-Key")
	return result, nil
}

func (h *Handler) snapshot(generation uint64) api.EndpointSnapshot {
	endpoints := make([]api.Endpoint, 0, len(h.ordered))
	for _, item := range h.ordered {
		outstanding := item.outstanding.Load()
		metricsAt := time.Unix(0, item.metricsAt.Load())
		endpoints = append(endpoints, api.Endpoint{
			ID:             item.config.ID,
			Model:          item.config.Model,
			Healthy:        item.healthy.Load(),
			Ready:          item.ready.Load(),
			MetricAge:      time.Since(metricsAt),
			Running:        int(item.running.Load()),
			Waiting:        int(item.waiting.Load()),
			Outstanding:    int(outstanding),
			KVUsage:        math.Float64frombits(item.kvUsage.Load()),
			TTFTEWMA:       time.Duration(item.ttftEWMA.Load()),
			PrefixAffinity: item.config.PrefixAffinity,
		})
	}
	return api.EndpointSnapshot{Generation: generation, CollectedAt: time.Now(), Endpoints: endpoints}
}

func (h *Handler) serveMetrics(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprintf(writer, "# HELP router_requests_total Requests completed by backend and HTTP status.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE router_requests_total counter\n")
	h.metrics.mu.Lock()
	keys := make([]metricKey, 0, len(h.metrics.requests))
	for key := range h.metrics.requests {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Endpoint == keys[j].Endpoint {
			return keys[i].Code < keys[j].Code
		}
		return keys[i].Endpoint < keys[j].Endpoint
	})
	for _, key := range keys {
		_, _ = fmt.Fprintf(
			writer,
			"router_requests_total{endpoint=%q,code=%q} %d\n",
			key.Endpoint,
			strconv.Itoa(key.Code),
			h.metrics.requests[key],
		)
	}
	h.metrics.mu.Unlock()
	_, _ = fmt.Fprintf(writer, "# HELP router_admission_total Requests rejected by priority and bounded reason.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE router_admission_total counter\n")
	h.metrics.mu.Lock()
	admissionKeys := make([]admissionMetricKey, 0, len(h.metrics.admissions))
	for key := range h.metrics.admissions {
		admissionKeys = append(admissionKeys, key)
	}
	sort.Slice(admissionKeys, func(i, j int) bool {
		if admissionKeys[i].Priority == admissionKeys[j].Priority {
			return admissionKeys[i].Reason < admissionKeys[j].Reason
		}
		return admissionKeys[i].Priority < admissionKeys[j].Priority
	})
	for _, key := range admissionKeys {
		_, _ = fmt.Fprintf(writer, "router_admission_total{priority=%q,reason=%q} %d\n", key.Priority, key.Reason, h.metrics.admissions[key])
	}
	h.metrics.mu.Unlock()
	_, _ = fmt.Fprintf(writer, "# HELP router_backend_healthy Whether the latest thresholded backend health state is healthy.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE router_backend_healthy gauge\n")
	for _, item := range h.ordered {
		healthy := 0
		if item.healthy.Load() {
			healthy = 1
		}
		_, _ = fmt.Fprintf(
			writer,
			"router_backend_healthy{endpoint=%q} %d\n",
			item.config.ID,
			healthy,
		)
	}
	_, _ = fmt.Fprintf(writer, "# HELP router_backend_health_check_age_seconds Seconds since the latest backend health check.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE router_backend_health_check_age_seconds gauge\n")
	now := time.Now()
	for _, item := range h.ordered {
		checkedAt := item.checkedAt.Load()
		age := 0.0
		if checkedAt > 0 {
			age = now.Sub(time.Unix(0, checkedAt)).Seconds()
			if age < 0 {
				age = 0
			}
		}
		_, _ = fmt.Fprintf(
			writer,
			"router_backend_health_check_age_seconds{endpoint=%q} %.6f\n",
			item.config.ID,
			age,
		)
	}
	_, _ = fmt.Fprintf(writer, "# HELP router_outstanding_requests Requests currently assigned to a backend.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE router_outstanding_requests gauge\n")
	for _, item := range h.ordered {
		_, _ = fmt.Fprintf(
			writer,
			"router_outstanding_requests{endpoint=%q} %d\n",
			item.config.ID,
			item.outstanding.Load(),
		)
	}
}

func (m *metrics) record(endpoint string, code int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[metricKey{Endpoint: endpoint, Code: code}]++
}

func (m *metrics) recordAdmission(priority, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.admissions[admissionMetricKey{Priority: priority, Reason: reason}]++
}

var hopHeaders = []string{
	"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
	"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
}

func removeHopHeaders(header http.Header) {
	for _, value := range header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			header.Del(strings.TrimSpace(token))
		}
	}
	for _, key := range hopHeaders {
		header.Del(key)
	}
}

func copyResponse(writer http.ResponseWriter, reader io.Reader, flush bool) error {
	buffer := make([]byte, 32*1024)
	for {
		count, readErr := reader.Read(buffer)
		if count > 0 {
			if _, writeErr := writer.Write(buffer[:count]); writeErr != nil {
				return writeErr
			}
			if flush {
				if flusher, ok := writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

func writeJSONError(writer http.ResponseWriter, status int, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"error": map[string]string{
			"message": message,
			"type":    "router_error",
		},
	})
}
