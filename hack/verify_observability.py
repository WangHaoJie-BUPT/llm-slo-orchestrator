#!/usr/bin/env python3
import argparse
import json
from pathlib import Path
import sys
from urllib.parse import urlencode
from urllib.request import urlopen


REQUIRED_METRICS = {
    "router_requests_total",
    "router_backend_healthy",
    "router_backend_health_check_age_seconds",
    "router_outstanding_requests",
    "vllm:time_to_first_token_seconds_bucket",
    "vllm:inter_token_latency_seconds_bucket",
    "vllm:kv_cache_usage_perc",
    "vllm:num_requests_running",
    "vllm:num_requests_waiting",
}


def dashboard_expressions(root: Path) -> list[str]:
    dashboard = json.loads((root / "dashboards/llm-slo-overview.json").read_text(encoding="utf-8"))
    if dashboard.get("uid") != "llm-slo-overview":
        raise ValueError("dashboard UID is not stable")
    panels = dashboard.get("panels")
    if not isinstance(panels, list) or len(panels) < 9:
        raise ValueError("dashboard must contain all overview panels")
    panel_ids = [panel.get("id") for panel in panels]
    if len(panel_ids) != len(set(panel_ids)):
        raise ValueError("dashboard panel IDs must be unique")
    if any(panel.get("datasource", {}).get("uid") != "prometheus" for panel in panels):
        raise ValueError("every panel must use the provisioned datasource")
    expression_list = [
        target.get("expr", "") for panel in panels for target in panel.get("targets", [])
    ]
    expressions = "\n".join(expression_list)
    missing = sorted(metric for metric in REQUIRED_METRICS if metric not in expressions)
    if missing:
        raise ValueError(f"dashboard is missing metric queries: {missing}")
    datasource = (root / "deploy/compose/grafana/provisioning/datasources/prometheus.yaml").read_text(encoding="utf-8")
    provider = (root / "deploy/compose/grafana/provisioning/dashboards/provider.yaml").read_text(encoding="utf-8")
    if "uid: prometheus" not in datasource or "url: http://prometheus:9090" not in datasource:
        raise ValueError("Prometheus datasource provisioning is invalid")
    if "path: /var/lib/grafana/dashboards" not in provider:
        raise ValueError("dashboard provider path is invalid")
    return expression_list


def verify_runtime(grafana_url: str, prometheus_url: str, expressions: list[str]) -> None:
    with urlopen(grafana_url + "/api/health", timeout=10) as response:
        if json.load(response).get("database") != "ok":
            raise ValueError("Grafana database is not ready")
    with urlopen(grafana_url + "/api/datasources/uid/prometheus", timeout=10) as response:
        if json.load(response).get("uid") != "prometheus":
            raise ValueError("provisioned datasource is unavailable")
    with urlopen(grafana_url + "/api/dashboards/uid/llm-slo-overview", timeout=10) as response:
        if json.load(response).get("dashboard", {}).get("uid") != "llm-slo-overview":
            raise ValueError("provisioned dashboard is unavailable")
    with urlopen(prometheus_url + "/-/ready", timeout=10) as response:
        if response.status != 200:
            raise ValueError("Prometheus is not ready")
    for expression in expressions:
        query_url = prometheus_url + "/api/v1/query?" + urlencode({"query": expression})
        with urlopen(query_url, timeout=10) as response:
            result = json.load(response)
        if result.get("status") != "success":
            raise ValueError(f"Prometheus rejected dashboard query: {expression}")


def main() -> int:
    parser = argparse.ArgumentParser(description="Verify observability provisioning")
    parser.add_argument("--root", type=Path, default=Path("."))
    parser.add_argument("--runtime", action="store_true")
    parser.add_argument("--grafana-url", default="http://127.0.0.1:3000")
    parser.add_argument("--prometheus-url", default="http://127.0.0.1:9090")
    args = parser.parse_args()
    try:
        expressions = dashboard_expressions(args.root)
        if args.runtime:
            verify_runtime(args.grafana_url, args.prometheus_url, expressions)
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"observability verification failed: {error}", file=sys.stderr)
        return 1
    print("observability: OK" + (" (runtime)" if args.runtime else ""))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
