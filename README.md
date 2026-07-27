# LLM SLO Orchestrator

SLO-aware routing and performance engineering for self-hosted vLLM on Kubernetes.

## Status

W0 environment and contract freeze is committed. The W1 digest-pinned vLLM
single-endpoint path is runtime-validated on the local RTX 5070 Ti, including
health, metrics, OpenAI-compatible streaming, GuideLLM smoke, and an exploratory
0.5-16 requests/second capacity curve. W2 now includes a working standalone
streaming router, deterministic two-backend L1 test, and a real vLLM L2 router
path. The temporary kind deployment is also validated with two fake backends.
W3 adds generated evidence reports, thresholded active backend health, a
deterministic failure/recovery campaign, and evidence-backed model profile
reconciliation.

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

Core command contracts are kept in the Makefile. These commands are runnable on
the validated host:

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

With the pinned vLLM service running, validate the same path against the real
model endpoint:

```bash
make e2e-router-gpu
```

## W2 Kubernetes Demo

Build the local static images, create a temporary kind cluster with the pinned
node image, deploy the router and two fake backends, and run the complete HTTP,
SSE, metrics, and cleanup checks:

```bash
make e2e-kind
```

The command writes a manifest and summary under `evidence/w2-kind-*`, retains
raw logs and responses locally, and deletes the temporary cluster before it
returns. This is Kubernetes wiring evidence using fake backends, not a
real-model performance or routing-improvement claim.

## W3 Backend Resilience

The router probes each backend `/health` endpoint independently. Consecutive
failure and recovery thresholds prevent a single probe result from flapping the
routing set. Current health and check age are exported as bounded-label
Prometheus gauges. Run the deterministic process failure and recovery campaign
with:

```bash
make fault-smoke
```

The campaign stops one fake backend, verifies that traffic continues through the
survivor without 5xx responses, restarts the backend, and verifies re-entry into
the round-robin set. Override the production-facing defaults with the router
flags `--health-interval`, `--health-timeout`,
`--health-failure-threshold`, and `--health-success-threshold`.

## W3 Capacity Profile Reconciliation

The strict `ModelBackendProfile` binds the pinned model, engine image, hardware,
engine settings, tested capacity bucket, and its W1 source evidence. Generate the
deterministic policy/profile generations and reconciliation state with:

```bash
make profile-reconcile
```

`make verify` checks that `config/reconciled-profile.json` is current. The
recorded 16 requests/second point is the highest short-run rate tested for the
declared workload, not a saturation estimate or production guarantee.

## W4 Observability Dashboard

Start the provisioned Prometheus and Grafana services with:

```bash
make dashboard-up
python3 hack/verify_observability.py --runtime
```

The dashboard is available at `http://127.0.0.1:3000/d/llm-slo-overview` and
includes router traffic/failures/health plus vLLM TTFT, TPOT, queue, and KV cache
views. Provisioning and every PromQL expression are runtime-validated.

## W4 Tenant Admission

Requests may provide `X-Tenant-ID` and `X-Request-Priority`. The router enforces
the policy's per-tenant token bucket before picker evaluation and returns an
observable HTTP 429 with `Retry-After` when a tenant exhausts its allowance.
Tenant state is bounded and tenant identifiers never become metric labels.

## Evidence Boundary

The completed W1 smoke and calibration demonstrate a real, pinned single-endpoint
inference path over the tested workload range. They do not identify saturation,
compare routing policies, or establish production generalization. See
[`docs/adr/0001-scope-and-evidence-boundary.md`](docs/adr/0001-scope-and-evidence-boundary.md).
