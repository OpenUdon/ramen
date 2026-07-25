# Contributing

Ramen's native desired-state model is a UWS project plus Ramen reconciliation
metadata. Keep Terraform/OpenTofu and Ansible work in conversion adapters, and
keep live API mutation behind the trusted executor boundary.

## Development

Use the Go version declared in `go.mod`. The public gate must work without the
parent workspace or sibling repositories:

```bash
GOWORK=off go mod download
GOWORK=off go test ./... -count=1 -timeout=10m
GOWORK=off go vet ./...
git diff --check
```

The optional private udon adapter is not part of the public gate. Do not run or
record live cloud tests unless the relevant environment gate, disposable
resource scope, credentials, cost controls, and cleanup procedure are explicit.

Add focused regression coverage for behavioral changes. Public examples and
default tests must be credential-free and deterministic. Update the relevant
memory-bank milestone status when changing roadmap scope or contracts.

Security reports belong in the private channel described in
[SECURITY.md](SECURITY.md), not in public issues.
