# Security Policy

## Supported Versions

| Version | Supported |
| --- | --- |
| `main` | Yes |
| Latest GitHub release | Yes |
| Older releases | No |

## Reporting A Vulnerability

Use GitHub Private Vulnerability Reporting:

https://github.com/bupt20220407/llm-slo-orchestrator/security/advisories/new

Include affected versions, impact, reproduction steps, and a minimal redacted
example. Do not open a public issue containing credentials, prompts, private
model data, host paths, infrastructure identifiers, or an unpatched exploit.

The maintainer will acknowledge a report when practical, investigate it on the
current `main` branch, and coordinate disclosure after a fix is available. This
portfolio project does not provide a commercial response-time SLA.

## Repository Rules

- Never commit API keys, cloud credentials, registry tokens, or model access tokens.
- Never record Authorization headers or complete prompts in logs or metrics.
- Store only HMAC-derived prefix fingerprints with bounded retention.
- Bind local dashboards and inference endpoints to loopback by default.
- Keep the operator console on loopback unless an authenticated reverse proxy is
  deliberately configured.
- Leave console controls disabled by default. Use separate random console and
  router control tokens, and never place the router token in browser storage.
- Keep scenario execution on a fixed command allowlist with one active local
  job at a time; never expose arbitrary shell execution.
- Pin container images by digest and model downloads by immutable revision.
- Keep model weights, raw private traces, and large benchmark output outside Git.

Non-sensitive bugs and hardening suggestions may be reported through public
GitHub issues.
