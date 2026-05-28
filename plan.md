# ramen — Plan: Terraform-style Reconciliation over API Sources

`ramen` is a new standalone tool that simulates `terraform init / plan / apply /
destroy` but realizes resources through **API source operations** (AWS Smithy,
Google Discovery, OpenAPI) instead of Terraform provider plugins, and does
Terraform's key ideas *better* — notably a **SQLite state store that keeps full
history**. Input is Terraform/OpenTofu HCL; execution is delegated to the
existing trusted runtime (udon). There is no user-facing TF→UWS conversion step.

## Context

**Why a standalone module.** The desired-state paradigm (state, dependency DAG,
drift, lifecycle) is distinct from openudon's *authoring* and udon's *imperative
workflow execution*. A separate module (`github.com/OpenUdon/ramen`) gives a
clean product boundary and independent versioning, and consumes the existing
stack as libraries/executor.

**Module-boundary facts that shape this (verified):**
- udon (`github.com/genelet/udon`) exposes execution publicly: `spider`
  (`ExecuteLeaf`, `ExecuteRuntimePlanWithOptions`), `pkg/credentials`,
  `pkg/runtimeplan`, `pkg/execcache`, `pkg/uwsprofile`. **But its native-source
  lowering is internal** (`internal/uwsbridge`, `internal/sourceloader`) — not
  importable cross-module.
- openudon's TF→operation mapping is locked in `internal/tfconvert` (incl. the
  `resourceOps` CRUD adapter recently added there) — **not importable
  cross-module**. TF HCL parsing is the separate, importable
  `github.com/OpenUdon/tfconfig` module.
- apitools (`github.com/OpenUdon/apitools`) exposes source metadata + operation
  inventories publicly, but **no** resource→operation mapping (and is
  deliberately Terraform-neutral — the wrong home for TF mapping).
- Dependency edges today: udon→apitools; openudon→{apitools, tfconfig}. No cycles.

---

## Architecture

`ramen` is the Terraform engine. It does everything *except* execution:

```
github.com/OpenUdon/ramen
  cmd/ramen/      CLI: init | plan | apply | destroy | refresh
  tfmapping/      public TF resource -> {read,create,update,delete} operation mapping
                  (provider adapters + ResourceOps, ported from openudon's draft converter)
  graph/          desired-state graph + dependency DAG (from tfconfig references)
  state/          SQLite store (runs, resources, state_revisions, locks) + history
  reconcile/      plan/apply/destroy loop: diff desired vs current, DAG ordering
  exec/           udon integration: compose a UWS op per CRUD action, run it,
                  capture the response for diff/identity
```

**Reuse (libraries, not internals):**

| Need | Reuse |
|------|-------|
| HCL parse | `github.com/OpenUdon/tfconfig` (public) |
| Source metadata / operation inventory | `github.com/OpenUdon/apitools` (public parsers, `BuildAPISourceOperationInventory`, `OperationIndex`) |
| UWS document model | `github.com/OpenUdon/uws/uws1` (compose each action as an internal UWS operation) |
| Operation execution + credentials | udon **public** packages: `spider`, `pkg/runtimeplan`, `pkg/credentials`, `pkg/execcache`. NOT `internal/sourceloader`/`internal/uwsbridge`. |

**TF→operation mapping lives in `ramen/tfmapping/`** (the canonical home),
ported from the existing openudon draft-converter `resourceOps` design:
`aws_iam_role`→Get/Create/Update/DeleteRole identity `RoleName`;
`google_storage_bucket`→buckets get/insert/patch/delete identity `name`;
Smithy/Discovery-first source preference. openudon's `convert tf` keeps its
existing `internal/tfconvert` copy for now (some duplication); converge later
(openudon imports `ramen/tfmapping`, or both share it).

## udon integration — the one cross-module gap (open decision)

`ramen` must turn a mapped source operation into something udon executes. udon's
"native-source UWS doc → runtime plan" lowering is `internal/` (uwsbridge +
sourceloader), so a cross-module caller cannot build that plan today. Two routes:

- **Option A (recommended): library + a small udon export.** Add a public
  plan-builder facade to udon (e.g. `pkg/uwsbuild.FromDocument(doc, baseDir)` or
  `pkg/runtimeplan.FromUWSDocument(...)`) wrapping the existing internal bridge.
  `ramen` then imports `spider` + the facade + `pkg/credentials` and runs each
  action in-process, capturing `$response` for read/diff. One contained,
  well-scoped change to udon; no other udon behavior changes.
