# LLM SLO Orchestrator

SLO-aware routing and performance engineering for self-hosted vLLM on Kubernetes.

## Status

W0 environment and contract freeze is committed. The W1 digest-pinned vLLM
single-endpoint path is runtime-validated on the local RTX 5070 Ti, including
health, metrics, OpenAI-compatible streaming, GuideLLM smoke, and an exploratory
0.5-16 requests/second capacity curve. W2 now includes a working standalone
streaming router and deterministic two-backend L1 test.

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

## W1 GuideLLM Calibration

GuideLLM runs in an isolated local environment. Its exact working dependency set
is recorded in `loadgen/requirements.lock`; the load generator uses CPU-only
PyTorch and does not compete for the inference GPU.

```bash
python3 -m venv .venv
.venv/bin/pip install -r loadgen/requirements.lock
make verify-loadgen
make benchmark
```

`make benchmark` runs the declared low-rate smoke and exploratory capacity points
from `experiments/capacity.yaml`. Raw GuideLLM JSON/CSV, client logs, vLLM metrics,
manifest, and a compact summary are written below `evidence/w1-capacity-*`.

## W2 Standalone Router

The standalone router accepts repeated backend targets and forwards OpenAI HTTP
and SSE responses without buffering. The command-level fake test launches two
deterministic backends, verifies round-robin selection and metrics, and cleans up
all child processes:

```bash
make e2e-fake
```

For an interactive local run:

```bash
make build
./bin/fake-backend --listen 127.0.0.1:18001 --id fake-a
./bin/router --listen 127.0.0.1:8080 \
  --config config/e2e-fake-policy.yaml \
  --backend fake-a=http://127.0.0.1:18001
```

The router exposes `/health`, `/metrics`, and the selected endpoint in the
`X-LLM-Endpoint` response header. Request bodies and generated content are not
written to router logs.

## Evidence Boundary

The completed W1 smoke and calibration demonstrate a real, pinned single-endpoint
inference path over the tested workload range. They do not identify saturation,
compare routing policies, or establish production generalization. See
[`docs/adr/0001-scope-and-evidence-boundary.md`](docs/adr/0001-scope-and-evidence-boundary.md).
