#!/usr/bin/env python3
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
from typing import Any
from urllib.request import urlopen

import yaml


IMAGE_DIGEST = "sha256:f0b9a0dc75a9fca3b6811e3279367b2d6a448055a000bfd13859587d74cef268"
MODEL_REVISION = "c1899de289a04d12100db370d81485cdf75e47ca"
MODEL_WEIGHT_SHA256 = "f47f71177f32bcd101b7573ec9171e6a57f4f4d31148d38e382306f42996874b"


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


def run_guidellm(command: list[str], log_path: Path) -> None:
    environment = os.environ.copy()
    environment.update({"HF_HUB_OFFLINE": "1", "TRANSFORMERS_OFFLINE": "1"})
    result = subprocess.run(
        command,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        env=environment,
    )
    log_path.write_text(result.stdout, encoding="utf-8")
    if result.returncode != 0:
        raise RuntimeError(f"GuideLLM failed; see {log_path}")


def scalar(value: Any) -> str:
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return str(value)
    return json.dumps(str(value), ensure_ascii=True)


def successful(metrics: dict[str, Any], name: str) -> dict[str, Any]:
    return metrics[name]["successful"]


def summarize(report_path: Path, objectives: dict[str, float]) -> list[dict[str, Any]]:
    report = json.loads(report_path.read_text(encoding="utf-8"))
    rows: list[dict[str, Any]] = []
    for benchmark in report["benchmarks"]:
        metrics = benchmark["metrics"]
        totals = metrics["request_totals"]
        duration = float(benchmark["duration"])
        strategy = benchmark["config"]["strategy"]
        ttft = successful(metrics, "time_to_first_token_ms")
        tpot = successful(metrics, "time_per_output_token_ms")
        latency = successful(metrics, "request_latency")
        output_tokens = successful(metrics, "output_token_count")
        row = {
            "strategy": strategy["type_"],
            "requested_rate": strategy.get("rate"),
            "duration_seconds": round(duration, 3),
            "successful": totals["successful"],
            "errored": totals["errored"],
            "incomplete": totals["incomplete"],
            "achieved_requests_per_second": round(
                successful(metrics, "requests_per_second")["mean"], 3
            ),
            "output_tokens_per_second": round(output_tokens["total_sum"] / duration, 3),
            "request_latency_p50_ms": round(latency["percentiles"]["p50"] * 1000, 3),
            "request_latency_p95_ms": round(latency["percentiles"]["p95"] * 1000, 3),
            "ttft_p50_ms": round(ttft["percentiles"]["p50"], 3),
            "ttft_p95_ms": round(ttft["percentiles"]["p95"], 3),
            "tpot_p50_ms": round(tpot["percentiles"]["p50"], 3),
            "tpot_p95_ms": round(tpot["percentiles"]["p95"], 3),
        }
        row["candidate_slo_met"] = (
            row["errored"] == 0
            and row["incomplete"] == 0
            and row["ttft_p95_ms"] <= objectives["ttftP95Ms"]
            and row["tpot_p95_ms"] <= objectives["tpotP95Ms"]
        )
        rows.append(row)
    return rows


def guidellm_command(
    executable: str,
    base_url: str,
    tokenizer: str,
    input_tokens: int,
    output_tokens: int,
    seed: int,
    profile: str,
    constraint: str,
    json_path: Path,
    csv_path: Path,
) -> list[str]:
    return [
        executable,
        "run",
        "--backend",
        f"kind=openai_http,target={base_url},request_format=/v1/chat/completions",
        "--profile",
        profile,
        "--constraint",
        constraint,
        "--constraint",
        "kind=max_errors,count=1",
        "--tokenizer",
        f"kind=huggingface_auto,model={tokenizer}",
        "--data",
        f"kind=synthetic_text,prompt_tokens={input_tokens},output_tokens={output_tokens}",
        "--seed",
        f"kind=static,value={seed}",
        "--output",
        f"kind=json,path={json_path}",
        "--output",
        f"kind=csv,path={csv_path}",
        "--disable-console-interactive",
    ]


