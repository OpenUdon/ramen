# OpenUdon And Ramen Evidence Alignment

Ramen and OpenUdon can share neutral Evidence records without sharing product
semantics or importing each other.

## Shared Neutral Record Shape

Both products may use `github.com/OpenUdon/evidence/async` for:

- execution request evidence;
- execution response evidence;
- status observations;
- confirmation-read observations.

The shared record shape can describe who requested execution, which operation
was handed to an executor, which runtime hints were supplied, and what the
executor or confirmation read reported.

## Ramen-Owned Meaning

Ramen interprets those records only inside Ramen reconciliation:

- desired-state convergence;
- desired hashes;
- read-before-write baseline checks;
- read-after-write convergence checks;
- delete confirmation;
- SQLite state and revision history;
- `ramen.approval.v1` and `ramen.governance.v1`.

An async response with outcome `accepted` means the executor invocation was
accepted. It does not mean the resource converged or that state can be updated.

## OpenUdon-Owned Meaning

OpenUdon owns package/run behavior:

- workflow authoring and review packages;
- approval templates;
- package digests;
- trusted-runner handoff;
- run evidence and sidecar references.

OpenUdon can forward neutral async evidence, but it should not reinterpret
Ramen confirmation reads as desired-state convergence.

## Boundary Rules

- Ramen must not import OpenUdon packages.
- OpenUdon must not import Ramen packages for runner evidence forwarding.
- Shared code should live in Evidence only when the shape is product-neutral.
- Credential values, raw executor stdout/stderr, live provider payloads, and
  product-specific state semantics must not be placed in neutral records.

## Current Ramen Check

Ramen's executor evidence tests validate generated async records through the
Evidence async validators and keep convergence assertions out of those shared
records. Cross-repo sidecar fixture comparison remains deferred until OpenUdon
has a stable committed sidecar fixture.
