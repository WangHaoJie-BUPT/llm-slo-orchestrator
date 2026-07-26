package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validPolicy = `apiVersion: inference.bupt.dev/v1alpha1
kind: InferenceSLOPolicy
metadata: {name: test-policy}
spec:
  model: model
  objectives: {ttftP95Ms: 800, tpotP95Ms: 80, availability: 0.995}
  routing:
    strategy: round-robin
    weights: {queue: 35, ttftRisk: 35, prefix: 20, utilization: 10}
    metricFreshnessSeconds: 15
  admission: {maxPredictedTTFTMs: 1200, lowPriorityAction: reject, retryAfterSeconds: 2}
  fairness: {tenantBurst: 8, tenantRatePerSecond: 2}
`

func TestReconcileAndStatusFile(t *testing.T) {
	directory := t.TempDir()
	evidence := filepath.Join(directory, "manifest.yaml")
	if err := os.WriteFile(evidence, []byte("schemaVersion: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(directory, "policy.yaml")
	profilePath := filepath.Join(directory, "profile.yaml")
	if err := os.WriteFile(policyPath, []byte(validPolicy), 0o600); err != nil {
		t.Fatal(err)
	}
	profileContents := strings.Replace(validProfile, "evidence/manifest.yaml", evidence, 1)
	if err := os.WriteFile(profilePath, []byte(profileContents), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := Reconcile(policyPath, profilePath)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if status.Status != "ready" || status.CapacityPoints != 1 || status.MaxSafeQPS != 4 {
		t.Fatalf("status = %#v", status)
	}
	output := filepath.Join(directory, "status.json")
	changed, err := WriteStatus(output, status)
	if err != nil || !changed {
		t.Fatalf("WriteStatus() = %t, %v", changed, err)
	}
	changed, err = WriteStatus(output, status)
	if err != nil || changed {
		t.Fatalf("second WriteStatus() = %t, %v", changed, err)
	}
	if err := CheckStatus(output, status); err != nil {
		t.Fatalf("CheckStatus() error = %v", err)
	}
}

func TestReconcileRejectsModelMismatchAndStalePolicy(t *testing.T) {
	directory := t.TempDir()
	evidence := filepath.Join(directory, "manifest.yaml")
	_ = os.WriteFile(evidence, []byte("schemaVersion: 1\n"), 0o600)
	policyPath := filepath.Join(directory, "policy.yaml")
	profilePath := filepath.Join(directory, "profile.yaml")
	_ = os.WriteFile(policyPath, []byte(validPolicy), 0o600)
	profileContents := strings.Replace(validProfile, "evidence/manifest.yaml", evidence, 1)
	_ = os.WriteFile(profilePath, []byte(strings.Replace(profileContents, "model: model", "model: other", 1)), 0o600)
	if _, err := Reconcile(policyPath, profilePath); err == nil {
		t.Fatal("expected model mismatch")
	}
	_ = os.WriteFile(profilePath, []byte(strings.Replace(profileContents, "samplingPeriodSeconds: 20", "samplingPeriodSeconds: 4", 1)), 0o600)
	if _, err := Reconcile(policyPath, profilePath); err == nil {
		t.Fatal("expected freshness mismatch")
	}
}
