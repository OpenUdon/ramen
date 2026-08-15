# Terraform/OpenTofu Conversion

`ramen convert` and its explicit `ramen convert tf` form statically convert
Terraform/OpenTofu HCL facts into a native UWS/Ramen desired-state project and
review artifacts. The converter reads HCL through `tfconfig`, matches objects
to local API source operations through `tfmapping` and `apitools`, and never
loads or executes Terraform/OpenTofu, providers, modules, backends, API
operations, or UWS workflows.

```bash
ramen convert tf \
  --config-dir DIR \
  --api-source KIND:ID=PATH \
  --action create \
  --target ADDRESS \
  --out DIR \
  --strict
```

Conversion is an experimental migration and review aid. The generated native
profile is the planning contract; Terraform provenance remains review-only and
is not sent to the trusted executor.

## Inputs

- `--config-dir DIR` selects the Terraform/OpenTofu configuration directory.
  It defaults to the current directory and is parsed statically through the
  public `tfconfig` API.
- `--api-source KIND:ID=PATH` supplies a local API source. Supported kinds are
  `openapi`, `aws-smithy`, and `google-discovery`; the flag may be repeated.
- `--openapi ID=PATH` is the retained OpenAPI shorthand.
- `--action create|update|delete|replace` selects the desired mapping action.
- `--target ADDRESS` restricts conversion to a Terraform address and may be
  repeated.
- `--out DIR` selects the review package directory and defaults to
  `.ramen/convert`.
- `--strict` returns a failure when strict diagnostics remain. Static review
  artifacts and diagnostics retain the existing converter behavior.

Inputs and staged API source documents are untrusted. Source staging rejects
unsafe overlaps and unowned pre-existing generated directories. Credentials
remain symbolic binding names; values are never copied from provider
configuration into generated artifacts.

## Outputs

The output package includes:

- `project.uws.yaml` and `project.uws.hcl`, the native project artifacts;
- `workflows/workflow.uws.yaml` and `workflow.uws.hcl`, the conversion review
  workflow;
- `project.md` and `expected/review.md`, the human review boundary;
- `expected/conversion.json`, `mappings.json`, `plan.json`, and `plan.md`;
- `expected/diagnostics.json` and `diagnostics.md`;
- staged local API source documents required by the native project.

The supporting conversion, mapping, plan, diagnostics, and source-staging
artifacts keep their existing formats. The schemas described below cover the
native project profile and UWS-carried Terraform review metadata.

## Native Project Contract

Every generated project carries `x-ramen-desired-state` with discriminator
`ramen.project.v1`. This is the Ramen planning and reconciliation contract: it
contains API source references, variables, resources, lifecycle, operation
roles, identity and request/response bindings, schema paths, normalizers,
runtime hints, redaction, and review metadata.

Ramen validates the extension against its embedded `project.v1` JSON Schema
before typed decoding, then runs the existing semantic and cross-reference
validation. Fixed structures reject unknown fields. Deliberately open maps,
including resource `attributes` and documented `metadata` maps, may carry
arbitrary application values.

The discriminator remains `ramen.project.v1`. This hardening does not add a v2
or a permissive compatibility decoder: fields that were never part of the
declared structure are rejected instead of being silently discarded. Custom
data belongs under a documented `metadata` map.

## Terraform Provenance Contract

Every converted operation carries review-only request metadata under
`x-ramen-terraform`:

```yaml
x-ramen-terraform:
  attributes:
    name: example
  object:
    address: example_resource.main
    kind: resource
    name: main
    type: example_resource
  version: ramen.terraform.provenance.v1
```

The required fields are:

- `version`: exactly `ramen.terraform.provenance.v1`;
- `object`: non-empty `address`, `type`, and `name`, plus `kind` equal to
  `resource` or `data_source`;
- `attributes`: the arbitrary nested static Terraform values retained for
  review;
- optional `identity_attributes`: strict entries containing `name`,
  `terraform_path`, optional request/response paths, and the required marker.

`x-ramen-credential-bindings` may carry unique non-empty symbolic credential
binding names. It never carries credential values.

The metadata is validated by an embedded schema and internal typed helpers.
Unknown fixed metadata fields, missing discriminators, malformed object or
identity entries, duplicate credential bindings, and empty TODO values fail
closed. Standard UWS request locations such as `path`, `query`, `header`,
`cookie`, and `body` remain governed by UWS and the selected API source.

## Operation Binding

Mapped operations are source-bound through UWS `sourceDescription` and
`sourceOperationId`. Their API request values use standard UWS request
locations, and they carry no operation profile or TODO.

An unresolved mapping is review scaffolding rather than an executable source
operation. It carries:

```yaml
x-uws-operation-profile: ramen.terraform-review-todo.1.0
request:
  x-ramen-todo: operation.unresolved
```

The selector key remains the UWS core `x-uws-operation-profile`; Ramen owns its
Terraform-specific value and the TODO request metadata. An unresolved
operation must not pretend to have a source binding.

`ramen validate --project` validates Terraform metadata whenever an operation
carries a Terraform request key or the current/retired review profile. Invalid
metadata emits `validate.terraform_metadata_invalid`.

## Compatibility Break

Unversioned `x-ramen-terraform` and the retired `ramen-review-todo` selector are
not accepted. Ramen does not translate them and provides no artifact migration
command. Regenerate an old conversion package from its HCL and API source
inputs with the current `ramen convert` command.

Native projects that do not contain Terraform conversion metadata are
unaffected. Planning and execution do not consume Terraform provenance, and
executor-ready action documents continue to omit `x-ramen-terraform` and
unresolved review operations.
