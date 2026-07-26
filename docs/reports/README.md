# Reports

Reports are generated from evidence manifests and raw request records. W0 does not
contain measured inference results. W1 records a runtime smoke and exploratory
single-endpoint capacity curve. Comparative routing work remains pending.
W2 adds deterministic standalone-router evidence, a real pinned-vLLM router path,
and a temporary kind deployment with two fake backends. The kind report is L1
Kubernetes wiring evidence; the real-endpoint report is L2 inference-path
evidence.
W3 adds thresholded backend health and deterministic process failure/recovery
evidence. It remains L1 because the disrupted backend is synthetic.
