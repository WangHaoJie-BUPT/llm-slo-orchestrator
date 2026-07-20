package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"inference.bupt.dev/llm-slo-orchestrator/internal/api"
)

type Document struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

type Metadata struct {
	Name string `yaml:"name"`
}

type Spec struct {
	Model      string     `yaml:"model"`
	Objectives Objectives `yaml:"objectives"`
	Routing    Routing    `yaml:"routing"`
	Admission  Admission  `yaml:"admission"`
	Fairness   Fairness   `yaml:"fairness"`
}

type Objectives struct {
	TTFTP95Ms    int64   `yaml:"ttftP95Ms"`
	TPOTP95Ms    int64   `yaml:"tpotP95Ms"`
	Availability float64 `yaml:"availability"`
}

type Routing struct {
	Strategy               string  `yaml:"strategy"`
	Weights                Weights `yaml:"weights"`
	MetricFreshnessSeconds int64   `yaml:"metricFreshnessSeconds"`
}

type Weights struct {
	Queue       int `yaml:"queue"`
	TTFTRisk    int `yaml:"ttftRisk"`
	Prefix      int `yaml:"prefix"`
	Utilization int `yaml:"utilization"`
}

type Admission struct {
	MaxPredictedTTFTMs int64  `yaml:"maxPredictedTTFTMs"`
	LowPriorityAction  string `yaml:"lowPriorityAction"`
	RetryAfterSeconds  int64  `yaml:"retryAfterSeconds"`
}

type Fairness struct {
	TenantBurst         int     `yaml:"tenantBurst"`
	TenantRatePerSecond float64 `yaml:"tenantRatePerSecond"`
}

var supportedStrategies = map[string]struct{}{
	"round-robin":         {},
	"random":              {},
	"least-outstanding":   {},
	"least-queue":         {},
	"slo-prefix-affinity": {},
}

func Load(path string) (Document, api.Policy, error) {
	file, err := os.Open(path)
	if err != nil {
		return Document{}, api.Policy{}, fmt.Errorf("open policy: %w", err)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)

	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, api.Policy{}, fmt.Errorf("decode policy: %w", err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Document{}, api.Policy{}, errors.New("policy must contain exactly one YAML document")
		}
		return Document{}, api.Policy{}, fmt.Errorf("decode trailing policy content: %w", err)
	}

	policy, err := document.Policy()
	if err != nil {
		return Document{}, api.Policy{}, err
	}
	return document, policy, nil
}

func (d Document) Policy() (api.Policy, error) {
	if d.APIVersion != "inference.bupt.dev/v1alpha1" {
		return api.Policy{}, fmt.Errorf("unsupported apiVersion %q", d.APIVersion)
	}
	if d.Kind != "InferenceSLOPolicy" {
		return api.Policy{}, fmt.Errorf("unsupported kind %q", d.Kind)
	}
	if d.Metadata.Name == "" {
		return api.Policy{}, errors.New("metadata.name is required")
	}
	if _, ok := supportedStrategies[d.Spec.Routing.Strategy]; !ok {
		return api.Policy{}, fmt.Errorf("unsupported routing strategy %q", d.Spec.Routing.Strategy)
	}
	if d.Spec.Admission.LowPriorityAction != "reject" && d.Spec.Admission.LowPriorityAction != "accept" {
		return api.Policy{}, fmt.Errorf("unsupported lowPriorityAction %q", d.Spec.Admission.LowPriorityAction)
	}

	policy := api.Policy{
		Model:           d.Spec.Model,
		Strategy:        d.Spec.Routing.Strategy,
		MetricFreshness: time.Duration(d.Spec.Routing.MetricFreshnessSeconds) * time.Second,
		Objectives: api.Objectives{
			TTFTP95:      time.Duration(d.Spec.Objectives.TTFTP95Ms) * time.Millisecond,
			TPOTP95:      time.Duration(d.Spec.Objectives.TPOTP95Ms) * time.Millisecond,
			Availability: d.Spec.Objectives.Availability,
		},
		Weights: api.RoutingWeights{
			Queue:       d.Spec.Routing.Weights.Queue,
			TTFTRisk:    d.Spec.Routing.Weights.TTFTRisk,
			Prefix:      d.Spec.Routing.Weights.Prefix,
			Utilization: d.Spec.Routing.Weights.Utilization,
		},
		Admission: api.AdmissionPolicy{
			MaxPredictedTTFT:  time.Duration(d.Spec.Admission.MaxPredictedTTFTMs) * time.Millisecond,
			LowPriorityAction: d.Spec.Admission.LowPriorityAction,
			RetryAfter:        time.Duration(d.Spec.Admission.RetryAfterSeconds) * time.Second,
		},
		Fairness: api.FairnessPolicy{
			TenantBurst:         d.Spec.Fairness.TenantBurst,
			TenantRatePerSecond: d.Spec.Fairness.TenantRatePerSecond,
		},
	}

	if err := policy.Validate(); err != nil {
		return api.Policy{}, fmt.Errorf("validate policy: %w", err)
	}
	return policy, nil
}
