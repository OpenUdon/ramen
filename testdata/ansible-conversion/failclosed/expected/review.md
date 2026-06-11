# Ansible Conversion Review

Generated artifacts are static review scaffolding. Ramen did not execute Ansible, modules, inventory connections, API source operations, or UWS workflows.

## Conversion Summary

- Playbook: `../../testdata/ansible-conversion/failclosed/input/playbook.yml`
- Argspec documents: `1`
- Lowered operations: `4`
- Diagnostics: `7`
- Strict failures: `7`

## Artifact Paths

- UWS document: `../../testdata/ansible-conversion/failclosed/workflows/workflow.uws.yaml`
- HCL document: `../../testdata/ansible-conversion/failclosed/workflows/workflow.hcl`
- Diagnostics JSON: `../../testdata/ansible-conversion/failclosed/expected/diagnostics.json`
- Diagnostics Markdown: `../../testdata/ansible-conversion/failclosed/expected/diagnostics.md`
- Review Markdown: `../../testdata/ansible-conversion/failclosed/expected/review.md`

## Lowered Operations

| Operation | Source | Module | Workflow Steps |
|---|---|---|---|
| `singly_guarded_child` | `builtin` | `ansible.builtin.service` | singly_guarded_child |
| `consumer_of_skipped_producer` | `builtin` | `ansible.builtin.shell` | consumer_of_skipped_producer |
| `consumer_before_future_producer` | `builtin` | `ansible.builtin.shell` | consumer_before_future_producer |
| `future_producer` | `builtin` | `ansible.builtin.shell` | future_producer |

## Diagnostics Summary

- `error`: `7`

## Strict Gate

Status: `not-enforced`. `7` strict-failure diagnostics were emitted; rerun with `--strict` to make them exit non-zero.
