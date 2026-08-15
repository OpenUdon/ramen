# Ramen Scope And Boundaries

Ramen owns desired-state reconciliation:

- native UWS/Ramen project loading and validation;
- Ramen reconciliation metadata over UWS;
- API source operation inventory consumption through public `apitools`;
- Terraform/OpenTofu HCL conversion through public `tfconfig` via
  `ramen convert`;
- optional read-only Terraform provider-schema snapshot validation during
  conversion, without provider loading or API-operation authority;
- resource-to-operation mapping, including public `tfmapping` for conversion
  compatibility;
- dependency graph construction;
- deterministic plan and diff output;
- SQLite state, locks, and revision history;
- refresh, apply, and import orchestration;
- public executor interfaces and optional trusted executor adapters.

The native lifecycle—project loading, validation, graphing, planning, approved
apply, reconciliation, and state—is the product center. Terraform/OpenTofu and
Ansible conversion plus assisted authoring are adapter paths into native
artifacts. Imperative `run` is adjacent and does not create desired-state
resources.

Static Ansible inventory selection is client-language analysis only: bounded
host/group/variable facts may shape a workflow, while inventory plugins,
connections, SSH, vault, facts gathering, and module execution remain outside
Ramen.
The [client-language conversion model](client-language-conversion.md) is the
canonical ownership and evidence decision guide for both conversion adapters.

For v0.1.x, the supported Go packages are `project`, `validate`, `graph`,
`plan`, `apply`, `reconcile`, `state`, and `executor`. Other exported packages
remain experimental before v1. The executor integration is an in-process Go
interface that requires both capability declaration and execution; public
release binaries include mock execution only.

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
builds. Reading an inert, operator-supplied provider-schema JSON snapshot does
not load or execute its provider. The optional udon adapter is behind the
explicit `udon` build tag.

See [compatibility.md](compatibility.md) for the pre-1.0 compatibility and
artifact-version policy.
