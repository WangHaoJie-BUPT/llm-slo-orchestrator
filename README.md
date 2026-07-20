# LLM SLO Orchestrator

SLO-aware routing and performance engineering for self-hosted vLLM on Kubernetes.

## Status

W0 environment and contract freeze. The repository currently provides stable API
boundaries, version locks, verification commands, and deployment scaffolding. It
does not yet claim a working vLLM request path or any performance improvement.

## Scope

The four-week portfolio release will provide:

- an OpenAI-compatible streaming router written in Go;
- pluggable round-robin, random, least-outstanding, and SLO-aware pickers;
- immutable endpoint snapshots with health and metric freshness gates;
- priority-aware admission control with observable 429 responses;
- vLLM, router, client-latency, Prometheus, and Grafana evidence;
- repeatable GuideLLM workloads, fault campaigns, and generated reports.

Gateway API Inference Extension, autoscaling, distributed serving, and Ascend are
enhancements. They do not block the four-week release.

## Prerequisites

- Ubuntu 24.04 under WSL 2
- Docker Desktop with WSL integration
- Go 1.26.5
- Python 3.12
- GNU Make
- kubectl 1.36.2, kind 0.32.0, and Helm 4.2.3
- NVIDIA GPU only for `e2e-gpu` and accelerator benchmarks

The validated host and candidate runtime versions are recorded in
[`versions.lock.yaml`](versions.lock.yaml). Direct Docker Hub access is unreliable
on the current network, so pinned mirror references are used without changing
their content digests.

## W0 Commands

```bash
make verify
make compose-config
go run ./cmd/router --version
go run ./cmd/router --check-config config/example-policy.yaml
```

Commands for later milestones already exist and fail explicitly until their
implementation lands:

```bash
make e2e-fake
make e2e-gpu
make benchmark
make report
```

## Evidence Boundary

Until `make e2e-gpu` runs a pinned model on a real accelerator, this repository
only demonstrates control-plane correctness, interfaces, deployment contracts,
and environment reproducibility. See
[`docs/adr/0001-scope-and-evidence-boundary.md`](docs/adr/0001-scope-and-evidence-boundary.md).
