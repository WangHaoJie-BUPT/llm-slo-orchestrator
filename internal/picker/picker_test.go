package picker

import (
	"testing"
	"time"

	"github.com/bupt20220407/llm-slo-orchestrator/internal/api"
)

func TestRoundRobinUsesSnapshotGeneration(t *testing.T) {
	policy := testPolicy("round-robin")
	snapshot := testSnapshot()
	request := api.Request{ID: "request-1", Priority: api.PriorityOnlineNormal}

	first, err := (RoundRobin{}).Pick(request, snapshot, policy)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	snapshot.Generation++
	second, err := (RoundRobin{}).Pick(request, snapshot, policy)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if first.SelectedEndpoint == second.SelectedEndpoint {
		t.Fatalf("consecutive generations selected %q", first.SelectedEndpoint)
	}
}

func TestLeastOutstanding(t *testing.T) {
	policy := testPolicy("least-outstanding")
	snapshot := testSnapshot()
	snapshot.Endpoints[0].Outstanding = 3
	snapshot.Endpoints[1].Outstanding = 1

	decision, err := (LeastOutstanding{}).Pick(api.Request{ID: "request-1"}, snapshot, policy)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if decision.SelectedEndpoint != "b" {
		t.Fatalf("selected endpoint = %q", decision.SelectedEndpoint)
	}
}

func TestPickerRejectsStaleOrUnhealthyEndpoints(t *testing.T) {
	policy := testPolicy("least-queue")
	snapshot := testSnapshot()
	snapshot.Endpoints[0].Healthy = false
	snapshot.Endpoints[1].MetricAge = 2 * policy.MetricFreshness

	if _, err := (LeastQueue{}).Pick(api.Request{ID: "request-1"}, snapshot, policy); err != ErrNoEligibleEndpoint {
		t.Fatalf("Pick() error = %v, want %v", err, ErrNoEligibleEndpoint)
	}
}

func TestSLOAwarePrefersLowerRiskAndRejectsSlowBatch(t *testing.T) {
	policy := testPolicy("slo-prefix-affinity")
	policy.Admission.MaxPredictedTTFT = 50 * time.Millisecond
	snapshot := testSnapshot()
	snapshot.Endpoints[0].Outstanding = 10
	snapshot.Endpoints[0].TTFTEWMA = 100 * time.Millisecond
	snapshot.Endpoints[1].PrefixAffinity = 1
	snapshot.Endpoints[1].TTFTEWMA = 20 * time.Millisecond

	request := api.Request{ID: "request-1", Priority: api.PriorityBatchLow, PromptTokens: 128}
	decision, err := (SLOAware{}).Pick(request, snapshot, policy)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if decision.SelectedEndpoint != "b" {
		t.Fatalf("selected endpoint = %q", decision.SelectedEndpoint)
	}
	if decision.Admission != "accept" {
		t.Fatalf("admission = %q", decision.Admission)
	}

	snapshot.Endpoints[1].Outstanding = 10
	decision, err = (SLOAware{}).Pick(request, snapshot, policy)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if decision.Admission != "reject" {
		t.Fatalf("admission = %q", decision.Admission)
	}
}

func testSnapshot() api.EndpointSnapshot {
	return api.EndpointSnapshot{
		CollectedAt: time.Now(),
		Endpoints: []api.Endpoint{
			{ID: "a", Model: "model", Healthy: true, Ready: true, MetricAge: time.Second},
			{ID: "b", Model: "model", Healthy: true, Ready: true, MetricAge: time.Second},
		},
	}
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
