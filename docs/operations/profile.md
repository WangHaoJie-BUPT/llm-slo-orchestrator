# Model Backend Profile

The selected local profile is `config/qwen3-0.6b-profile.yaml`. It binds the
immutable Qwen3-0.6B revision, vLLM image digest, RTX 5070 Ti hardware, engine
settings, sampling period, capacity bucket, and source evidence manifest.

The single capacity point records 16 requests/second for approximately 128 input
and 64 output tokens with a measured TTFT p95 of 61.001 ms. In this schema the
field is named `safeQPS`, but the evidence only establishes that 16 requests per
second passed the short exploratory test. The run did not find saturation and
does not establish a production capacity guarantee.

Generate or refresh deterministic reconciliation state with:

```bash
make profile-reconcile
```

`make verify` runs the controller in `--check` mode and fails when the generated
state is stale, the policy and profile models differ, metric freshness exceeds
three sampling periods, strict YAML validation fails, or source evidence is
missing.

For continuous local reconciliation:

```bash
bin/profile-controller \
  --policy config/example-policy.yaml \
  --profile config/qwen3-0.6b-profile.yaml \
  --output config/reconciled-profile.json
```

Continuous mode updates only the local status file. Runtime policy reload and
Kubernetes CRD reconciliation remain separate enhancements.
