# Ansible Conversion

`ramen convert ansible` statically converts a supported subset of Ansible
playbooks into reviewable UWS 1.6 workflow artifacts:

```bash
ramen convert ansible --playbook FILE --argspec ID=PATH --project-dir DIR --roles-path DIR --collections-path DIR --inventory FILE --extra-var NAME=VALUE --out DIR --ignore-unsupported
```

The command does not execute Ansible, modules, inventory connections, API
source operations, or UWS workflows. It is a conversion and review aid only.

## Inputs

- `--playbook FILE` is required and must point to an Ansible playbook YAML
  file.
- `--argspec ID=PATH` supplies a collection argspec document. The flag may be
  repeated. `ID` is the raw argspec source ID; conversion sanitizes it for the
  emitted UWS source description name and rejects IDs that would collide after
  sanitization. `PATH` is recorded as that source URL.
- `--project-dir DIR` records the static project root used for file
  resolution. It defaults to the playbook directory.
- `--roles-path DIR`, `--collections-path DIR`, `--inventory FILE`, and
  `--extra-var NAME=VALUE` are repeatable static-resolution inputs. The
  converter records them in review artifacts. When at least one `--inventory`
  input is supplied, non-local plays lower host-targeted task steps as UWS
  stage-1 host fan-out over `$inputs.hosts`, with each step binding
  `inputs.host` to `$item`. The converter does not parse inventory files or
  lower connection details; those remain runtime-owned.
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

If unsupported constructs produce strict-failure diagnostics, or if no task can
be lowered, the workflow files are not written by default, but
`expected/diagnostics.*` and `expected/review.md` are still written. Rerun with
`--ignore-unsupported` to write a partial workflow that omits unsupported
constructs.

`expected/review.md` includes the conversion summary, artifact paths, lowered
operation table, diagnostics summary, and strict-gate status.

## Unsupported Constructs

By default, unsupported constructs are explicit conversion failures. The CLI
prints each strict-failure diagnostic, writes diagnostics and review markdown,
does not write UWS/HCL workflow artifacts, and exits with code `3`. Usage
errors exit with code `2`; unexpected converter errors exit with code `1`.

Use `--ignore-unsupported` only when a partial workflow is acceptable. In that
mode, Ramen still records diagnostics, but writes UWS/HCL artifacts with the
unsupported tasks, handlers, or control-flow constructs omitted.

## Supported Subset

The current subset is intentionally fail-closed. It can lower simple static
tasks, literal and variable-backed arguments, simple loops, simple `when`
comparisons, registers that refer to already-lowered producers, and notified
handlers. Static `pre_tasks`, `tasks`, and `post_tasks` lower in play order.
With `--inventory`, non-local task steps lower to UWS stage-1 host fan-out
using `$inputs.hosts`.

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
- notified handlers after `--inventory` host fan-out, because UWS 1.6
  aggregates `forEach` outputs and does not expose per-host changed gating
- play-level `roles`
- inventory file expansion and connection behavior

This boundary keeps conversion review-first: unsupported behavior is visible in
diagnostics, and no converted artifact should be treated as approved for trusted
execution without human review.
