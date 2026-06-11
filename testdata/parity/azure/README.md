# Azure Provider Parity Fixtures

This tree is reserved for the Zxx Azure provider/runtime parity lane.

Z-lane parity compares Azure API-visible observations from:

- OpenTofu with `hashicorp/azurerm` using the lane-pinned fixture
  constraint;
- Terraform with the same `hashicorp/azurerm` fixture constraint;
- Ramen with Azure ARM OpenAPI source metadata and the optional `udon`
  executor.

Default tests validate committed static metadata, HCL parsing, native project
validation, operation IDs, request bindings, observation artifacts, and
sanitized recorded replay artifacts. Z01, Z02, Z04, and Z05 have committed
sanitized live recordings. Z03 remains a read/static fixture lane, and Z06
records the static readiness/closure of the Z02 settle-path re-recording.
Live Azure parity recording is opt-in through `RAMEN_AZURE_PARITY=1` and requires an explicit
`RAMEN_AZURE_PARITY_LANE=<lane>` selection. Recording updates are guarded by
`RAMEN_AZURE_PARITY_RECORD_UPDATE=1`.

Live runs must not commit state databases, plan artifacts, tokens,
subscription IDs, tenant IDs, client IDs, raw response bodies, or generated
executor output. Use disposable names with the lane prefix and verify
post-delete absence before preserving any sanitized observation summary.
Future mutation runs must use the minimum number of smallest practical
resources, avoid large or high-cost Azure resources, and destroy or clean up
every resource they create before any recording update is accepted.

See `LIVE.md` for the current live guardrail contract. Z01, Z02, Z04, and Z05
are live-enabled by metadata and have committed sanitized recordings.
Additional recording updates still require an explicit operator-scoped run.
Z07 records broader Azure follow-up work as parked until a specific low-risk
candidate, focused ARM fixture, cleanup policy, and default credential-free
checks are selected.
Z08 is selected as the next static-first follow-up to close Resource Group
read/import evidence from Z03 before any new Azure mutation lane.

Live mutation tests are excluded from regular test suites. They require the
`azurelive` build tag and a specific `go test -run` selection.