def write_manifest(path: Path, values: dict[str, Any]) -> None:
    lines = [
        "schemaVersion: 1",
        f"runId: {scalar(values['run_id'])}",
        "evidenceLevel: L2",
        f"startedAt: {scalar(values['started_at'])}",
        f"endedAt: {scalar(values['ended_at'])}",
        "repository:",
        f"  branch: {scalar(values['branch'])}",
        f"  commit: {scalar(values['commit'])}",
        f"  dirtyAtStart: {scalar(values['dirty'])}",
        "inputs:",
        f"  versionsLockSha256: {scalar(values['versions_sha256'])}",
        f"  loadgenLockSha256: {scalar(values['loadgen_sha256'])}",
        f"  scenarioSha256: {scalar(values['scenario_sha256'])}",
        "runtime:",
        f"  imageId: {scalar(values['image_id'])}",
        f"  expectedImageDigest: {scalar(IMAGE_DIGEST)}",
        '  model: "Qwen/Qwen3-0.6B"',
        f"  modelRevision: {scalar(MODEL_REVISION)}",
        f"  modelWeightSha256: {scalar(MODEL_WEIGHT_SHA256)}",
        f"  containerId: {scalar(values['container_id'])}",
        f"  nvidiaSmi: {scalar(values['nvidia_smi'])}",
        "loadgen:",
        f"  guidellm: {scalar(values['guidellm_version'])}",
        f"  python: {scalar(values['python_version'])}",
        "checks:",
        f"  smoke: {scalar(values['smoke_passed'])}",
        f"  capacityCurve: {scalar(values['capacity_passed'])}",
        f"  zeroRequestErrors: {scalar(values['zero_errors'])}",
        "cleanup:",
        "  serviceRetainedForLocalUse: true",
    ]
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description="Run and record the W1 GuideLLM calibration")
    parser.add_argument("--base-url", default="http://127.0.0.1:8000")
    parser.add_argument("--scenario", type=Path, required=True)
    parser.add_argument("--evidence-dir", type=Path, required=True)
    parser.add_argument("--guidellm", default=str(Path(sys.executable).with_name("guidellm")))
    args = parser.parse_args()

    scenario = yaml.safe_load(args.scenario.read_text(encoding="utf-8"))
    evidence_dir = args.evidence_dir
    raw_dir = evidence_dir / "raw"
    logs_dir = evidence_dir / "logs"
    metrics_dir = evidence_dir / "prometheus"
    raw_dir.mkdir(parents=True, exist_ok=True)
    logs_dir.mkdir(parents=True, exist_ok=True)
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
    health = run("docker", "inspect", "--format", "{{.State.Health.Status}}", container_id)
    if health != "healthy":
        raise RuntimeError(f"vLLM container health is {health}")

    image_id = run("docker", "inspect", "--format", "{{.Image}}", container_id)
    if image_id != IMAGE_DIGEST:
        raise RuntimeError(f"unexpected image ID: {image_id}")
    nvidia_smi = run(
        "nvidia-smi",
        "--query-gpu=name,memory.total,driver_version",
        "--format=csv,noheader,nounits",
    )
    guidellm_version = run(args.guidellm, "--version")
    python_version = run(sys.executable, "--version")

    with urlopen(f"{args.base_url.rstrip('/')}/metrics", timeout=30) as response:
        (metrics_dir / "before.prom").write_bytes(response.read())

    request = scenario["request"]
    smoke = scenario["smoke"]
    capacity = scenario["capacity"]
    common = {
        "executable": args.guidellm,
        "base_url": args.base_url,
        "tokenizer": scenario["tokenizerPath"],
        "input_tokens": request["inputTokens"],
        "output_tokens": request["outputTokens"],
        "seed": scenario["seed"],
    }

    smoke_command = guidellm_command(
        **common,
        profile=f"kind=constant,rate={smoke['ratePerSecond']}",
        constraint=f"kind=max_requests,count={smoke['count']}",
        json_path=raw_dir / "smoke.json",
        csv_path=raw_dir / "smoke.csv",
    )
    run_guidellm(smoke_command, logs_dir / "guidellm-smoke.log")

    capacity_command = guidellm_command(
        **common,
        profile=(
            f"kind=constant,rate={capacity['ratesPerSecond'][0]},"
            f"warmup={capacity['warmupFraction']},cooldown={capacity['cooldownFraction']}"
        ),
        constraint=f"kind=max_duration,seconds={capacity['durationSecondsPerRate']}",
        json_path=raw_dir / "capacity.json",
        csv_path=raw_dir / "capacity.csv",
    )
    capacity_command.extend(
        ["--override", "profile.rate", ",".join(str(rate) for rate in capacity["ratesPerSecond"])]
    )
    run_guidellm(capacity_command, logs_dir / "guidellm-capacity.log")

    with urlopen(f"{args.base_url.rstrip('/')}/metrics", timeout=30) as response:
        (metrics_dir / "after.prom").write_bytes(response.read())
    (logs_dir / "vllm.log").write_text(
        run("docker", "logs", "--since", started_at, container_id) + "\n", encoding="utf-8"
    )

    objectives = scenario["objectives"]
    smoke_rows = summarize(raw_dir / "smoke.json", objectives)
    capacity_rows = summarize(raw_dir / "capacity.json", objectives)
    smoke_passed = len(smoke_rows) == 1 and smoke_rows[0]["successful"] == smoke["count"]
    capacity_passed = len(capacity_rows) == len(capacity["ratesPerSecond"]) and all(
        row["successful"] > 0 for row in capacity_rows
    )
    zero_errors = all(row["errored"] == 0 and row["incomplete"] == 0 for row in smoke_rows + capacity_rows)
    safe_rates = [row["requested_rate"] for row in capacity_rows if row["candidate_slo_met"]]
    summary = {
        "run_id": evidence_dir.name,
        "evidence_level": "L2",
        "status": "passed" if smoke_passed and capacity_passed and zero_errors else "failed",
        "candidate_objectives": objectives,
        "smoke": smoke_rows[0] if smoke_rows else None,
        "capacity": capacity_rows,
        "highest_tested_rate_meeting_candidate_slo": max(safe_rates) if safe_rates else None,
        "claim_boundary": "single-host exploratory calibration; not a production capacity guarantee",
    }
    (evidence_dir / "summary.json").write_text(
        json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    ended_at = utc_now()
    write_manifest(
        evidence_dir / "manifest.yaml",
        {
            "run_id": evidence_dir.name,
            "started_at": started_at,
            "ended_at": ended_at,
            "branch": branch,
            "commit": commit,
            "dirty": dirty,
            "versions_sha256": sha256(Path("versions.lock.yaml")),
            "loadgen_sha256": sha256(Path("loadgen/requirements.lock")),
            "scenario_sha256": sha256(args.scenario),
            "image_id": image_id,
            "container_id": container_id,
            "nvidia_smi": nvidia_smi,
            "guidellm_version": guidellm_version,
            "python_version": python_version,
            "smoke_passed": smoke_passed,
            "capacity_passed": capacity_passed,
            "zero_errors": zero_errors,
        },
    )
    print(json.dumps(summary, indent=2, sort_keys=True))
    return 0 if summary["status"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
