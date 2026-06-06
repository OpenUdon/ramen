# Azure Parity Live Guardrails

Default Azure parity tests are credential-free. Live runs are separate and
must be explicitly scoped by the operator.

## Shared Gates

Live Azure parity requires:

- explicit live test selection:
  `go test -tags 'azurelive udon' . -run '^TestAzureProviderParityLive$'`
- `RAMEN_AZURE_PARITY=1`
- `RAMEN_AZURE_PARITY_LANE=<lane>`
- optional `RAMEN_AZURE_PARITY_BASELINE=both` only when a legacy
  OpenTofu+Terraform+Ramen comparison is explicitly needed; the default live
  baseline is OpenTofu plus Ramen
- `RAMEN_AZURE_PARITY_RECORD_UPDATE=1` only after reviewing sanitized output
- `UDON_CREDENTIAL_AZURE_AUTH` minted at runtime from `az account get-access-token`;
  the Ramen+udon live harness refreshes the Azure bearer token before udon
  operations so long-running Cosmos waits do not outlive a static token
- Terraform/OpenTofu AzureRM credentials mapped from:
  - `AZURE_SUBSCRIPTION_ID` to `ARM_SUBSCRIPTION_ID`
  - `AZURE_TENANT_ID` to `ARM_TENANT_ID`
  - `AZURE_CLIENT_ID` to `ARM_CLIENT_ID`
  - `AZURE_CLIENT_SECRET` to `ARM_CLIENT_SECRET`

Never commit `.ramen/`, Terraform state, plan files, raw response payloads,
subscription IDs, tenant IDs, client IDs, secrets, or tokens.

Future live recordings include elapsed `duration_ms` fields. Replay comparison
normalizes those durations so timing can be inspected without making
credential-free replay flaky.

The live mutation test is not compiled into regular `go test ./...` runs. The
`azurelive` build tag is required in addition to the environment gates above.

## Z01 Azure SQL

Z01 may use live execution after the operator explicitly supplies the existing
SQL scope:

- `RAMEN_AZURE_SQL_RESOURCE_GROUP`
- `RAMEN_AZURE_SQL_SERVER`
- disposable database name with `ramen-parity-z01-*`

The live waiter budget must be at least 30 attempts. After delete, verify
absence with Azure CLI before preserving any sanitized observation summary:

```bash
az sql db show \
  --resource-group "$RAMEN_AZURE_SQL_RESOURCE_GROUP" \
  --server "$RAMEN_AZURE_SQL_SERVER" \
  --name "$RAMEN_AZURE_SQL_DATABASE"
```

If normal cleanup fails, use the explicit fallback:

```bash
az sql db delete \
  --resource-group "$RAMEN_AZURE_SQL_RESOURCE_GROUP" \
  --server "$RAMEN_AZURE_SQL_SERVER" \
  --name "$RAMEN_AZURE_SQL_DATABASE" \
  --yes
```

## Z02 Cosmos DB

Z02 may use live execution only after the operator explicitly approves Cosmos
DB cost exposure for the scoped run. The recorded fixture used:

- isolated `ramen-parity-z02-*` resource groups dedicated to the parity run;
- approved cost exposure for the subscription/resource group;
- globally unique disposable `ramen-parity-z02-*` account names;
- post-delete account absence verification;
- isolated resource-group deletion and absence verification;
- no committed account IDs, keys, endpoint URLs, raw payloads, or state.

The Z02 native fixture declares `runtime_hints.settle` for the bounded
pre-delete read barrier because Cosmos DB can hold an exclusive operation lock
briefly after account creation. Default replay/static tests validate that
metadata without live Azure access. The committed Z02 live recording was
refreshed after the A04 general settle migration and exercises the general
settle path; any future re-recording remains explicit opt-in.

## Planned Z03-Z06 Safety

Z03-Z06 default checks are static or read-only. Any future live mutation must
use the minimum number of smallest practical disposable resources, avoid large
or high-cost Azure resources, and destroy or clean up every resource it creates
before any sanitized recording update is accepted. Mutation-capable native
fixtures should carry retry/waiter metadata and use `runtime_hints.settle`
where a bounded pre-delete read barrier is needed for reliable cleanup.
