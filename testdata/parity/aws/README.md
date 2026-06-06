# AWS Provider Parity Fixtures

This tree is reserved for the `Wxx` AWS provider/runtime parity lane.

AWS parity is OpenTofu-baseline-first for new live work. Historical provider
corpus fixtures may still mention Terraform/OpenTofu conversion input, but new
mutation parity runs use OpenTofu plus Ramen unless an explicit broader review
requires otherwise.

Default tests are credential-free. Live AWS mutation is not enabled by this
tree yet; future live lanes must require explicit environment gates, use the
minimum practical disposable resources, avoid high-cost resources, and verify
cleanup before any sanitized observation is committed.
