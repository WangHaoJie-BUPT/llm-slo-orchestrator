# Eight-Minute Demonstration

## Before the session

Keep the pinned vLLM, Prometheus, and Grafana Compose services healthy. Run
`make demo-real` once and retain the printed temporary `summary.json` path. The
demo does not add generated evidence to the repository.

## 0:00-0:45 - Scope and evidence

Open `docs/testing/evidence-levels.md` and `experiments/h1-h4.json`. State that
the workstation supplies L0/L1 and single-endpoint L2 proof. H1-H3 benefits are
not claimed because they require two independent real endpoints (L3).

## 0:45-1:45 - Real model path

Show the healthy pinned Qwen3-0.6B vLLM endpoint, then the `real_gpu_path` entry
in the demo summary. Point out the OpenAI-compatible HTTP/SSE checks and the
selected endpoint header.

## 1:45-3:15 - Routing and admission

Run or show `make e2e-fake`. Demonstrate deterministic round-robin over two
backends, immediate SSE forwarding, priority parsing, bounded per-tenant rate
limits, and observable 429 responses. Use unit results for picker mechanics;
do not present fake timing as a performance comparison.

## 3:15-4:30 - Fault and freshness

Show the `fault_recovery` result. Explain consecutive health thresholds,
survivor-only routing, recovery, stale-metric exclusion, and the one-retry
pre-header boundary. Interrupted SSE is not replayed.

## 4:30-5:30 - Kubernetes wiring

Show `docs/reports/w2-kind-router.md` and the retained evidence summary. The kind
cluster used two fake backends and was deleted after validation. This proves
deployment wiring, not accelerator policy gains.

## 5:30-6:30 - Dashboard

Open `http://127.0.0.1:3000/d/llm-slo-overview`. Show router traffic,
admission, health/check age, outstanding requests, TTFT/TPOT, queue depth, and
KV-cache panels. The demo runner verifies Grafana provisioning and asks live
Prometheus to parse every panel query.

## 6:30-7:20 - Capacity and profile

Show `docs/reports/w1-guidellm-capacity.md` and
`config/qwen3-0.6b-profile.yaml`. The recorded 16 requests/second point is the
highest short-run tested point, not a saturation or production guarantee. The
profile controller rejects drift between policy, model, image, hardware, and
source evidence.

## 7:20-8:00 - Limits and reproduction

Open the demo `summary.json`, then finish with `make verify`, `make demo`, and
`make demo-real`. State the remaining work plainly: an L2 mixed-priority H4
campaign and an L3 two-real-endpoint H1-H3 campaign.
