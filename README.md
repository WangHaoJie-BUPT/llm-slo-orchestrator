# LLM SLO Orchestrator

SLO-aware routing and performance engineering for self-hosted vLLM on Kubernetes.

## Status

W0 environment and contract freeze is committed. The W1 digest-pinned vLLM
single-endpoint path is runtime-validated on the local RTX 5070 Ti, including
health, metrics, non-streaming, and streaming OpenAI-compatible requests.

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
make benchmark
make report
```

## W1 GPU Smoke

The model directory must contain the exact Qwen3-0.6B revision frozen in
`versions.lock.yaml`. The default location is on the E drive:

```bash
make verify-model
make gpu-up
make e2e-gpu
make gpu-down
```

`make e2e-gpu` retains the service after the run and writes raw request, metrics,
container-log, manifest, and summary evidence under a timestamped `evidence/w1-*`
directory. Override `MODEL_DIR`, `VLLM_BASE_URL`, or `W1_EVIDENCE_DIR` explicitly
when reproducing another run.

## Evidence Boundary

The completed W1 smoke demonstrates a real, pinned single-endpoint inference
path. It does not establish a capacity curve, comparative performance, routing
improvement, or production generalization. See
[`docs/adr/0001-scope-and-evidence-boundary.md`](docs/adr/0001-scope-and-evidence-boundary.md).
