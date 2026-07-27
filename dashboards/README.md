# Dashboards

`llm-slo-overview.json` is automatically provisioned by the observability Compose
profile. It covers router traffic, 5xx responses, backend health and check age,
outstanding requests, vLLM TTFT/TPOT p95, KV usage, and running/waiting requests.

Start Prometheus and Grafana with `make dashboard-up`, then open
`http://127.0.0.1:3000/d/llm-slo-overview`. Screenshots are supporting evidence
only; raw Prometheus snapshots and run manifests remain the source of truth.
