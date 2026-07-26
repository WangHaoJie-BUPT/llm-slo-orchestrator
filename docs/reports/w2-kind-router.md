# W2 Kubernetes Fake Topology

- Date: 2026-07-26
- Evidence level: L1
- Run ID: `w2-kind-20260726T182748+0800`
- Result: Passed

## Scope

The temporary kind run created a unique Kubernetes cluster from the pinned
`kindest/node:v1.36.1` digest, loaded locally built static `scratch` images, and
applied `deploy/kubernetes/w2-fake.yaml`. The topology contains a router and two
deterministic fake OpenAI-compatible backends in namespace `llm-slo`.

## Checks

| Check | Result |
| --- | --- |
| Kubernetes cluster and API readiness | passed, Kubernetes `v1.36.1` |
| Three Deployment rollouts | passed |
| Router health through Service port-forward | passed |
| Round-robin endpoint sequence | `fake-a`, `fake-b`, `fake-a` |
| Non-streaming responses | passed |
| Streaming SSE across multiple delta chunks | passed |
| Router Prometheus counters | passed |
| Port-forward and temporary cluster cleanup | passed |

The manifest references clean commit
`060c2005475bffff3c3a3abe5938af2fd1dc6705` and records the Kubernetes manifest
hash, local image IDs, fixed kind node digest, runtime version, cleanup state,
and the selected endpoint sequence. Raw resource listings, events, logs, HTTP
responses, SSE data, and metrics are retained under the run directory locally.

## Evidence Boundary

This run validates Kubernetes wiring, local image loading, Service discovery,
HTTP/SSE forwarding, deterministic selection, metrics, and cleanup. Fake
backends do not support real-model latency or routing-improvement claims. The
cluster is intentionally deleted after each run; persistent deployment and
multi-real-endpoint comparison remain later work.
