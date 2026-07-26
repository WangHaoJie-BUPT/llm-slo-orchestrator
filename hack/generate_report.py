#!/usr/bin/env python3
import argparse
import json
from pathlib import Path
import sys
from typing import Any


VALID_LEVELS = {"L0", "L1", "L2", "L3", "L4"}
VALID_STATUSES = {"passed", "failed"}


def load_runs(evidence_root: Path) -> list[dict[str, Any]]:
    runs: list[dict[str, Any]] = []
    for summary_path in sorted(evidence_root.glob("*/summary.json")):
        manifest_path = summary_path.with_name("manifest.yaml")
        if not manifest_path.is_file():
            raise ValueError(f"missing manifest for {summary_path}")
        try:
            summary = json.loads(summary_path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as error:
            raise ValueError(f"invalid summary {summary_path}: {error}") from error
        if not isinstance(summary, dict):
            raise ValueError(f"summary must be an object: {summary_path}")
        run_id = summary.get("run_id")
        if run_id != summary_path.parent.name:
            raise ValueError(f"run_id does not match directory: {summary_path}")
        if summary.get("evidence_level") not in VALID_LEVELS:
            raise ValueError(f"invalid evidence level: {summary_path}")
        if summary.get("status") not in VALID_STATUSES:
            raise ValueError(f"invalid status: {summary_path}")
        checks = summary.get("checks", {})
        if not isinstance(checks, dict) or any(not isinstance(value, bool) for value in checks.values()):
            raise ValueError(f"checks must be a boolean object: {summary_path}")
        runs.append(summary)
    if not runs:
        raise ValueError(f"no evidence summaries found below {evidence_root}")
    return runs


def check_counts(run: dict[str, Any]) -> tuple[int, int]:
    checks = run.get("checks", {})
    if checks:
        passed = sum(1 for value in checks.values() if value)
        return passed, len(checks) - passed
    capacity = run.get("capacity")
    if isinstance(capacity, list):
        points = ([run["smoke"]] if isinstance(run.get("smoke"), dict) else []) + capacity
        results = [point.get("candidate_slo_met") for point in points if isinstance(point, dict)]
        if results and all(isinstance(value, bool) for value in results):
            passed = sum(1 for value in results if value)
            return passed, len(results) - passed
    passed = run.get("checks_passed", 0)
    failed = run.get("checks_failed", 0)
    if not isinstance(passed, int) or not isinstance(failed, int) or passed < 0 or failed < 0:
        raise ValueError(f"invalid check counts for {run['run_id']}")
    return passed, failed


def render(runs: list[dict[str, Any]]) -> str:
    passed_runs = sum(1 for run in runs if run["status"] == "passed")
    highest_level = max(runs, key=lambda run: int(run["evidence_level"][1:]))["evidence_level"]
    lines = [
        "# Generated Evidence Status",
        "",
        "This file is generated from tracked `evidence/*/summary.json` files. Run",
        "`make report` to regenerate it; do not transcribe measurements into this table.",
        "",
        "## Summary",
        "",
        f"- Runs: {len(runs)}",
        f"- Passed: {passed_runs}",
        f"- Failed: {len(runs) - passed_runs}",
        f"- Highest evidence level represented: {highest_level}",
        "",
        "## Runs",
        "",
        "| Run | Level | Status | Checks passed | Checks failed |",
        "| --- | --- | --- | ---: | ---: |",
    ]
    for run in runs:
        checks_passed, checks_failed = check_counts(run)
        run_id = run["run_id"]
        lines.append(
            f"| [`{run_id}`](../../evidence/{run_id}/summary.json) | "
            f"{run['evidence_level']} | {run['status']} | {checks_passed} | {checks_failed} |"
        )
    lines.extend(
        [
            "",
            "## Claim Boundary",
            "",
            "The table is an inventory, not a cross-run benchmark. L1 fake-backend",
            "runs validate wiring only. L2 runs validate one real accelerator endpoint.",
            "Routing-improvement claims require L3 evidence with independent real endpoints.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate the evidence status report")
    parser.add_argument("--evidence-root", type=Path, default=Path("evidence"))
    parser.add_argument("--output", type=Path, default=Path("docs/reports/generated-status.md"))
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()

    try:
        contents = render(load_runs(args.evidence_root))
    except ValueError as error:
        print(error, file=sys.stderr)
        return 1
    if args.check:
        try:
            current = args.output.read_text(encoding="utf-8")
        except OSError as error:
            print(f"cannot read generated report: {error}", file=sys.stderr)
            return 1
        if current != contents:
            print(f"generated report is stale: run make report ({args.output})", file=sys.stderr)
            return 1
        print(f"generated report: OK ({args.output})")
        return 0

    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_suffix(args.output.suffix + ".tmp")
    temporary.write_text(contents, encoding="utf-8", newline="\n")
    temporary.replace(args.output)
    print(f"generated report: {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
