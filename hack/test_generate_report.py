import json
from pathlib import Path
import tempfile
import unittest

from generate_report import load_runs, render


class GenerateReportTest(unittest.TestCase):
    def write_run(self, root: Path, run_id: str, summary: dict) -> None:
        run_dir = root / run_id
        run_dir.mkdir()
        (run_dir / "manifest.yaml").write_text("schemaVersion: 1\n", encoding="utf-8")
        (run_dir / "summary.json").write_text(json.dumps(summary), encoding="utf-8")

    def test_load_and_render_orders_runs(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.write_run(
                root,
                "w2-z",
                {"run_id": "w2-z", "evidence_level": "L1", "status": "passed", "checks": {"a": True}},
            )
            self.write_run(
                root,
                "w0-a",
                {
                    "run_id": "w0-a",
                    "evidence_level": "L0",
                    "status": "passed",
                    "checks_passed": 2,
                    "checks_failed": 0,
                },
            )
            contents = render(load_runs(root))
            self.assertLess(contents.index("w0-a"), contents.index("w2-z"))
            self.assertIn("Highest evidence level represented: L1", contents)
            self.assertIn("| 2 | 0 |", contents)

    def test_rejects_directory_mismatch(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.write_run(
                root,
                "actual",
                {"run_id": "other", "evidence_level": "L1", "status": "passed", "checks": {}},
            )
            with self.assertRaisesRegex(ValueError, "run_id does not match"):
                load_runs(root)

    def test_render_counts_capacity_points(self) -> None:
        contents = render(
            [
                {
                    "run_id": "capacity",
                    "evidence_level": "L2",
                    "status": "passed",
                    "smoke": {"candidate_slo_met": True},
                    "capacity": [{"candidate_slo_met": True}, {"candidate_slo_met": False}],
                }
            ]
        )
        self.assertIn("| 2 | 1 |", contents)


if __name__ == "__main__":
    unittest.main()
