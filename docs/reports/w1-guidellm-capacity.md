# W1 GuideLLM Capacity Calibration

- Date: 2026-07-24
- Evidence level: L2
- Run ID: `w1-capacity-20260724T011155+0800`
- Result: Passed

## Method

GuideLLM 0.7.1 generated synthetic chat requests with approximately 128 input
tokens and exactly 64 output tokens against the pinned Qwen3-0.6B vLLM endpoint.
The low-rate smoke sent 10 requests at 0.2 requests/second. Capacity points used
constant arrival rates, a 20-second window, and 10% warmup and cooldown, leaving
16 measured seconds per point.

Candidate objectives were TTFT p95 <= 800 ms and TPOT p95 <= 80 ms. They are test
objectives, not production SLO commitments.

## Results

| Requested req/s | Successful | Errors | E2E p95 ms | TTFT p95 ms | TPOT p95 ms | Output tok/s |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 0.5 | 8 | 0 | 381.892 | 159.479 | 5.962 | 32 |
| 1 | 16 | 0 | 226.374 | 42.329 | 3.532 | 64 |
| 2 | 32 | 0 | 242.415 | 49.228 | 3.781 | 128 |
| 4 | 64 | 0 | 239.551 | 45.304 | 3.740 | 256 |
| 8 | 129 | 0 | 262.961 | 70.496 | 4.105 | 516 |
| 16 | 259 | 0 | 290.569 | 61.001 | 4.537 | 1,036 |

The 10-request smoke also completed without errors. Its TTFT p95 was 133.546 ms
and TPOT p95 was 6.604 ms.

## Interpretation

Every tested point met the candidate objectives and achieved its requested rate.
The experiment did not reach saturation. Therefore, 16 requests/second is only
the highest validated point for this model and request shape, not the endpoint's
maximum capacity. Longer repetitions and higher rates are required before making
a capacity-limit or confidence-interval claim.

## Evidence Boundary

The manifest references clean commit `07313ffb502c25dbad3b2a3400eee99aedec8223`.
Raw GuideLLM JSON/CSV, client logs, vLLM logs, and Prometheus snapshots remain in
the local evidence directory. The results apply to one RTX 5070 Ti, one endpoint,
one model revision, and one prompt/output bucket; they do not establish routing
improvement or production-scale behavior.
