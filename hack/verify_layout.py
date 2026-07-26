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
    "deploy/compose/compose.yaml",
    "deploy/docker/fake-backend.Dockerfile",
    "deploy/docker/router.Dockerfile",
    "deploy/kubernetes/w2-fake.yaml",
    "docs/adr/0001-scope-and-evidence-boundary.md",
    "docs/adr/0002-standalone-router-before-epp.md",
    "docs/compatibility-matrix.md",
    "docs/operations/slo.md",
    "docs/reports/w0-environment-freeze.md",
    "docs/reports/w1-guidellm-capacity.md",
    "docs/reports/w2-fake-router.md",
    "docs/reports/w2-kind-router.md",
    "docs/reports/w2-real-router.md",
    "docs/reports/generated-status.md",
    "evidence/README.md",
    "evidence/w0-20260720/manifest.yaml",
    "evidence/w0-20260720/summary.json",
    "internal/api/types.go",
    "internal/backend/client.go",
    "internal/clock/clock.go",
    "internal/config/config.go",
    "internal/picker/picker.go",
    "internal/router/handler.go",
    "experiments/capacity.yaml",
    "hack/e2e_fake.py",
    "hack/e2e_kind.py",
    "hack/e2e_router_gpu.py",
    "hack/fault_smoke.py",
    "hack/generate_report.py",
    "hack/test_generate_report.py",
    "hack/verify_loadgen.py",
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
