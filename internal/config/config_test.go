package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `apiVersion: inference.bupt.dev/v1alpha1
kind: InferenceSLOPolicy
metadata:
  name: online-chat
spec:
  model: qwen3-0.6b-pinned
  objectives:
    ttftP95Ms: 800
    tpotP95Ms: 80
    availability: 0.995
  routing:
    strategy: slo-prefix-affinity
    weights: {queue: 35, ttftRisk: 35, prefix: 20, utilization: 10}
    metricFreshnessSeconds: 15
  admission:
    maxPredictedTTFTMs: 1200
    lowPriorityAction: reject
    retryAfterSeconds: 2
  fairness:
    tenantBurst: 8
    tenantRatePerSecond: 2
`

func TestLoad(t *testing.T) {
	path := writePolicy(t, validYAML)
	document, policy, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if document.Metadata.Name != "online-chat" {
		t.Fatalf("metadata name = %q", document.Metadata.Name)
	}
	if policy.Weights.Sum() != 100 {
		t.Fatalf("weight sum = %d", policy.Weights.Sum())
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	path := writePolicy(t, strings.Replace(validYAML, "  model:", "  unexpected: true\n  model:", 1))
	if _, _, err := Load(path); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}

func TestLoadRejectsInvalidWeights(t *testing.T) {
	path := writePolicy(t, strings.Replace(validYAML, "queue: 35", "queue: 34", 1))
	if _, _, err := Load(path); err == nil {
		t.Fatal("expected invalid weights to be rejected")
	}
}

func writePolicy(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	return path
}
