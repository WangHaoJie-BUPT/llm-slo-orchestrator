#!/usr/bin/env python3
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import subprocess
from typing import Any, BinaryIO
from urllib.request import urlopen
import uuid

from e2e_fake import completion, free_port, run, sha256, start, stop, wait_healthy, yaml_scalar


NODE_IMAGE = (
    "docker.m.daocloud.io/kindest/node:v1.36.1@"
    "sha256:3489c7674813ba5d8b1a9977baea8a6e553784dab7b84759d1014dbd78f7ebd5"
)


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


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
        f"  kubernetesManifestSha256: {yaml_scalar(values['manifest_sha256'])}",
        f"  routerImage: {yaml_scalar(values['router_image'])}",
        f"  routerImageId: {yaml_scalar(values['router_image_id'])}",
        f"  backendImage: {yaml_scalar(values['backend_image'])}",
        f"  backendImageId: {yaml_scalar(values['backend_image_id'])}",
        f"  kindNodeImage: {yaml_scalar(NODE_IMAGE)}",
        "runtime:",
        f"  clusterName: {yaml_scalar(values['cluster_name'])}",
        f"  kindVersion: {yaml_scalar(values['kind_version'])}",
        f"  kubernetesVersion: {yaml_scalar(values['kubernetes_version'])}",
        "checks:",
        f"  clusterReady: {yaml_scalar(values['cluster_ready'])}",
        f"  deploymentsReady: {yaml_scalar(values['deployments_ready'])}",
        f"  routerHealth: {yaml_scalar(values['router_health'])}",
        f"  roundRobin: {yaml_scalar(values['round_robin'])}",
        f"  nonStreaming: {yaml_scalar(values['non_streaming'])}",
        f"  streaming: {yaml_scalar(values['streaming'])}",
        f"  metrics: {yaml_scalar(values['metrics'])}",
        "cleanup:",
        f"  portForwardStopped: {yaml_scalar(values['port_forward_stopped'])}",
        f"  clusterDeleted: {yaml_scalar(values['cluster_deleted'])}",
    ]
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def capture(command: list[str], destination: Path) -> bool:
    result = subprocess.run(command, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    destination.write_text(result.stdout, encoding="utf-8")
    return result.returncode == 0


def streamed_content(body: bytes) -> str:
    parts: list[str] = []
    for line in body.splitlines():
        if not line.startswith(b"data: ") or line == b"data: [DONE]":
            continue
        event = json.loads(line.removeprefix(b"data: "))
        choices = event.get("choices") or []
        if choices:
            content = choices[0].get("delta", {}).get("content")
            if content:
                parts.append(content)
    return "".join(parts)


def main() -> int:
    parser = argparse.ArgumentParser(description="Run the W2 fake topology in a temporary kind cluster")
    parser.add_argument("--router-image", required=True)
    parser.add_argument("--backend-image", required=True)
    parser.add_argument("--manifest", type=Path, default=Path("deploy/kubernetes/w2-fake.yaml"))
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
    router_image_id = run("docker", "image", "inspect", "--format", "{{.Id}}", args.router_image)
    backend_image_id = run("docker", "image", "inspect", "--format", "{{.Id}}", args.backend_image)
    kind_version = run("kind", "version")
    cluster_name = f"w2-{datetime.now().strftime('%Y%m%d%H%M%S')}-{uuid.uuid4().hex[:6]}"
    context = f"kind-{cluster_name}"
    router_url = f"http://127.0.0.1:{free_port()}"

    checks = {
        "cluster_ready": False,
        "deployments_ready": False,
        "router_health": False,
        "round_robin": False,
        "non_streaming": False,
        "streaming": False,
        "metrics": False,
    }
    selected_endpoints: list[str] = []
    kubernetes_version = "unavailable"
    failure = ""
    cluster_created = False
    cluster_attempted = False
    cluster_deleted = False
    port_forward: subprocess.Popen[bytes] | None = None
    port_forward_log: BinaryIO | None = None
    port_forward_stopped = True

    try:
        cluster_attempted = True
        run("kind", "create", "cluster", "--name", cluster_name, "--image", NODE_IMAGE, "--wait", "180s")
        cluster_created = True
        checks["cluster_ready"] = True
        version_data = json.loads(run("kubectl", "--context", context, "version", "-o", "json"))
        kubernetes_version = version_data["serverVersion"]["gitVersion"]

        run("kind", "load", "docker-image", args.router_image, "--name", cluster_name)
        run("kind", "load", "docker-image", args.backend_image, "--name", cluster_name)
        run("kubectl", "--context", context, "apply", "-f", str(args.manifest))
        run(
            "kubectl",
            "--context",
            context,
            "-n",
            "llm-slo",
            "set",
            "image",
            "deployment/router",
            f"router={args.router_image}",
        )
        for deployment in ("fake-a", "fake-b"):
            run(
                "kubectl",
                "--context",
                context,
                "-n",
                "llm-slo",
                "set",
                "image",
                f"deployment/{deployment}",
                f"backend={args.backend_image}",
            )
        for deployment in ("fake-a", "fake-b", "router"):
            run(
                "kubectl",
                "--context",
                context,
                "-n",
                "llm-slo",
                "rollout",
                "status",
                f"deployment/{deployment}",
                "--timeout=120s",
            )
        checks["deployments_ready"] = True

        port_forward_log = (logs_dir / "port-forward.log").open("wb")
        port_forward = start(
            [
                "kubectl",
                "--context",
                context,
                "-n",
                "llm-slo",
                "port-forward",
                "service/router",
                f"{router_url.rsplit(':', 1)[1]}:8080",
                "--address=127.0.0.1",
            ],
            port_forward_log,
        )
        wait_healthy(router_url, timeout=30)
        checks["router_health"] = True

        first_status, first_headers, first_body = completion(router_url, False)
        second_status, second_headers, second_body = completion(router_url, False)
        selected_endpoints.extend(
            [first_headers.get("x-llm-endpoint", ""), second_headers.get("x-llm-endpoint", "")]
        )
        checks["round_robin"] = selected_endpoints == ["fake-a", "fake-b"]
        checks["non_streaming"] = (
            first_status == 200
            and second_status == 200
            and b"FAKE_fake-a" in first_body
            and b"FAKE_fake-b" in second_body
        )
        (raw_dir / "non-streaming-a.json").write_bytes(first_body)
        (raw_dir / "non-streaming-b.json").write_bytes(second_body)

        stream_status, stream_headers, stream_body = completion(router_url, True)
        selected_endpoints.append(stream_headers.get("x-llm-endpoint", ""))
        checks["round_robin"] = checks["round_robin"] and selected_endpoints[2] == "fake-a"
        checks["streaming"] = (
            stream_status == 200
            and streamed_content(stream_body) == "FAKE_fake-a"
            and b"data: [DONE]" in stream_body
        )
        (raw_dir / "stream.txt").write_bytes(stream_body)

        with urlopen(router_url + "/metrics", timeout=10) as response:
            metrics_body = response.read()
        (raw_dir / "router.prom").write_bytes(metrics_body)
        checks["metrics"] = (
            b'router_requests_total{endpoint="fake-a",code="200"} 2' in metrics_body
            and b'router_requests_total{endpoint="fake-b",code="200"} 1' in metrics_body
        )
    except Exception as error:
        failure = f"{type(error).__name__}: {error}"
    finally:
        if cluster_created:
            capture(
                ["kubectl", "--context", context, "-n", "llm-slo", "get", "all", "-o", "wide"],
                raw_dir / "kubernetes-resources.txt",
            )
            capture(
                ["kubectl", "--context", context, "-n", "llm-slo", "get", "events", "--sort-by=.lastTimestamp"],
                raw_dir / "kubernetes-events.txt",
            )
            for deployment in ("fake-a", "fake-b", "router"):
                capture(
                    ["kubectl", "--context", context, "-n", "llm-slo", "logs", f"deployment/{deployment}"],
                    logs_dir / f"{deployment}.log",
                )
        if port_forward is not None:
            port_forward_stopped = stop([port_forward])
        if port_forward_log is not None:
            port_forward_log.close()
        if cluster_attempted:
            try:
                run("kind", "delete", "cluster", "--name", cluster_name)
                cluster_deleted = True
            except Exception as error:
                if not failure:
                    failure = f"cluster cleanup failed: {error}"

    ended_at = utc_now()
    passed = all(checks.values()) and port_forward_stopped and cluster_deleted and not failure
    summary = {
        "run_id": evidence_dir.name,
        "evidence_level": "L1",
        "status": "passed" if passed else "failed",
        "checks": checks,
        "selected_endpoints": selected_endpoints,
        "cluster_name": cluster_name,
        "kubernetes_version": kubernetes_version,
        "port_forward_stopped": port_forward_stopped,
        "cluster_deleted": cluster_deleted,
        "failure": failure or None,
        "claim_boundary": "Kubernetes wiring with fake backends; not real inference or policy improvement evidence",
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
            "manifest_sha256": sha256(args.manifest),
            "router_image": args.router_image,
            "router_image_id": router_image_id,
            "backend_image": args.backend_image,
            "backend_image_id": backend_image_id,
            "cluster_name": cluster_name,
            "kind_version": kind_version,
            "kubernetes_version": kubernetes_version,
            **checks,
            "port_forward_stopped": port_forward_stopped,
            "cluster_deleted": cluster_deleted,
        },
    )
    print(json.dumps(summary, indent=2, sort_keys=True))
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
