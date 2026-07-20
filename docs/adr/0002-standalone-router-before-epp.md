# ADR-0002: Build the Standalone Router Before EPP

- Status: Accepted
- Date: 2026-07-20

## Context

Gateway API Inference Extension is the long-term integration target, but installing
and debugging a gateway, InferencePool, Endpoint Picker Protocol adapter, and
conformance suite would delay validation of the core routing algorithm.

## Decision

The four-week release uses a standalone OpenAI-compatible reverse proxy. Protocol
translation remains outside the picker core. The following interfaces are frozen
in W0:

- `picker.Picker`: pure selection over request, immutable snapshot, and policy;
- `backend.Client`: streaming backend request with cancellation and deadline;
- `clock.Clock`: injectable time source;
- API request, policy, snapshot, decision, and contribution types.

An EPP adapter may later translate Gateway requests into the same types. It must
not fork or reimplement picker, state, or admission logic.

## Consequences

- Unit tests need no Kubernetes packages or network I/O.
- W1/W2 can validate streaming and routing before a gateway is present.
- EPP is accepted only when the same picker tests and decisions pass through the
  adapter.

## Rejected Alternatives

- Start with EPP: too many installation and conformance failure modes precede the
  first measurable request path.
- Couple picker code to HTTP or Prometheus: it prevents deterministic replay and
  reuse by an EPP adapter.

