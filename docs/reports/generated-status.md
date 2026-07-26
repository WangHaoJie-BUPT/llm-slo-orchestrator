# Generated Evidence Status

This file is generated from tracked `evidence/*/summary.json` files. Run
`make report` to regenerate it; do not transcribe measurements into this table.

## Summary

- Runs: 6
- Passed: 6
- Failed: 0
- Highest evidence level represented: L2

## Runs

| Run | Level | Status | Checks passed | Checks failed |
| --- | --- | --- | ---: | ---: |
| [`w0-20260720`](../../evidence/w0-20260720/summary.json) | L0 | passed | 9 | 0 |
| [`w1-20260722T231141+0800`](../../evidence/w1-20260722T231141+0800/summary.json) | L2 | passed | 4 | 0 |
| [`w1-capacity-20260724T011155+0800`](../../evidence/w1-capacity-20260724T011155+0800/summary.json) | L2 | passed | 7 | 0 |
| [`w2-fake-20260724T013723+0800`](../../evidence/w2-fake-20260724T013723+0800/summary.json) | L1 | passed | 5 | 0 |
| [`w2-kind-20260726T182748+0800`](../../evidence/w2-kind-20260726T182748+0800/summary.json) | L1 | passed | 7 | 0 |
| [`w2-router-gpu-20260724T014708+0800`](../../evidence/w2-router-gpu-20260724T014708+0800/summary.json) | L2 | passed | 6 | 0 |

## Claim Boundary

The table is an inventory, not a cross-run benchmark. L1 fake-backend
runs validate wiring only. L2 runs validate one real accelerator endpoint.
Routing-improvement claims require L3 evidence with independent real endpoints.
