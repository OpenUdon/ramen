# Cloudflare Provider Parity Fixtures

This tree is reserved for the `Cxx` Cloudflare provider/runtime parity lane.
Default tests validate static fixtures and sanitized replay metadata without
Cloudflare credentials, Terraform/OpenTofu execution, provider plugins, udon,
or network access.

The initial staged scope is:

- `C01`: R2 bucket create/read/update/no-op/delete lifecycle candidate.
- `C02`: R2 bucket read-missing candidate.
- `C03`: R2 bucket metadata variants, static first.
- `C04`: D1 database create/read, static first.
- `C05`: D1 UUID/delete unlock planning, static first.
