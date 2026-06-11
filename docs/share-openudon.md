# Share Ramen/OpenUdon Primitives Deliberately

## Context

Ramen and OpenUdon are sibling modules with no Go dependency between them.
Ramen does not import OpenUdon, and OpenUdon should not import Ramen. Both sit
on the shared `uws` and `apitools` substrate, but they own different product
surfaces:

- Ramen owns desired-state reconciliation: resource-operation mapping,
  dependency graphs, state, diffs, import, refresh, apply, destroy, and
  Ramen-specific run/audit history.
- OpenUdon owns workflow authoring, review package generation, approval
  templates, package digests, and trusted-runner handoff for UWS projects.

Sharing with OpenUdon is directionally useful, especially around governance
and approval contracts. The next move is not a broad `uwskit` extraction of
executor, CLI, governance, and run behavior. The immediate work should
classify which Ramen/OpenUdon primitives are truly reusable, compare the
approval and governance contracts, and avoid moving behavior before the shared
contract is stable.

## Recommendation

Treat shared code as an outcome of contract comparison, not as the starting
point. Keep Ramen and OpenUdon import-independent while the two projects prove
where their boundaries really overlap.

The likely reusable center is governance and approval metadata: policy inputs,
approval requirements, approver identity, digest binding, audit intent, and
versioning rules. Executor interfaces, imperative run scaffolding, CLI helpers,
and redaction may become shareable later, but each has more product-specific
coupling today.

The current Ramen-side priority is to finish the evidence-gated API execution
tracks first: M27 live Azure lifecycle evidence, A03 retry/waiter policy, D02
internal delete confirmation, and A04 settle barriers. Ramen should contribute
stable plan approval, executor handoff, run/audit, governance, and sanitized
feedback contracts to the OpenUdon review, but OpenUdon should continue to lead
package review, approval-template, package digest, and runner-handoff
requirements. That Ramen-side contribution now includes concrete A03/D02/A04
contracts: approval-bound runtime hints, provider-neutral executor
retry/read-waiter behavior, read-confirmed delete semantics, and
Ramen-orchestrated pre-delete settle metadata for native resources with read
roles.

M47 records the current Ramen-side state of this item as parked and
OpenUdon-led. Ramen can contribute reviewed contract input, but should not
start an implementation milestone, import OpenUdon product packages, or extract
a shared module until OpenUdon has a concrete approval/governance proposal.

## Next Moves

1. Compare Ramen `governance` artifacts with OpenUdon approval and
   review-handoff artifacts.

   Inventory the public and internal schemas, digest inputs, policy decisions,
   approval requirements, approver metadata, package/run handoff fields, and
   audit records. Record which fields are semantically identical, which are
   product-specific, and which only look similar because they use the same
   words.

2. Define the shared approval/governance core and versioning strategy.

   The shared core should describe the smallest stable contract both projects
   can honor without forcing either product to inherit the other's workflow.
   Version it explicitly. Leave Ramen-specific desired-state bindings and
   OpenUdon-specific package/review bindings outside the core unless they are
   genuinely common.

3. Keep Ramen and OpenUdon import-independent.

   Do not make either application import the other. Use `uws` and `apitools`
   as the existing shared substrate. Introduce a third module only if the
   contract comparison produces a stable shared core with clear owners, tests,
   release expectations, and standalone `GOWORK=off` behavior.

4. Treat redaction as a supporting extraction only if needed.

   Ramen's redaction helpers may be small enough to share, but they should not
   drive the migration. Extract redaction only if governance or executor
   sharing needs a common redaction contract for durable artifacts.

## Future Migration Ideas

### Shared Governance Module

After the contract comparison, a narrow shared module may make sense for
approval requirements, policy decisions, approver metadata, digest binding
helpers, and schema version constants. This should be a small governance core,
not a home for Ramen reconciliation or OpenUdon packaging behavior.

### Shared Executor Interfaces

Executor sharing should wait until Ramen resource-action execution and
OpenUdon package-runner execution prove they have a real common core. Ramen's
executor boundary is shaped by desired-state actions, state writes, and
resource lifecycle semantics. OpenUdon's runner handoff is shaped by reviewed
workflow packages and trusted execution. Shared interfaces are useful only if
they do not erase those differences.

### Shared Run Skeleton

A common imperative run skeleton may be possible after the recorder/storage
boundary is clear. Ramen stores run and event history in SQLite beside
desired-state state, while OpenUdon may need package-runner or review-handoff
records instead. Any shared run package would need a storage-agnostic recorder
interface before code moves.

### Redaction Helpers

Common redaction helpers may be worth extracting if both projects need the same
path/key matching, digest-safe value handling, and artifact redaction rules.
Keep this small and subordinate to the governance/executor contract needs.

### CLI Helpers

CLI helper sharing is the lowest priority. Repeated flag parsing, JSON output,
signal context setup, and positional argument helpers can be shared later if
duplication becomes painful. They are not important enough to justify a new
module by themselves.

## Do Not Do Yet

- Do not make OpenUdon import Ramen.
- Do not migrate Ramen `run` wholesale.
- Do not force OpenUdon's approval schema into Ramen's current
  `ramen.governance.v1`, or force Ramen's governance schema into OpenUdon's
  current approval model.
- Do not create a broad `uwskit` module before the shared approval/governance
  contract is stable.
- Do not move Ramen-owned desired-state packages such as `plan`, `apply`,
  `destroy`, `refresh`, `import`, `graph`, `state`, `project`, `tfmapping`, or
  `convert tf`.

## Verification For Any Later Extraction

If a later extraction becomes justified, each phase should keep the touched
repo green:

- `go test ./...` and `go vet ./...` in Ramen.
- Equivalent OpenUdon tests for any adopted approval or runner contract.
- Standalone `GOWORK=off` builds once a shared module is tagged.
- Behavior parity for approval digests, review handoff, `ramen apply --mock`,
  and `ramen run` summaries where those paths are touched.

This document is strategic guidance, not an implementation plan for an
immediate shared module.
