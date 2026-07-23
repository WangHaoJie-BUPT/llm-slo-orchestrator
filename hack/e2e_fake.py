#!/usr/bin/env python3
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import socket
import subprocess
import time
from typing import Any, BinaryIO
from urllib.request import Request, urlopen


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run(*command: str) -> str:
    return subprocess.run(
        command,
        check=True,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    ).stdout.strip()


def free_port() -> int:
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def wait_healthy(base_url: str, timeout: float = 15) -> None:
    deadline = time.monotonic() + timeout
    last_error: Exception | None = None
    while time.monotonic() < deadline:
        try:
            with urlopen(base_url + "/health", timeout=1) as response:
                if response.status == 200:
                    return
        except Exception as error:
            last_error = error
        time.sleep(0.05)
    raise RuntimeError(f"service did not become healthy: {base_url}: {last_error}")


def completion(base_url: str, stream: bool) -> tuple[int, dict[str, str], bytes]:
    payload = {
        "model": "qwen3-0.6b-pinned",
        "stream": stream,
        "messages": [{"role": "user", "content": "hello"}],
    }
    request = Request(
        base_url + "/v1/chat/completions",
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urlopen(request, timeout=10) as response:
        headers = {key.lower(): value for key, value in response.headers.items()}
        return response.status, headers, response.read()


def yaml_scalar(value: Any) -> str:
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return str(value)
    return json.dumps(str(value), ensure_ascii=True)


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
        "checks:",
        f"  health: {yaml_scalar(values['health'])}",
        f"  roundRobin: {yaml_scalar(values['round_robin'])}",
        f"  nonStreaming: {yaml_scalar(values['non_streaming'])}",
        f"  streaming: {yaml_scalar(values['streaming'])}",
        f"  metrics: {yaml_scalar(values['metrics'])}",
        "cleanup:",
        f"  processesStopped: {yaml_scalar(values['processes_stopped'])}",
    ]
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def start(command: list[str], log_file: BinaryIO) -> subprocess.Popen[bytes]:
    return subprocess.Popen(command, stdout=log_file, stderr=subprocess.STDOUT)


def stop(processes: list[subprocess.Popen[bytes]]) -> bool:
    for process in reversed(processes):
        if process.poll() is None:
            process.terminate()
    stopped = True
    for process in reversed(processes):
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
            stopped = False
    return stopped


def main() -> int:
    parser = argparse.ArgumentParser(description="Run the W2 command-level fake backend test")
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
    processes_stopped = False
    checks = {
        "health": False,
        "round_robin": False,
        "non_streaming": False,
        "streaming": False,
        "metrics": False,
    }
    try:
        for index, backend_id in enumerate(("fake-a", "fake-b")):
            log_file = (logs_dir / f"{backend_id}.log").open("wb")
            log_files.append(log_file)
            processes.append(
                start(
                    [
                        str(args.backend),
                        "--listen",
                        f"127.0.0.1:{ports[index]}",
                        "--id",
                        backend_id,
                        "--base-delay",
                        "2ms",
                        "--token-delay",
                        "2ms",
                    ],
                    log_file,
                )
            )
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
                ],
                router_log,
            )
        )
        wait_healthy(router_url)
        checks["health"] = True

        first_status, first_headers, first_body = completion(router_url, False)
        second_status, second_headers, second_body = completion(router_url, False)
        first_endpoint = first_headers.get("x-llm-endpoint")
        second_endpoint = second_headers.get("x-llm-endpoint")
        checks["round_robin"] = (first_endpoint, second_endpoint) == ("fake-a", "fake-b")
        checks["non_streaming"] = (
            first_status == 200
            and second_status == 200
            and b"FAKE_fake-a" in first_body
            and b"FAKE_fake-b" in second_body
        )

        stream_status, stream_headers, stream_body = completion(router_url, True)
        checks["streaming"] = (
            stream_status == 200
            and stream_headers.get("x-llm-endpoint") == "fake-a"
            and b"data: [DONE]" in stream_body
            and b"FAKE_" in stream_body
            and b"fake-a" in stream_body
        )
        (raw_dir / "stream.txt").write_bytes(stream_body)

        with urlopen(router_url + "/metrics", timeout=5) as response:
            metrics_body = response.read()
        (raw_dir / "router.prom").write_bytes(metrics_body)
        checks["metrics"] = (
            b'router_requests_total{endpoint="fake-a",code="200"} 2' in metrics_body
            and b'router_requests_total{endpoint="fake-b",code="200"} 1' in metrics_body
        )
    finally:
        processes_stopped = stop(processes)
        for log_file in log_files:
            log_file.close()

    ended_at = utc_now()
    passed = all(checks.values()) and processes_stopped
    summary = {
        "run_id": evidence_dir.name,
        "evidence_level": "L1",
        "status": "passed" if passed else "failed",
        "checks": checks,
        "selected_endpoints": ["fake-a", "fake-b", "fake-a"],
        "processes_stopped": processes_stopped,
    }
    (evidence_dir / "summary.json").write_text(
        json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8"
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
