# Static Conversion Contract

`ramen convert tf` and `ramen convert ansible` are static, review-first
adapters. They share command policy and reporting contracts, but they do not
share a semantic payload: Terraform/OpenTofu describes desired remote state,
while Ansible describes client-side workflow steps whose module invocation is
owned by a bound runtime.

Neither converter executes Terraform, OpenTofu, providers, Ansible, modules,
inventory connections, API operations, or UWS workflows.
Terraform conversion may read an operator-supplied offline provider-schema
snapshot as client-configuration validation evidence; it never obtains one by
running Terraform/OpenTofu or a provider, and API source documents remain the
server-operation contract.

## Modes And Exits

Both commands accept `--mode strict|partial`:

- `strict` withholds semantic project/workflow payloads when strict-failure
  diagnostics remain, retains reports for review, and exits `3`;
- `partial` writes the supported subset, records every omission or symbolic
  fallback, and exits `0` when conversion itself succeeds.

Omitted mode is `strict` for both converters. Terraform's `--strict` remains a
deprecated alias for `--mode strict`; Ansible's `--strict` and
`--ignore-unsupported` remain deprecated aliases for strict and partial.
Contradictory selections are usage errors.

Exit `0` means complete conversion or explicitly allowed partial output, `1`
means an operational or input-ingestion failure, `2` means command usage is
invalid, and `3` means the strict conversion gate rejected semantic output.

## Common Reports

Every completed conversion attempt writes these Ramen-owned reports under the
output directory:

- `expected/diagnostics.json`, validated against the embedded
  `ramen.convert.diagnostics.v1` schema;
- `expected/manifest.json`, validated against the embedded
  `ramen.convert.manifest.v1` schema;
- converter-specific Markdown diagnostics and review evidence.

The manifest identifies the converter and effective mode, records complete,
partial, or failed status, hashes regular-file inputs and emitted artifacts,
summarizes converted/symbolic/unsupported/ignored coverage, points to the
diagnostic report, and states `execution.performed: false`. It has no timestamp,
absolute host path, credential value, or raw inline Ansible extra-variable
value. Arrays use deterministic ordering so identical inputs and invocation
paths produce byte-identical reports.

Ansible inventory and extra-vars files are digest evidence. Resolved
non-connection inventory facts can affect static lowering, but SSH/connection
fields and secret-like literal extra vars never enter generated workflows.

The common envelope owns reporting only. Terraform's native desired-state and
provenance contracts remain defined in
[Terraform/OpenTofu Conversion](terraform-conversion.md); Ansible's module-call
and provenance contracts remain defined in
[Ansible Conversion](ansible-conversion.md).

Installed Ramen binaries embed the report schemas and validate them without
repository-relative files or network access.
