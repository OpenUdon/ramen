# Cloudflare Provider Parity Fixtures

This tree is reserved for the `Cxx` Cloudflare provider/runtime parity lane.
Default tests validate static fixtures and sanitized replay metadata without
Cloudflare credentials, Terraform/OpenTofu execution, provider plugins, udon,
or network access.

The initial staged scope is:

- `C01`: R2 bucket create/read/update/no-op/delete lifecycle candidate.
- `C02`: R2 bucket read-missing candidate.
- `C03`: R2 bucket metadata variants with opt-in live smoke coverage.
- `C04`: D1 database create/read with opt-in live smoke coverage and direct
  D1 delete cleanup.
- `C05`: D1 response-derived UUID/delete unlock with an opt-in live runner;
  Ramen delete is exercised through the response-derived UUID. D1 update
  remains unclaimed.
