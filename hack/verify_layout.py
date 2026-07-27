#!/usr/bin/env python3
from pathlib import Path
import sys


REQUIRED_PATHS = (
    ".github/workflows/ci.yml",
    "cmd/fake-backend/main.go",
    "cmd/router/main.go",
    "cmd/profile-controller/main.go",
    "config/e2e-fake-policy.yaml",
    "config/example-policy.yaml",
    "config/qwen3-0.6b-profile.yaml",
    "config/reconciled-profile.json",
    "deploy/compose/compose.yaml",
    "deploy/compose/grafana/provisioning/dashboards/provider.yaml",
    "deploy/compose/grafana/provisioning/datasources/prometheus.yaml",
    "dashboards/llm-slo-overview.json",
    "deploy/docker/fake-backend.Dockerfile",
    "deploy/docker/router.Dockerfile",
    "deploy/kubernetes/w2-fake.yaml",
    "docs/adr/0001-scope-and-evidence-boundary.md",
    "docs/adr/0002-standalone-router-before-epp.md",
    "docs/adr/0003-file-profile-reconciliation.md",
    "docs/compatibility-matrix.md",
    "docs/demo/8-minute.md",
    "docs/operations/slo.md",
    "docs/operations/profile.md",
    "docs/project-overview-zh-CN.md",
    "docs/reports/w0-environment-freeze.md",
    "docs/reports/w1-guidellm-capacity.md",
    "docs/reports/w2-fake-router.md",
    "docs/reports/w2-kind-router.md",
    "docs/reports/w2-real-router.md",
    "docs/reports/w3-fault-recovery.md",
    "docs/reports/generated-status.md",
    "docs/testing/fault-matrix.md",
    "evidence/README.md",
    "evidence/w0-20260720/manifest.yaml",
    "evidence/w0-20260720/summary.json",
    "internal/api/types.go",
    "internal/admission/limiter.go",
    "internal/backend/client.go",
    "internal/backend/metrics.go",
    "internal/clock/clock.go",
    "internal/config/config.go",
    "internal/picker/picker.go",
    "internal/profile/profile.go",
    "internal/profile/reconciler.go",
    "internal/router/handler.go",
    "experiments/capacity.yaml",
    "experiments/h1-h4.json",
    "hack/demo.py",
    "hack/e2e_fake.py",
    "hack/e2e_kind.py",
    "hack/e2e_router_gpu.py",
    "hack/fault_smoke.py",
    "hack/generate_report.py",
    "hack/test_generate_report.py",
    "hack/test_verify_experiments.py",
    "hack/verify_loadgen.py",
    "hack/verify_experiments.py",
    "hack/verify_observability.py",
    "hack/w1_guidellm.py",
    "loadgen/requirements.lock",
    "versions.lock.yaml",
)


def main() -> int:
    missing = [path for path in REQUIRED_PATHS if not Path(path).exists()]
    if missing:
        for path in missing:
            print(f"missing required path: {path}", file=sys.stderr)
        return 1
    print("repository layout: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
