# Hardening Roadmap Intake Note

This file captured an AI-native enterprise hardening proposal. It is not the
canonical roadmap. Per Ramen documentation rules, canonical roadmap and status
tracking live in `memory-bank/milestone.md` and `memory-bank/status-*.md`.

The proposal has been folded into the memory bank as:

- `M13`: long-lived SQLite state hardening;
- `S02`: state maintenance subcommands;
- `M14`: executor boundary and live udon hardening;
- `M15`: AI-assist contract;
- `M16`: parameterization and values;
- `M17`: enterprise governance;
- `M18`: imperative run mode.

The original proposal's `M11` drift finding is stale for this checkout. M11 is
complete: the CLI now uses one command-scoped signal context, the positional
argument helper exists, `ramen convert --help` succeeds, and apply skipped
summary accounting is fixed.

The remaining seed findings are now tracked in canonical status files:

- `executor/udon/adapter.go` still returns successful live execution without
  projecting identity, computed attributes, or missing-state evidence into
  `executor.Result`; see `memory-bank/status-M14.md`.
- Ramen still has no native variables/values layer; see
  `memory-bank/status-M16.md`.
- Enterprise adoption still needs stronger state durability, executor
  capability negotiation, policy hooks, packaging, and operational support
  contracts; see `memory-bank/status-M13.md`, `memory-bank/status-M14.md`,
  `memory-bank/status-M17.md`, and `memory-bank/status-M12.md`.
