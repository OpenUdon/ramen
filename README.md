# Ramen

Ramen is a planned public API-source desired-state engine. Its native project
format is a UWS project plus Ramen reconciliation metadata for resource
identity, lifecycle, operation roles, hashes, state matching, and redaction
policy. Terraform/OpenTofu HCL remains an optional authoring and migration
input through `ramen convert tf`, which uses `github.com/OpenUdon/tfconfig` to
produce native Ramen/UWS project artifacts.

Ramen maps desired API resources to operations from sources such as AWS Smithy
JSON, Google Discovery, and OpenAPI, computes desired-state plans, records
SQLite state history, and hands approved UWS execution documents to an
explicit trusted executor boundary.

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

Ramen does not import Terraform code, OpenTofu internals, Terraform providers,
provider plugins, provider SDKs, or private udon packages in default public
builds. The optional udon adapter is planned behind an explicit `udon` build
tag.

## Commands

Implemented public commands:

```bash
ramen convert tf
ramen init
ramen plan
ramen apply --auto-approve --mock
ramen refresh --mock
ramen destroy --auto-approve --mock
ramen import
```

The current implementation supports native UWS/Ramen project input through
`--project`, while preserving the HCL-derived lifecycle path from the first
milestones as transitional compatibility. `ramen convert tf` writes
`project.uws.yaml` in that native format. The first lifecycle surface is
mock-backed in default public builds where execution is required. Live executor
wiring remains opt-in behind trusted adapters.

## Status Lanes

Ramen uses zero-padded status lanes:

```text
M01, M02, M03, ... M09, M10, M11, ...
I01, I02, I03, ... I09, I10, I11, ...
P01, P02, P03, ... P09, P10, P11, ...
S01, S02, S03, ... S09, S10, S11, ...
```

Lane meanings:

- `Mxx`: main milestone scope and cross-cutting delivery records.
- `Ixx`: `ramen init` command-specific tasks.
- `Pxx`: `ramen plan` command-specific tasks.
- `Sxx`: sidecar migration or compatibility tracks.

The native UWS desired-state project model is tracked in the completed
[memory-bank/status-M08.md](memory-bank/status-M08.md). The active plan-control
track is [memory-bank/status-P02.md](memory-bank/status-P02.md).

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
