# Ramen

Ramen is a public API-source desired-state engine. Its native project
format is a UWS project plus Ramen reconciliation metadata for resource
identity, lifecycle, operation roles, hashes, state matching, and redaction
policy. Terraform/OpenTofu HCL remains an optional authoring and migration
input through `ramen convert tf`, which uses `github.com/OpenUdon/tfconfig` to
produce native Ramen/UWS project artifacts.

Ramen maps desired API resources to operations from sources such as AWS Smithy
JSON, Google Discovery, and OpenAPI, computes desired-state plans, records
SQLite state history, and hands approved UWS execution documents to an
explicit trusted executor boundary.

Ramen uses shared `github.com/OpenUdon/evidence/...` primitives for neutral
evidence where the behavior is product-independent. Current shared use covers
SHA-256 digest helpers, redaction pattern handling behind Ramen's stricter
secret-keyword policy, and approval requirement evaluation behind Ramen-owned
governance wire formats. Ramen-specific behavior, including
`ramen.approval.v1`, `ramen.policy.v1`, state history, reconciliation, and
executor orchestration, remains Ramen-owned.

## Why Ramen

Ramen is for teams that have API source documents and need desired-state
workflows without centering the system on provider plugins, backend
compatibility, module download, or Terraform/OpenTofu plan files. Typical
adopters include API platform teams, internal developer platform teams,
operators managing APIs without mature providers, teams migrating HCL-shaped
declarations, and UWS/OpenUdon users who need stateful reconciliation around
workflow documents.

Ramen differs from adjacent tools in a narrow way:

- Terraform/OpenTofu are provider-backed infrastructure runtimes; Ramen is an
  API-source reconciliation engine with HCL conversion as an adapter.
- Generic workflow runners execute steps; Ramen adds identity matching,
  dependency graphs, deterministic plans, import, refresh evidence, state
  history, and approval-artifact checks.
- Generated SDKs provide imperative API calls; Ramen provides operator
  workflows and durable desired-state records.
- OpenUdon owns workflow authoring and review packaging; Ramen owns
  desired-state graphing, state, diff, and reconciliation.

## Adoption Readiness

Community users should be able to evaluate Ramen locally through native project
examples, mock execution, readable plans, stable diagnostics, clear non-goals,
and `ramen convert tf` for HCL migration. Enterprise users should look for
approval artifacts, redacted SQLite state history, reproducible plans, explicit
trusted executor boundaries, no credential value storage, and future policy
integration points.

Ready now: provider-free native validation, graphing, planning, HCL conversion
scaffolding, local state history, mock-backed apply/refresh/destroy flows,
plan approval metadata, read-only show/state inspection, and documented safety
boundaries. Still experimental: broader resource mappings, live executor
adapters, policy hooks, parameterization ergonomics, release packaging, and
operational support contracts.

The canonical roadmap and project memory live in the symlinked harness files:

```text
AGENTS.md
memory-bank/
evolution/
```

In this checkout those paths point to `../tofu/ramen`, so planning history is
tracked in the shared tofu harness repository while remaining easy to read from
the Ramen repo. Short non-roadmap notes belong in [docs/notes.md](docs/notes.md).

## Scope

Ramen owns desired-state reconciliation:

- native UWS/Ramen project loading and validation;
- Ramen reconciliation metadata over UWS;
- API source operation inventory consumption through public `apitools`;
- Terraform/OpenTofu HCL conversion through public `tfconfig` via
  `ramen convert tf`;
- resource-to-operation mapping, including public `tfmapping` for conversion
  compatibility;
- dependency graph construction;
- deterministic plan and diff output;
- SQLite state, locks, and revision history;
- refresh, apply, destroy, and import orchestration;
- public executor interfaces and optional trusted executor adapters.

Shared evidence helpers live in `github.com/OpenUdon/evidence/...` only for
neutral digest, artifact, diagnostic, redaction, and approval primitives. Ramen
uses them behind Ramen-owned package and wire boundaries; they do not own Ramen
plan, policy, state, reconciliation, or executor semantics.

Ramen does not import Terraform code, OpenTofu internals, Terraform providers,
provider plugins, provider SDKs, or private udon packages in default public
builds. The optional udon adapter is planned behind an explicit `udon` build
tag.

## Commands

Implemented public commands:

```bash
ramen convert tf
ramen init
ramen validate --project DIR --json
ramen graph --project DIR --format json
ramen force-unlock LOCK_HOLDER --state PATH
ramen plan --project DIR --target ADDRESS --exclude ADDRESS --replace ADDRESS
ramen plan --project DIR --destroy --out plan.json
ramen apply --auto-approve --mock
ramen refresh --mock
ramen destroy --auto-approve --mock
ramen import
ramen version --json
```

The current implementation supports native UWS/Ramen project input through
`--project`, while preserving the HCL-derived lifecycle path from the first
milestones as transitional compatibility. `ramen convert tf` writes
`project.uws.yaml` in that native format. The first lifecycle surface is
mock-backed in default public builds where execution is required. Live executor
wiring remains opt-in behind trusted adapters. `ramen version --json` reports
local build metadata without network checks.

## Status Lanes

Ramen uses zero-padded status lanes:

```text
M01, M02, M03, ... M09, M10, M11, ...
A01, A02, A03, ... A09, A10, A11, ...
D01, D02, D03, ... D09, D10, D11, ...
F01, F02, F03, ... F09, F10, F11, ...
G01, G02, G03, ... G09, G10, G11, ...
I01, I02, I03, ... I09, I10, I11, ...
P01, P02, P03, ... P09, P10, P11, ...
R01, R02, R03, ... R09, R10, R11, ...
S01, S02, S03, ... S09, S10, S11, ...
V01, V02, V03, ... V09, V10, V11, ...
```

Lane meanings:

- `Mxx`: main milestone scope, cross-cutting delivery records, common command
  infrastructure, and non-subcommand migration history.
- `Axx`: `ramen apply` command-specific tasks.
- `Dxx`: `ramen destroy` command-specific tasks.
- `Fxx`: `ramen force-unlock` command-specific tasks.
- `Gxx`: `ramen graph` command-specific tasks.
- `Ixx`: `ramen init` and `ramen import` command-specific tasks.
- `Pxx`: `ramen plan` command-specific tasks.
- `Rxx`: `ramen refresh` command-specific tasks.
- `Sxx`: `ramen show` and `ramen state` command-specific tasks.
- `Vxx`: `ramen validate` and `ramen version` command-specific tasks.

The native UWS desired-state project model is tracked in the completed
[memory-bank/status-M08.md](memory-bank/status-M08.md). Plan approval controls
are tracked in completed [memory-bank/status-P02.md](memory-bank/status-P02.md).
Accepted subcommand lane normalization is tracked in completed
[memory-bank/status-M10.md](memory-bank/status-M10.md). Adoption positioning is
tracked in completed [memory-bank/status-M12.md](memory-bank/status-M12.md).

## Development Checks

Harness/documentation check:

```bash
git -C ../tofu diff --check -- ramen
```

Planned public module checks once Go code exists:

```bash
go test ./...
go vet ./...
git diff --check
```

Optional udon adapter check when the private sibling checkout is available:

```bash
go test -tags udon ./...
```
