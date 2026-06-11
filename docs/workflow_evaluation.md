# Workflow Evaluation Fixtures

`testdata/workflow-eval/manifest.json` records workflow-only conversion
evidence that does not fit the T01 NL-to-Ramen desired-state corpus.

The first entries are Ansible playbook conversions from
`ramen convert ansible`. They link to the original playbook, generated UWS
workflow, generated HCL, diagnostics JSON, and review markdown. These entries
are static conversion evidence only; they are not approval packages, training
rows, executor inputs, or live-run claims.

Default tests validate referenced files, UWS schema validity, diagnostics
counts, and review artifact presence without running Ansible, UWS workflows,
providers, API operations, or live cloud services.
