# ADR-0001: Freeze Scope and Evidence Boundary

- Status: Accepted
- Date: 2026-07-20

## Context

The project must demonstrate real model serving, request routing, SLO measurement,
and reproducible experiments within four weeks. It must not repeat an existing
synthetic Kubernetes scheduler project or imply production-scale performance from
a single consumer GPU.

## Decision

The portfolio release contains one standalone OpenAI-compatible router, a stable
picker interface, fake and vLLM backends, Prometheus/Grafana observability,
GuideLLM workloads, admission control, fault campaigns, and generated evidence.

Evidence is classified as follows:

- L0: unit, property, fuzz, and race tests with in-memory state;
- L1: Docker/kind deployment with deterministic fake backends;
- L2: one real accelerator and one pinned model endpoint;
- L3: two independent real accelerator endpoints;
- L4: a second hardware vendor or accelerator family.

Claims must not exceed the highest reproducible evidence level. W0 is L0
environment and contract evidence. Source compatibility is not runtime evidence.

## Consequences

- W1 can claim real inference only after a manifest-backed GPU smoke test.
- Multi-endpoint policy improvement requires L3; fake backends cannot support that
  claim.
- Negative or statistically overlapping results remain in the evidence set.
- Gateway/EPP, autoscaling, disaggregated serving, and Ascend remain enhancements.

## Rejected Alternatives

- Extending the prior synthetic scheduler project: it does not answer TTFT/TPOT or
  real model questions.
- Implementing a custom inference engine: it expands scope without improving the
  routing and evidence objective.
- Reporting dashboard screenshots without raw records: screenshots are not
  reproducible evidence.

