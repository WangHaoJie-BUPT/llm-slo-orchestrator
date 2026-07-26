# Compatibility Matrix

Status: W3 deterministic backend failure and recovery validated, 2026-07-27.

## Validated Host Path

| Layer | Pinned value | Evidence | Status |
| --- | --- | --- | --- |
| Windows | Windows 11 Pro 25H2, build 26200.8875 | local system query | validated |
| WSL | 2.7.10, kernel 6.18.33.2-2 | `wsl --version` | validated |
| Ubuntu | 24.04.4 LTS | `/etc/os-release` | validated |
| GPU | RTX 5070 Ti, 16,303 MiB | Windows and WSL `nvidia-smi` | validated |
| Driver | 596.21, CUDA capability 13.2 | Windows and container `nvidia-smi` | validated |
| Docker | Desktop 4.82.0, Engine 29.6.1 | Windows and Ubuntu clients | validated |
| GPU container | CUDA 13.0.2 base image | container `nvidia-smi` | validated |
| Kubernetes | kind 0.32.0 / Kubernetes 1.36.1 | temporary Ready cluster | validated |

## Candidate Inference Path

| Component | Pin | Compatibility basis | Runtime status |
| --- | --- | --- | --- |
| vLLM | 0.25.1 | fixed linux/amd64 image digest, CUDA 13.0.2 | W1 smoke validated |
| GPU architecture | `sm_120` | image build target plus real RTX 5070 Ti run | W1 smoke validated |
| Model | Qwen/Qwen3-0.6B | real non-streaming and streaming requests | W1 smoke validated |
| Model revision | `c1899de289a04d12100db370d81485cdf75e47ca` | downloaded weight size and SHA-256 | validated |
| GuideLLM | 0.7.1 | locked Python 3.12 environment, CPU-only torch 2.9.1 | W1 calibrated |

## Application Path

| Component | Evidence | Status |
| --- | --- | --- |
| Standalone Go router | command-level two-fake-backend HTTP/SSE test | L1 validated |
| Router to vLLM | real Qwen3-0.6B non-streaming and SSE requests | L2 validated |
| Router metrics | bounded endpoint/status counters | validated |
| Kubernetes deployment | kind `v1.36.1` temporary cluster, router plus two fake backends | L1 validated |
| Active backend health | thresholded failure removal and recovery with two fake processes | L1 validated |

The W1 runs completed `/health`, `/metrics`, non-streaming, streaming, a 10-request
GuideLLM smoke, and constant-rate points from 0.5 through 16 requests/second.
Docker Desktop's WSL GPU mapping did not expose UVA, so vLLM's documented
`VLLM_USE_V2_MODEL_RUNNER=0` switch was required. This validates the exercised
single-endpoint path, not every vLLM kernel or workload. The W2 kind run validates
Kubernetes wiring only; it does not add real-model or multi-endpoint performance
claims.

## Network Constraint

Direct Docker Hub resolution is unreliable on the current network. W0 used
`docker.m.daocloud.io`; W1 used `docker.1ms.run` after DaoCloud rate limiting.
Both references are verified against upstream content digests. This does not
authorize mutable tags. Every executable image in a run manifest must include a
platform-specific digest.

Hugging Face is not directly reachable. W1 used the declared mirror, requested
the immutable revision above, and verified the downloaded weight hash before the
first GPU run.

## Not Required

- Full Hyper-V and Windows Containers are not required for the WSL2 Linux path.
- A Windows CUDA Toolkit is not required because CUDA user-space libraries live in
  the container and the Windows NVIDIA driver is exposed through WSL.
- A second Docker Engine inside Ubuntu is prohibited while Docker Desktop is the
  selected engine; two daemons would create ambiguous images, sockets, and cleanup.
