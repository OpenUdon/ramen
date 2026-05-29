# Ramen test corpus from terraform-provider-aws (TF + Smithy → UWS)

## Context

A large, **self-growing** test corpus that takes Terraform configs from the AWS
provider (`../terraform-provider-aws`), pairs each with the matching AWS Smithy
model, and converts it to a native UWS/Ramen project in **both YAML and HCL**.
Purpose: regression-gate `ramen convert tf`, provide clean reference fixtures,
and measure mapping coverage so it grows as `tfmapping` (and later AI-authoring)
expands.

The pipeline scans broadly but **emits narrowly**: only conversions that map
cleanly (no `mapping.unsupported_*` / fallback-only diagnostic) *and* round-trip
through HCL become corpus entries. Re-running the generator yields more entries
automatically as coverage grows — the generator, not the data, is the durable
asset.

### Findings that shaped this (verified)
- Provider: ~266 services, ~580 `aws_*` types. Configs are ~14.3K Go
  `testAcc*Config*` string-builders (need Go execution) + ~3K statically-readable
  files (2,492 `main_gen.tf`, 402 `.tf`, 592 `.gtpl`). Layout:
  `internal/service/<svc>/<resource>_test.go`; configs name `resource "aws_<svc>_<resource>"`.
- Smithy corpus: **29 real models** at
  `../apitools/catalog-openapi-cache/aws-smithy/aws-<svc>-smithy-model.json`.
  `apitools` reads supplied paths only (no fetch).
- `tfmapping` maps 8 managed AWS types today (s3_bucket,
  s3_bucket_accelerate_configuration, s3_bucket_public_access_block,
  s3_bucket_versioning, iam_role, iam_role_policy, lambda_function,
  lambda_function_url) plus
  `aws_caller_identity` as a data source. All mapped AWS types are in
  s3/iam/lambda, which have Smithy models.

### Decisions
- **Source:** static testdata and supported `.gtpl` templates first; Go-builders later.
- **Smithy scope:** only the existing 29 models.
- **Output:** clean conversions only (drop fallback/unsupported).

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
  resolves `aws-<svc>-smithy-model.json` (with an alias map for naming quirks);
  scans static Terraform files and renderable `.gtpl` templates; copies each
  provider config into the entry's `input/` and converts from there (so the
  recorded `config_dir` matches what the test reproduces); gates on clean
  diagnostics **and** HCL round-trip; preserves an existing semantically-equal
  `.hcl` and prunes stale entries (stable, idempotent regeneration); writes
  `manifest.json` + `COVERAGE.md`.
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
testdata/corpus/manifest.json + COVERAGE.md
```
Large Smithy models are **referenced by relative path** in `meta.json` (not
copied) to avoid multi-MB repo bloat.

---

## Results

```
corpusgen: emitted=53 considered=308 services=3
           dropped(unsupported=235 no-resource=7 no-model=0 diagnostics=13 hcl=0 template=0)
```
- **53 clean entries**: iam = 14, lambda = 11, s3 = 28.
- 235 dropped: config used a resource type ramen does not map yet.
- 13 dropped: fallback/unsupported/error diagnostics during conversion.
- 0 dropped on HCL round-trip (was 10 before the `dethcl` string-escaping fix).
- 0 dropped on template rendering.
- All suites green: `ramen` build/vet/test, `uws`, `udon`, `dethcl`.
- Regeneration is idempotent (committed `.hcl` is byte-stable across runs).

### Why only 53 (the funnel)

53 is **not** the number of Terraform configs in the provider — it is what
survives a deliberately narrow funnel. The provider has ~266 services, ~580
resource types, and thousands of test configs (~14.3K Go `testAcc*Config*`
builders + ~3K static files). The corpus is the intersection of three
intentional filters:

```
~266 services, thousands of configs (whole provider)
  └─ services with a mapped type AND a local Smithy model → 3 (iam, s3, lambda)
       └─ static testdata and supported .gtpl templates   → 308 inputs considered
            ├─ 235 dropped: uses a resource type ramen does not map yet
            │            (e.g. aws_iam_server_certificate, aws_s3_object,
            │             aws_s3_bucket_lifecycle_configuration)
            ├─  13 dropped: fallback/unsupported/error diagnostics
            ├─   0 dropped: HCL round-trip (fixed)
            ├─   0 dropped: template render failed
            └─  53 emitted: iam 14, lambda 11, s3 28
```

So `53 = clean-only × 3 mapped services × static/template-source-only`. Each
narrowing is a chosen scope (clean conversions only; the 29 local Smithy models;
static and supported template testdata first), **not** a ceiling. The set grows
automatically — no hand-editing — along three axes:

1. **Map more resource types** in `tfmapping` (or via openudon AI-authoring) —
   the biggest lever; only 8 managed AWS types map today, which is why only 3
   services are scanned.
2. **Source breadth** — add more template helper stubs and later execute or
   extract the ~14.3K Go config-builders, growing "considered" from 308 toward
   thousands.
3. **More Smithy models** beyond the 29 makes more services eligible.

With template rendering, `aws_s3_bucket_public_access_block`, and
`aws_s3_bucket_versioning` now added, the current clean corpus is 53 entries.
The same funnel still applies: only diagnostic-clean outputs from mapped types
in services with local Smithy models are emitted.

---

## Methods to get more tests with existing Smithy

The current local Smithy-backed services are `iam`, `lambda`, and `s3`. More
tests can be added without new Smithy models by improving mappings, request
bindings, and fixture intake inside those services.

1. **Map more Terraform resource types for already-covered Smithy services.**
   This is the highest-yield path because existing
   `../terraform-provider-aws` fixtures become clean corpus entries once their
   Terraform types are mapped. Completed examples include
   `aws_s3_bucket_public_access_block` and `aws_s3_bucket_versioning`. Good
   next candidates include `aws_s3_bucket_server_side_encryption_configuration`,
   `aws_s3_bucket_cors_configuration`,
   `aws_s3_bucket_website_configuration`,
   `aws_s3_bucket_ownership_controls`, `aws_lambda_permission`,
   `aws_iam_policy`, `aws_iam_user`, and `aws_iam_instance_profile`.

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

1. **Coverage is mapping-bound.** Only 8 managed AWS types map today, so the
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
