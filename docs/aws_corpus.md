# Ramen provider test corpus (Terraform fixtures + API sources → UWS)

## Context

A large, **self-growing** test corpus that takes Terraform configs from sibling
provider checkouts, pairs each with the matching API source document, and
converts it to a native UWS/Ramen project in **both YAML and HCL**. Purpose:
regression-gate `ramen convert tf`, provide clean reference fixtures, and
measure mapping coverage so it grows as `tfmapping` (and later AI-authoring)
expands.

Current provider inputs:

- AWS: `../terraform-provider-aws` + local AWS Smithy models under
  `../apitools/catalog-openapi-cache/aws-smithy`.
- Google: `../terraform-provider-google` + local Google Discovery documents
  under `../apitools/catalog-openapi-cache/google-discovery`.
- AzureRM: `../terraform-provider-azurerm` + local Azure OpenAPI documents
  under `../apitools/catalog-openapi-cache/openapi`.
- Cloudflare: `../terraform-provider-cloudflare` + a checked-in reduced
  Cloudflare OpenAPI fixture for the mapped R2/D1 operations under
  `testdata/api-sources/cloudflare-r2-d1-openapi.json`. The full cached
  Cloudflare OpenAPI document is currently just over the local 20 MiB source
  read limit, so the corpus uses the focused fixture instead of copying or
  requiring the full document.
- Kubernetes: `../terraform-provider-kubernetes` examples + the pinned
  Kubernetes Swagger artifact under
  `../apitools/catalog-openapi-cache/openapi/kubernetes-v1-19-2-swagger.json`.

The pipeline scans broadly but **emits narrowly**: only conversions that map
cleanly (no `mapping.unsupported_*` / fallback-only diagnostic) *and* round-trip
through HCL become corpus entries. Re-running the generator yields more entries
automatically as coverage grows — the generator, not the data, is the durable
asset.

Negative cases live separately under `testdata/diagnostic-corpus`. Those tests
assert expected diagnostics for unsupported, ambiguous, malformed, or incomplete
conversions without weakening the clean corpus contract.

### Findings that shaped this (verified)
- Provider: ~266 services, ~580 `aws_*` types. Configs are ~14.3K Go
  `testAcc*Config*` string-builders (need Go execution) + ~3K statically-readable
  files (2,492 `main_gen.tf`, 402 `.tf`, 592 `.gtpl`). Layout:
  `internal/service/<svc>/<resource>_test.go`; configs name `resource "aws_<svc>_<resource>"`.
- Smithy corpus: **29 real models** at
  `../apitools/catalog-openapi-cache/aws-smithy/aws-<svc>-smithy-model.json`.
  `apitools` reads supplied paths only (no fetch).
- `tfmapping` maps 10 managed AWS types today (s3_bucket,
  s3_bucket_accelerate_configuration, s3_bucket_public_access_block,
  s3_bucket_versioning, iam_role, iam_role_policy, iam_user, lambda_function,
  lambda_function_url, lambda_permission) plus
  `aws_caller_identity` as a data source. All mapped AWS types are in
  s3/iam/lambda, which have Smithy models.
- Google provider storage fixtures are mostly Go acceptance-test string
  literals, not AWS-style `testdata` directories. `cmd/corpusgen` mines those
  raw Terraform snippets, normalizes provider test placeholders to deterministic
  corpus values, and keeps only snippets that pass the same clean-conversion
  gate. Current Google mapping coverage is `google_storage_bucket` as both a
  resource and data source, backed by the local Cloud Storage Discovery document.
- AzureRM fixtures are also Go acceptance-test string literals. Current Azure
  mapping coverage is `azurerm_cosmosdb_account` as both a resource and data
  source, backed by the local Azure Cosmos DB Resource Manager OpenAPI document.
  AzureRM Cosmos snippets include provider test `azurerm_resource_group`
  scaffolding, but there is no local Resource Groups OpenAPI document, so the
  generator normalizes that scaffold into literal `resource_group_name` and
  `location` values before conversion.
