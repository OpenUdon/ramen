# Ansible Conversion

`ramen convert ansible` statically converts a supported subset of Ansible
playbooks into reviewable UWS 1.6 workflow artifacts:

```bash
ramen convert ansible --playbook FILE --argspec ID=PATH --out DIR --strict
```

The command does not execute Ansible, modules, inventory connections, API
source operations, or UWS workflows. It is a conversion and review aid only.

## Inputs

- `--playbook FILE` is required and must point to an Ansible playbook YAML
  file.
- `--argspec ID=PATH` supplies a collection argspec document. The flag may be
  repeated. `ID` becomes the UWS source description name, and `PATH` is recorded
  as that source URL.
- Argspec documents must use the `uws.ansible.1.0` shape:
  - top-level `argspec: uws.ansible.1.0`
  - collection name, such as `ansible.builtin`
  - module entries keyed by FQCN, such as `ansible.builtin.file`
  - argument metadata for required fields and accepted enum values

## Outputs

With at least one lowered task, `DIR` contains:

- `workflows/workflow.uws.yaml`
- `workflows/workflow.hcl`
- `expected/diagnostics.json`
- `expected/diagnostics.md`
- `expected/review.md`

If no task can be lowered, the workflow files are not written, but
`expected/diagnostics.*` and `expected/review.md` are still written.

`expected/review.md` includes the conversion summary, artifact paths, lowered
operation table, diagnostics summary, and strict-gate status.

## Strict Mode

Without `--strict`, conversion can write review artifacts even when unsupported
constructs produce strict-failure diagnostics. With `--strict`, the CLI exits
with code `3` when any strict-failure diagnostic is present. Usage errors exit
with code `2`; unexpected converter errors exit with code `1`.

## Supported Subset

The current subset is intentionally fail-closed. It can lower simple static
tasks, literal and variable-backed arguments, simple loops, simple `when`
comparisons, registers that refer to already-lowered producers, and notified
handlers.

Unsupported or review-only constructs become diagnostics instead of guessed
workflow behavior, including:

- complex Jinja2 expressions, filters, and runtime facts
- dynamic includes
- unknown modules or modules missing from supplied argspecs
- `delegate_to`, `run_once`, and target-changing directives
- block `rescue` or `always`
- combined block-level and task-level `when` guards
- multiple `when` conditions that require logical AND
- handlers with their own `when` guard
- play-level sections such as `pre_tasks`, `post_tasks`, and `roles`
- inventory fan-out and connection behavior

This boundary keeps conversion review-first: unsupported behavior is visible in
diagnostics, and no converted artifact should be treated as approved for trusted
execution without human review.
