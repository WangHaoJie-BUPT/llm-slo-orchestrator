package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"inference.bupt.dev/llm-slo-orchestrator/internal/api"
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
}

type target struct {
	config      TargetConfig
	proxy       *httputil.ReverseProxy
	outstanding atomic.Int64
}

type metrics struct {
	mu       sync.Mutex
	requests map[metricKey]uint64
}

type metricKey struct {
	Endpoint string
	Code     int
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
		metrics: metrics{requests: make(map[metricKey]uint64)},
	}
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
		proxy := httputil.NewSingleHostReverseProxy(baseURL)
		proxy.FlushInterval = -1
		proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, proxyErr error) {
			writeJSONError(writer, http.StatusBadGateway, "backend request failed")
		}
		item := &target{config: config, proxy: proxy}
		handler.targets[config.ID] = item
		handler.ordered = append(handler.ordered, item)
	}
	sort.Slice(handler.ordered, func(i, j int) bool {
		return handler.ordered[i].config.ID < handler.ordered[j].config.ID
	})
	return handler, nil
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
		writeJSONError(writer, http.StatusTooManyRequests, "request rejected by admission policy")
		return
	}

	selected := h.targets[decision.SelectedEndpoint]
	selected.outstanding.Add(1)
	defer selected.outstanding.Add(-1)

	request.Header.Set("X-Request-ID", routingRequest.ID)
	writer.Header().Set("X-LLM-Endpoint", decision.SelectedEndpoint)
	writer.Header().Set("X-LLM-Request-ID", routingRequest.ID)
	writer.Header().Set("X-LLM-Decision-Reason", decision.Reason)
	writer.Header().Set("X-LLM-Policy-Generation", strconv.FormatUint(decision.PolicyGeneration, 10))
	status := &statusWriter{ResponseWriter: writer}
	selected.proxy.ServeHTTP(status, request)
	h.metrics.record(decision.SelectedEndpoint, status.statusCode())
	if h.logger != nil {
		h.logger.Printf(
			"request_id=%s endpoint=%s status=%d reason=%s candidates=%d",
			routingRequest.ID,
			decision.SelectedEndpoint,
			status.statusCode(),
			decision.Reason,
			decision.CandidateCount,
		)
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

	result := api.Request{ID: requestID, Priority: priority}
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
		endpoints = append(endpoints, api.Endpoint{
			ID:             item.config.ID,
			Model:          item.config.Model,
			Healthy:        item.config.Healthy,
			Ready:          item.config.Ready,
			Running:        item.config.Running,
			Waiting:        item.config.Waiting,
			Outstanding:    int(outstanding),
			KVUsage:        item.config.KVUsage,
			TTFTEWMA:       item.config.TTFTEWMA,
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

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status != 0 {
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *statusWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
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
