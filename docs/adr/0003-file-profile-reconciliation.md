# ADR-0003: Reconcile Profiles from Strict Files Before CRDs

- Status: Accepted
- Date: 2026-07-27

## Context

The four-week release needs evidence-backed model capacity profiles and stable
generations for replay. It does not need another controller-runtime, RBAC,
webhook, and generated-client implementation before the routing evidence is
complete.

## Decision

`profile-controller` strictly loads one `InferenceSLOPolicy` and one
`ModelBackendProfile`, verifies model compatibility, enforces the metric
freshness-to-sampling-period rule, and requires the declared source evidence to
exist. It derives deterministic generations from the exact input bytes and
atomically writes a compact reconciliation status.

The command supports one-shot generation, continuous file reconciliation, and a
read-only check mode for CI. The status is advisory rollout metadata. It does not
claim to mutate vLLM, reserve capacity, or provide a Kubernetes CRD control loop.

## Consequences

- Capacity values cannot exist without a source evidence manifest.
- Policy or profile edits change their generation deterministically.
- CI detects stale generated reconciliation status.
- A later router reload API or EPP adapter may consume the same status and schema
  without changing picker interfaces.
