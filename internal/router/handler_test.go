package router

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"inference.bupt.dev/llm-slo-orchestrator/internal/api"
	"inference.bupt.dev/llm-slo-orchestrator/internal/picker"
)

func TestHandlerRoundRobinAndMetrics(t *testing.T) {
	backendA := newFakeBackend(t, "a")
	backendB := newFakeBackend(t, "b")
	handler := newTestHandler(t, "round-robin", []TargetConfig{
		{ID: "a", BaseURL: backendA.URL, Healthy: true, Ready: true},
		{ID: "b", BaseURL: backendB.URL, Healthy: true, Ready: true},
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	firstEndpoint := postCompletion(t, server.URL, false).Header.Get("X-LLM-Endpoint")
	secondEndpoint := postCompletion(t, server.URL, false).Header.Get("X-LLM-Endpoint")
	if firstEndpoint != "a" || secondEndpoint != "b" {
		t.Fatalf("selected endpoints = %q, %q", firstEndpoint, secondEndpoint)
	}

	response, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if !bytes.Contains(body, []byte(`router_requests_total{endpoint="a",code="200"} 1`)) {
		t.Fatalf("metrics missing endpoint a counter:\n%s", body)
	}
}

func TestHandlerStreamsSSE(t *testing.T) {
	backend := newFakeBackend(t, "stream")
	handler := newTestHandler(t, "least-outstanding", []TargetConfig{
		{ID: "stream", BaseURL: backend.URL, Healthy: true, Ready: true},
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	response := postCompletion(t, server.URL, true)
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	var events []string
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "data: ") {
			events = append(events, scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan stream: %v", err)
	}
	if len(events) != 3 || events[2] != "data: [DONE]" {
		t.Fatalf("stream events = %#v", events)
	}
}

func TestHandlerRejectsWhenNoEndpointIsEligible(t *testing.T) {
	backend := newFakeBackend(t, "unhealthy")
	handler := newTestHandler(t, "least-outstanding", []TargetConfig{
		{ID: "unhealthy", BaseURL: backend.URL, Healthy: false, Ready: true},
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	response := postCompletion(t, server.URL, false)
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestHandlerAppliesBatchAdmission(t *testing.T) {
	backend := newFakeBackend(t, "slow")
	policy := testPolicy("slo-prefix-affinity")
	policy.Admission.MaxPredictedTTFT = 50 * time.Millisecond
	selectedPicker, _ := picker.New(policy.Strategy)
	handler, err := New(policy, selectedPicker, []TargetConfig{
		{ID: "slow", BaseURL: backend.URL, Healthy: true, Ready: true, TTFTEWMA: time.Second},
	}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"model"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-Priority", string(api.PriorityBatchLow))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST completion: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if response.Header.Get("Retry-After") != "2" {
		t.Fatalf("Retry-After = %q", response.Header.Get("Retry-After"))
	}
}

func TestHandlerUpdatesEndpointHealthAndMetrics(t *testing.T) {
	backend := newFakeBackend(t, "dynamic")
	handler := newTestHandler(t, "round-robin", []TargetConfig{
		{ID: "dynamic", BaseURL: backend.URL, Healthy: true, Ready: true},
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	checkedAt := time.Now().Add(-10 * time.Millisecond)
	if err := handler.SetEndpointHealth("dynamic", false, checkedAt); err != nil {
		t.Fatalf("SetEndpointHealth() error = %v", err)
	}
	response := postCompletion(t, server.URL, false)
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status while unhealthy = %d", response.StatusCode)
	}

	metricsResponse, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	metricsBody, _ := io.ReadAll(metricsResponse.Body)
	metricsResponse.Body.Close()
	if !bytes.Contains(metricsBody, []byte(`router_backend_healthy{endpoint="dynamic"} 0`)) {
		t.Fatalf("metrics missing unhealthy gauge:\n%s", metricsBody)
	}
	if !bytes.Contains(metricsBody, []byte(`router_backend_health_check_age_seconds{endpoint="dynamic"}`)) {
		t.Fatalf("metrics missing health age:\n%s", metricsBody)
	}

	if err := handler.SetEndpointHealth("dynamic", true, time.Now()); err != nil {
		t.Fatalf("SetEndpointHealth() recovery error = %v", err)
	}
	recovered := postCompletion(t, server.URL, false)
	defer recovered.Body.Close()
	if recovered.StatusCode != http.StatusOK {
		t.Fatalf("status after recovery = %d", recovered.StatusCode)
	}
}

func TestHandlerRejectsUnknownHealthTarget(t *testing.T) {
	backend := newFakeBackend(t, "known")
	handler := newTestHandler(t, "round-robin", []TargetConfig{
		{ID: "known", BaseURL: backend.URL, Healthy: true, Ready: true},
	})
	if err := handler.SetEndpointHealth("missing", false, time.Now()); err == nil {
		t.Fatal("expected unknown endpoint error")
	}
}

func newTestHandler(t *testing.T, strategy string, targets []TargetConfig) *Handler {
	t.Helper()
	policy := testPolicy(strategy)
	selectedPicker, err := picker.New(strategy)
	if err != nil {
		t.Fatalf("picker.New() error = %v", err)
	}
	handler, err := New(policy, selectedPicker, targets, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return handler
}

func testPolicy(strategy string) api.Policy {
	return api.Policy{
		Generation:      1,
		Model:           "model",
		Strategy:        strategy,
		MetricFreshness: 15 * time.Second,
		Objectives: api.Objectives{
			TTFTP95:      800 * time.Millisecond,
			TPOTP95:      80 * time.Millisecond,
			Availability: 0.99,
		},
		Weights: api.RoutingWeights{Queue: 35, TTFTRisk: 35, Prefix: 20, Utilization: 10},
		Admission: api.AdmissionPolicy{
			MaxPredictedTTFT:  1200 * time.Millisecond,
			LowPriorityAction: "reject",
			RetryAfter:        2 * time.Second,
		},
		Fairness: api.FairnessPolicy{TenantBurst: 8, TenantRatePerSecond: 2},
	}
}

func newFakeBackend(t *testing.T, id string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if bytes.Contains(body, []byte(`"stream":true`)) {
			writer.Header().Set("Content-Type", "text/event-stream")
			flusher := writer.(http.Flusher)
			_, _ = fmt.Fprintf(writer, "data: {\"id\":%q,\"choices\":[{\"delta\":{\"content\":\"FAKE_\"}}]}\n\n", id)
			flusher.Flush()
			_, _ = fmt.Fprintf(writer, "data: {\"id\":%q,\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", id, id)
			flusher.Flush()
			_, _ = writer.Write([]byte("data: [DONE]\n\n"))
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer, `{"id":%q,"choices":[{"message":{"content":%q}}]}`, id, "FAKE_"+id)
	}))
	t.Cleanup(server.Close)
	return server
}

func postCompletion(t *testing.T, baseURL string, stream bool) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"model":"model","stream":%t,"messages":[{"role":"user","content":"hello"}]}`, stream)
	request, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST completion: %v", err)
	}
	if !stream {
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
	}
	return response
}
