# Ansible Conversion Review

Generated artifacts are static review scaffolding. Ramen did not execute Ansible, modules, inventory connections, API source operations, or UWS workflows.

## Conversion Summary

- Playbook: `../../testdata/ansible-conversion/multinotify/input/playbook.yml`
- Project directory: `../../testdata/ansible-conversion/multinotify/input`
- Argspec documents: `1`
- Roles paths: `none`
- Collections paths: `none`
- Inventory inputs: `none`
- Extra vars: `none`
- Lowered operations: `3`
- Diagnostics: `0`
- Strict failures: `0`

## Artifact Paths

- UWS document: `../../testdata/ansible-conversion/multinotify/workflows/workflow.uws.yaml`
- HCL document: `../../testdata/ansible-conversion/multinotify/workflows/workflow.hcl`
- Diagnostics JSON: `../../testdata/ansible-conversion/multinotify/expected/diagnostics.json`
- Diagnostics Markdown: `../../testdata/ansible-conversion/multinotify/expected/diagnostics.md`
- Review Markdown: `../../testdata/ansible-conversion/multinotify/expected/review.md`

## Lowered Operations

| Operation | Source | Module | Workflow Steps |
|---|---|---|---|
| `install_nginx` | `builtin` | `ansible.builtin.apt` | install_nginx |
| `deploy_nginx_config` | `builtin` | `ansible.builtin.template` | deploy_nginx_config |
| `restart_nginx` | `builtin` | `ansible.builtin.service` | restart_nginx_run_1, restart_nginx_run_2 |

## Diagnostics Summary

No diagnostics. The playbook lowered cleanly.

## Unsupported Gate

Status: `pass`. No strict-failure diagnostics were emitted.
