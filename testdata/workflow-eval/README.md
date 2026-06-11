# Workflow Evaluation Manifest

`manifest.json` indexes workflow-only conversion evidence. It references
source-specific fixture artifacts in place and does not copy generated
workflows into this directory.

Accepted categories:

- `ansible-conversion`: static `ramen convert ansible` fixtures with generated
  UWS workflow artifacts, diagnostics, and review markdown.

New categories require a matching source-specific fixture corpus, diagnostics,
review artifact coverage, default regression checks, and a memory-bank update
that keeps the source out of T01 unless it is intentionally reclassified as
NL-to-Ramen desired-state training data.
