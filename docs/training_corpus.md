# Ramen training corpus

T01 generates a provenance-aware NL-to-Ramen training/evaluation corpus under
`testdata/training`.

## Files

- `testdata/training/manifest.json`: machine-readable
  `ramen.training.v1` manifest.
- `testdata/training/COVERAGE.md`: generated counts by tier, provider, and
  service.

The training corpus links to existing workflow fixtures. It does not copy
native Ramen/UWS projects into `testdata/training`.

## Tiers

Gold rows are parity-reviewed workflow rows from `docs/parity_nl.md`. T01
includes runnable workflow rows only, including GitHub H01-H03 and excluding
Z06/H04 because they are not standalone training workflows. Their
`natural_language.goal_source` value is `curated`.

Silver rows are regenerated clean conversion-corpus entries from
`testdata/corpus/manifest.json`. They pass `ramen convert` cleanly and strict
native validation with local API source documents. They are useful NL-to-Ramen
examples but are not parity-backed and should not be treated as universal
correctness evidence. Their natural-language goals are deterministic generated
templates, carry `natural_language.goal_source` `generated`, and may duplicate
or nearly duplicate across provider-version, example, or corpus variants.

## Provenance

Each row records:

- natural-language goal text;
- natural-language goal source: `curated` for parity-reviewed gold rows and
  `generated` for conversion-corpus silver templates;
- linked workflow paths and primary workflow path;
- optional source HCL path;
- provider, service, resource/data-source types, and API source paths;
- source repository/path/doc provenance;
- conversion status: `parity-reviewed` for gold, `clean` for silver;
- strict validation status and summary.

`confidence` is a coarse, uncalibrated tier prior rather than a statistical
probability: gold parity-reviewed rows use `1.0`, and clean-converted silver
rows use `0.72`.

Silver rows also record an equivalent `go run ./cmd/ramen convert ...`
command for reproducing the conversion shape.

## Exclusions

Ansible conversion fixtures under `testdata/ansible-conversion` are not T01
rows. They are static UWS workflow conversion evidence from
`ramen convert ansible`, not NL-to-Ramen desired-state project data. A future
Txx milestone may define a separate workflow-only training/evaluation shape,
but T01 keeps those fixtures out of `testdata/training/manifest.json`.

## Regeneration

```bash
go run ./cmd/corpusgen
go run ./cmd/corpusgen --check
go run ./cmd/traininggen
go run ./cmd/traininggen --check
go test . -run TestTraining -count=1
```

Default generation and tests use local fixtures only. They do not run live
provider calls, OpenTofu/Terraform provider execution, apply, refresh, or
mutation commands.
