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

## Source Expansion Policy

T02 currently accepts only `ansible-conversion` entries produced by
`ramen convert ansible`. Additional workflow conversion sources should be
added only when they meet the same review boundary:

- static conversion only, with no source runtime, provider, inventory, API, or
  UWS execution in the default path;
- generated workflow artifacts, diagnostics JSON, and review markdown checked
  into a source-specific fixture corpus;
- manifest entries that point to existing artifacts rather than copying them
  into `testdata/workflow-eval`;
- a default regression test that validates references, UWS schema validity,
  diagnostic counts, and review sections; and
- an explicit memory-bank status update explaining why the source is workflow
  evidence rather than T01 NL-to-Ramen desired-state training data.

Terraform/OpenTofu conversion evidence remains in the existing conversion
corpus and T01 desired-state training lanes unless a later milestone defines a
workflow-only slice for that source.
