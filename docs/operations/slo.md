# Initial SLO Contract

Status: definition only. No target has been validated on the selected hardware.

## User Metrics

- TTFT: request body sent to first non-empty generated token received by client.
- TPOT: elapsed time between adjacent generated tokens after the first token.
- E2E: request start to stream completion.
- Availability: valid completions divided by attempts, with 429 and 5xx reported
  separately.
- Goodput: completed requests per second satisfying both TTFT and TPOT objectives.

## Candidate Objectives

The example policy uses TTFT p95 <= 800 ms and TPOT p95 <= 80 ms only as initial
test targets. W1 calibration may revise them. Reports must identify the model,
hardware, prompt/output buckets, arrival process, sample count, and confidence
interval.

## Measurement Rules

- Client timestamps are the user-latency source of truth.
- vLLM histograms are used for correlation and decomposition.
- Warm-up, failed requests, 429, cancellation, and lost events are reported.
- Prometheus labels cannot contain request IDs, prompts, tenant IDs, or endpoint
  instance IDs with unbounded cardinality.

