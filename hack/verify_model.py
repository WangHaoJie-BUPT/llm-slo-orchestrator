#!/usr/bin/env python3
import argparse
import hashlib
from pathlib import Path
import re
import sys


REQUIRED_FILES = (
    "config.json",
    "generation_config.json",
    "merges.txt",
    "model.safetensors",
    "tokenizer.json",
    "tokenizer_config.json",
    "vocab.json",
)


def locked_value(text: str, key: str) -> str:
    match = re.search(rf"^\s{{4}}{re.escape(key)}:\s*(?:\"([^\"]+)\"|([^\s]+))\s*$", text, re.MULTILINE)
    if match is None:
        raise ValueError(f"missing model lock value: {key}")
    return match.group(1) or match.group(2)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(8 * 1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser(description="Verify the frozen local model snapshot")
    parser.add_argument("--model-dir", type=Path, required=True)
    parser.add_argument("--lock-file", type=Path, default=Path("versions.lock.yaml"))
    args = parser.parse_args()

    lock_text = args.lock_file.read_text(encoding="utf-8")
    expected_hash = locked_value(lock_text, "primaryWeightSha256")
    expected_bytes = int(locked_value(lock_text, "primaryWeightBytes"))
    expected_revision = locked_value(lock_text, "revision")

    missing = [name for name in REQUIRED_FILES if not (args.model_dir / name).is_file()]
    if missing:
        print(f"model snapshot is incomplete; missing: {', '.join(missing)}", file=sys.stderr)
        return 1

    weight = args.model_dir / "model.safetensors"
    actual_bytes = weight.stat().st_size
    actual_hash = sha256(weight)
    if actual_bytes != expected_bytes:
        print(f"weight size mismatch: expected {expected_bytes}, got {actual_bytes}", file=sys.stderr)
        return 1
    if actual_hash != expected_hash:
        print(f"weight SHA-256 mismatch: expected {expected_hash}, got {actual_hash}", file=sys.stderr)
        return 1

    print(f"model snapshot: OK ({expected_revision})")
    print(f"model.safetensors: {actual_bytes} bytes, sha256:{actual_hash}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
