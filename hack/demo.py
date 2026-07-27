#!/usr/bin/env python3
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import time
from typing import Any


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def stage_name(value: str) -> str:
    return value.replace("_", "-")


def tail(path: Path, line_count: int = 20) -> str:
    try:
        return "\n".join(path.read_text(encoding="utf-8", errors="replace").splitlines()[-line_count:])
    except OSError:
        return ""


def run_stage(name: str, command: list[str], output_dir: Path) -> dict[str, Any]:
    log_path = output_dir / f"{stage_name(name)}.log"
    print(f"[{name}] running")
    started_at = utc_now()
    start = time.monotonic()
    with log_path.open("w", encoding="utf-8", newline="\n") as log:
        result = subprocess.run(command, stdout=log, stderr=subprocess.STDOUT, text=True, check=False)
    duration = round(time.monotonic() - start, 3)
    status = "passed" if result.returncode == 0 else "failed"
    print(f"[{name}] {status} ({duration:.3f}s)")
    if result.returncode:
        excerpt = tail(log_path)
        if excerpt:
            print(excerpt, file=sys.stderr)
    return {
        "name": name,
        "status": status,
        "command": command,
        "return_code": result.returncode,
        "started_at": started_at,
        "duration_seconds": duration,
        "log": str(log_path),
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="Run the local core demonstration")
    parser.add_argument("--real-gpu", action="store_true", help="include the single real vLLM L2 path")
    parser.add_argument("--backend-url", default="http://127.0.0.1:8000")
    parser.add_argument("--grafana-url", default="http://127.0.0.1:3000")
    parser.add_argument("--prometheus-url", default="http://127.0.0.1:9090")
    parser.add_argument("--output-dir", type=Path)
    args = parser.parse_args()

    timestamp = datetime.now().strftime("%Y%m%dT%H%M%S")
    output_dir = args.output_dir or Path(tempfile.gettempdir()) / f"llm-slo-demo-{timestamp}"
    output_dir.mkdir(parents=True, exist_ok=False)
    started_at = utc_now()
    stages: list[dict[str, Any]] = []
    python = sys.executable

    commands = [
        ("verify", ["make", "verify"]),
        ("build", ["make", "build"]),
        (
            "fake_path",
            [
                python,
                "hack/e2e_fake.py",
                "--router",
                "bin/router",
                "--backend",
                "bin/fake-backend",
                "--evidence-dir",
                str(output_dir / "fake-path"),
            ],
        ),
        (
            "fault_recovery",
            [
                python,
                "hack/fault_smoke.py",
                "--router",
                "bin/router",
                "--backend",
                "bin/fake-backend",
                "--evidence-dir",
                str(output_dir / "fault-recovery"),
            ],
        ),
        (
            "observability_runtime",
            [
                python,
                "hack/verify_observability.py",
                "--runtime",
                "--grafana-url",
                args.grafana_url,
                "--prometheus-url",
                args.prometheus_url,
            ],
        ),
    ]
    if args.real_gpu:
        commands.append(
            (
                "real_gpu_path",
                [
                    python,
                    "hack/e2e_router_gpu.py",
                    "--router",
                    "bin/router",
                    "--backend-url",
                    args.backend_url,
                    "--evidence-dir",
                    str(output_dir / "real-gpu-path"),
                ],
            )
        )

    failure = None
    for name, command in commands:
        result = run_stage(name, command, output_dir)
        stages.append(result)
        if result["status"] == "failed":
            failure = name
            break

    passed = failure is None
    summary = {
        "schema_version": 1,
        "run_id": output_dir.name,
        "status": "passed" if passed else "failed",
        "started_at": started_at,
        "ended_at": utc_now(),
        "real_gpu_requested": args.real_gpu,
        "highest_evidence_level": "L2" if passed and args.real_gpu else "L1",
        "failed_stage": failure,
        "stages": stages,
        "output_dir": str(output_dir),
        "claim_boundary": (
            "This demo validates local mechanics and, when requested, one real accelerator endpoint. "
            "It does not establish the H1-H3 multi-endpoint policy benefits, which require L3 evidence."
        ),
    }
    summary_path = output_dir / "summary.json"
    summary_path.write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8", newline="\n")
    print(f"demo summary: {summary_path}")
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
