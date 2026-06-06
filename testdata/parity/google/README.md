# Google Provider Parity Fixtures

This tree is reserved for Google Cloud provider/runtime parity tracks. The
`Yxx` prefix is used because `Gxx` already belongs to the `ramen graph` command
history.

Default tests are credential-free. Y01 is a static-only Google Cloud Storage
Bucket parity lane over Google Discovery metadata and does not run OpenTofu,
provider plugins, `gcloud`, live GCP APIs, or udon.

Y02 and Y03 add opt-in real GCP lanes behind the `googlelive` build tag and
explicit environment gates. Y02 is read-only and observes an operator-provided
existing bucket. Y03 creates one disposable empty bucket at a time, updates a
label, reads it, deletes it, and verifies absence.

No Google live recording is committed by default. Recording updates require
`RAMEN_GOOGLE_PARITY_RECORD_UPDATE=1` after reviewing sanitized observations.

See `LIVE.md` for the current live guardrail contract.
