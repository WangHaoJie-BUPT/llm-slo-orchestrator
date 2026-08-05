# Third-Party Notices

This repository is licensed under Apache License 2.0. It does not vendor model
weights, Python wheels, Go module source trees, container image layers, or
third-party executables. The components below are referenced as dependencies,
development tools, or optional runtime services and remain subject to their own
licenses.

## Source Dependency

| Component | Pinned version | License | Source |
| --- | --- | --- | --- |
| `gopkg.in/yaml.v3` | 3.0.1 | MIT and Apache-2.0 | https://github.com/go-yaml/yaml |
| React | 19.0.0 | MIT | https://github.com/facebook/react |
| React DOM | 19.0.0 | MIT | https://github.com/facebook/react |
| Lucide React | 0.468.0 | ISC | https://github.com/lucide-icons/lucide |

The exact Go module checksums are recorded in `go.sum`. The exact web dependency
graph and integrity hashes are recorded in `web/package-lock.json`; the embedded
production bundle is generated from these locked dependencies.

## Development Dependencies

| Component | Pinned version | License | Source |
| --- | --- | --- | --- |
| Vite | 6.4.3 | MIT | https://github.com/vitejs/vite |
| TypeScript | 5.7.3 | Apache-2.0 | https://github.com/microsoft/TypeScript |
| Vitest | 3.2.7 | MIT | https://github.com/vitest-dev/vitest |

## Optional Runtime And Validation Components

| Component | Pinned version | License | Source |
| --- | --- | --- | --- |
| vLLM | 0.25.1 | Apache-2.0 | https://github.com/vllm-project/vllm |
| GuideLLM | 0.7.1 | Apache-2.0 | https://github.com/vllm-project/guidellm |
| Qwen3-0.6B | revision `c1899de289a04d12100db370d81485cdf75e47ca` | Apache-2.0 | https://huggingface.co/Qwen/Qwen3-0.6B |
| Prometheus | 3.13.1 | Apache-2.0 | https://github.com/prometheus/prometheus |
| Grafana | 13.1.0 | AGPL-3.0-only | https://github.com/grafana/grafana |
| Kubernetes | 1.36.x | Apache-2.0 | https://github.com/kubernetes/kubernetes |
| kind | 0.32.0 | Apache-2.0 | https://github.com/kubernetes-sigs/kind |
| Helm | 4.2.3 | Apache-2.0 | https://github.com/helm/helm |

`loadgen/requirements.lock` records the complete resolved Python environment
used for the GuideLLM evidence runs. Those packages are downloaded from their
upstream package indexes and are not redistributed by this repository. Review
the license metadata of every locked package before redistributing a built
load-generator environment.

The Compose file references unmodified upstream container images through
content digests. Image layers are not part of this Git repository. In
particular, Grafana remains licensed under AGPL-3.0-only. Anyone who modifies or
redistributes Grafana must comply with the Grafana license, including applicable
source-offer obligations. The dashboard and provisioning files authored in this
repository are licensed under the repository's Apache-2.0 license; they do not
include Grafana source code.

Model weights are intentionally excluded. Downloading or redistributing a model
requires separate compliance with the model publisher's terms and the pinned
revision's license.

Version and digest details are maintained in `versions.lock.yaml`. If a pinned
component changes, update this notice and verify the new upstream license before
merging the change.
