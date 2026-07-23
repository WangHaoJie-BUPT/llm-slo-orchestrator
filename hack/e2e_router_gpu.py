#!/usr/bin/env python3
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import time
from typing import Any
from urllib.request import Request, urlopen

from e2e_fake import free_port, run, sha256, start, stop, wait_healthy, yaml_scalar


IMAGE_DIGEST = "sha256:f0b9a0dc75a9fca3b6811e3279367b2d6a448055a000bfd13859587d74cef268"
MODEL_REVISION = "c1899de289a04d12100db370d81485cdf75e47ca"
MODEL_WEIGHT_SHA256 = "f47f71177f32bcd101b7573ec9171e6a57f4f4d31148d38e382306f42996874b"


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def completion(base_url: str, stream: bool) -> tuple[int, dict[str, str], bytes, float]:
    payload = {
        "model": "qwen3-0.6b",
        "stream": stream,
        "messages": [{"role": "user", "content": "Reply with exactly: W2_OK"}],
        "temperature": 0,
        "max_tokens": 16,
        "chat_template_kwargs": {"enable_thinking": False},
    }
    request = Request(
        base_url + "/v1/chat/completions",
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    started = time.perf_counter()
    with urlopen(request, timeout=120) as response:
        headers = {key.lower(): value for key, value in response.headers.items()}
        body = response.read()
        elapsed_ms = (time.perf_counter() - started) * 1000
        return response.status, headers, body, elapsed_ms


def write_manifest(path: Path, values: dict[str, Any]) -> None:
    lines = [
        "schemaVersion: 1",
        f"runId: {yaml_scalar(values['run_id'])}",
        "evidenceLevel: L2",
        f"startedAt: {yaml_scalar(values['started_at'])}",
        f"endedAt: {yaml_scalar(values['ended_at'])}",
        "repository:",
        f"  branch: {yaml_scalar(values['branch'])}",
        f"  commit: {yaml_scalar(values['commit'])}",
        f"  dirtyAtStart: {yaml_scalar(values['dirty'])}",
        "inputs:",
        f"  policySha256: {yaml_scalar(values['policy_sha256'])}",
        f"  routerBinarySha256: {yaml_scalar(values['router_sha256'])}",
        "runtime:",
        f"  imageId: {yaml_scalar(values['image_id'])}",
        f"  expectedImageDigest: {yaml_scalar(IMAGE_DIGEST)}",
        '  model: "Qwen/Qwen3-0.6B"',
        f"  modelRevision: {yaml_scalar(MODEL_REVISION)}",
        f"  modelWeightSha256: {yaml_scalar(MODEL_WEIGHT_SHA256)}",
        f"  containerId: {yaml_scalar(values['container_id'])}",
        "checks:",
        f"  routerHealth: {yaml_scalar(values['router_health'])}",
        f"  backendHealth: {yaml_scalar(values['backend_health'])}",
        f"  selectedEndpointHeader: {yaml_scalar(values['endpoint_header'])}",
        f"  nonStreaming: {yaml_scalar(values['non_streaming'])}",
        f"  streaming: {yaml_scalar(values['streaming'])}",
        f"  routerMetrics: {yaml_scalar(values['router_metrics'])}",
        "cleanup:",
        f"  routerStopped: {yaml_scalar(values['router_stopped'])}",
        "  vllmRetainedForLocalUse: true",
    ]
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description="Validate the W2 router against the real vLLM endpoint")
    parser.add_argument("--router", type=Path, required=True)
    parser.add_argument("--backend-url", default="http://127.0.0.1:8000")
    parser.add_argument("--policy", type=Path, default=Path("config/example-policy.yaml"))
    parser.add_argument("--evidence-dir", type=Path, required=True)
    args = parser.parse_args()

    evidence_dir = args.evidence_dir
    logs_dir = evidence_dir / "logs"
    raw_dir = evidence_dir / "raw"
    metrics_dir = evidence_dir / "prometheus"
    logs_dir.mkdir(parents=True, exist_ok=True)
    raw_dir.mkdir(parents=True, exist_ok=True)
    metrics_dir.mkdir(parents=True, exist_ok=True)

    started_at = utc_now()
    dirty = bool(run("git", "status", "--porcelain"))
    commit = run("git", "rev-parse", "HEAD")
    branch = run("git", "branch", "--show-current")
    container_id = run(
        "docker", "compose", "-f", "deploy/compose/compose.yaml", "--profile", "gpu", "ps", "-q", "vllm"
    )
    if not container_id:
        raise RuntimeError("vLLM Compose container is not running")
    image_id = run("docker", "inspect", "--format", "{{.Image}}", container_id)
    if image_id != IMAGE_DIGEST:
        raise RuntimeError(f"unexpected vLLM image ID: {image_id}")

    wait_healthy(args.backend_url)
    with urlopen(args.backend_url + "/metrics", timeout=30) as response:
        (metrics_dir / "vllm-before.prom").write_bytes(response.read())

    router_url = f"http://127.0.0.1:{free_port()}"
    router_log = (logs_dir / "router.log").open("wb")
    router = start(
        [
            str(args.router),
            "--listen",
            router_url.removeprefix("http://"),
            "--config",
            str(args.policy),
            "--backend",
            f"vllm={args.backend_url}",
        ],
        router_log,
    )
    checks = {
        "router_health": False,
        "backend_health": True,
        "endpoint_header": False,
        "non_streaming": False,
        "streaming": False,
        "router_metrics": False,
    }
    router_stopped = False
    timings: dict[str, float] = {}
    try:
        wait_healthy(router_url)
        checks["router_health"] = True
        status, headers, body, elapsed_ms = completion(router_url, False)
        timings["non_streaming_total_ms"] = round(elapsed_ms, 3)
        non_streaming = json.loads(body)
        content = non_streaming["choices"][0]["message"]["content"]
        checks["endpoint_header"] = headers.get("x-llm-endpoint") == "vllm"
        checks["non_streaming"] = status == 200 and "W2_OK" in content
        (raw_dir / "non-streaming.json").write_bytes(body)

        stream_status, stream_headers, stream_body, stream_ms = completion(router_url, True)
        timings["streaming_total_ms"] = round(stream_ms, 3)
        checks["endpoint_header"] = checks["endpoint_header"] and stream_headers.get("x-llm-endpoint") == "vllm"
        checks["streaming"] = stream_status == 200 and b"W2_OK" in stream_body and b"data: [DONE]" in stream_body
        (raw_dir / "stream.txt").write_bytes(stream_body)

        with urlopen(router_url + "/metrics", timeout=30) as response:
            router_metrics = response.read()
        (metrics_dir / "router.prom").write_bytes(router_metrics)
        checks["router_metrics"] = b'router_requests_total{endpoint="vllm",code="200"} 2' in router_metrics
        with urlopen(args.backend_url + "/metrics", timeout=30) as response:
            (metrics_dir / "vllm-after.prom").write_bytes(response.read())
    finally:
        router_stopped = stop([router])
        router_log.close()

    ended_at = utc_now()
    passed = all(checks.values()) and router_stopped
    summary = {
        "run_id": evidence_dir.name,
        "evidence_level": "L2",
        "status": "passed" if passed else "failed",
        "checks": checks,
        "timings": timings,
        "selected_endpoint": "vllm",
        "router_stopped": router_stopped,
        "vllm_retained": True,
        "claim_boundary": "single real endpoint routing validation; not a policy comparison",
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
            "image_id": image_id,
            "container_id": container_id,
            **checks,
            "router_stopped": router_stopped,
        },
    )
    print(json.dumps(summary, indent=2, sort_keys=True))
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
