# Ramen

Ramen is a planned public OpenTofu-desired-state engine over API source
documents. It reads Terraform/OpenTofu HCL through `github.com/OpenUdon/tfconfig`,
maps resource facts to API operations from sources such as AWS Smithy JSON,
Google Discovery, and OpenAPI, computes desired-state plans, records SQLite
state history, and hands approved mutations to an explicit trusted executor
boundary.

`plan.md` is the original design seed. The canonical roadmap and project memory
live in the symlinked harness files:

```text
AGENTS.md
memory-bank/
evolution/
```

In this checkout those paths point to `../tofu/ramen`, so planning history is
tracked in the shared tofu harness repository while remaining easy to read from
the Ramen repo.

## Scope

Ramen owns desired-state reconciliation:

- Terraform/OpenTofu HCL loading through public `tfconfig`;
- resource-to-operation mapping in public `tfmapping`;
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
```

Later lifecycle commands:

```bash
ramen refresh
ramen destroy
ramen import
```

The first `apply` surface is confirmation-gated and mock-backed in default
public builds. Live executor wiring remains opt-in behind trusted adapters.

## Milestone Pattern

Ramen uses one zero-padded milestone sequence:

```text
M01, M02, M03, ... M09, M10, M11, ...
```

The `M` prefix is the only milestone lane. The two-digit minimum keeps files
and tables sorting naturally after the roadmap reaches `M10`. Each milestone
has a matching task file such as `memory-bank/status-M01.md`.

Current first-pass roadmap:

- `M01`: harness and boundary bootstrap.
- `M02`: shared `tfmapping` and source metadata.
- `M03`: static `init` and `plan` engine.
- `M04`: trusted execution adapter and gated `apply`.
- `M05`: state history, refresh, destroy, and import.

Sidecar milestones use a separate `S` sequence for cross-repo migrations:

- `S01`: move all `openudon convert tf` functionality into Ramen so OpenUdon
  no longer owns Terraform/OpenTofu conversion or imports `tfconfig`.

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
