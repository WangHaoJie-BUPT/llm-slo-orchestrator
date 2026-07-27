# Fault Matrix

This matrix separates deterministic local checks from claims that require real
accelerators or production infrastructure. Passing an L0/L1 row does not raise
it to L2 or L3.

| Fault | Expected behavior | Automated evidence | Level | Remaining production gap |
| --- | --- | --- | --- | --- |
| Endpoint process crash | Health thresholds remove the endpoint, survivor carries traffic, recovered endpoint re-enters | `make fault-smoke` | L1 | Repeat with independent real accelerator endpoints and network fault injection |
| Stale backend metrics | Picker excludes endpoint after freshness TTL; failed scrape does not refresh age | Go tests for metrics collection and picker freshness | L0 | Validate under delayed and lossy production telemetry |
| Transport failure before response headers | Router retries once on an alternate healthy endpoint and reports `X-LLM-Retry-Count: 1` | Go router tests | L0 | Inject proxy resets against real endpoints |
| Mid-stream disconnect | Router terminates the response and never replays after client-visible SSE bytes | Go router tests; backup request count remains zero | L0 | Validate client behavior and real engine disconnect signatures |
| Tenant overload | Bounded token bucket returns 429 and `Retry-After`; tenant IDs stay out of metric labels | Go admission and router tests | L0 | Run a sustained multi-tenant fairness campaign |
| Long prompt flood | Admission and queue-aware picker mechanics remain bounded | Policy and component tests only | L0 | Run calibrated 4K-prompt mixed-priority load on real endpoints |

The local demonstration runs the endpoint crash campaign and the static/runtime
checks that cover the other implemented mechanisms. H1-H3 performance claims
remain gated on L3 evidence from two independent real accelerator endpoints.
