# Google Provider Parity Fixtures

This tree is reserved for Google Cloud provider/runtime parity tracks. The
`Yxx` prefix is used because `Gxx` already belongs to the `ramen graph` command
history.

Default tests are credential-free. Y01 is a static-only Google Cloud Storage
Bucket parity lane over Google Discovery metadata and does not run OpenTofu,
provider plugins, `gcloud`, live GCP APIs, or udon.

Y02 and Y03 add real GCP lanes behind the `googlelive` build tag and explicit
environment gates for recording updates. Y02 is read-only, observes an
operator-provided existing bucket, and has a committed sanitized recording.
Y03 creates one disposable empty bucket at a time, updates a label, reads it,
deletes it, verifies absence, and has a committed sanitized
`live.observations.json` recording.

Y04 and Y06 are GCS mutation lanes with committed sanitized recordings. Y04
compares bucket read-missing behavior after out-of-band deletion. Y06 creates
and deletes managed folders in disposable hierarchical-namespace support
buckets. Y05 creates and deletes one tiny object in a disposable support bucket
per runtime and records metadata-only observations.

New Google live recordings are committed only after explicit
`RAMEN_GOOGLE_PARITY_RECORD_UPDATE=1` promotion, cleanup verification, and
sanitization review.
Y07 records the broader Google Cloud follow-up review that selected Y08 for
GCS bucket label/metadata drift. No separate new-resource lane is active from
that review.
Y08 records bucket label/metadata drift using empty disposable
`ramen-parity-y08-*` buckets and committed sanitized live observations.

See `LIVE.md` for the current live guardrail contract.
