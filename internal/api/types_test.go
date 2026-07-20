package api

import (
	"testing"
	"time"
)

func validPolicy() Policy {
	return Policy{
		Generation:      1,
		Model:           "qwen3-0.6b-pinned",
		Strategy:        "slo-prefix-affinity",
		MetricFreshness: 15 * time.Second,
		Objectives: Objectives{
			TTFTP95:      800 * time.Millisecond,
			TPOTP95:      80 * time.Millisecond,
			Availability: 0.995,
		},
		Weights: RoutingWeights{
			Queue:       35,
			TTFTRisk:    35,
			Prefix:      20,
			Utilization: 10,
		},
		Admission: AdmissionPolicy{
			MaxPredictedTTFT:  1200 * time.Millisecond,
			LowPriorityAction: "reject",
			RetryAfter:        2 * time.Second,
		},
		Fairness: FairnessPolicy{
			TenantBurst:         8,
			TenantRatePerSecond: 2,
		},
	}
}

func TestPolicyValidate(t *testing.T) {
	if err := validPolicy().Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
}

func TestPolicyValidateRejectsInvalidWeights(t *testing.T) {
	policy := validPolicy()
	policy.Weights.Queue = 34

	if err := policy.Validate(); err == nil {
		t.Fatal("expected invalid weight sum to be rejected")
	}
}

func TestPolicyValidateRequiresRetryAfter(t *testing.T) {
	policy := validPolicy()
	policy.Admission.RetryAfter = 0

	if err := policy.Validate(); err == nil {
		t.Fatal("expected missing Retry-After to be rejected")
	}
}

func TestPolicyValidateRejectsNegativeWeight(t *testing.T) {
	policy := validPolicy()
	policy.Weights.Queue = -1
	policy.Weights.TTFTRisk = 71

	if err := policy.Validate(); err == nil {
		t.Fatal("expected negative weight to be rejected")
	}
}
