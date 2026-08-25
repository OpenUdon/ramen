# Provider-Equivalent Hardening Without Provider Cloning

This note records which OpenTofu/provider behaviors Ramen should adopt as
general desired-state engine features before live equivalence testing, and
which behaviors should stay deferred until mappings and live or recorded
equivalence tests prove they are needed.

OpenTofu providers are a reference for useful behavior, not a compatibility
target. Ramen should not reimplement provider internals, import provider SDKs,
or depend on Terraform/OpenTofu runtime behavior. The target is narrower:

> For each explicitly mapped resource type, prove that Ramen can converge the
> same intended API-visible state as the reference tool for the supported
> fixture set.

The [client-language conversion model](client-language-conversion.md)
distinguishes optional provider-schema client evidence from API-source server
authority and the trusted execution boundary.

## Feature Decisions

| Provider behavior | Ramen direction | Rationale |
|---|---|---|
| Schema validation | Implement as Ramen-native validation. | Needed before mutation. Validate required attributes, unknown attributes, enum values, sensitive fields, operation roles, identity metadata, and unsupported nested blocks from API-source and mapping metadata. |
| Expand/flatten logic | Implement as request/response bindings. | Ramen needs Terraform/native attribute to API request shaping and API response to state shaping, but this should be mapping metadata and binding rules, not copied provider code. |
| Computed defaults | Implement selectively. | Needed to avoid false diffs after read/apply. Defaults should come from API reads, source metadata, explicit mapping defaults, or recorded equivalence evidence. |
| Refresh/read-before-write | Implement generally. | Required for drift detection, idempotency, and update-vs-create decisions. Direct live use remains behind the trusted executor boundary. |
| Read-after-write | Implement generally. | Required to store response-derived identity/computed state and to verify convergence after mutation. |
| ForceNew/update semantics | Implement as lifecycle metadata. | Ramen needs replace-on-change, create-only, updateable, immutable, and ignored/computed path metadata per mapped resource type. |
| Retries and waiters | Implement as executor/reconcile policy. | Real API systems need retryable error handling, backoff, timeout, and wait-until-stable predicates. Provider-specific predicates must be mapping-driven. |
| Eventual consistency handling | Implement through waiters and post-write reads. | Consistency windows are a property of API execution, not only of providers. Ramen should model them explicitly rather than hiding them in provider code. |
| Custom diff logic | Implement only common normalizers first. | Useful general cases include JSON semantic equivalence, unordered collections, case normalization, empty/null/absent equivalence, and redacted/sensitive placeholders. Arbitrary custom diff code should require tests. |
| Import/state ID conventions | Implement as identity metadata. | Import and future plan/apply need stable identity fields, response paths, and external IDs. Avoid ad hoc provider-style opaque ID parsing where structured identity can be modeled. |
| Destroy semantics | Implement per operation role. | Some resources delete, some detach, disable, suspend, or remove configuration. This cannot be fully generic; mappings must say what destroy means. |
| Provider-specific edge cases | Defer until proven. | Handle only when mapping-specific live or recorded equivalence tests show the edge case matters. Encode it as metadata, normalizers, waiters, or narrow adapters. |

## General Primitives

1. **Mapping schema**
   Required, optional, computed, sensitive, identity, enum, immutable,
   response-derived identity, create-only, updateable, replace-on-change,
   read-only, and ignored paths. The public `tfmapping.SchemaPath` contract is
   the place for native execution metadata. Conversion may use an offline
   provider-schema snapshot to validate Terraform client configuration, but it
   does not import that snapshot into `tfmapping` or treat it as lifecycle/API
   authority.

2. **Request and response binding**
   Attribute-to-request paths for create/update/delete/read and response-to-state
   paths for identities, response-derived identities, computed attributes, and
   drift evidence. `tfmapping.RequestBinding` and `tfmapping.ResponseBinding`
   cover resources whose create request uses one configured value while later
   read/update/delete calls require an identifier returned by create/read.

3. **Diff normalization**
   General normalizers for JSON strings, unordered sets/lists where order is not
   semantically meaningful, empty/null/absent equivalence, casing, and sensitive
   redaction placeholders. `tfmapping.NormalizerKind` intentionally only names
   common provider-neutral cases; arbitrary mapping-owned diff code remains
   deferred until equivalence tests justify it.

