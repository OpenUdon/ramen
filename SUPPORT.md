# Ramen v0.1 Support Policy

Ramen v0.1 is a public beta for API and internal-platform teams embedding a
provider-free desired-state engine.

## Supported Core

The `project`, `validate`, `graph`, `plan`, `apply`, `reconcile`, `state`, and
`executor` Go packages are the supported v0.1 core. Ramen will not
intentionally make breaking changes to those package contracts within the
v0.1.x line. A necessary breaking change will be released as v0.2.0 and called
out in release notes.

The native-core versioned artifacts emitted by these packages—including
project, inputs, graph, plan and approval, apply and reconciliation, state
export and audit, and executor feedback and recording documents—remain
backward-compatible within v0.1.x. Readers continue to reject unknown future
versions rather than guessing.

The `authoring`, `diagnostic`, `governance`, `run`, and `tfmapping` Go packages,
their adapter-specific outputs, and broader resource mappings remain
experimental before v1. CLI diagnostic codes remain documented behavior even
though the diagnostic Go package is experimental.

## Execution

The supported integration seam is the in-process `executor.Executor` Go
interface. An executor declares capabilities and executes an approved request.
Credential values remain in executor-owned configuration.

Public release binaries include mock execution only. The build-tagged udon
adapter, live provider behavior, remote state, and an external executor
protocol are not part of the v0.1 support promise.

## Platforms and Help

Release archives target Linux, macOS, and Windows on amd64 and arm64. Go module
consumers use the Go version declared in `go.mod`.

Questions and reproducible bugs may be filed in GitHub Issues. Support is
community best effort; no uptime, response-time, or operational SLA is
provided.
