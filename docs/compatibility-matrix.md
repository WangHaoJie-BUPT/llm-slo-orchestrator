# Compatibility Matrix

Status: W0 candidate freeze, 2026-07-20.

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
| vLLM | 0.25.1 | official image uses CUDA 13.0.2 and Python 3.12 | pending W1 |
| GPU architecture | `sm_120` | vLLM Dockerfile includes architecture 12.0 | source-validated |
| Model | Qwen/Qwen3-0.6B | vLLM >=0.8.5 documented by model author | pending W1 |
| Model revision | `c1899de289a04d12100db370d81485cdf75e47ca` | read-only mirror metadata | revalidate on download |
| GuideLLM | 0.7.1 | Python >=3.10,<4.0; host Python 3.12 | pending W1 |

The table distinguishes source compatibility from runtime evidence. An `sm_120`
build target is necessary but not sufficient proof that every kernel used by the
selected model runs correctly. W1 must still complete a streaming request,
`/health`, `/metrics`, and a manifest-backed smoke run.

## Network Constraint

Direct Docker Hub resolution is unreliable on the current network. W0 uses
explicit `docker.m.daocloud.io` references and verifies the upstream digest. This
does not authorize mutable tags. Every executable image in a run manifest must
include a platform-specific digest.

Hugging Face is not directly reachable. Model download tooling must use a declared
mirror, request the immutable revision above, and verify the downloaded weight
hash before the first GPU run.

## Not Required

- Full Hyper-V and Windows Containers are not required for the WSL2 Linux path.
- A Windows CUDA Toolkit is not required because CUDA user-space libraries live in
  the container and the Windows NVIDIA driver is exposed through WSL.
- A second Docker Engine inside Ubuntu is prohibited while Docker Desktop is the
  selected engine; two daemons would create ambiguous images, sockets, and cleanup.

