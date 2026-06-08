# Ramen Scope And Boundaries

Ramen owns desired-state reconciliation:

- native UWS/Ramen project loading and validation;
- Ramen reconciliation metadata over UWS;
- API source operation inventory consumption through public `apitools`;
- Terraform/OpenTofu HCL conversion through public `tfconfig` via
  `ramen convert`;
- resource-to-operation mapping, including public `tfmapping` for conversion
  compatibility;
- dependency graph construction;
- deterministic plan and diff output;
- SQLite state, locks, and revision history;
- refresh, apply, and import orchestration;
- public executor interfaces and optional trusted executor adapters.

Ramen uses shared `github.com/OpenUdon/evidence/...` primitives only for
neutral behavior where the shape is product-independent. Current shared use
covers SHA-256 digest helpers, artifact and diagnostic records, redaction
pattern handling behind Ramen's stricter secret-keyword policy, approval
requirement evaluation behind Ramen-owned governance wire formats, and neutral
async executor evidence records.

Ramen-specific behavior remains Ramen-owned, including `ramen.approval.v1`,
`ramen.policy.v1`, desired hashes, state history, reconciliation, delete
confirmation, and executor orchestration. The OpenUdon/Ramen evidence boundary
is summarized in [openudon-evidence-alignment.md](openudon-evidence-alignment.md).

Ramen does not import Terraform code, OpenTofu internals, Terraform providers,
provider plugins, provider SDKs, or private udon packages in default public
builds. The optional udon adapter is behind the explicit `udon` build tag.
