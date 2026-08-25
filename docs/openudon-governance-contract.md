# OpenUdon Governance Contract Input

This note is Ramen's M47 input to an OpenUdon-led approval and governance
review. It is not a Ramen implementation plan and does not change the import
boundary: Ramen must not import OpenUdon product packages, and OpenUdon should
not import Ramen desired-state packages.

## Shared Candidate Core

A future shared contract should stay small and product-neutral. The candidate
common fields are:

- contract version and producer identity;
- policy input digests and policy decision summaries;
- required approvals, approval status, and approval expiry;
- approver identity claims and approval timestamps;
- digest binding between the reviewed artifact and the approved artifact;
- audit and evidence references that are stable outside either product.

The shared core should not include Ramen resource lifecycle semantics or
OpenUdon package runner semantics.

## Ramen-Owned Fields

Ramen-owned governance data stays in Ramen artifacts until a reviewed neutral
contract exists:

- desired-state resource addresses, actions, dependencies, and lifecycle roles;
- state baseline identifiers and desired-hash inputs;
- plan options, targeted resources, and approval controls;
- executor selection and trusted-boundary capability checks;
- reconciliation, state history, and convergence outcomes.

These fields are part of desired-state reconciliation. They should not move into
OpenUdon review packages as generic package semantics.

## OpenUdon-Owned Fields

OpenUdon-owned governance data remains OpenUdon-led:

- review package manifests and package layout;
- approval templates and reviewer workflow;
- package digests and package immutability rules;
- trusted-runner handoff and package-run status;
- OpenUdon-specific review UI or authoring metadata.

Ramen can consume a reviewed neutral approval/governance shape later, but it
should not preempt OpenUdon package-review behavior.

## Neutral Module Requirements

If a shared package becomes justified, it should be a narrow public module or a
neutral `uws`/Evidence extension with:

- explicit schema versioning;
- standalone `GOWORK=off` tests;
- no imports from `github.com/OpenUdon/ramen` or
  `github.com/OpenUdon/openudon`;
- digest parity tests for approval-bound artifacts;
- redaction rules for durable governance and evidence records.

The default path remains documentation and contract review until OpenUdon
accepts or requests a concrete cross-repo shape.

## Non-Goals

- Do not create a broad shared `uwskit` module from this item.
- Do not move Ramen `plan`, `apply`, `run`, `state`, `project`, `graph`,
  `import`, `refresh`, or `destroy` behavior.
- Do not move OpenUdon package generation, package review, approval-template,
  package digest, or runner-handoff behavior.
- Do not add a Ramen dependency on OpenUdon or an OpenUdon dependency on Ramen.

## Verification

Ramen enforces the current boundary with:

```bash
go test ./tests -run TestNoForbiddenProviderRuntimeImports -count=1
```

Any later implementation that consumes a reviewed shared contract must also run
the full Ramen default gates and the matching OpenUdon gates for the accepted
contract.
