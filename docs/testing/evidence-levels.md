# Evidence Levels

| Level | Environment | Required proof | Allowed claim |
| --- | --- | --- | --- |
| L0 | Go/Python, fake clock/backend | unit/property/race results | algorithm and validation correctness |
| L1 | Compose or kind, fake backend | deployment, metrics, failure recovery | control-plane closure |
| L2 | one GPU/NPU, real model | streaming, metrics, capacity curve | single-endpoint inference behavior |
| L3 | two real endpoints | controlled baseline comparisons | multi-endpoint policy comparison |
| L4 | second hardware family | compatibility report and raw evidence | portability differences |

Every report states its level. A higher-level environment does not automatically
validate every claim; the specific experiment and raw records must be present.

