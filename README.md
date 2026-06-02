# Ramen

Ramen is a public API-source desired-state engine. Its native project
format is a UWS project plus Ramen reconciliation metadata for resource
identity, lifecycle, operation roles, hashes, state matching, and redaction
policy. Terraform/OpenTofu HCL remains an optional authoring and migration
input through `ramen convert`, which uses `github.com/OpenUdon/tfconfig` to
produce native Ramen/UWS project artifacts.

Ramen maps desired API resources to operations from sources such as AWS Smithy
JSON, Google Discovery, and OpenAPI, computes desired-state plans, records
SQLite state history, and hands approved UWS execution documents to an
explicit trusted executor boundary.

Ramen uses shared `github.com/OpenUdon/evidence/...` primitives for neutral
evidence where the behavior is product-independent. Current shared use covers
SHA-256 digest helpers, redaction pattern handling behind Ramen's stricter
secret-keyword policy, and approval requirement evaluation behind Ramen-owned
governance wire formats. Ramen-specific behavior, including
`ramen.approval.v1`, `ramen.policy.v1`, state history, reconciliation, and
executor orchestration, remains Ramen-owned.

## Why Ramen

Ramen is for teams that have API source documents and need desired-state
workflows without centering the system on provider plugins, backend
compatibility, module download, or Terraform/OpenTofu plan files. Typical
adopters include API platform teams, internal developer platform teams,
operators managing APIs without mature providers, teams migrating HCL-shaped
declarations, and UWS/OpenUdon users who need stateful reconciliation around
workflow documents.

Ramen differs from adjacent tools in a narrow way:

- Terraform/OpenTofu are provider-backed infrastructure runtimes; Ramen is an
  API-source reconciliation engine with HCL conversion as an adapter.
- Generic workflow runners execute steps; Ramen adds identity matching,
  dependency graphs, deterministic plans, import, refresh evidence, state
  history, and approval-artifact checks.
- Generated SDKs provide imperative API calls; Ramen provides operator
  workflows and durable desired-state records.
- OpenUdon owns workflow authoring and review packaging; Ramen owns
  desired-state graphing, state, diff, and reconciliation.

## Adoption Readiness

Community users should be able to evaluate Ramen locally through native project
examples, mock execution, readable plans, stable diagnostics, clear non-goals,
and `ramen convert` for HCL migration. Enterprise users should look for
approval artifacts, redacted SQLite state history, reproducible plans, explicit
trusted executor boundaries, no credential value storage, and future policy
integration points.

Ready now: provider-free native validation, graphing, planning, HCL conversion
scaffolding, local state history, mock-backed apply/refresh flows,
plan approval metadata, read-only show/state inspection, and documented safety
boundaries. The first `ramen author` wrapper can also draft a native project
from prompt-safe API operation context without provider execution. `ramen icot`
adds the interactive local-metadata path for drafting API-method projects,
including read/list, DELETE, POST, PUT, and PATCH actions, from local API
source documents. Still
experimental: broader resource mappings, live executor
adapters, policy hooks, parameterization ergonomics, release packaging, and
operational support contracts.


## Scope

Ramen owns desired-state reconciliation:

- native UWS/Ramen project loading and validation;
- Ramen reconciliation metadata over UWS;
- API source operation inventory consumption through public `apitools`;
- Terraform/OpenTofu HCL conversion through public `tfconfig` via
  `ramen convert`;
- resource-to-operation mapping, including public `tfmapping` for conversion
  compatibility;
- dependency graph construction;
- deterministic plan and diff output;
- SQLite state, locks, and revision history;
- refresh, apply, and import orchestration;
- public executor interfaces and optional trusted executor adapters.

Shared evidence helpers live in `github.com/OpenUdon/evidence/...` only for
neutral digest, artifact, diagnostic, redaction, and approval primitives. Ramen
uses them behind Ramen-owned package and wire boundaries; they do not own Ramen
plan, policy, state, reconciliation, or executor semantics.

Ramen does not import Terraform code, OpenTofu internals, Terraform providers,
provider plugins, provider SDKs, or private udon packages in default public
builds. The optional udon adapter is planned behind an explicit `udon` build
tag.

## Commands

Implemented public commands:

```bash
ramen author --context context.json --goal "Manage widgets"
ramen icot --goal "List all Azure resources" --api-source openapi:azure=azure.json --no-llm --validate --graph
ramen convert
ramen init
ramen validate --project DIR --json
ramen graph --project DIR --format json
ramen force-unlock LOCK_HOLDER --state PATH
ramen plan --project DIR --target ADDRESS --exclude ADDRESS --replace ADDRESS
ramen plan --project DIR --out plan.json
ramen apply --plan plan.json --auto-approve --mock
ramen refresh --mock
ramen import
ramen version --json
```

The current implementation supports native UWS/Ramen project input through
`--project`, while preserving the HCL-derived lifecycle path from the first
milestones as transitional compatibility. `ramen convert` writes
`project.uws.yaml` in that native format. The first lifecycle surface is
mock-backed in default public builds where execution is required. Live executor
wiring remains opt-in behind trusted adapters. `ramen version --json` reports
local build metadata without network checks.

### Azure API-First Example

Start from a local Azure Resource Manager OpenAPI file and draft a read-only
project:

```bash
go run ./cmd/ramen icot \
  --goal "List Azure resources in the selected subscription" \
  --api-source openapi:azure-resources=../azure-rest-api-specs/specification/resources/resource-manager/Microsoft.Resources/resources/stable/2025-04-01/resources.json \
  --out ./.ramen/azure-read \
  --no-transcript \
  --validate \
  --graph
```

Choose `Resources_List` if prompted for an operation ID. Then plan the read
without embedding credentials in the project:

```bash
go run ./cmd/ramen plan \
  --project ./.ramen/azure-read \
  --action read \
  --var azure_subscription_id="$AZURE_SUBSCRIPTION_ID" \
  --out ./.ramen/azure-read/read-plan.json
```

Default public execution can stay mock-backed:

```bash
go run ./cmd/ramen apply \
  --plan ./.ramen/azure-read/read-plan.json \
  --var azure_subscription_id="$AZURE_SUBSCRIPTION_ID" \
  --auto-approve \
  --mock \
  --out ./.ramen/azure-read/mock-apply
```

Live Azure reads require an explicit trusted executor and a short-lived access
token supplied through the operator environment:

```bash
UDON_CREDENTIAL_AZURE_AUTH="$(az account get-access-token \
  --resource https://management.azure.com/ \
  --query accessToken \
  -o tsv)" \
go run -tags udon ./cmd/ramen apply \
  --plan ./.ramen/azure-read/read-plan.json \
  --var azure_subscription_id="$AZURE_SUBSCRIPTION_ID" \
  --auto-approve \
  --executor udon \
  --udon-output ./.ramen/azure-read/udon \
  --out ./.ramen/azure-read/apply
```

Do not commit `.ramen/` state, live response payloads, subscription IDs,
tenant IDs, client IDs, secrets, or access tokens. Mutating Azure examples
should use disposable resources, explicit scoped permissions, tags, cost
guardrails, and a verified cleanup command.

## Development Checks

Harness/documentation check:

```bash
git -C ../tofu diff --check -- ramen
```

Planned public module checks once Go code exists:

```bash
go test ./...
go vet ./...
git diff --check
```

Optional udon adapter check when the private sibling checkout is available:

```bash
go test -tags udon ./...
```
