# W2 Standalone Router Fake-Backend Test

- Date: 2026-07-24
- Evidence level: L1
- Run ID: `w2-fake-20260724T013723+0800`
- Result: Passed

## Scope

The command-level test built and launched one real router process and two
deterministic fake backend processes on dynamic localhost ports. The router used
the strict `config/e2e-fake-policy.yaml` policy and round-robin picker.

## Checks

| Check | Result |
| --- | --- |
| Router and backend health | passed |
| First three selections | `fake-a`, `fake-b`, `fake-a` |
| Non-streaming OpenAI-compatible response | passed |
| Streaming SSE response and `[DONE]` terminator | passed |
| Bounded-label Prometheus counters | passed |
| Child-process cleanup | passed |

The manifest references clean commit
`485c0a044e2cdf0905aac0f5ca466318de03b149` and records hashes for the router
binary, fake backend binary, and routing policy.

## Evidence Boundary

This run validates standalone process wiring, transparent HTTP/SSE forwarding,
deterministic selection, response decision headers, and metrics. Fake backends do
not support claims about real model latency or routing improvement. Real vLLM
router-path validation and Kubernetes deployment remain separate W2 steps.
