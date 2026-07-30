package profile

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bupt20220407/llm-slo-orchestrator/internal/config"
)

type Status struct {
	SchemaVersion     int     `json:"schema_version"`
	Status            string  `json:"status"`
	Model             string  `json:"model"`
	PolicyName        string  `json:"policy_name"`
	PolicyGeneration  uint64  `json:"policy_generation"`
	PolicyStrategy    string  `json:"policy_strategy"`
	ProfileName       string  `json:"profile_name"`
	ProfileGeneration uint64  `json:"profile_generation"`
	CapacityPoints    int     `json:"capacity_points"`
	MaxSafeQPS        float64 `json:"max_safe_qps"`
	SourceEvidence    string  `json:"source_evidence"`
}

func Reconcile(policyPath, profilePath string) (Status, error) {
	policyBytes, err := os.ReadFile(policyPath)
	if err != nil {
		return Status{}, fmt.Errorf("read policy: %w", err)
	}
	profileBytes, err := os.ReadFile(profilePath)
	if err != nil {
		return Status{}, fmt.Errorf("read profile: %w", err)
	}
	policyDocument, policy, err := config.Load(policyPath)
	if err != nil {
		return Status{}, err
	}
	profileDocument, err := Load(profilePath)
	if err != nil {
		return Status{}, err
	}
	if policy.Model != profileDocument.Spec.Model {
		return Status{}, fmt.Errorf("policy model %q does not match profile model %q", policy.Model, profileDocument.Spec.Model)
	}
	maximumFreshness := 3 * time.Duration(profileDocument.Spec.SamplingPeriodSeconds) * time.Second
	if policy.MetricFreshness > maximumFreshness {
		return Status{}, fmt.Errorf("policy metric freshness %s exceeds three profile sampling periods (%s)", policy.MetricFreshness, maximumFreshness)
	}
	if _, err := os.Stat(profileDocument.Spec.SourceEvidence); err != nil {
		return Status{}, fmt.Errorf("profile source evidence: %w", err)
	}
	maxSafeQPS := 0.0
	for _, point := range profileDocument.Spec.Capacity {
		if point.SafeQPS > maxSafeQPS {
			maxSafeQPS = point.SafeQPS
		}
	}
	return Status{
		SchemaVersion:     1,
		Status:            "ready",
		Model:             policy.Model,
		PolicyName:        policyDocument.Metadata.Name,
		PolicyGeneration:  generation(policyBytes),
		PolicyStrategy:    policy.Strategy,
		ProfileName:       profileDocument.Metadata.Name,
		ProfileGeneration: generation(profileBytes),
		CapacityPoints:    len(profileDocument.Spec.Capacity),
		MaxSafeQPS:        maxSafeQPS,
		SourceEvidence:    profileDocument.Spec.SourceEvidence,
	}, nil
}

func MarshalStatus(status Status) ([]byte, error) {
	contents, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal reconciliation status: %w", err)
	}
	return append(contents, '\n'), nil
}

func WriteStatus(path string, status Status) (bool, error) {
	contents, err := MarshalStatus(status)
	if err != nil {
		return false, err
	}
	current, readErr := os.ReadFile(path)
	if readErr == nil && bytes.Equal(current, contents) {
		return false, nil
	}
	if readErr != nil && !os.IsNotExist(readErr) {
		return false, fmt.Errorf("read reconciliation output: %w", readErr)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create reconciliation output directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".profile-status-*")
	if err != nil {
		return false, fmt.Errorf("create reconciliation output: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return false, fmt.Errorf("write reconciliation output: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return false, fmt.Errorf("set reconciliation output mode: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return false, fmt.Errorf("close reconciliation output: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return false, fmt.Errorf("replace reconciliation output: %w", err)
	}
	return true, nil
}

func CheckStatus(path string, status Status) error {
	expected, err := MarshalStatus(status)
	if err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read reconciliation output: %w", err)
	}
	if !bytes.Equal(current, expected) {
		return fmt.Errorf("reconciliation output is stale: run make profile-reconcile")
	}
	return nil
}

func generation(contents []byte) uint64 {
	digest := sha256.Sum256(contents)
	return binary.BigEndian.Uint64(digest[:8])
}
