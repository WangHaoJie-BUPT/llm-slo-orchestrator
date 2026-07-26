#!/usr/bin/env python3
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import subprocess
import time
from typing import Any, BinaryIO
from urllib.request import urlopen

from e2e_fake import completion, free_port, run, sha256, start, stop, wait_healthy, yaml_scalar


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def wait_metric(base_url: str, expected: bytes, timeout: float = 10) -> bytes:
    deadline = time.monotonic() + timeout
    latest = b""
    while time.monotonic() < deadline:
        try:
            with urlopen(base_url + "/metrics", timeout=1) as response:
                latest = response.read()
            if expected in latest:
                return latest
        except Exception:
            pass
        time.sleep(0.025)
    raise RuntimeError(f"metric did not reach expected state: {expected!r}")


def write_manifest(path: Path, values: dict[str, Any]) -> None:
    lines = [
        "schemaVersion: 1",
        f"runId: {yaml_scalar(values['run_id'])}",
        "evidenceLevel: L1",
        f"startedAt: {yaml_scalar(values['started_at'])}",
        f"endedAt: {yaml_scalar(values['ended_at'])}",
        "repository:",
        f"  branch: {yaml_scalar(values['branch'])}",
        f"  commit: {yaml_scalar(values['commit'])}",
        f"  dirtyAtStart: {yaml_scalar(values['dirty'])}",
        "inputs:",
        f"  policySha256: {yaml_scalar(values['policy_sha256'])}",
        f"  routerBinarySha256: {yaml_scalar(values['router_sha256'])}",
        f"  backendBinarySha256: {yaml_scalar(values['backend_sha256'])}",
        "  healthInterval: \"50ms\"",
        "  healthTimeout: \"25ms\"",
        "  failureThreshold: 2",
        "  successThreshold: 2",
        "checks:",
        f"  initialHealthy: {yaml_scalar(values['initial_healthy'])}",
        f"  failureDetected: {yaml_scalar(values['failure_detected'])}",
        f"  survivingTraffic: {yaml_scalar(values['surviving_traffic'])}",
        f"  recoveryDetected: {yaml_scalar(values['recovery_detected'])}",
        f"  recoveredTraffic: {yaml_scalar(values['recovered_traffic'])}",
        f"  healthMetrics: {yaml_scalar(values['health_metrics'])}",
        "cleanup:",
        f"  processesStopped: {yaml_scalar(values['processes_stopped'])}",
    ]
    path.write_text("\n".join(lines) + "\n", encoding="utf-8", newline="\n")