- Kubernetes examples are static Terraform files under
  `../terraform-provider-kubernetes/examples`. Current Kubernetes mapping
  coverage is `kubernetes_namespace` and `kubernetes_namespace_v1` as resources
  and data sources, backed by the pinned Kubernetes Swagger artifact in
  `../apitools`.
- Cloudflare fixtures are static Terraform files under
  `../terraform-provider-cloudflare/internal/services/<service>/testdata`.
  Current Cloudflare mapping coverage is `cloudflare_r2_bucket` and
  `cloudflare_d1_database` as resources, backed by a reduced local OpenAPI
  fixture that preserves the Cloudflare R2/D1 operation IDs and request
  parameter names used by the mappings.

### Decisions
- **Source:** AWS static testdata and supported `.gtpl` templates first; Google
  and AzureRM raw Terraform snippets from Go acceptance tests where no static
  testdata is available; Kubernetes static provider examples; Cloudflare static
  service testdata.
- **API source scope:** only local API source documents; for AWS this means the
  existing 29 Smithy models, and for Google this currently means the local
  Discovery documents in `../apitools`; for AzureRM this currently means local
  OpenAPI documents in `../apitools`; for Kubernetes this currently uses the
  pinned `../apitools` Kubernetes Swagger artifact copied from the sibling
  provider fixture; for Cloudflare this currently uses a
  focused checked-in OpenAPI subset because the full cached Cloudflare OpenAPI
  document exceeds the default local source read limit.
- **Output:** clean conversions only (drop fallback/unsupported).

### Future: AWSCC / Cloud Control

`terraform-provider-awscc` is intentionally not folded into the current AWS
Smithy corpus. AWSCC resources are generated around AWS Cloud Control /
CloudFormation resource schemas (`AWS::...` resource types), not direct
per-service Smithy operations. If Ramen adds AWSCC coverage later, it should be
a separate UWS API source kind for CloudFormation or Cloud Control resource
schemas, preserving resource-type lifecycle semantics, identifiers, replacement
behavior, and stabilization metadata. A synthetic OpenAPI projection may be a
compatibility adapter, but it should not be the canonical source model.

---

## Implemented

### ramen
- **`convert tf` emits HCL** — `internal/tfconvert/tfconvert.go` now writes
  `project.uws.hcl` + `workflows/workflow.uws.hcl` via `uws/convert.MarshalHCL`
  (`writeDocumentFormats`); `Result` gained `NativeProjectHCLPath`/`UWSHCLPath`;
  `cmd/ramen` prints `native-hcl`/`uws-hcl`.
- **`tfmapping.Registry.SupportedTypes()`** — public enumerator backed by
  per-mapper lists (`awsMapper`/`googleMapper`), deduped and sorted. Generator
  uses it instead of duplicating the hardcoded set.
- **`cmd/corpusgen`** — derives the services to scan from `SupportedTypes()`;
  resolves AWS Smithy, Google Discovery, Azure OpenAPI, Cloudflare OpenAPI, and
  Kubernetes OpenAPI source documents; scans AWS static Terraform files and
  renderable `.gtpl` templates; mines Google and AzureRM raw Terraform snippets
  from Go acceptance tests; scans Cloudflare static service testdata and
  Kubernetes static examples; copies each provider config into the entry's
  `input/` and converts from there (so the recorded `config_dir` matches what
  the test reproduces); gates on clean diagnostics **and** HCL round-trip;
  preserves an existing semantically-equal `.hcl` and prunes stale entries
  (stable, idempotent regeneration); writes `manifest.json` + `COVERAGE.md`.
- **`corpus_test.go`** — re-converts each entry and asserts `project.uws.yaml`
  and `plan.json` match byte-for-byte (deterministic) and `project.uws.hcl` is
  *structurally* equal to the YAML (`HCLToJSON`/`YAMLToJSON` + `DeepEqual`, since
  HCL key order is not stable). CI regression gate.
- **`testdata/corpus/`** — generated entries + `manifest.json` + `COVERAGE.md`.

