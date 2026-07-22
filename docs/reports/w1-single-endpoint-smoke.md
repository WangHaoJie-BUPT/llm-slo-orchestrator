# W1 Single-Endpoint Runtime Smoke

- Date: 2026-07-22
- Evidence level: L2
- Run ID: `w1-20260722T231141+0800`
- Result: Passed

## Runtime

The run used vLLM 0.25.1 at the frozen linux/amd64 digest, Qwen3-0.6B at revision
`c1899de289a04d12100db370d81485cdf75e47ca`, and an RTX 5070 Ti with driver
596.21. The model weight was 1,503,300,328 bytes and matched the frozen SHA-256.
The evidence manifest references clean repository commit
`5b6a85dbdc1d6798dadeb49bd6f79f41aa3ae41d`.

## Checks

| Check | Result |
| --- | --- |
| `GET /health` | passed |
| `GET /metrics` | passed, 46,615-byte Prometheus snapshot |
| Non-streaming `POST /v1/chat/completions` | passed, content `W1_OK` |
| Streaming `POST /v1/chat/completions` | passed, content `W1_OK` |

The first non-streaming request took 25,529.450 ms and included first-request
compilation. The following streaming request produced its first content token in
34.620 ms and completed in 42.456 ms. These timings verify evidence capture and
must not be treated as a benchmark or steady-state comparison.

## Compatibility Finding

vLLM 0.25.1 selected its V2 model runner by default, which exited because Docker
Desktop's WSL GPU mapping did not expose CUDA Unified Virtual Addressing. The
fixed image documents `VLLM_USE_V2_MODEL_RUNNER=0`; using that switch allowed the
same image, CUDA runtime, model, and API path to complete successfully.

## Evidence Boundary

This run validates one local endpoint and the exercised API paths. GuideLLM load
generation, capacity curves, routing policies, fault campaigns, Kubernetes, and
comparative performance claims remain out of scope for this smoke.
