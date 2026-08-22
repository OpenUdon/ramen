# v0.1 Compatibility Contract

Ramen's v0.1 compatibility boundary is deliberately smaller than its exported
repository surface.

## Stable During v0.1.x

- Native UWS/Ramen project loading through `project`.
- Provider-free validation and graph construction through `validate` and
  `graph`.
- Deterministic plan and approved apply orchestration through `plan` and
  `apply`.
- Local state and reconciliation through `state` and `reconcile`.
- The in-process trusted execution contract in `executor`.
- Existing native-core `*.v1` artifact shapes and CLI diagnostic codes.

Additive fields and methods may be introduced in v0.1.x. Existing fields,
accepted inputs, and behavior will not intentionally be removed or redefined.
Readers fail closed on unknown future artifact versions. SQLite state migrates
forward and rejects databases created by a newer unsupported schema; downgrade
compatibility is not promised.

`ramen.project.v1` fixed structures are schema-closed: unknown fields are not
accepted compatibility extension points and fail before typed decoding. Custom
project data belongs in documented `metadata` maps. Terraform conversion
metadata is experimental and requires `ramen.terraform.provenance.v1`; old
unversioned conversion packages must be regenerated rather than translated.

## Experimental Before v1

Authoring, iCoT, Terraform/OpenTofu conversion, Ansible conversion, static
governance helpers, imperative runbooks, and provider-specific mapping breadth
may evolve between pre-1.0 releases. They remain bounded adapters into Ramen's
native model and do not imply Terraform/OpenTofu, Ansible, provider, backend,
or plan-file runtime compatibility.

Public binaries remain mock-backed for execution. Live mutations require a
separately trusted executor implementation and are outside the v0.1 release
contract.

Native browser desired-state support consumes the UWS browser 1.5-1.7 and
browser-authentication 1.0-1.1 contracts. Ramen validates and digest-binds
those artifacts, but browser authoring and UI approval remain OpenUdon-owned,
and live browser/session/credential behavior remains trusted-runtime-owned.
See [Browser Desired State](browser-desired-state.md).

See [Terraform/OpenTofu conversion](terraform-conversion.md) for the exact
review-metadata and reconversion boundary.
