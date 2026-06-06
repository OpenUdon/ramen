# Google Cloud Parity Live Guardrails

Default Google parity tests are credential-free. Live runs are separate and
must be explicitly scoped by the operator.

## Shared Gates

Live Google parity requires:

- explicit live test selection:
  `go test -tags 'googlelive udon' . -run '^TestGoogleProviderParityLive$'`
- `RAMEN_GOOGLE_PARITY=1`
- `RAMEN_GOOGLE_PARITY_LANE=y02` or `RAMEN_GOOGLE_PARITY_LANE=y03`
- `RAMEN_GOOGLE_TOFU=/path/to/tofu`
- `GOOGLE_APPLICATION_CREDENTIALS=/path/to/google-credentials.json`
- `RAMEN_GOOGLE_PROJECT=<project-id>`
- `RAMEN_GOOGLE_PARITY_RECORD_UPDATE=1` only after reviewing sanitized output

The Ramen+udon live path renders the workflow with a
`google_service_account_file` provider appendix that points at
`GOOGLE_APPLICATION_CREDENTIALS`. Service-account JSON and ADC
`authorized_user` JSON are both accepted by the test harness. Credential JSON
and minted access tokens are never written into committed fixtures.

The live harness uses `gcloud storage buckets describe` for observation and
`gcloud storage rm --recursive` only as the Y03 cleanup fallback. Live runs must
not commit state databases, plan artifacts, service-account files, access
tokens, raw response bodies, project IDs from real runs, or generated executor
output.

## Y02 Read-Only Bucket Observation

Y02 requires:

- `RAMEN_GOOGLE_EXISTING_BUCKET=<bucket-name>`

Y02 does not create, update, delete, or clean up any bucket. It compares
OpenTofu data-source reads with Ramen+udon `storage.buckets.get` reads for the
operator-provided bucket.

## Y03 Disposable Bucket Mutation

Y03 creates bucket names with the `ramen-parity-y03-*` prefix. It creates one
empty bucket per runtime, applies a metadata-label update, reads the bucket,
deletes it, and verifies absence.

Optional:

- `RAMEN_GOOGLE_LOCATION=US` to override the default bucket location.

Y03 must not create objects, IAM bindings, retention locks, requester-pays
settings, public ACLs, or long-lived buckets. If normal cleanup fails, use the
explicit fallback and verify absence:

```bash
gcloud storage rm --recursive gs://<ramen-parity-y03-*> --quiet
gcloud storage buckets describe gs://<name> --format=json
```
