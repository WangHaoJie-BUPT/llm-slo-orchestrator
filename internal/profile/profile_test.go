package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validProfile = `apiVersion: inference.bupt.dev/v1alpha1
kind: ModelBackendProfile
metadata:
  name: test
spec:
  model: model
  revision: abc123
  engineImage: registry/engine@sha256:abc
  hardware: {vendor: nvidia, product: test, count: 1, memoryGiB: 16}
  engine: {tensorParallel: 1, maxModelLen: 4096, gpuMemoryUtilization: 0.75}
  samplingPeriodSeconds: 20
  capacity:
    - {inputTokens: 128, outputTokens: 64, safeQPS: 4, ttftP95Ms: 60}
  sourceEvidence: evidence/manifest.yaml
`

func TestLoadProfile(t *testing.T) {
	path := writeProfile(t, validProfile)
	document, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if document.Spec.Capacity[0].SafeQPS != 4 {
		t.Fatalf("safe QPS = %f", document.Spec.Capacity[0].SafeQPS)
	}
}

func TestLoadProfileRejectsUnknownAndDuplicateBuckets(t *testing.T) {
	unknown := writeProfile(t, strings.Replace(validProfile, "  model:", "  unknown: true\n  model:", 1))
	if _, err := Load(unknown); err == nil {
		t.Fatal("expected unknown field error")
	}
	duplicate := writeProfile(t, strings.Replace(
		validProfile,
		"  sourceEvidence:",
		"    - {inputTokens: 128, outputTokens: 64, safeQPS: 5, ttftP95Ms: 50}\n  sourceEvidence:",
		1,
	))
	if _, err := Load(duplicate); err == nil {
		t.Fatal("expected duplicate bucket error")
	}
}

func writeProfile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return path
}
