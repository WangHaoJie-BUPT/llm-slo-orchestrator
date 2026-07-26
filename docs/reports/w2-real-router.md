# W2 Real vLLM Router Path

- Date: 2026-07-24
- Evidence level: L2
- Run ID: `w2-router-gpu-20260724T014708+0800`
- Result: Passed

## Scope

A temporary standalone router process used the strict example SLO policy and a
single `vllm=http://127.0.0.1:8000` backend. The backend was the W1 pinned vLLM
0.25.1 container serving the fixed Qwen3-0.6B revision on the RTX 5070 Ti.

## Checks

| Check | Result |
| --- | --- |
| Router and backend health | passed |
| Selected endpoint response header | `vllm` |
| Non-streaming real-model response | passed, content `W2_OK` |
| Streaming response across multiple SSE delta chunks | passed, content `W2_OK` |
| Router Prometheus counter | passed, two HTTP 200 requests |
| Temporary router cleanup | passed |
| vLLM retained for local use | yes |

The manifest references clean commit
`3a50bca224869fa6d1d19c4668706886da0d2c09` and records the router binary,
policy, image, model revision, and model weight hashes.

## Evidence Boundary

The measured request totals were 211.647 ms for non-streaming and 99.458 ms for
streaming completion. They are smoke timings, not a router-overhead benchmark.
Only one real endpoint was available, so this run does not compare picker
policies or prove routing improvement. Kubernetes deployment is covered separately
by the W2 kind report. Active health probing, metric ingestion, and fault campaigns
remain later W3 work.
