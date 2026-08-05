# Eight-Minute Demonstration

## Before the session

Keep the pinned vLLM, Prometheus, and Grafana Compose services healthy. Run
`make demo-real` once and retain the printed temporary `summary.json` path. The
demo does not add generated evidence to the repository. Start the router and
operator console, then open `http://127.0.0.1:8090`.

## 0:00-0:45 - Scope and evidence

Open the console Evidence view and select the experiment boundary report. State that
the workstation supplies L0/L1 and single-endpoint L2 proof. H1-H3 benefits are
not claimed because they require two independent real endpoints (L3).

## 0:45-1:45 - Real model path

Use the Playground to send a streaming request to the pinned Qwen3-0.6B model.
Point out TTFT, duration, selected endpoint, decision reason, retry count, and
the retained `real_gpu_path` entry in the demo summary.

## 1:45-3:15 - Routing and admission

Use the Overview topology and backend table. Run or show `make e2e-fake`.
Demonstrate deterministic round-robin over two
backends, immediate SSE forwarding, priority parsing, bounded per-tenant rate
limits, and observable 429 responses. Use unit results for picker mechanics;
do not present fake timing as a performance comparison.

## 3:15-4:30 - Fault and freshness

Open Scenarios and run the allowlisted failure/recovery job, or show its retained
`fault_recovery` result. Explain consecutive health thresholds,
survivor-only routing, recovery, stale-metric exclusion, and the one-retry
pre-header boundary. Interrupted SSE is not replayed.

## 4:30-5:30 - Kubernetes wiring

Show `docs/reports/w2-kind-router.md` and the retained evidence summary. The kind
cluster used two fake backends and was deleted after validation. This proves
deployment wiring, not accelerator policy gains.

## 5:30-6:30 - Dashboard

Use the console Grafana link to open the dashboard. Show router traffic,
admission, health/check age, outstanding requests, TTFT/TPOT, queue depth, and
KV-cache panels. The demo runner verifies Grafana provisioning and asks live
Prometheus to parse every panel query.

## 6:30-7:20 - Capacity and profile

Open the capacity report in the console Evidence view and show
`config/qwen3-0.6b-profile.yaml`. The recorded 16 requests/second point is the
highest short-run tested point, not a saturation or production guarantee. The
profile controller rejects drift between policy, model, image, hardware, and
source evidence.

## 7:20-8:00 - Limits and reproduction

Open the demo `summary.json`, then finish with `make verify`, `make demo`, and
`make demo-real`. State the remaining work plainly: an L2 mixed-priority H4
campaign and an L3 two-real-endpoint H1-H3 campaign.
