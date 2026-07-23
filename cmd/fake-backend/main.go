package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

var version = "w2-dev"

type fakeServer struct {
	id         string
	model      string
	baseDelay  time.Duration
	tokenDelay time.Duration
	requests   atomic.Uint64
	active     atomic.Int64
}

func main() {
	showVersion := flag.Bool("version", false, "print version")
	listenAddress := flag.String("listen", "127.0.0.1:18001", "HTTP listen address")
	id := flag.String("id", "fake-a", "backend ID")
	model := flag.String("model", "qwen3-0.6b-pinned", "served model name")
	baseDelay := flag.Duration("base-delay", 5*time.Millisecond, "delay before the first response byte")
	tokenDelay := flag.Duration("token-delay", 5*time.Millisecond, "delay between streaming chunks")
	flag.Parse()
	if *showVersion {
		fmt.Printf("llm-slo-fake-backend %s\n", version)
		return
	}

	backend := &fakeServer{
		id:         *id,
		model:      *model,
		baseDelay:  *baseDelay,
		tokenDelay: *tokenDelay,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", backend.health)
	mux.HandleFunc("/metrics", backend.metrics)
	mux.HandleFunc("/v1/models", backend.models)
	mux.HandleFunc("/v1/chat/completions", backend.completions)

	server := &http.Server{
		Addr:              *listenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
	}()
	log.Printf("fake backend id=%s listening=%s", backend.id, *listenAddress)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
}

func (server *fakeServer) health(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte("ok\n"))
}

func (server *fakeServer) metrics(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(writer, "fake_backend_requests_total{id=%q} %d\n", server.id, server.requests.Load())
	_, _ = fmt.Fprintf(writer, "fake_backend_active_requests{id=%q} %d\n", server.id, server.active.Load())
}

func (server *fakeServer) models(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"object": "list",
		"data":   []map[string]any{{"id": server.model, "object": "model"}},
	})
}

func (server *fakeServer) completions(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		http.Error(writer, "invalid JSON", http.StatusBadRequest)
		return
	}
	server.requests.Add(1)
	server.active.Add(1)
	defer server.active.Add(-1)
	if !sleep(request.Context(), server.baseDelay) {
		return
	}

	requestID := request.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = "fake-" + strconv.FormatUint(server.requests.Load(), 10)
	}
	if payload.Stream {
		server.stream(writer, request, requestID)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"id":      requestID,
		"object":  "chat.completion",
		"model":   payload.Model,
		"backend": server.id,
		"choices": []map[string]any{{
			"index":         0,
			"finish_reason": "stop",
			"message": map[string]string{
				"role":    "assistant",
				"content": "FAKE_" + server.id,
			},
		}},
	})
}

func (server *fakeServer) stream(writer http.ResponseWriter, request *http.Request, requestID string) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	chunks := []string{"FAKE_", server.id}
	for _, content := range chunks {
		data, _ := json.Marshal(map[string]any{
			"id":     requestID,
			"object": "chat.completion.chunk",
			"model":  server.model,
			"choices": []map[string]any{{
				"index": 0,
				"delta": map[string]string{"content": content},
			}},
		})
		_, _ = fmt.Fprintf(writer, "data: %s\n\n", data)
		flusher.Flush()
		if !sleep(request.Context(), server.tokenDelay) {
			return
		}
	}
	_, _ = writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func sleep(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
