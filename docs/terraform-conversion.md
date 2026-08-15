# Terraform/OpenTofu Conversion

`ramen convert` and its explicit `ramen convert tf` form statically convert
Terraform/OpenTofu HCL facts into a native UWS/Ramen desired-state project and
review artifacts. The converter reads HCL through `tfconfig`, matches objects
to local API source operations through `tfmapping` and `apitools`, and never
executes Terraform/OpenTofu, providers, modules, backends, API operations, or
UWS workflows. `tfconfig` may read already-present local module source; Ramen
never downloads a module.

```bash
ramen convert tf \
  --config-dir DIR \
  --api-source KIND:ID=PATH \
  --provider-schema PROVIDER=SCHEMA.json \
  --action create \
  --target ADDRESS \
  --out DIR \
  --mode strict
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
- `--provider-schema ID=PATH` optionally supplies a local JSON snapshot
  compatible with `terraform providers schema -json`. The ID is a full
  provider address or an unambiguous final provider name such as `aws`. The
  flag may be repeated. Ramen reads this file only; it never invokes
  Terraform/OpenTofu or a provider plugin to obtain a schema.
- `--action create|update|delete|replace` selects the desired mapping action.
- `--target ADDRESS` restricts conversion to a Terraform address and may be
  repeated.
- `--out DIR` selects the review package directory and defaults to
  `.ramen/convert`.
- `--mode strict|partial` selects the common conversion gate. Omitted mode
  defaults to `strict`. `--strict` remains a deprecated alias for
  `--mode strict`; use `--mode partial` explicitly for review-only output with
  symbolic or omitted semantics.

Inputs and staged API source documents are untrusted. Source staging rejects
unsafe overlaps and unowned pre-existing generated directories. Credentials
remain symbolic binding names; values are never copied from provider
configuration into generated artifacts.

## Offline Provider-Schema Evidence

An optional provider-schema snapshot validates the Terraform client-language
side of conversion. Format `1.0` snapshots select one provider deterministically
by exact address or unique final name and are limited to a 32 MiB regular JSON
file. For matching resource and data-source types, Ramen validates root
attributes and nested block names, required attributes/blocks, computed-only
configuration, and provider attribute-mode consistency. Mismatches are stable
`provider_schema.*` strict diagnostics with Terraform source identity.

The snapshot is evidence, not the server contract. It cannot select an API
operation, define request/response bindings or authentication, grant execution
authority, or replace `--api-source`. API source documents remain authoritative
for server operations; Ramen's mapping metadata remains authoritative for
desired-state lifecycle and binding behavior. Selected evidence is summarized
in `expected/conversion.json` and review Markdown, while the common manifest
records the safe input filename and SHA-256 digest. The complete provider schema
is neither staged nor copied into generated semantic artifacts.

## Semantic-Loss Gate

Strict mode inventories source facts that the current adapter cannot represent
faithfully. Each fact produces a source-aware strict diagnostic and a matching
`unsupported` manifest coverage item. The current gate covers:

- dynamic, negative, oversized, wrong-shaped, or otherwise unresolved
  resource/data-source `count` and `for_each` instances;
- instance expressions that use `count.index`, `each.key`, or `each.value`
  anywhere other than an exact whole-attribute reference;
- all ephemeral-resource and module-call instances;
- resource lifecycle policy;
- module calls outside the loaded, exactly-once, statically resolvable local
  subset described below;
- ephemeral resources;
- `moved`, `import`, and `removed` state-transition blocks;
- configuration `check` assertions.

Strict conversion retains reports and exits `3` when any of these facts are
present. `--mode partial` may be used to inspect the lowerable base objects,
but the reports continue to identify every omitted semantic. Ramen does not
run an expression runtime, download modules, inspect state, or load providers
in this gate.

## Bounded Static Expansion

Ramen expands a deliberately small instance subset when the `tfconfig` facts
are sufficient without Terraform evaluation:

- literal non-negative integer `count` and literal object/map `for_each`;
- literal string-set `for_each` when the selected `tfconfig` revision exposes
  the collection as a set;
- no more than 256 instances for one resource or data-source declaration;
- canonical addresses such as `aws_instance.web[0]` and
  `aws_instance.web["blue"]`, with no additional base object;
- exact whole-attribute `count.index`, `each.key`, and `each.value`
  substitution. Templates, arithmetic, indexing, function calls, and other
  composite expressions remain strict semantic loss.

A base `--target` selects all statically expanded instances; a canonical
instance address selects that instance. Dependencies retain canonical
instance addresses and a reference to an expanded declaration conservatively
depends on its instances.

Ramen also lowers an already-loaded local module call when it has no `count`,
`for_each`, `depends_on`, or consumed module output; its parent call chain is
supported; every declared input resolves from a literal call value, an exact
parent-variable reference, or a literal child default; and provider mappings
resolve statically. Child objects retain their module address while using the
mapped parent provider binding. Missing/remote module source, sensitive or
unresolved values, non-exact variable expressions, output consumers, and
module instances keep `terraform.module_call_unsupported` (and the applicable
instance diagnostic) instead of producing an equivalence claim.

## Outputs

The output package includes:

- `project.uws.yaml` and `project.uws.hcl`, the native project artifacts;
- `workflows/workflow.uws.yaml` and `workflow.uws.hcl`, the conversion review
  workflow;
- `project.md` and `expected/review.md`, the human review boundary;
- `expected/conversion.json`, `mappings.json`, `plan.json`, and `plan.md`;
- `expected/diagnostics.json` and `diagnostics.md`;
- `expected/manifest.json`, the deterministic common conversion record;
- staged local API source documents required by the native project.

Strict failures retain `project.md`, the `expected/` review evidence, staged
API source inputs, diagnostics, and manifest, but suppress both native project
formats and both workflow formats. A rerun into the same output directory also
removes stale semantic payloads from an earlier partial conversion. Strict
failure exits `3`; operational failures exit `1` and usage errors exit `2`.

The supporting conversion, mapping, plan, diagnostics, and source-staging
artifacts keep their existing formats. The schemas described below cover the
native project profile and UWS-carried Terraform review metadata.
The shared report envelope is defined in the
[static conversion contract](conversion.md).

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
carries `x-ramen-terraform`, an unknown `x-ramen-terraform-*` key, or the
current/retired review profile. Generic `x-ramen-credential-bindings` or
`x-ramen-todo` request metadata alone does not activate the Terraform contract.
Invalid metadata emits `validate.terraform_metadata_invalid`.

## Compatibility Break

Unversioned `x-ramen-terraform` and the retired `ramen-review-todo` selector are
not accepted. Ramen does not translate them and provides no artifact migration
command. Regenerate an old conversion package from its HCL and API source
inputs with the current `ramen convert` command.

Native projects that do not contain Terraform conversion metadata are
unaffected. Planning and execution do not consume Terraform provenance, and
executor-ready action documents continue to omit `x-ramen-terraform` and
unresolved review operations.
