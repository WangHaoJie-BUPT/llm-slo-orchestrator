#!/usr/bin/env python3
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess
import time
from typing import Any
from urllib.request import Request, urlopen


IMAGE = "docker.1ms.run/vllm/vllm-openai:v0.25.1@sha256:f0b9a0dc75a9fca3b6811e3279367b2d6a448055a000bfd13859587d74cef268"
MODEL_REVISION = "c1899de289a04d12100db370d81485cdf75e47ca"
MODEL_WEIGHT_SHA256 = "f47f71177f32bcd101b7573ec9171e6a57f4f4d31148d38e382306f42996874b"


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def run(*command: str) -> str:
    return subprocess.run(
        command,
        check=True,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    ).stdout.strip()


def get(url: str, timeout: int = 30) -> tuple[int, bytes]:
    with urlopen(url, timeout=timeout) as response:
        return response.status, response.read()


def post_json(url: str, payload: dict[str, Any], timeout: int = 120) -> tuple[int, dict[str, Any], float]:
    request = Request(
        url,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    started = time.perf_counter()
    with urlopen(request, timeout=timeout) as response:
        body = json.load(response)
        elapsed_ms = (time.perf_counter() - started) * 1000
        return response.status, body, elapsed_ms


def stream_json(
    url: str, payload: dict[str, Any], raw_path: Path, timeout: int = 120
) -> tuple[int, int, float, float, str]:
    request = Request(
        url,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    started = time.perf_counter()
    first_content_ms: float | None = None
    event_count = 0
    content_parts: list[str] = []
    with urlopen(request, timeout=timeout) as response, raw_path.open("wb") as raw:
        status = response.status
        for line in response:
            raw.write(line)
            if line.startswith(b"data: ") and line.strip() != b"data: [DONE]":
                event_count += 1
                event = json.loads(line.removeprefix(b"data: "))
                content = event.get("choices", [{}])[0].get("delta", {}).get("content")
                if content:
                    content_parts.append(content)
                    if first_content_ms is None:
                        first_content_ms = (time.perf_counter() - started) * 1000
    total_ms = (time.perf_counter() - started) * 1000
    if first_content_ms is None:
        raise RuntimeError("stream returned no content tokens")
    return status, event_count, first_content_ms, total_ms, "".join(content_parts)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def yaml_string(value: Any) -> str:
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return str(value)
    return json.dumps(str(value), ensure_ascii=True)


def write_manifest(path: Path, values: dict[str, Any]) -> None:
    lines = [
        "schemaVersion: 1",
        f"runId: {yaml_string(values['run_id'])}",
        "evidenceLevel: L2",
        f"startedAt: {yaml_string(values['started_at'])}",
        f"endedAt: {yaml_string(values['ended_at'])}",
        "repository:",
        f"  branch: {yaml_string(values['branch'])}",
        f"  commit: {yaml_string(values['commit'])}",
        f"  dirtyAtStart: {yaml_string(values['dirty'])}",
        "versionsLock:",
        '  path: "versions.lock.yaml"',
        f"  sha256: {yaml_string(values['versions_sha256'])}",
        "runtime:",
        f"  image: {yaml_string(IMAGE)}",
        f"  imageId: {yaml_string(values['image_id'])}",
        f"  containerId: {yaml_string(values['container_id'])}",
        '  model: "Qwen/Qwen3-0.6B"',
        f"  modelRevision: {yaml_string(MODEL_REVISION)}",
        f"  modelWeightSha256: {yaml_string(MODEL_WEIGHT_SHA256)}",
        "hardware:",
        f"  nvidiaSmi: {yaml_string(values['nvidia_smi'])}",
        "scenario:",
        '  name: "w1-single-endpoint-smoke"',
        "  seed: 20260720",
        f"  baseUrl: {yaml_string(values['base_url'])}",
        "checks:",
        f"  health: {yaml_string(values['health'])}",
        f"  metrics: {yaml_string(values['metrics'])}",
        f"  nonStreaming: {yaml_string(values['non_streaming'])}",
        f"  streaming: {yaml_string(values['streaming'])}",
        "cleanup:",
        "  serviceRetainedForLocalUse: true",
    ]
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description="Run and record the W1 vLLM smoke checks")
    parser.add_argument("--base-url", default="http://127.0.0.1:8000")
    parser.add_argument("--model", default="qwen3-0.6b")
    parser.add_argument("--evidence-dir", type=Path, required=True)
    args = parser.parse_args()

    evidence_dir = args.evidence_dir
    logs_dir = evidence_dir / "logs"
    metrics_dir = evidence_dir / "prometheus"
    logs_dir.mkdir(parents=True, exist_ok=True)
    metrics_dir.mkdir(parents=True, exist_ok=True)

    started_at = utc_now()
    repository_dirty = bool(run("git", "status", "--porcelain"))
    commit = run("git", "rev-parse", "HEAD")
    branch = run("git", "branch", "--show-current")
    versions_sha = sha256(Path("versions.lock.yaml"))
    container_id = run("docker", "compose", "-f", "deploy/compose/compose.yaml", "--profile", "gpu", "ps", "-q", "vllm")
    if not container_id:
        raise RuntimeError("vLLM Compose container is not running")
    image_id = run("docker", "inspect", "--format", "{{.Image}}", container_id)
    nvidia_smi = run(
        "nvidia-smi",
        "--query-gpu=name,memory.total,driver_version",
        "--format=csv,noheader,nounits",
    )

    endpoint = f"{args.base_url.rstrip('/')}/v1/chat/completions"
    payload = {
        "model": args.model,
        "messages": [{"role": "user", "content": "Reply with exactly: W1_OK"}],
        "temperature": 0,
        "max_tokens": 16,
        "chat_template_kwargs": {"enable_thinking": False},
    }

    health_status, health_body = get(f"{args.base_url.rstrip('/')}/health")
    metrics_status, metrics_body = get(f"{args.base_url.rstrip('/')}/metrics")
    (metrics_dir / "vllm.prom").write_bytes(metrics_body)
    non_status, non_body, non_total_ms = post_json(endpoint, {**payload, "stream": False})
    (logs_dir / "non-streaming-response.json").write_text(
        json.dumps(non_body, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    stream_status, event_count, first_token_ms, stream_total_ms, stream_content = stream_json(
        endpoint, {**payload, "stream": True}, logs_dir / "stream-sse.txt"
    )

    non_content = non_body.get("choices", [{}])[0].get("message", {}).get("content", "")

    checks = {
        "health": health_status == 200,
        "metrics": metrics_status == 200 and b"# HELP" in metrics_body,
        "non_streaming": non_status == 200 and "W1_OK" in non_content,
        "streaming": stream_status == 200 and event_count > 0 and "W1_OK" in stream_content,
    }
    request_records = [
        {
            "kind": "non_streaming",
            "status": non_status,
            "total_ms": round(non_total_ms, 3),
            "response_id": non_body.get("id"),
            "usage": non_body.get("usage"),
        },
        {
            "kind": "streaming",
            "status": stream_status,
            "event_count": event_count,
            "first_token_ms": round(first_token_ms, 3),
            "total_ms": round(stream_total_ms, 3),
            "content": stream_content,
        },
    ]
    with (evidence_dir / "requests.jsonl").open("w", encoding="utf-8") as stream:
        for record in request_records:
            stream.write(json.dumps(record, sort_keys=True) + "\n")

    container_logs = run("docker", "logs", container_id)
    (logs_dir / "vllm.log").write_text(container_logs + "\n", encoding="utf-8")
    ended_at = utc_now()
    passed = all(checks.values())
    summary = {
        "run_id": evidence_dir.name,
        "evidence_level": "L2",
        "status": "passed" if passed else "failed",
        "checks": checks,
        "health_body_bytes": len(health_body),
        "metrics_body_bytes": len(metrics_body),
        "non_streaming_total_ms": round(non_total_ms, 3),
        "stream_first_token_ms": round(first_token_ms, 3),
        "stream_total_ms": round(stream_total_ms, 3),
        "stream_event_count": event_count,
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
            "dirty": repository_dirty,
            "versions_sha256": versions_sha,
            "image_id": image_id,
            "container_id": container_id,
            "nvidia_smi": nvidia_smi,
            "base_url": args.base_url,
            **checks,
        },
    )
    print(json.dumps(summary, indent=2, sort_keys=True))
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
