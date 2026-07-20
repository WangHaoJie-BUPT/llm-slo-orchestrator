#!/usr/bin/env python3
from pathlib import Path
import re
import sys


LOCK_FILE = Path("versions.lock.yaml")
REQUIRED_MARKERS = (
    "frozenAt:",
    "dockerDesktop:",
    "go:",
    "vllm:",
    "guidellm:",
    "revision:",
    "prometheus:",
    "grafana:",
    "linuxAmd64Digest:",
)


def main() -> int:
    text = LOCK_FILE.read_text(encoding="utf-8")
    errors: list[str] = []

    if re.search(r"\blatest\b", text, flags=re.IGNORECASE):
        errors.append("mutable 'latest' reference is forbidden")

    for marker in REQUIRED_MARKERS:
        if marker not in text:
            errors.append(f"missing required marker: {marker}")

    for line_number, line in enumerate(text.splitlines(), start=1):
        key, separator, raw_value = line.partition(":")
        if not separator or "digest" not in key.lower():
            continue
        value = raw_value.strip().strip('"')
        if not re.fullmatch(r"sha256:[0-9a-f]{64}", value):
            errors.append(f"line {line_number}: invalid digest {value!r}")

    revision_match = re.search(r'^\s+revision:\s+"([0-9a-f]+)"$', text, re.MULTILINE)
    if revision_match is None or len(revision_match.group(1)) != 40:
        errors.append("model revision must be a 40-character commit SHA")

    if errors:
        for error in errors:
            print(f"versions.lock.yaml: {error}", file=sys.stderr)
        return 1

    print("versions.lock.yaml: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