4. **Read-plan-apply-read loop**
   A reconcile flow that can read current remote state, build a plan from the
   latest evidence, execute through the trusted boundary, then read again to
   confirm convergence and store state.

5. **Lifecycle semantics**
   Operation roles for create, read, update, replace, delete, suspend, detach,
   remove-config, and no-op, with explicit diagnostics when a requested action
   has no safe mapping. `tfmapping.OperationRole` and
   `tfmapping.LifecycleSemantics` define the vocabulary; per-resource behavior
   still has to be populated by mapping metadata and tested before live use.

6. **Retry, waiter, and settle policy**
   Generic backoff, timeout, retryable error classes, post-write read polling,
   per-operation success predicates, and bounded read-backed settle barriers
   before follow-on mutations.

7. **State identity model**
   Stable identity fields and response-derived IDs stored in state so future
   plans depend on API-visible identity, not only on configuration text.

## Provider-Clone Boundary

Ramen may read provider fixture files and explicit provider-schema JSON
snapshots as inert conversion/test input, but it must not link provider runtime
code into public builds. A snapshot validates only the foreign client
configuration; API sources and reviewed Ramen mapping metadata remain
authoritative for execution. The boundary test in `tests/boundary_test.go` enforces
the current rule:

- no Terraform provider packages, Terraform/OpenTofu runtime internals,
  provider plugins, or `tfconfig/_upstream` imports;
- no private udon imports unless the file is explicitly guarded by
  `//go:build udon`;
- useful provider behavior must be represented as public mapping metadata,
  bindings, normalizers, lifecycle roles, retry/waiter/settle policy, or
  trusted executor contracts.

The contract types added for M20 are intentionally metadata-only. Validation,
planning, refresh, import, apply, and destroy consume them in the later command
tracks, and live-behavior claims remain parked until recorded or live
equivalence evidence exists.

M46 closes the current hardening ownership audit by codifying the remaining
escape hatches as explicit guardrails. Identity-changing update metadata emits
`validate.identity_update_unsupported` rather than silently planning a rename;
arbitrary custom diff behavior remains rejected as `validate.normalizer_unknown`;
opaque import parser work stays closed for the current mapped non-AWS set
because structured identity metadata is sufficient; and `ramen state
async-evidence` remains read-only inspection with no resume or resubmit flags.
Any future feature in these areas needs a narrow status lane, mapping metadata,
and fixture or recorded/live evidence.

V03 carries the first consumer slice: native project resources can now declare
`schema`, `request_bindings`, `response_bindings`, `normalizers`,
`mapping_lifecycle`, and `required_operations`, and `ramen validate` enforces
the static parts of those contracts without provider execution.

P04 carries those contracts into plan artifacts and desired hashes. The mapping
hash input includes `ramen.mapping-metadata.v1` plus schema, request/response
bindings, normalizers, mapping lifecycle, and required operation metadata, so
approval artifacts become stale when those hardening contracts change.

R02 consumes the read-side subset during refresh: response bindings project
executor result values into identity/computed state, declared normalizers are
applied before drift comparison, and response-bound sensitive paths are redacted
before state/history writes.

I03 reuses the same identity model for import. Native project import validation
accepts identity fields declared by `identity_attributes`, schema identity paths,
and response-derived identity bindings, and reports stable `import.*`
diagnostics when metadata or supplied identity values are incomplete.

A02 keeps live apply behavior parked but tightens the non-live safety surface:
apply now reports stable `apply.*` diagnostics for missing approval, missing
executor selection, invalid action documents, unsupported executor capabilities,
executor failures, and unsuccessful executor results. Public apply coverage
remains mock-backed and credential-free.

D02 makes destroy semantics explicit in planning and execution. Native project
destroy planning can use delete, detach, disable, suspend, remove-config, or
no-op roles, and destroy fails closed with `destroy.operation_missing` when no
safe operation role exists.

## Deferral Policy

Features that alter live API behavior, claim provider equivalence, or depend on
resource-specific semantics should be parked until both conditions are true:

- The relevant mapping has enough schema, lifecycle, request, response, and
  identity metadata to make the behavior deterministic.
- A live or recorded equivalence test compares Ramen behavior with the
  reference tool for the supported fixture set.

Until then, Ramen may add static validation, metadata contracts, diagnostics,
and recorded test harnesses, but should avoid broad claims that `ramen apply`
matches `opentofu apply`.
