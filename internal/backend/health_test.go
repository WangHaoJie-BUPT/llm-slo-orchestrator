package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthStateThresholds(t *testing.T) {
	state := healthState{healthy: true}
	if !state.observe(false, 2, 2) {
		t.Fatal("one failure changed a healthy target")
	}
	if state.observe(false, 2, 2) {
		t.Fatal("two failures did not mark target unhealthy")
	}
	if state.observe(true, 2, 2) {
		t.Fatal("one success changed an unhealthy target")
	}
	if !state.observe(true, 2, 2) {
		t.Fatal("two successes did not restore target")
	}
}

func TestHealthMonitorReportsFailureAndRecovery(t *testing.T) {
	var available atomic.Bool
	available.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if !available.Load() {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	monitor := HealthMonitor{
		Interval:         5 * time.Millisecond,
		Timeout:          100 * time.Millisecond,
		FailureThreshold: 1,
		SuccessThreshold: 1,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan HealthUpdate, 32)
	finished := make(chan error, 1)
	go func() {
		finished <- monitor.Run(ctx, []HealthTarget{{ID: "a", BaseURL: server.URL, InitiallyHealthy: true}}, func(update HealthUpdate) {
			updates <- update
		})
	}()

	waitForHealth(t, updates, true)
	available.Store(false)
	waitForHealth(t, updates, false)
	available.Store(true)
	waitForHealth(t, updates, true)
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("monitor did not stop with its context")
	}
}

func waitForHealth(t *testing.T, updates <-chan HealthUpdate, healthy bool) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case update := <-updates:
			if update.Healthy == healthy {
				return
			}
		case <-deadline:
			t.Fatalf("did not observe healthy=%t", healthy)
		}
	}
}