### Upstream bug fixes required to produce valid HCL
- **`../horizon` `dethcl/encoding.go`** — `loopHash` ignored the attribute-vs-block
  context, emitting nested maps as `operations = { create { … } }` (block syntax
  inside an object literal), which is invalid HCL. Now honors `equal` so nested
  maps use `key = { … }`. Only rewrites previously-*invalid* output;
  dethcl/uws/udon/ramen suites all still pass.
- **`../uws` `uws1/hcl.go` `escapeForHCL`** — did not escape HCL interpolation
  introducers, so values like `"${var.rName}"` were mis-parsed. Now escapes
  `${`→`$${` and `%{`→`%%{` (the parser reverses them; round trip is symmetric).
- **`../horizon` `dethcl/encoding.go`** — `encodePrimitiveOrRecurse` wrapped
  strings with `fmt.Sprintf("\"%s\"", item)` and **no escaping**, so embedded
  JSON (e.g. `assume_role_policy` with `\"`/newlines) produced a bare `"` that
  terminated the HCL string. Added `hclEscapeString` (escapes `\`, `"`, and
  control chars; deliberately leaves `$`/`%` to the caller so uws's interpolation
  escaping is not double-applied). This recovered the previously-dropped entries,
  including all of lambda.

### Entry layout
```
testdata/corpus/aws/<service>/<Resource>/<variant>/
  input/*.tf          copied provider config (convert reads this dir)
  project.uws.yaml    converted native project (deterministic golden)
  project.uws.hcl     HCL serialization (structurally equal to the YAML)
  plan.json           conversion plan artifact (deterministic golden)
  diagnostics.json    convert diagnostics (clean)
  meta.json           service, resource_types, data_sources, smithy model refs, source_dir

testdata/corpus/google/<service>/go/<provider_test_file>_<n>/
  input/main.tf       normalized Terraform snippet mined from provider Go tests
  project.uws.yaml    converted native project (deterministic golden)
  project.uws.hcl     HCL serialization (structurally equal to the YAML)
  plan.json           conversion plan artifact (deterministic golden)
  diagnostics.json    convert diagnostics (clean)
  meta.json           provider, service, types, api_sources, source_dir

testdata/corpus/azurerm/<service>/go/<provider_test_file>_<n>/
  input/main.tf       normalized Terraform snippet mined from provider Go tests
  project.uws.yaml    converted native project (deterministic golden)
  project.uws.hcl     HCL serialization (structurally equal to the YAML)
  plan.json           conversion plan artifact (deterministic golden)
  diagnostics.json    convert diagnostics (clean)
  meta.json           provider, service, types, api_sources, source_dir

testdata/corpus/kubernetes/<service>/<provider_example_path>/
  input/main.tf       copied Terraform example
  project.uws.yaml    converted native project (deterministic golden)
  project.uws.hcl     HCL serialization (structurally equal to the YAML)
  plan.json           conversion plan artifact (deterministic golden)
  diagnostics.json    convert diagnostics (clean)
  meta.json           provider, service, types, api_sources, source_dir

testdata/corpus/cloudflare/<service>/<provider_testdata_file>/
  input/main.tf       normalized Terraform fixture copied from provider testdata
  project.uws.yaml    converted native project (deterministic golden)
  project.uws.hcl     HCL serialization (structurally equal to the YAML)
  plan.json           conversion plan artifact (deterministic golden)
  diagnostics.json    convert diagnostics (clean)
  meta.json           provider, service, types, api_sources, source_dir
testdata/corpus/manifest.json + COVERAGE.md
```
Large API source documents are **referenced by relative path** in `meta.json`
(not copied) to avoid multi-MB repo bloat.

---

## Results

```
corpusgen: emitted=157 considered=822 services=8
           dropped(unsupported=555 no-resource=58 no-model=0 diagnostics=52 hcl=0 template=0)
```
- **157 clean entries**: AWS iam = 23, AWS lambda = 18, AWS s3 = 28,
  Google storage = 45, AzureRM cosmos = 31, Cloudflare R2 bucket = 8,
  Cloudflare D1 database = 1, Kubernetes core = 3.
