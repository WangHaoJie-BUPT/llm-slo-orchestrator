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
