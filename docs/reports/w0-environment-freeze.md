# W0 Environment and Contract Freeze

- Date: 2026-07-20
- Evidence level: L0
- Result: Passed

## Outcome

The local Windows, WSL2, Ubuntu, Docker Desktop, NVIDIA container runtime, and kind
paths are operational. The repository now has a pinned candidate runtime, strict
policy configuration, stable core interfaces, command contracts, CI, and local
verification.

## Verified

- WSL 2.7.10 and Ubuntu 24.04.4 run from the E drive.
- Docker Desktop data is stored on the E drive.
- A CUDA 13.0.2 container sees the RTX 5070 Ti and 16,303 MiB memory.
- A kind 0.32.0 / Kubernetes 1.36.1 cluster reached Ready and was removed.
- `go vet`, unit tests, race tests, and both command builds pass.
- The policy loader rejects unknown YAML fields and invalid weights.
- Version and repository-layout audits pass.
- The observability Compose file passes configuration parsing.

## Candidate Runtime

vLLM 0.25.1 is pinned by linux/amd64 digest. Its official Dockerfile uses CUDA
13.0.2, Python 3.12, and includes compute architecture 12.0. Qwen3-0.6B is pinned by
revision and weight hash. These facts support a W1 compatibility attempt; they do
not prove model serving yet.

## Remaining Before W1 Exit

- Download the model through a declared mirror and verify the weight hash.
- Pull the pinned vLLM linux/amd64 image.
- Start `/health`, `/metrics`, non-streaming, and streaming requests.
- Capture an immutable run manifest and client/server timing evidence.

## Reproduce

```bash
GOPROXY=https://goproxy.cn,direct make verify
make compose-config
./bin/router --check-config config/example-policy.yaml
```

The W0 repository is committed locally using the configured Git author identity.
Its immutable commit reference is recorded in the W0 evidence manifest.
