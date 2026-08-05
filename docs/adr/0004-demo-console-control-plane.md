# ADR 0004: Separate Demo Console Control Plane

- Status: accepted
- Date: 2026-08-05

## Context

The router data plane already exposes OpenAI-compatible HTTP and SSE forwarding,
health, Prometheus metrics, admission, and retry behavior. Demonstrations still
required several terminals and disconnected report files. A browser interface
can make endpoint eligibility and routing outcomes inspectable, but adding
general administration or command execution to the router would expand its
security and reliability boundary.

## Decision

Add a separate Go `console` process with an embedded React application.

The router exposes a read-only structured status snapshot. Its optional strategy
control route exists only when a bearer token is explicitly configured. The
console proxies chat completions without buffering SSE, indexes only
`docs/reports` and `evidence`, and never records request or response content.

Console mutations require all of the following:

- explicit `--demo-mode`;
- a non-empty console bearer token;
- a same-origin browser request;
- a fixed strategy or scenario identifier;
- no other active scenario job.

The router control token remains server-side and is distinct from the token
entered in the browser. Scenario commands are compiled into an allowlist; the
console has no generic command endpoint. Both processes bind to loopback by
default.

## Consequences

The data plane remains usable without Node.js, the console, tokens, or writable
control routes. A clean Go build uses committed embedded assets, while CI
rebuilds them from `web/package-lock.json` and fails on drift. Frontend work adds
a Node.js dependency to development and requires web license notices.

The console improves inspection and local operation but does not raise the
evidence level of an experiment. H1-H3 still require two independent real
accelerator endpoints for L3 benefit claims, and the formal H4 mixed-priority
real-model campaign remains separate.