- **Option B: udon-binary trusted handoff.** `ramen` emits a per-action (or
  batched) UWS workflow + run-config and invokes the `udon` binary, reusing the
  existing trusted-handoff path. No udon code change, looser coupling, but
  coarser response capture and process-per-batch overhead.

Recommend A — clean in-process execution and structured read-back, which
`plan`/diff and identity capture need.

## Lifecycle commands

- **`init`** — create/migrate the SQLite state DB; resolve & cache source metadata.
- **`plan`** (dry-run, default-safe) — refresh live state via read ops, diff vs
  SQLite baseline + desired config, emit create/update/delete/no-op diff. No mutation.
- **`apply`** — DAG order; per resource: refresh → diff → (create|update|delete|skip);
  capture identity/computed attrs from responses; write a SQLite revision.
  Confirmation gate unless `--auto-approve`.
- **`destroy`** — delete tracked resources in reverse DAG order.
- **`refresh`** — update stored state from live reads (drift report), no apply.

## SQLite state schema (history is the differentiator)

- `runs(run_id, command, started_at, finished_at, summary, status)`
- `resources(resource_id, address, provider, source_kind, source_id, identity_json, current_attrs_json, status, updated_run_id)`
- `state_revisions(rev_id, resource_id, run_id, action, before_json, after_json, diff_json, created_at)` — full history, one row per change.
- `locks(lock_id, holder, acquired_at)` — single-writer lock via `BEGIN IMMEDIATE`.

Better-than-Terraform: complete, queryable revision history vs Terraform's
current+one-backup.

## Reconcile loop (per resource, DAG order)

1. Resolve desired attrs from TF config (+ interpolated refs from applied resources).
2. Current state: SQLite baseline → if present, optionally read-live for drift;
   if absent and `ReadByIdentity`, attempt read-by-name.
3. Diff desired vs current → action.
4. `plan`: record intended action. `apply`: execute the mapped op via udon;
   capture identity + computed attrs; write a revision.
5. On failure: halt dependents, record partial state, emit deterministic diagnostics.

Dependency DAG built from `tfconfig` attribute references (resource A → resource B);
topological apply, reverse for destroy.

## Phasing

- **P0 — module scaffold + execution seam.** Stand up `ramen`; resolve the udon
  integration (Option A: add the public plan-builder facade to udon, or B). Prove
  one mapped operation executes via udon from `ramen` with response captured.
  **Smallest end-to-end slice.**
- **P1 — read-before-write apply, no DB.** Port `tfmapping`; build `graph`;
  reconcile create/update via live read+diff. Re-run = no-op.
- **P2 — SQLite state + history + `plan`/diff.** Baseline, dry-run plan, drift report.
- **P3 — destroy + delete-on-removal + locking + `import`.**
- **P4 — parity hardening.** Waiters/eventual consistency, computed read-back,
  parallel apply, partial-failure recovery (hardest Terraform-parity items).

## Boundaries & safety

- `ramen` **mutates live cloud state** — execution stays behind udon's trusted
  boundary, requires explicit credentials, defaults to `plan`, gates
  `apply`/`destroy` behind confirmation (`--auto-approve` to bypass).
- Honest parity gap: no provider-internal orchestration initially (waiters,
  computed attrs, multi-call create) — P4.

## Reuse from existing work

The `resourceOps`/`providerAdapter` CRUD extension already in openudon
`internal/tfconvert` (+ `resourceops_test.go`) is the **design seed** for
`ramen/tfmapping/`. P1 ports the design into `ramen`. Decide later whether to
delete the openudon copy and have `convert tf` import `ramen/tfmapping`, or keep
both.

## Open decisions

1. **udon integration** — Option A (add a public plan-builder facade to udon;
   recommended) vs Option B (udon-binary handoff).
2. **Mapping convergence** — duplicate now / converge later, vs shared public
   `tfmapping` consumed by both `ramen` and openudon from the start.
3. (Carried) input = TF HCL first; state DB = local `.ramen/state.db` first,
   remote backend later; identity-without-state → diagnostic in P1-P2, `import` in P3.

## Verification

- **P0**: one mapped operation runs via udon from `ramen`; response captured.
- **P1**: drive `aws_iam_role` + `google_storage_bucket` fixtures against a
  mock/record HTTP transport (no live cloud). Apply twice → second run no-op.
- **P2**: `plan` diff correct; one SQLite revision per change; out-of-band drift
  detected on `refresh`.
- **P3**: `destroy` reverse-DAG; lock blocks concurrent `apply`.
- Per-module unit + integration tests; mock transport (or mock udon executor) so
  CI makes no live cloud calls.
