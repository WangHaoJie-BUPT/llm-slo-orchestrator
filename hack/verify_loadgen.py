#!/usr/bin/env python3
import importlib.metadata
from pathlib import Path
import subprocess
import sys


EXPECTED = {
    "guidellm": "0.7.1",
    "torch": "2.9.1+cpu",
}


def main() -> int:
    errors: list[str] = []
    lock_text = Path("loadgen/requirements.lock").read_text(encoding="utf-8")
    for package, expected in EXPECTED.items():
        actual = importlib.metadata.version(package)
        if actual != expected:
            errors.append(f"{package}: expected {expected}, got {actual}")
        if f"{package}=={expected}" not in lock_text.lower():
            errors.append(f"{package}: missing exact version from requirements.lock")

    check = subprocess.run(
        [sys.executable, "-m", "pip", "check"],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    if check.returncode != 0:
        errors.append(check.stdout.strip())

    if errors:
        for error in errors:
            print(f"loadgen: {error}", file=sys.stderr)
        return 1

    print("loadgen environment: OK")
    for package, expected in EXPECTED.items():
        print(f"{package}: {expected}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
