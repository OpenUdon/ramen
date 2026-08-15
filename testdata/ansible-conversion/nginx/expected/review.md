# Ansible Conversion Review

Generated artifacts are static review scaffolding. Ramen did not execute Ansible, modules, inventory connections, API source operations, or UWS workflows.

## Conversion Summary

- Playbook: `../../testdata/ansible-conversion/nginx/input/playbook.yml`
- Project directory: `../../testdata/ansible-conversion/nginx/input`
- Argspec documents: `1`
- Roles paths: `none`
- Collections paths: `none`
- Inventory inputs: `none`
- Extra vars: `none`
- Lowered operations: `3`
- Diagnostics: `1`
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
| `install_nginx` | `builtin` | `ansible.builtin.apt` | install_nginx |
| `deploy_nginx_config` | `builtin` | `ansible.builtin.template` | deploy_nginx_config |
| `restart_nginx` | `builtin` | `ansible.builtin.service` | restart_nginx |

## Diagnostics Summary

- `info`: `1`

## Unsupported Gate

Status: `pass`. No strict-failure diagnostics were emitted.
