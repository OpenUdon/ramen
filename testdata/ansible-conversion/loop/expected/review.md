# Ansible Conversion Review

Generated artifacts are static review scaffolding. Ramen did not execute Ansible, modules, inventory connections, API source operations, or UWS workflows.

## Conversion Summary

- Playbook: `../../testdata/ansible-conversion/loop/input/playbook.yml`
- Project directory: `../../testdata/ansible-conversion/loop/input`
- Argspec documents: `1`
- Roles paths: `none`
- Collections paths: `none`
- Inventory inputs: `none`
- Extra vars: `none`
- Lowered operations: `1`
- Diagnostics: `0`
- Strict failures: `0`

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
| `create_app_directories` | `builtin` | `ansible.builtin.file` | create_app_directories |

## Diagnostics Summary

No diagnostics. The playbook lowered cleanly.

## Unsupported Gate

Status: `pass`. No strict-failure diagnostics were emitted.
