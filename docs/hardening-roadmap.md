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

The remaining seed findings were folded into canonical status files:

- M14 closed the stale live udon finding by requiring output projection before
  non-delete live success can produce `executor.Result`.
- M16 added the native variables/values layer with deterministic values files,
  CLI assignments, redaction, and digest-bound approval.
- M17 added policy hooks, approval routing, audit export, and local workspace
  isolation. Enterprise adoption still needs packaging and operational support
  contracts; see `memory-bank/status-M12.md`.
