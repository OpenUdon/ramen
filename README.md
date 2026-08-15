# Ramen

Ramen is the stateful reconciliation engine that turns API-source-backed
desired resources into deterministic, reviewable UWS plans. It records local
SQLite state and history, then hands only approved action documents to an
explicit trusted executor boundary.

The native source of truth is a UWS project plus Ramen metadata for identity,
lifecycle, operation roles, hashes, state matching, and redaction. OpenAPI,
AWS Smithy JSON, and Google Discovery describe the available operations.
Terraform/OpenTofu and Ansible conversion plus AI-assisted authoring are
optional on-ramps into that native model, not alternative runtimes.

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
- Generated SDKs and CLIs such as `az` and `gcloud` provide imperative API
  calls; Ramen provides operator workflows and durable desired-state records.

## Adoption Readiness

Community users should be able to evaluate Ramen locally through native project
examples, mock execution, readable plans, stable diagnostics, clear non-goals,
and `ramen convert` for HCL migration. Enterprise users should look for
approval artifacts, redacted SQLite state history, reproducible plans, explicit
trusted executor boundaries, no credential value storage, and future policy
integration points.

Ready in v0.1: provider-free native validation, graphing and planning; local
state history; digest-bound approval artifacts; mock-backed apply and refresh;
read-only state inspection; a supported in-process executor contract; and a
credential-free native example.

Experimental before v1: broader resource mappings, live executor adapters,
policy integrations, authoring and conversion adapters, imperative runbooks,
and parameterization ergonomics. See [SUPPORT.md](SUPPORT.md) and
[the compatibility contract](docs/compatibility.md) for the exact boundary.

See [docs/evidence-index.md](docs/evidence-index.md) for the current
credential-free, recorded replay, and sanitized live evidence that backs these
claims.

## Installation

Install the CLI with Go:

```bash
go install github.com/OpenUdon/ramen/cmd/ramen@v0.1.0
ramen version --json
```

Linux, macOS, and Windows archives for amd64 and arm64 are attached to the
GitHub v0.1.0 release with a `SHA256SUMS` file. Ramen requires the Go version
declared in `go.mod` when used as a library.

## Native Quick Start

The local widget example uses only a checked-in OpenAPI document and the mock
executor. It performs no network calls and needs no credentials:

```bash
ramen init \
  --project ./examples/widget \
  --state /tmp/ramen-widget-state.db
ramen validate --project ./examples/widget
ramen graph --project ./examples/widget
ramen plan \
  --project ./examples/widget \
  --state /tmp/ramen-widget-state.db \
  --out /tmp/ramen-widget-plan.json
ramen apply \
  --plan /tmp/ramen-widget-plan.json \
  --state /tmp/ramen-widget-state.db \
  --auto-approve \
  --mock \
  --out /tmp/ramen-widget-apply
ramen state list --state /tmp/ramen-widget-state.db
```

See [examples/widget](examples/widget) for the project, API source, and
annotated workflow.

## Commands

The native desired-state lifecycle is:

```bash
ramen init
ramen validate --project DIR --json
ramen graph --project DIR --format json
ramen plan --project DIR --target ADDRESS --exclude ADDRESS --replace ADDRESS
ramen plan --project DIR --out plan.json
ramen apply --plan plan.json --auto-approve --mock
ramen refresh --mock
ramen import
ramen show plan.json
ramen state list
ramen force-unlock LOCK_HOLDER --state PATH
ramen version --json
```

Experimental on-ramps create or convert native artifacts:

```bash
ramen author --context context.json --goal "Manage widgets"
ramen icot --goal "List resources" --openapi api=api.json --network ask --no-llm --validate --graph
ramen convert
ramen convert ansible --playbook playbook.yml --argspec-dir argspecs
```

See the shared [static conversion contract](docs/conversion.md), the
[Terraform/OpenTofu conversion contract](docs/terraform-conversion.md), and the
[Ansible conversion contract](docs/ansible-conversion.md) for their modes,
reports, static inputs, Ramen-owned metadata, validation, artifacts, and
compatibility boundaries.

`ramen run` is an adjacent imperative UWS runbook command; it does not create
desired-state resources. Default release builds include mock execution only.
Platform teams integrate a trusted runtime through the supported
[`executor.Executor`](https://pkg.go.dev/github.com/OpenUdon/ramen/executor)
interface. `ramen version --json` reports local build metadata without network
checks or telemetry.

`ramen icot` uses a dependency-aware v2 interview. It inspects repeatable
`--api-source KIND:ID=PATH`, `--openapi ID=PATH`, and `--source-root PATH`
inputs before asking questions; supports OpenAPI/Swagger, Google Discovery,
AWS Smithy, AsyncAPI, GraphQL, OpenRPC, gRPC/protobuf, and OData; and shows all
currently ready decisions as one round. `--prompt-mode full|normal|fast`
controls safe defaults, while forced or uncertain decisions are always shown.
Broad goals must select one active workflow; later workflows remain
non-executable candidates in the generated project.

Remote source lookup is separate and bounded by `--network never|ask|allow`.
Interactive mode defaults to `ask`; agent mode is effectively `never` unless
`allow` is explicit. No project or selected source is written until the full
proposal is approved. Structured technical deferrals can save a non-runnable
draft and resumable v2 session; default interview state stays under
`OUT/.icot/`, and `--resume SESSION` reopens deferred leaves for promotion.
`--agent` and `--print` never write deliverables.

### Azure API-First Example

Start from a local Azure Resource Manager OpenAPI file and draft a read-only
project:

```bash
go run ./cmd/ramen icot \
  --goal "List Azure resources in the selected subscription" \
  --api-source openapi:azure-resources=../<azure-resources>/resources.json \
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
  --var azure_subscription_id="<subscription-id>" \
  --out ./.ramen/azure-read/read-plan.json
```

Mock execution path:

```bash
go run ./cmd/ramen apply \
  --plan ./.ramen/azure-read/read-plan.json \
  --var azure_subscription_id="<subscription-id>" \
  --auto-approve \
  --mock \
  --out ./.ramen/azure-read/mock-apply
```

Live Azure reads require a short-lived access token supplied through the
operator environment:

```bash
UDON_CREDENTIAL_AZURE_AUTH="$(az account get-access-token \
  --resource https://management.azure.com/ \
  --query accessToken \
  -o tsv)" \
go run -tags udon ./cmd/ramen apply \
  --plan ./.ramen/azure-read/read-plan.json \
  --var azure_subscription_id="<subscription-id>" \
  --auto-approve \
  --executor udon \
  --udon-output ./.ramen/azure-read/udon \
  --out ./.ramen/azure-read/apply
```

Example placeholders only: `<subscription-id>`, `<resource-group>`, `<server>`, `<database>`.

Do not copy real IDs/tokens into tracked artifacts.

Do not commit `.ramen/` state, live response payloads, subscription IDs,
tenant IDs, client IDs, secrets, or access tokens. Mutating Azure examples
should use disposable resources, explicit scoped permissions, tags, cost
guardrails, and a verified cleanup command.

## Development Checks

```bash
GOWORK=off go mod download
GOWORK=off go test ./... -count=1 -timeout=10m
GOWORK=off go vet ./...
git diff --check
```

The canonical memory bank is tracked under `../tofu/ramen` in a sibling
development checkout, but public builds and tests do not require that checkout.

Optional udon adapter check when the private sibling checkout is available:

```bash
go test -tags udon ./...
```
