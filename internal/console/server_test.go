package console

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerStatusChatEvidenceAndControl(t *testing.T) {
	var controlAuthorization string
	router := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/router/status":
			writeJSON(writer, http.StatusOK, map[string]any{
				"status": "ok", "model": "model", "strategy": "round-robin", "generation": 1,
				"collectedAt": time.Now(), "objectives": map[string]any{"ttftP95Ms": 800, "tpotP95Ms": 80, "availability": 0.99},
				"endpoints": []map[string]any{{"id": "a", "model": "model", "healthy": true, "ready": true, "metricsFresh": true}},
			})
		case "/v1/chat/completions":
			if request.Header.Get("X-Tenant-ID") != "tenant-a" {
				t.Errorf("tenant header = %q", request.Header.Get("X-Tenant-ID"))
			}
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.Header().Set("X-LLM-Endpoint", "a")
			writer.Header().Set("X-LLM-Retry-Count", "0")
			_, _ = writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n"))
		case "/admin/v1/strategy":
			controlAuthorization = request.Header.Get("Authorization")
			writeJSON(writer, http.StatusOK, map[string]any{"strategy": "least-outstanding"})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(router.Close)

	repository := t.TempDir()
	mustWrite(t, filepath.Join(repository, "docs", "reports", "status.md"), "# Status report\n")
	mustWrite(t, filepath.Join(repository, "evidence", "run-1", "summary.json"), `{"status":"passed","evidence_level":"L1"}`)
	server, err := New(Config{
		RouterURL: router.URL, RepositoryRoot: repository, Model: "model", DemoMode: true,
		ControlToken: "console-secret", RouterControlToken: "router-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	application := httptest.NewServer(server.Handler())
	t.Cleanup(application.Close)

	statusHTTPResponse, err := http.Get(application.URL + "/api/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	var status statusResponse
	if err := json.NewDecoder(statusHTTPResponse.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	statusHTTPResponse.Body.Close()
	if status.Status != "ok" || status.Router.Strategy != "round-robin" || status.EvidenceCount != 2 {
		t.Fatalf("unexpected status: %#v", status)
	}
	if statusHTTPResponse.Header.Get("Content-Security-Policy") == "" {
		t.Fatal("security headers are missing")
	}

	chatRequest, _ := http.NewRequest(http.MethodPost, application.URL+"/api/v1/chat/completions", strings.NewReader(`{"model":"model","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	chatRequest.Header.Set("Content-Type", "application/json")
	chatRequest.Header.Set("X-Tenant-ID", "tenant-a")
	chatResponse, err := http.DefaultClient.Do(chatRequest)
	if err != nil {
		t.Fatal(err)
	}
	chatBody, _ := io.ReadAll(chatResponse.Body)
	chatResponse.Body.Close()
	if chatResponse.Header.Get("X-LLM-Endpoint") != "a" || !bytes.Contains(chatBody, []byte("[DONE]")) {
		t.Fatalf("chat response endpoint=%q body=%s", chatResponse.Header.Get("X-LLM-Endpoint"), chatBody)
	}

	evidenceResponse, err := http.Get(application.URL + "/api/v1/evidence")
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		Items []evidenceItem `json:"items"`
	}
	_ = json.NewDecoder(evidenceResponse.Body).Decode(&evidence)
	evidenceResponse.Body.Close()
	if len(evidence.Items) != 2 {
		t.Fatalf("evidence items = %#v", evidence.Items)
	}
	traversal, _ := http.Get(application.URL + "/api/v1/evidence/file?id=../go.mod")
	traversal.Body.Close()
	if traversal.StatusCode != http.StatusBadRequest {
		t.Fatalf("traversal status = %d", traversal.StatusCode)
	}

	unauthorized, _ := http.NewRequest(http.MethodPost, application.URL+"/api/v1/control/strategy", strings.NewReader(`{"strategy":"least-outstanding"}`))
	unauthorized.Header.Set("Content-Type", "application/json")
	unauthorizedResponse, _ := http.DefaultClient.Do(unauthorized)
	unauthorizedResponse.Body.Close()
	if unauthorizedResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized control status = %d", unauthorizedResponse.StatusCode)
	}
	authorized, _ := http.NewRequest(http.MethodPost, application.URL+"/api/v1/control/strategy", strings.NewReader(`{"strategy":"least-outstanding"}`))
	authorized.Header.Set("Authorization", "Bearer console-secret")
	authorized.Header.Set("Content-Type", "application/json")
	authorizedResponse, err := http.DefaultClient.Do(authorized)
	if err != nil {
		t.Fatal(err)
	}
	authorizedResponse.Body.Close()
	if authorizedResponse.StatusCode != http.StatusOK || controlAuthorization != "Bearer router-secret" {
		t.Fatalf("authorized status=%d router auth=%q", authorizedResponse.StatusCode, controlAuthorization)
	}
}

func TestJobManagerSerializesAndCancelsJobs(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	runner := func(ctx context.Context, _ string, _ []string, output io.Writer) error {
		_, _ = output.Write([]byte("started\n"))
		started <- struct{}{}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	}
	manager := NewJobManager(t.TempDir(), runner)
	job, err := manager.Start("verify")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("job did not start")
	}
	if _, err := manager.Start("fault-smoke"); err == nil {
		t.Fatal("expected concurrent job rejection")
	}
	if _, err := manager.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, _ := manager.Get(job.ID)
		if current.Status == "cancelled" {
			if len(current.Logs) < 2 {
				t.Fatalf("job logs = %#v", current.Logs)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job was not cancelled")
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