- 555 dropped: config used a resource type ramen does not map yet.
- 52 dropped: fallback/unsupported/error diagnostics during conversion.
- 0 dropped on HCL round-trip (was 10 before the `dethcl` string-escaping fix).
- 0 dropped on template rendering.
- Regeneration is idempotent (committed `.hcl` is byte-stable across runs).

### Why only 157 (the funnel)

157 is **not** the number of Terraform configs in the providers — it is what
survives a deliberately narrow funnel. The AWS provider has ~266 services, ~580
resource types, and thousands of test configs (~14.3K Go `testAcc*Config*`
builders + ~3K static files). The Google provider adds another fixture style:
Terraform snippets embedded in Go acceptance tests. AzureRM follows that same
Go-snippet style for the first Cosmos DB corpus. Kubernetes currently uses
static provider examples, and Cloudflare uses static service testdata. The
corpus is the intersection of intentional filters:

```
~266 services, thousands of configs (whole provider)
  └─ services with a mapped type AND a local Smithy model → 3 (iam, s3, lambda)
       └─ static testdata and supported .gtpl templates   → 308 inputs considered
            ├─ 214 dropped: uses a resource type ramen does not map yet
            │            (e.g. aws_iam_server_certificate, aws_s3_object,
            │             aws_s3_bucket_lifecycle_configuration)
            ├─  18 dropped: fallback/unsupported/error diagnostics
            ├─   0 dropped: HCL round-trip (fixed)
            ├─   0 dropped: template render failed
            └─  69 emitted: iam 23, lambda 18, s3 28
```

Google:

```
mapped resource services with local Discovery docs → 1 (storage)
  └─ Go test Terraform snippets considered       → 148 inputs
       ├─ 91 dropped: uses a Terraform type ramen does not map yet
       ├─  3 dropped: no managed resource in the snippet
       ├─  9 dropped: fallback/unsupported/error diagnostics
       └─ 45 emitted: storage bucket resource/data-source coverage
```

AzureRM:

```
mapped resource services with local OpenAPI docs → 1 (cosmos)
  └─ Go test Terraform snippets considered     → 192 inputs
       ├─ 132 dropped: uses a Terraform type ramen does not map yet
       ├─   4 dropped: no managed resource in the snippet
       ├─  25 dropped: fallback/unsupported/error diagnostics
       └─  31 emitted: Cosmos DB account resource/data-source coverage
```

Kubernetes:

```
mapped resource services with local OpenAPI docs → 1 (core)
  └─ static examples considered                → 165 inputs
       ├─ 118 dropped: uses a Terraform type ramen does not map yet
       ├─  44 dropped: no managed resource in the snippet
       └─   3 emitted: namespace resource coverage
```

Cloudflare:

```
mapped resource services with local OpenAPI docs → 2 (r2_bucket, d1_database)
  └─ static service testdata considered        → 9 inputs
       └─ 9 emitted: R2 bucket and D1 database resource coverage
```

So `157 = clean-only × mapped provider services × locally available API source
documents × supported fixture extraction`. Each narrowing is a chosen scope
(clean conversions only; local API source documents; supported fixture sources
first), **not** a ceiling. The set grows
automatically — no hand-editing — along three axes:

1. **Map more resource types** in `tfmapping` (or via openudon AI-authoring) —
   the biggest lever; only 10 managed AWS types map today, which is why only 3
   services are scanned.
2. **Source breadth** — add more template helper stubs and later execute or
   extract more provider Go config-builders, growing "considered" from hundreds
   toward thousands.
3. **More local API source documents** beyond the current AWS Smithy,
   Google Discovery, Azure OpenAPI, Kubernetes Swagger, and focused Cloudflare
   OpenAPI set make more services eligible.

