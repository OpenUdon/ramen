# Ansible Conversion Review

Generated artifacts are static review scaffolding. Ramen did not execute Ansible, modules, inventory connections, API source operations, or UWS workflows.

## Conversion Summary

- Playbook: `../../testdata/ansible-conversion/nginx/input/playbook.yml`
- Argspec documents: `1`
- Lowered operations: `3`
- Diagnostics: `1`
- Strict failures: `0`

## Artifact Paths

- UWS document: `../../testdata/ansible-conversion/nginx/workflows/workflow.uws.yaml`
- HCL document: `../../testdata/ansible-conversion/nginx/workflows/workflow.hcl`
- Diagnostics JSON: `../../testdata/ansible-conversion/nginx/expected/diagnostics.json`
- Diagnostics Markdown: `../../testdata/ansible-conversion/nginx/expected/diagnostics.md`
- Review Markdown: `../../testdata/ansible-conversion/nginx/expected/review.md`

## Lowered Operations

| Operation | Source | Module | Workflow Steps |
|---|---|---|---|
| `install_nginx` | `builtin` | `ansible.builtin.apt` | install_nginx |
| `deploy_nginx_config` | `builtin` | `ansible.builtin.template` | deploy_nginx_config |
| `restart_nginx` | `builtin` | `ansible.builtin.service` | restart_nginx |

## Diagnostics Summary

- `info`: `1`

## Strict Gate

Status: `pass`. No strict-failure diagnostics were emitted.
