# Initial SLO Contract

Status: W1 exploratory calibration completed on the selected local hardware.

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

The W1 GuideLLM run met both candidate objectives at every tested constant-rate
point from 0.5 through 16 requests/second for approximately 128 input and 64
output tokens. The run did not reach saturation, so 16 requests/second is the
highest validated test point, not an estimated maximum capacity.

## Measurement Rules

- Client timestamps are the user-latency source of truth.
- vLLM histograms are used for correlation and decomposition.
- Warm-up, failed requests, 429, cancellation, and lost events are reported.
- Prometheus labels cannot contain request IDs, prompts, tenant IDs, or endpoint
  instance IDs with unbounded cardinality.

## Backend Health Gates

The router performs independent HTTP `GET /health` probes for every configured
backend. The default interval is two seconds, timeout is one second, failure
threshold is two consecutive checks, and recovery threshold is one successful
check. A thresholded unhealthy endpoint is removed before picker evaluation and
is added back only after its recovery threshold is met.

`router_backend_healthy{endpoint}` exposes the current binary state and
`router_backend_health_check_age_seconds{endpoint}` exposes probe freshness.
Endpoint labels come only from startup configuration; request IDs, tenant data,
and model content are never used as metric labels.

## Tenant Admission

Every request is assigned to `X-Tenant-ID`, defaulting to `default`. A per-tenant
token bucket uses `tenantBurst` and `tenantRatePerSecond` from the strict policy.
Exhaustion returns HTTP 429 with `Retry-After` without consuming backend capacity.
Buckets are isolated by tenant, capped at 4,096 entries, and idle entries are
reclaimed before admitting new tenant state.

Tenant identifiers are limited to 128 bytes and never appear in Prometheus
labels. `router_admission_total{priority,reason}` records only bounded priority
and reason values, including `tenant-rate-limit` and
`predicted-slo-violation`.

## Metrics Freshness

The router collects each backend `/metrics` endpoint outside the request path.
It adapts vLLM running, waiting, KV-cache usage, and TTFT cumulative counters into
atomic endpoint state. TTFT uses consecutive sum/count deltas and an EWMA. A
scrape failure does not refresh the collection timestamp.

Picker snapshots calculate `MetricAge` from the last successful collection. An
endpoint older than `metricFreshnessSeconds` is excluded even when its health
probe still succeeds. Collection interval and timeout default to two seconds and
one second and can be changed with `--metrics-interval` and `--metrics-timeout`.
