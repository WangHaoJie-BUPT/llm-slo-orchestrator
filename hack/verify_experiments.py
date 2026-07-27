#!/usr/bin/env python3
import json
from pathlib import Path
import sys
from typing import Any


EXPECTED_IDS = ["H1", "H2", "H3", "H4"]
EXPECTED_DIMENSIONS = {
    "arrival": ["constant", "poisson", "burst-trace-replay"],
    "prompt_tokens": [128, 1024, 4096],
    "output_tokens": [64, 256, 512],
    "prefix_reuse_percent": [0, 50, 80],
    "priority": ["online-high", "online-normal", "batch-low"],
}


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def verify(document: Any) -> None:
    require(isinstance(document, dict), "experiment suite must be a JSON object")
    require(document.get("schema_version") == 1, "schema_version must be 1")
    require(document.get("suite") == "h1-h4", "suite must be h1-h4")
    repetitions = document.get("repetitions")
    require(isinstance(repetitions, int) and repetitions >= 5, "repetitions must be at least 5")
    require(
        document.get("calibration_load_multipliers") == [0.5, 0.8, 1.0, 1.2],
        "calibration load multipliers must remain fixed",
    )
    require(document.get("dimensions") == EXPECTED_DIMENSIONS, "experiment dimensions are incomplete")

    hypotheses = document.get("hypotheses")
    require(isinstance(hypotheses, list), "hypotheses must be a list")
    require(
        [item.get("id") for item in hypotheses if isinstance(item, dict)] == EXPECTED_IDS,
        "hypotheses must contain H1-H4 in order",
    )
    for hypothesis in hypotheses:
        hypothesis_id = hypothesis["id"]
        for field in (
            "statement",
            "baseline",
            "candidate",
            "primary_metrics",
            "required_level",
            "local_status",
        ):
            require(bool(hypothesis.get(field)), f"{hypothesis_id} is missing {field}")
        require(
            isinstance(hypothesis["primary_metrics"], list),
            f"{hypothesis_id} primary_metrics must be a list",
        )

    for hypothesis in hypotheses[:3]:
        require(
            hypothesis["required_level"] == "L3",
            f"{hypothesis['id']} must retain the two-real-endpoint L3 gate",
        )
        require(
            hypothesis["local_status"] == "mechanics-only",
            f"{hypothesis['id']} cannot claim a local policy result",
        )
    require(hypotheses[3]["required_level"] in {"L2", "L3"}, "H4 requires real-model evidence")
    require(
        "two independent real accelerator endpoints" in document.get("claim_boundary", ""),
        "claim boundary must state the H1-H3 hardware requirement",
    )


def main() -> int:
    path = Path("experiments/h1-h4.json")
    try:
        verify(json.loads(path.read_text(encoding="utf-8")))
    except (OSError, json.JSONDecodeError, ValueError, KeyError) as error:
        print(f"experiment verification failed: {error}", file=sys.stderr)
        return 1
    print("experiment definitions: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