With template rendering, `aws_s3_bucket_public_access_block`,
`aws_s3_bucket_versioning`, `aws_iam_user`, and `aws_lambda_permission` now
added, the current AWS clean corpus is 69 entries; Google storage adds 45 and
AzureRM Cosmos adds 31, Cloudflare R2/D1 adds 9, and Kubernetes core namespaces
add 3 for 157 total. The same funnel still applies: only diagnostic-clean
outputs from mapped types in
services with local API source documents are emitted.

---

## Methods to get more tests with existing Smithy

The current local Smithy-backed services are `iam`, `lambda`, and `s3`. More
tests can be added without new Smithy models by improving mappings, request
bindings, and fixture intake inside those services.

1. **Map more Terraform resource types for already-covered Smithy services.**
   This is the highest-yield path because existing
   `../terraform-provider-aws` fixtures become clean corpus entries once their
   Terraform types are mapped. Completed examples include
   `aws_s3_bucket_public_access_block`, `aws_s3_bucket_versioning`,
   `aws_iam_user`, and `aws_lambda_permission`. Good next candidates include
   `aws_s3_bucket_server_side_encryption_configuration`,
   `aws_s3_bucket_cors_configuration`,
   `aws_s3_bucket_website_configuration`,
   `aws_s3_bucket_ownership_controls`, `aws_iam_policy`, and
   `aws_iam_instance_profile`.

2. **Improve request bindings for nested Smithy request structures.** Some
   candidate resources already have obvious operations, but their Terraform
   attributes need accurate nested/list/block body bindings before the output is
   useful. Examples include S3 encryption rules, lifecycle rules, CORS rules,
   website routing blocks, and ownership controls.

3. **Mine more sibling provider fixtures.** `cmd/corpusgen` scans
   `../terraform-provider-aws`; as mappings improve, its existing static and
   template fixtures automatically produce more tests. The preferred workflow is
   to add a mapping, run `go run ./cmd/corpusgen`, and keep only the new clean
   entries.

4. **Render more provider templates.** The generator can render `.gtpl`
   fixtures, but additional provider helper stubs may unlock more template
   cases. This can grow coverage without hand-writing new Terraform fixtures.

5. **Add focused synthetic fixtures under `testdata`.** For mappings that are
   hard to extract from provider fixtures, add small, explicit Terraform inputs
   that isolate one resource and one behavior. These are useful for proving a
   request binding before wider provider-fixture intake.

6. **Add a negative/diagnostic corpus.** Unsupported, ambiguous, malformed, or
   incomplete mappings should be tested separately from the clean corpus. These
   tests should assert the expected diagnostic instead of allowing misleading
   generated UWS/Ramen artifacts.

7. **Add mapper-only unit tests.** Before adding full corpus entries, unit-test
   supported type enumeration, lifecycle operation IDs, identity attributes, and
   Terraform attribute to Smithy request-key hints. This catches mapping mistakes
   before corpus generation has to diagnose them indirectly.

---

## Gaps & next steps

1. **Coverage is mapping-bound.** Only 10 managed AWS types map today, so the
   clean set is small by design. It grows automatically as `tfmapping` (or
   openudon AI-authoring) maps more types — just re-run `corpusgen`.
   `COVERAGE.md` tracks progress.
2. **Source breadth.** Static `main_gen.tf`/`*.tf` and supported `.gtpl`
   templates are scanned. Pending: execute or otherwise extract the ~14.3K
   `testAcc*Config*` Go builders.
3. **Smithy scope.** Limited to the 29 local models; other services are skipped.
4. **Cross-repo release.** The `uws` and `dethcl` fixes are consumed via the
   parent `go.work`; they must be released for standalone (`GOWORK=off`) builds.
5. **`convert` records dependencies on skipped provider-local data sources**
   (e.g. `data.aws_partition`), so generated projects don't pass
   `ramen validate` (dangling-dependency errors). The corpus test therefore does
   not gate on `validate`; worth fixing in `convert` separately.

---

## Regenerate & verify
```
go run ./cmd/corpusgen          # rebuild the corpus + COVERAGE.md
go test .                       # corpus regression (byte YAML/plan, structural HCL)
go build ./... && go vet ./... && go test ./...   # full ramen suite
```
