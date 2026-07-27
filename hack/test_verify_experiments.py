#!/usr/bin/env python3
import copy
import json
from pathlib import Path
import unittest

from verify_experiments import verify


class VerifyExperimentsTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.document = json.loads(Path("experiments/h1-h4.json").read_text(encoding="utf-8"))

    def test_repository_definition_is_valid(self) -> None:
        verify(copy.deepcopy(self.document))

    def test_rejects_too_few_repetitions(self) -> None:
        document = copy.deepcopy(self.document)
        document["repetitions"] = 4
        with self.assertRaisesRegex(ValueError, "at least 5"):
            verify(document)

    def test_rejects_lower_h1_evidence_gate(self) -> None:
        document = copy.deepcopy(self.document)
        document["hypotheses"][0]["required_level"] = "L2"
        with self.assertRaisesRegex(ValueError, "retain.*L3 gate"):
            verify(document)

    def test_rejects_local_h3_performance_claim(self) -> None:
        document = copy.deepcopy(self.document)
        document["hypotheses"][2]["local_status"] = "validated"
        with self.assertRaisesRegex(ValueError, "cannot claim"):
            verify(document)


if __name__ == "__main__":
    unittest.main()
