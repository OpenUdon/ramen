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
   create-only, updateable, replace-on-change, read-only, and ignored paths.

2. **Request and response binding**
   Attribute-to-request paths for create/update/delete/read and response-to-state
   paths for identities, computed attributes, and drift evidence.

3. **Diff normalization**
   General normalizers for JSON strings, unordered sets/lists where order is not
   semantically meaningful, empty/null/absent equivalence, casing, and sensitive
   redaction placeholders.

4. **Read-plan-apply-read loop**
   A reconcile flow that can read current remote state, build a plan from the
   latest evidence, execute through the trusted boundary, then read again to
   confirm convergence and store state.

5. **Lifecycle semantics**
   Operation roles for create, read, update, replace, delete, suspend, detach,
   remove-config, and no-op, with explicit diagnostics when a requested action
   has no safe mapping.

6. **Retry and waiter policy**
   Generic backoff, timeout, retryable error classes, post-write read polling,
   and per-operation success predicates.

7. **State identity model**
   Stable identity fields and response-derived IDs stored in state so future
   plans depend on API-visible identity, not only on configuration text.

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

