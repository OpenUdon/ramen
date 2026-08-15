# Ansible Conversion Review

Generated artifacts are static review scaffolding. Ramen did not execute Ansible, modules, inventory connections, API source operations, or UWS workflows.

## Conversion Summary

- Playbook: `../../testdata/ansible-conversion/failclosed/input/playbook.yml`
- Project directory: `../../testdata/ansible-conversion/failclosed/input`
- Argspec documents: `1`
- Roles paths: `none`
- Collections paths: `none`
- Inventory inputs: `none`
- Extra vars: `none`
- Lowered operations: `6`
- Diagnostics: `5`
- Strict failures: `5`

## Artifact Paths

- UWS document: `workflows/workflow.uws.yaml`
- HCL document: `workflows/workflow.hcl`
- Diagnostics JSON: `expected/diagnostics.json`
- Diagnostics Markdown: `expected/diagnostics.md`
- Review Markdown: `expected/review.md`
- Manifest: `expected/manifest.json`

## Lowered Operations

| Operation | Source | Module | Workflow Steps |
|---|---|---|---|
| `doubly_guarded_child` | `builtin` | `ansible.builtin.service` | doubly_guarded_child |
| `singly_guarded_child` | `builtin` | `ansible.builtin.service` | singly_guarded_child |
| `consumer_of_skipped_producer` | `builtin` | `ansible.builtin.shell` | consumer_of_skipped_producer |
| `consumer_before_future_producer` | `builtin` | `ansible.builtin.shell` | consumer_before_future_producer |
| `future_producer` | `builtin` | `ansible.builtin.shell` | future_producer |
| `guarded_restart` | `builtin` | `ansible.builtin.service` | guarded_restart |

## Diagnostics Summary

- `error`: `5`

## Unsupported Gate

Status: `ignored`. `5` strict-failure diagnostics were emitted, but `--ignore-unsupported` allowed partial workflow artifacts to be written without unsupported constructs.
