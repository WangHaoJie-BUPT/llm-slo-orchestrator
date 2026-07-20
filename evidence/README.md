# Evidence Directory

Generated run directories are ignored by Git. Each run must contain at least:

```text
evidence/<run-id>/
  manifest.yaml
  requests.jsonl
  summary.json
  prometheus/
  logs/
```

The manifest records commit, dirty state, versions lock hash, image/model digests,
hardware, scenario, seed, start/end time, and cleanup result. Reports are generated
from these files and must not contain manually transcribed measurements.

