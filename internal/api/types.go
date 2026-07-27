package api

import (
	"errors"
	"fmt"
	"time"
)

type Priority string

const (
	PriorityOnlineHigh   Priority = "online-high"
	PriorityOnlineNormal Priority = "online-normal"
	PriorityBatchLow     Priority = "batch-low"
)

type Request struct {
	ID           string
	Model        string
	Tenant       string
	Priority     Priority
	PromptTokens int
	PrefixKey    string
	Deadline     time.Time
}

type Endpoint struct {
	ID             string
	Model          string
	Healthy        bool
	Ready          bool
	MetricAge      time.Duration
	Running        int
	Waiting        int
	Outstanding    int
	KVUsage        float64
	TTFTEWMA       time.Duration
	PrefixAffinity float64
}

type EndpointSnapshot struct {
	Generation  uint64
	CollectedAt time.Time
	Endpoints   []Endpoint
}

type Objectives struct {
	TTFTP95      time.Duration
	TPOTP95      time.Duration
	Availability float64
}

type RoutingWeights struct {
	Queue       int
	TTFTRisk    int
	Prefix      int
	Utilization int
}

func (w RoutingWeights) Sum() int {
	return w.Queue + w.TTFTRisk + w.Prefix + w.Utilization
}

type AdmissionPolicy struct {
	MaxPredictedTTFT  time.Duration
	LowPriorityAction string
	RetryAfter        time.Duration
}

type FairnessPolicy struct {
	TenantBurst         int
	TenantRatePerSecond float64
}

type Policy struct {
	Generation      uint64
	Model           string
	Strategy        string
	MetricFreshness time.Duration
	Objectives      Objectives
	Weights         RoutingWeights
	Admission       AdmissionPolicy
	Fairness        FairnessPolicy
}

func (p Policy) Validate() error {
	if p.Model == "" {
		return errors.New("model is required")
	}
	if p.Objectives.TTFTP95 <= 0 || p.Objectives.TPOTP95 <= 0 {
		return errors.New("TTFT and TPOT objectives must be positive")
	}
	if p.Objectives.Availability <= 0 || p.Objectives.Availability > 1 {
		return errors.New("availability must be in (0, 1]")
	}
	if p.MetricFreshness <= 0 {
		return errors.New("metric freshness must be positive")
	}
	if p.Weights.Sum() != 100 {
		return fmt.Errorf("routing weights must sum to 100, got %d", p.Weights.Sum())
	}
	if p.Weights.Queue < 0 || p.Weights.TTFTRisk < 0 || p.Weights.Prefix < 0 || p.Weights.Utilization < 0 {
		return errors.New("routing weights cannot be negative")
	}
	if p.Admission.MaxPredictedTTFT <= 0 {
		return errors.New("max predicted TTFT must be positive")
	}
	if p.Admission.LowPriorityAction == "reject" && p.Admission.RetryAfter <= 0 {
		return errors.New("reject admission policy requires a positive Retry-After")
	}
	if p.Fairness.TenantBurst <= 0 || p.Fairness.TenantRatePerSecond <= 0 {
		return errors.New("tenant burst and rate must be positive")
	}
	return nil
}

type FeatureContribution struct {
	Feature string
	Value   float64
	Weight  float64
	Score   float64
}

type Decision struct {
	RequestID        string
	PolicyGeneration uint64
	SelectedEndpoint string
	CandidateCount   int
	Reason           string
	PredictedTTFT    time.Duration
	MetricAge        time.Duration
	Admission        string
	Contributions    []FeatureContribution
}
