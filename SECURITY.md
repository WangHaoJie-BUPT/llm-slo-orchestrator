# Security Policy

## Supported State

The repository is private and pre-release. Security fixes are applied only to the
current main branch until the first portfolio release is frozen.

## Reporting

Do not open a public issue containing credentials, prompts, private model data,
host paths, or infrastructure identifiers. Report the issue privately to the
repository owner with reproduction steps and a minimal redacted example.

## Repository Rules

- Never commit API keys, cloud credentials, registry tokens, or model access tokens.
- Never record Authorization headers or complete prompts in logs or metrics.
- Store only HMAC-derived prefix fingerprints with bounded retention.
- Bind local dashboards and inference endpoints to loopback by default.
- Pin container images by digest and model downloads by immutable revision.
- Keep model weights, raw private traces, and large benchmark output outside Git.

