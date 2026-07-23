package picker

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"time"

	"inference.bupt.dev/llm-slo-orchestrator/internal/api"
)

var ErrNoEligibleEndpoint = errors.New("no eligible endpoint")

// Picker has pure-function semantics: implementations must not perform network I/O.
type Picker interface {
	Pick(request api.Request, snapshot api.EndpointSnapshot, policy api.Policy) (api.Decision, error)
}

func New(strategy string) (Picker, error) {
	switch strategy {
	case "round-robin":
		return RoundRobin{}, nil
	case "random":
		return Random{}, nil
	case "least-outstanding":
		return LeastOutstanding{}, nil
	case "least-queue":
		return LeastQueue{}, nil
	case "slo-prefix-affinity":
		return SLOAware{}, nil
	default:
		return nil, fmt.Errorf("unsupported picker strategy %q", strategy)
	}
}

type RoundRobin struct{}

func (RoundRobin) Pick(request api.Request, snapshot api.EndpointSnapshot, policy api.Policy) (api.Decision, error) {
	candidates := eligible(snapshot, policy)
	if len(candidates) == 0 {
		return api.Decision{}, ErrNoEligibleEndpoint
	}
	selected := candidates[snapshot.Generation%uint64(len(candidates))]
	return decision(request, snapshot, policy, selected, len(candidates), "round-robin"), nil
}

type Random struct{}

func (Random) Pick(request api.Request, snapshot api.EndpointSnapshot, policy api.Policy) (api.Decision, error) {
	candidates := eligible(snapshot, policy)
	if len(candidates) == 0 {
		return api.Decision{}, ErrNoEligibleEndpoint
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(request.ID))
	selected := candidates[hash.Sum64()%uint64(len(candidates))]
	return decision(request, snapshot, policy, selected, len(candidates), "deterministic-random"), nil
}

type LeastOutstanding struct{}

func (LeastOutstanding) Pick(request api.Request, snapshot api.EndpointSnapshot, policy api.Policy) (api.Decision, error) {
	return pickMinimum(request, snapshot, policy, "least-outstanding", func(endpoint api.Endpoint) float64 {
		return float64(endpoint.Outstanding)
	})
}

type LeastQueue struct{}

func (LeastQueue) Pick(request api.Request, snapshot api.EndpointSnapshot, policy api.Policy) (api.Decision, error) {
	return pickMinimum(request, snapshot, policy, "least-queue", func(endpoint api.Endpoint) float64 {
		return float64(endpoint.Waiting)
	})
}

type SLOAware struct{}

func (SLOAware) Pick(request api.Request, snapshot api.EndpointSnapshot, policy api.Policy) (api.Decision, error) {
	candidates := eligible(snapshot, policy)
	if len(candidates) == 0 {
		return api.Decision{}, ErrNoEligibleEndpoint
	}

	selected := candidates[0]
	selectedScore := math.Inf(1)
	var selectedContributions []api.FeatureContribution
	for _, endpoint := range candidates {
		queue := clamp(float64(endpoint.Running+endpoint.Waiting) / 16)
		ttftRisk := clamp(float64(predictedTTFT(endpoint, request)) / float64(policy.Objectives.TTFTP95))
		prefixPenalty := clamp(1 - endpoint.PrefixAffinity)
		utilization := clamp(endpoint.KVUsage)
		contributions := []api.FeatureContribution{
			contribution("queue", queue, policy.Weights.Queue),
			contribution("ttft-risk", ttftRisk, policy.Weights.TTFTRisk),
			contribution("prefix-miss", prefixPenalty, policy.Weights.Prefix),
			contribution("utilization", utilization, policy.Weights.Utilization),
		}
		score := 0.0
		for _, item := range contributions {
			score += item.Score
		}
		if score < selectedScore || (score == selectedScore && endpoint.ID < selected.ID) {
			selected = endpoint
			selectedScore = score
			selectedContributions = contributions
		}
	}

	result := decision(request, snapshot, policy, selected, len(candidates), "minimum-weighted-slo-risk")
	result.Contributions = selectedContributions
	result.PredictedTTFT = predictedTTFT(selected, request)
	if request.Priority == api.PriorityBatchLow && result.PredictedTTFT > policy.Admission.MaxPredictedTTFT {
		result.Admission = "reject"
	}
	return result, nil
}

func eligible(snapshot api.EndpointSnapshot, policy api.Policy) []api.Endpoint {
	result := make([]api.Endpoint, 0, len(snapshot.Endpoints))
	for _, endpoint := range snapshot.Endpoints {
		if endpoint.Model != policy.Model || !endpoint.Healthy || !endpoint.Ready {
			continue
		}
		if endpoint.MetricAge < 0 || endpoint.MetricAge > policy.MetricFreshness {
			continue
		}
		result = append(result, endpoint)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func pickMinimum(
	request api.Request,
	snapshot api.EndpointSnapshot,
	policy api.Policy,
	reason string,
	value func(api.Endpoint) float64,
) (api.Decision, error) {
	candidates := eligible(snapshot, policy)
	if len(candidates) == 0 {
		return api.Decision{}, ErrNoEligibleEndpoint
	}
	selected := candidates[0]
	minimum := value(selected)
	for _, endpoint := range candidates[1:] {
		if current := value(endpoint); current < minimum {
			selected = endpoint
			minimum = current
		}
	}
	return decision(request, snapshot, policy, selected, len(candidates), reason), nil
}

func decision(
	request api.Request,
	snapshot api.EndpointSnapshot,
	policy api.Policy,
	selected api.Endpoint,
	candidateCount int,
	reason string,
) api.Decision {
	return api.Decision{
		RequestID:        request.ID,
		PolicyGeneration: policy.Generation,
		SelectedEndpoint: selected.ID,
		CandidateCount:   candidateCount,
		Reason:           reason,
		PredictedTTFT:    predictedTTFT(selected, request),
		MetricAge:        selected.MetricAge,
		Admission:        "accept",
	}
}

func predictedTTFT(endpoint api.Endpoint, request api.Request) time.Duration {
	base := endpoint.TTFTEWMA
	if base <= 0 {
		base = 25 * time.Millisecond
	}
	queuePenalty := time.Duration(endpoint.Outstanding+endpoint.Waiting) * 10 * time.Millisecond
	promptPenalty := time.Duration(request.PromptTokens/128) * time.Millisecond
	return base + queuePenalty + promptPenalty
}

func contribution(feature string, value float64, weight int) api.FeatureContribution {
	weightFraction := float64(weight) / 100
	return api.FeatureContribution{
		Feature: feature,
		Value:   value,
		Weight:  weightFraction,
		Score:   value * weightFraction,
	}
}

func clamp(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}
