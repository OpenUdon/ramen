# Async Operations Boundary Proposal

This is a boundary proposal for async operations between Ramen and OpenUdon. It does not change schemas or behavior by itself.

## Shared Evidence Types

Ramen and OpenUdon should exchange portable execution evidence through stable, redaction-aware records.

- `execution_request`:
  - Operation identity, provider/project binding, principal intent, input arguments, and non-secret transport metadata.
- Runtime hints are included as mutable execution metadata, but are not part of desired-state comparison inputs. Ramen-owned hints such as settle barriers may be recorded through confirmation-read evidence instead of executor request fields.
- `execution_response`:
  - Mutation result (accepted, rejected, transient failure, fatal failure), raw status code/class, timing, transport correlation IDs, and digest references.
- `execution_status_observation`:
  - Opaque status payload (redacted where needed), last-seen provider status marker, observed terminality hint, and timestamps.
- `confirmation_read_observation`:
  - Read-result evidence for converge checks, including not-found / missing and projected identity/state deltas for read-backed confirmation flows.
- `attempt_metadata`:
  - `attempt_id`, `evidence_id`, ordering monotonicity marker, and actor annotations needed for audit replay.

Common expectations:

- Evidence is append-only at each reconciliation decision point.
- Sensitive operation fields are redacted or digested before persistence.
- Evidence should be sufficient to support resume/forensics without implying success unless a terminal confirmation condition is explicitly satisfied.
- Runtime hint evolution alone must not alter desired-state hash outcomes.

## Ramen-Owned Reconciliation Behavior

Ramen owns desired-state semantics and convergence interpretation.

- Mapping and planning:
  - desired resource graph, operation role resolution (`read`, `create`, `update`, `delete`), and per-resource execution intent.
- Executor interaction:
  - builds and sends execution request evidence, including executor-owned runtime hints for retries/waiters.
- Convergence confirmation:
  - consumes `confirmation_read_observation` and package/runner execution results.
  - records durable success only when terminal confirmation criteria are met.
- Delete confirmation policy:
  - confirmation requires a concrete read-backed flow (`read` role) before state removal and fail-closed behavior otherwise.
- Settle policy:
  - optional pre-delete settle barriers are Ramen-owned read-backed orchestration, not OpenUdon convergence semantics.
- State/history:
  - state transitions, revision writes, and terminal state records (including failure/unknown) remain in Ramen.

## OpenUdon-Owned Package / Run Behavior

OpenUdon owns package lifecycle and trusted run governance; it should not define convergence truth.

- Package creation and review:
  - authoring, approval packages, review metadata, and package digests.
- Trusted run execution:
  - runner handoff, transport, and execution lifecycle.
- Evidence transport:
  - forwarding of execution responses and status observations to Ramen-compatible records.
- Operational controls:
  - approval gating, auditability constraints, and runner policy enforcement in untrusted/trusted boundaries.

OpenUdon must remain execution-governance focused. It should not import Ramen state semantics, nor reinterpret confirmation as desired-state success.

## Boundary Matrix

- Artifact: `execution_request`
- Shared schema fields: operation identity, role, transport metadata, runtime hints, `request_id`
- Owner: Evidence (neutral record shape), product repos (embedding and policy)
- Producer: Ramen apply/run or OpenUdon package/run flow, depending on the product path
- Consumer: product-owned runner adapter, audit, or reconciliation code
- Trust boundary: crosses into trusted executor context
- Immutability: append-only per revision

- Artifact: `execution_response`
- Shared schema fields: status class, raw outcome, latency/error summary, evidence linkage
- Owner: Evidence (neutral record shape), product repos (transport and interpretation)
- Producer: Trusted runner
- Consumer: Ramen or OpenUdon product flow
- Trust boundary: comes from trusted runner channel
- Immutability: append-only

- Artifact: `execution_status_observation`
- Shared schema fields: correlation IDs, status markers, terminality hint, redacted status payload
- Owner: Evidence (neutral record shape), trusted runner/product adapter (capture), Ramen (decision input)
- Producer: trusted runner / provider polling adapters
- Consumer: product-owned audit or reconciliation code
- Trust boundary: comes from trusted runner channel
- Immutability: append-only, ordered

- Artifact: `confirmation_read_observation`
- Shared schema fields: read request/response summary, missing vs exists outcome, projected state fragments
- Owner: Evidence (neutral record shape), Ramen (authoritative convergence meaning)
- Producer: trusted runner path back to Ramen
- Consumer: Ramen
- Trust boundary: trusted runner → Ramen
- Immutability: append-only

## Deferred Shared Work (Future)

- Optional shared contract module for evidence record envelopes and digest/redaction metadata.
- Optional shared utilities for stable status vocabularies and evidence linkage fields.
- Optional shared terminal-state taxonomies, only after both products consume the same contract shapes.

## Non-Goals

- Changing desired hash behavior.
- Implementing full async operation handle capture and resumable polling in this doc-only step.
- Shifting desired-state convergence logic into OpenUdon.
- Making Ramen depend on OpenUdon internals for core reconciliation.
