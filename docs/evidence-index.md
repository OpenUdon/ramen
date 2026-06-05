# Ramen Evidence Index

This index summarizes the public-safe proof points that support current Ramen
claims. It separates credential-free fixtures, recorded replay, and sanitized
live evidence so adopters can inspect what has actually been proven.

## Credential-Free Default Evidence

- Native validation, graph, plan, apply/mock, refresh/mock, import, show,
  state, author, and iCoT tests run without provider credentials.
- `testdata/corpus` records the clean conversion and mapping corpus across AWS,
  Google, AzureRM, Kubernetes, and Cloudflare examples.
- `testdata/evidence/m28-sql` records Azure SQL native project fixtures for
  read, create/update, delete, runtime hints, and delete confirmation through
  mock execution.
- `executor/evidence_test.go` validates Ramen executor request, response,
  status, and confirmation-read records through the shared
  `github.com/OpenUdon/evidence/async` validators.

## Recorded Replay Evidence

- `testdata/equivalence/kubernetes` records Ramen-only Kubernetes namespace and
  selected namespaced-resource evidence that replays without a live cluster.
- `testdata/parity/kubernetes/k01` through `k06` record normalized
  API-visible observations for OpenTofu, Terraform, and Ramen+udon across
  Namespace, read-missing Namespace, ConfigMap, ServiceAccount, Role, and
  Opaque Secret lanes.
- `testdata/parity/kubernetes/k07` records RoleBinding parity across OpenTofu,
  Terraform, and Ramen+udon with HCL, reduced RBAC OpenAPI, native Ramen
  project, semantic replay assertions, and sanitized live observations.
- M34 uses the K07 recorded artifact as the evidence gate for advertising
  `kubernetes_role_binding_v1` in `tfmapping`.
- M35 adds `testdata/corpus/kubernetes/rbac/role_binding_v1/basic` as
  credential-free conversion evidence for the RoleBinding mapping.
- `testdata/parity/kubernetes/k08` records ClusterRole parity across
  OpenTofu, Terraform, and Ramen+udon with HCL, reduced RBAC OpenAPI, native
  Ramen project, semantic replay assertions, and sanitized live observations.
- M37 adds `testdata/corpus/kubernetes/rbac/cluster_role_v1/basic` as
  credential-free conversion evidence for the ClusterRole mapping.
- Replay tests must not require `kubectl`, `kind`, kubeconfig, Terraform,
  OpenTofu, provider plugins, private credentials, or network access.

## Sanitized Live Evidence

- M27 recorded Azure Resource Manager read evidence and a scoped Azure SQL
  create/read/delete lifecycle through approved `ramen apply --plan --executor
  udon` runs.
- M38 revalidated the Azure SQL live lane using local Azure profile readiness
  checks, credential-free static plan gates, and one disposable SQL database
  create/read/delete run through Ramen/udon. Live artifacts remained under
  ignored `.ramen` paths.
- M27/M28 evidence records only operation IDs, action counts, public-safe
  resource names, and command summaries. It does not commit tenant IDs,
  subscription IDs, client IDs, access tokens, state databases, plan files, or
  live response payloads.
- Kubernetes live parity remains opt-in through `RAMEN_K8S_PARITY=1`, refuses
  non-`kind-*` contexts, and writes committed observations only when explicitly
  recording.

## Validation Evidence

- V03 covers provider-neutral schema and mapping metadata checks: required and
  unknown attributes, type and enum checks, sensitive redaction coverage,
  required operation roles, binding-to-operation consistency, binding
  `operation_id` mismatches, retry/waiter hint shape, waiter read-role
  requirements, updateable/replacement-only schema conflicts, unknown
  normalizer rejection, and updateable identity-path warnings.
- Validation remains static and credential-free. It does not call provider
  APIs or infer provider-specific behavior from private SDKs.

## Projection Evidence

- M33 adds credential-free coverage that response bindings can traverse JSON
  strings and numeric array indexes. This supports embedded policy-document
  projection, such as reading a statement field out of a string-valued policy
  response, without provider-specific code.

## Async Evidence Boundary

- M29 maps Ramen executor requests, responses, status events, and confirmation
  reads into neutral Evidence async records.
- These records are evidence inputs only. An `accepted` executor response does
  not imply desired-state convergence, delete success, or state mutation.
- M30 persists normalized request, response, status, and confirmation-read
  records in local SQLite state and exposes read-only
  `ramen state async-evidence` inspection. Automatic resume/resubmit policy
  remains parked until operation handles and terminal observations are proven
  by fixtures.

## OpenUdon Boundary

- Ramen and OpenUdon do not import each other.
- Shared evidence belongs in `github.com/OpenUdon/evidence` when the record
  shape is product-neutral.
- Ramen owns desired-state convergence, state history, delete confirmation,
  governance, and plan approval semantics.
- OpenUdon owns package review, approval templates, package digests, and
  trusted-runner handoff.

## Review Checklist For New Evidence

- State whether the evidence is mock, recorded replay, or live sanitized.
- Keep default tests credential-free.
- Record operation IDs, action counts, diagnostics, and artifact paths instead
  of credential values or raw live payloads.
- Use explicit opt-in environment variables for live runs.
- Update the matching `memory-bank/status-*.md` file and this index when a new
  proof point changes a public claim.
