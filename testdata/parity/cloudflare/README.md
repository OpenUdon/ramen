# Cloudflare Provider Parity Fixtures

This tree is reserved for the `Cxx` Cloudflare provider/runtime parity lane.
Default tests validate static fixtures and sanitized replay metadata without
Cloudflare credentials, Terraform/OpenTofu execution, provider plugins, udon,
or network access.

The initial staged scope is:

- `C01`: R2 bucket create/read/update/no-op/delete lifecycle with committed
  sanitized OpenTofu, Terraform, and Ramen+udon observations.
- `C02`: R2 bucket read-missing with committed sanitized OpenTofu, Terraform,
  and Ramen+udon observations.
- `C03`: R2 bucket metadata variants with committed sanitized OpenTofu,
  Terraform, and Ramen+udon observations.
- `C04`: D1 database create/read with committed sanitized OpenTofu, Terraform,
  and Ramen+udon observations plus direct D1 delete cleanup.
- `C05`: D1 response-derived UUID/delete unlock with committed sanitized
  OpenTofu, Terraform, and Ramen+udon observations; Ramen delete is exercised
  through the response-derived UUID.
- `C06`: D1 `read_replication.mode` update with committed sanitized
  OpenTofu, Terraform, and Ramen+udon observations. The lane is intentionally
  bounded to the current D1 update field exposed by Cloudflare API and provider
  docs.
