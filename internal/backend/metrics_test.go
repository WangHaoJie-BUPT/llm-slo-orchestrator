package backend

import (
	"strings"
	"testing"
)

func TestParseMetrics(t *testing.T) {
	sample, err := parseMetrics(strings.NewReader(`vllm:num_requests_running{engine="0"} 2
vllm:num_requests_waiting{engine="0"} 3
vllm:kv_cache_usage_perc{engine="0"} 0.25
vllm:time_to_first_token_seconds_sum{engine="0"} 1.5
vllm:time_to_first_token_seconds_count{engine="0"} 4
`))
	if err != nil {
		t.Fatal(err)
	}
	if sample.running != 2 || sample.waiting != 3 || sample.kv != 0.25 || sample.ttftCount != 4 {
		t.Fatalf("sample = %#v", sample)
	}
}

func TestParseMetricsRejectsMissingRequiredValues(t *testing.T) {
	if _, err := parseMetrics(strings.NewReader("vllm:num_requests_running 1\n")); err == nil {
		t.Fatal("expected missing metrics error")
	}
}
