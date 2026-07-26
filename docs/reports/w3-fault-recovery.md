# W3 Backend Failure and Recovery

- Date: 2026-07-27
- Evidence level: L1
- Run ID: `w3-fault-20260727T011604+0800`
- Result: Passed

## Scope

The process-level campaign launched one router and two deterministic fake
backends. Active health probes used a 50 ms interval, 25 ms timeout, two-failure
removal threshold, and two-success recovery threshold. The accelerated settings
keep the test fast; production defaults remain two seconds, one second, two
failures, and one success.

## Checks

| Phase | Result |
| --- | --- |
| Initial health | both endpoints healthy |
| Failure injection | `fake-a` process stopped cleanly |
| Failure detection | `fake-a` gauge changed to zero |
| Traffic during failure | four HTTP 200 responses from `fake-b` |
| Recovery injection | `fake-a` restarted on the same port |
| Recovery detection | `fake-a` gauge returned to one |
| Traffic after recovery | `fake-a`, `fake-b`, `fake-a`, `fake-b` |
| Cleanup | all child processes stopped |

The manifest references clean commit
`a9609f9aa68f82bd733d8a300e0002beef1f7574` and records binary and policy hashes,
probe thresholds, checks, and cleanup state. Local raw evidence includes router
and backend logs, request bodies, and Prometheus metrics captured during failure
and after recovery.

## Evidence Boundary

This run validates deterministic failure detection, picker exclusion, surviving
traffic, and endpoint recovery with fake processes. It does not validate a real
accelerator failure, Kubernetes pod disruption, or comparative policy benefit.