def main() -> int:
    parser = argparse.ArgumentParser(description="Run deterministic backend failure and recovery checks")
    parser.add_argument("--router", type=Path, required=True)
    parser.add_argument("--backend", type=Path, required=True)
    parser.add_argument("--policy", type=Path, default=Path("config/e2e-fake-policy.yaml"))
    parser.add_argument("--evidence-dir", type=Path, required=True)
    args = parser.parse_args()

    evidence_dir = args.evidence_dir
    logs_dir = evidence_dir / "logs"
    raw_dir = evidence_dir / "raw"
    logs_dir.mkdir(parents=True, exist_ok=True)
    raw_dir.mkdir(parents=True, exist_ok=True)
    started_at = utc_now()
    dirty = bool(run("git", "status", "--porcelain"))
    commit = run("git", "rev-parse", "HEAD")
    branch = run("git", "branch", "--show-current")

    ports = [free_port(), free_port(), free_port()]
    backend_urls = [f"http://127.0.0.1:{ports[0]}", f"http://127.0.0.1:{ports[1]}"]
    router_url = f"http://127.0.0.1:{ports[2]}"
    processes: list[subprocess.Popen[bytes]] = []
    log_files: list[BinaryIO] = []
    checks = {
        "initial_healthy": False,
        "failure_detected": False,
        "surviving_traffic": False,
        "recovery_detected": False,
        "recovered_traffic": False,
        "health_metrics": False,
    }
    failure = ""
    selected_during_failure: list[str] = []
    selected_after_recovery: list[str] = []
    processes_stopped = False
    backend_a: subprocess.Popen[bytes] | None = None

    def launch_backend(backend_id: str, port: int, log_name: str) -> subprocess.Popen[bytes]:
        log_file = (logs_dir / log_name).open("wb")
        log_files.append(log_file)
        process = start(
            [
                str(args.backend),
                "--listen",
                f"127.0.0.1:{port}",
                "--id",
                backend_id,
                "--base-delay",
                "2ms",
                "--token-delay",
                "2ms",
            ],
            log_file,
        )
        processes.append(process)
        return process

    try:
        backend_a = launch_backend("fake-a", ports[0], "fake-a-before.log")
        launch_backend("fake-b", ports[1], "fake-b.log")
        for backend_url in backend_urls:
            wait_healthy(backend_url)

        router_log = (logs_dir / "router.log").open("wb")
        log_files.append(router_log)
        processes.append(
            start(
                [
                    str(args.router),
                    "--listen",
                    f"127.0.0.1:{ports[2]}",
                    "--config",
                    str(args.policy),
                    "--backend",
                    f"fake-a={backend_urls[0]}",
                    "--backend",
                    f"fake-b={backend_urls[1]}",
                    "--health-interval",
                    "50ms",
                    "--health-timeout",
                    "25ms",
                    "--health-failure-threshold",
                    "2",
                    "--health-success-threshold",
                    "2",
                ],
                router_log,
            )
        )
        wait_healthy(router_url)
        initial_metrics = wait_metric(router_url, b'router_backend_healthy{endpoint="fake-a"} 1')
        checks["initial_healthy"] = b'router_backend_healthy{endpoint="fake-b"} 1' in initial_metrics

        if not stop([backend_a]):
            raise RuntimeError("failed to stop fake-a cleanly")
        failed_metrics = wait_metric(router_url, b'router_backend_healthy{endpoint="fake-a"} 0')
        checks["failure_detected"] = b'router_backend_healthy{endpoint="fake-b"} 1' in failed_metrics
        (raw_dir / "metrics-during-failure.prom").write_bytes(failed_metrics)

        failure_records: list[dict[str, Any]] = []
        for _ in range(4):
            status, headers, body = completion(router_url, False)
            endpoint = headers.get("x-llm-endpoint", "")
            selected_during_failure.append(endpoint)
            failure_records.append({"status": status, "endpoint": endpoint, "body": json.loads(body)})
        checks["surviving_traffic"] = (
            selected_during_failure == ["fake-b"] * 4
            and all(record["status"] == 200 for record in failure_records)
        )
        (raw_dir / "requests-during-failure.json").write_text(
            json.dumps(failure_records, indent=2, sort_keys=True) + "\n", encoding="utf-8", newline="\n"
        )

        backend_a = launch_backend("fake-a", ports[0], "fake-a-after.log")
        wait_healthy(backend_urls[0])
        recovered_metrics = wait_metric(router_url, b'router_backend_healthy{endpoint="fake-a"} 1')
        checks["recovery_detected"] = b'router_backend_healthy{endpoint="fake-b"} 1' in recovered_metrics
        checks["health_metrics"] = b"router_backend_health_check_age_seconds" in recovered_metrics
        (raw_dir / "metrics-after-recovery.prom").write_bytes(recovered_metrics)

        recovery_records: list[dict[str, Any]] = []
        for _ in range(4):
            status, headers, body = completion(router_url, False)
            endpoint = headers.get("x-llm-endpoint", "")
            selected_after_recovery.append(endpoint)
            recovery_records.append({"status": status, "endpoint": endpoint, "body": json.loads(body)})
        checks["recovered_traffic"] = (
            "fake-a" in selected_after_recovery
            and "fake-b" in selected_after_recovery
            and all(record["status"] == 200 for record in recovery_records)
        )
        (raw_dir / "requests-after-recovery.json").write_text(
            json.dumps(recovery_records, indent=2, sort_keys=True) + "\n", encoding="utf-8", newline="\n"
        )
    except Exception as error:
        failure = f"{type(error).__name__}: {error}"
    finally:
        processes_stopped = stop(processes)
        for log_file in log_files:
            log_file.close()

    ended_at = utc_now()
    passed = all(checks.values()) and processes_stopped and not failure
    summary = {
        "run_id": evidence_dir.name,
        "evidence_level": "L1",
        "status": "passed" if passed else "failed",
        "checks": checks,
        "selected_during_failure": selected_during_failure,
        "selected_after_recovery": selected_after_recovery,
        "processes_stopped": processes_stopped,
        "failure": failure or None,
        "claim_boundary": "deterministic process fault with fake backends; not real accelerator resilience evidence",
    }
    (evidence_dir / "summary.json").write_text(
        json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8", newline="\n"
    )
    write_manifest(
        evidence_dir / "manifest.yaml",
        {
            "run_id": evidence_dir.name,
            "started_at": started_at,
            "ended_at": ended_at,
            "branch": branch,
            "commit": commit,
            "dirty": dirty,
            "policy_sha256": sha256(args.policy),
            "router_sha256": sha256(args.router),
            "backend_sha256": sha256(args.backend),
            **checks,
            "processes_stopped": processes_stopped,
        },
    )
    print(json.dumps(summary, indent=2, sort_keys=True))
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
