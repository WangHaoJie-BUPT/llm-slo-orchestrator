# Local Environment

## Selected Topology

```text
Windows 11
  -> WSL 2.7.10
     -> Ubuntu 24.04.4 (project tools)
     -> Docker Desktop 4.82.0 (Linux engine and NVIDIA runtime)
        -> Compose services
        -> kind Kubernetes nodes
        -> vLLM GPU container (W1)
```

The Ubuntu VHD is stored under `<redacted-local-path>`. Docker Desktop data is stored
under `<redacted-local-path>`. The project repository should be opened from Ubuntu when
running build and benchmark commands.

## Start Check

```bash
docker version
docker compose version
nvidia-smi
make verify
```

If `docker` is missing inside Ubuntu, start Docker Desktop and wait for WSL
integration. Do not install a second Docker daemon in Ubuntu.

## Network Policy

Use explicit mirror references from `versions.lock.yaml`. Do not disable TLS
verification. Do not silently change a digest when a mirror is unavailable.

If `proxy.golang.org` is unreachable on the local network, use:

```bash
GOPROXY=https://goproxy.cn,direct make verify
```

Keep `GOSUMDB=sum.golang.org`; the repository locks dependency sums in `go.sum`.

## W1 Local Inference

The frozen Qwen3-0.6B snapshot is stored outside the repository at
`/path/to/Qwen3-0.6B-c1899de`. Verify it before starting the container:

```bash
make verify-model
make gpu-up
curl --fail http://127.0.0.1:8000/health
make e2e-gpu
```

The Compose service sets Hugging Face and Transformers offline modes. It cannot
silently replace the pinned snapshot with a network download. Stop it with
`make gpu-down` when the GPU is needed for other work.

Docker Desktop's WSL GPU mapping does not expose the CUDA Unified Virtual
Addressing capability required by vLLM 0.25.1's V2 model runner. The Compose
service therefore sets the image's documented `VLLM_USE_V2_MODEL_RUNNER=0`
compatibility switch. The vLLM version, image digest, CUDA runtime, model, and
OpenAI API surface remain unchanged.
