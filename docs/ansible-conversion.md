# Ansible Conversion

`ramen convert ansible` statically converts a supported subset of Ansible
playbooks into reviewable UWS workflow artifacts. The default output is UWS 1.6
with `ansible-module` source binding. Use `--target-uws 1.5` to emit
extension-owned Ansible module-call leaves for UWS 1.5 compatibility. Features
that need unsupported semantics are strict diagnostics rather than
approximations:

```bash
ramen convert ansible --playbook FILE --argspec ID=PATH --project-dir DIR --roles-path DIR --collections-path DIR --inventory FILE --extra-var NAME=VALUE --target-uws 1.6 --out DIR --ignore-unsupported
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
- `--roles-path DIR` and `--collections-path DIR` are repeatable static
  resolution inputs for play-level `roles` and task-level `import_role`.
  Literal `import_tasks` paths are resolved relative to the file containing the
  import. Play-level `roles` and task-level `import_role` resolve through
  `--roles-path` or, when no roles path is supplied, `PROJECT/roles`; FQCN
  collection roles resolve under
  `--collections-path DIR/ansible_collections/NAMESPACE/COLLECTION/roles/ROLE`.
- `--inventory FILE` is repeatable and changes lowering posture: when at least
  one inventory input is supplied, non-local plays lower host-targeted task
  steps as UWS stage-1 host fan-out over `$inputs.hosts`, with each step
  binding `inputs.host` to `$item`. The converter does not parse inventory
  files or lower connection details; those remain runtime-owned.
- `--extra-var NAME=VALUE` or `--extra-var @file` is repeatable and recorded in
  review artifacts for provenance. It is not currently consumed for static
  variable precedence or expression lowering.
- Argspec documents must use the `uws.ansible.1.0` shape:
  - top-level `argspec: uws.ansible.1.0`
  - collection name, such as `ansible.builtin`
  - module entries keyed by FQCN, such as `ansible.builtin.file`
  - argument metadata for required fields and accepted enum values
- `--target-uws 1.6|1.5` selects the emitted operation binding shape. `1.6`
  emits first-class `sourceDescriptions[].type: ansible-module`; `1.5` emits
  extension-owned operations with `x-uws-operation-profile:
  uws.ansible-module-call.1.0` and `x-uws-ansible-module`.

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
tasks, literal and variable-backed arguments, simple loops, `when` comparisons
and condition lists, block/task guard combinations through nested `switch`
steps, registers that refer to already-lowered producers, and notified
handlers. Static `pre_tasks`, `tasks`, and `post_tasks` lower in play order.
Literal `import_tasks` files are expanded in place, including nested imports.
Static roles load `tasks/main.yml`, `handlers/main.yml`, `defaults/main.yml`,
and `vars/main.yml`; missing role tasks, missing imported files, cycles,
ambiguous role matches, and templated paths are strict diagnostics. Handler
`listen` aliases can be notified by name, and duplicate handler names or
aliases in one resolved play are strict diagnostics. Static play `vars`,
`vars_files`, role defaults, and role vars are emitted as
`components.variables` only when each source is a YAML map; conflicting values
fail closed instead of approximating Ansible precedence. Static task-local
`vars` lower into the task step's `inputs` and are visible to that task's
expression lowering as `$inputs.<name>`. With `--inventory`, non-local task
steps lower to UWS stage-1 host fan-out using `$inputs.hosts`.

When statically expressible, a single `changed_when` replaces the default
`changed` output expression, a single mechanically invertible `failed_when`
replaces the default module failure criterion, and
`until`/`retries`/`delay` lower to existing UWS `successCriteria` plus
`onFailure: retry`. `retries` maps to `retryLimit: retries - 1`, and `delay`
maps to `retryAfter` only when a retry action is emitted. `retries` or `delay`
without `until`, and `throttle` without host fan-out, emit non-strict
diagnostics and no control policy. `ignore_errors: true`, unsupported
host-fan-out `throttle`, multi-condition `changed_when`, and non-invertible or
multi-condition `failed_when` are strict diagnostics. `any_errors_fatal: true`
matches UWS 1.6 fail-fast behavior and emits no field.

## Lowering Contract

Ramen owns Ansible playbook lowering. UWS owns the resulting orchestration
objects and the bound runtime owns Ansible module execution. The UWS 1.6 and
UWS 1.5 targets share the same orchestration lowering; only the module leaf
binding differs.

| Ansible construct | Ramen lowering |
| --- | --- |
| Ordered `pre_tasks`, `tasks`, `post_tasks` | One UWS `sequence` workflow preserving resolved play order. |
| Module task | One UWS operation plus one operation step. UWS 1.6 binds to `sourceDescriptions[].type: ansible-module`; UWS 1.5 uses `uws.ansible-module-call.1.0`. |
| Literal or whole-reference args | `request.body` values, with `{{ var }}` lowered to `$variables.*`, `$inputs.*`, `$item`, or `$steps.*.outputs.*` when safe. |
| `when: expr` | Step `when` for one condition; nested `switch` guards for conjunctions that need more than one condition. |
| `when: a and b` | DNF conjunction lowered as nested `switch` guards so both conditions must pass. |
| `when: a or b` | DNF disjunction lowered as a `switch` with one case per disjunct when the task does not need a stable original step output for `register` or `notify`. |
| `not`, `is defined`, `is not defined` | Lowered to simple UWS comparisons when the operand is a lowerable reference. |
| `loop` / `with_items` | Step `forEach`; literal lists are hoisted to `components.variables`, and `$item` / `$index` are available inside the task. |
| `register` field reads | Later references like `result.rc` become `$steps.<producer>.outputs.rc`; the producer operation exposes the requested response path. |
| `changed_when` | A single lowerable condition replaces the operation's `outputs.changed`. |
| `failed_when` | A single mechanically invertible condition replaces the default failure criterion in `successCriteria`. |
| `until` + `retries` + `delay` | The `until` conditions append to `successCriteria`; static `retries` emits `onFailure` retry with `retryLimit: retries - 1`; static `delay` becomes `retryAfter`. |
| `notify` / handlers | One notifier gates the handler step on `$steps.<notifier>.outputs.changed == true`; multiple notifiers lower to one `switch` that runs the handler at most once. |
| Static `import_tasks` / `import_role` | Resolved before lowering; wrapper `when`, tags, task vars, retry directives, and condition directives are inherited when they do not conflict. |
| `--inventory` host fan-out | Non-local tasks get `forEach: $inputs.hosts` and `inputs.host: $item`; inventory parsing and connection details stay runtime-owned. |

## Support Matrix

| Category | Constructs | Behavior |
| --- | --- | --- |
| Supported | Static module tasks, simple args, simple `when`, `and`/`or`/`not`, `loop`, `register` field reads, handlers, static imports, static roles, `changed_when`, `failed_when`, `until`/`retries`/`delay` | Lowered into UWS core workflow objects and validated argspec-bound module leaves. |
| Partially supported | OR-guarded tasks, host fan-out, `throttle`, retries without `until`, static variables and role vars | Lowered only when a stable UWS meaning exists; otherwise emits diagnostics. |
| Runtime-owned | `become`, `become_user`, `become_method`, `environment`, `no_log`, inventory connection behavior, module invocation, credentials | Recorded as informational diagnostics or provenance; not emitted as UWS execution policy. |
| Review-only | `x-ansible` provenance, argspec references, project/role/collection/inventory inputs, extra-vars inputs | Included for review and reproducibility; they do not define UWS execution semantics. |
| Unsupported / fail-closed | Complex Jinja2, runtime facts, dynamic includes, unknown modules, `delegate_to`, `run_once`, `rescue`, `always`, non-static task vars, `ignore_errors`, unsafe handler/host fan-out combinations | Emits strict diagnostics and omits the affected task/handler unless `--ignore-unsupported` allows a partial artifact. |

Lowered operations and steps carry provenance-only `x-ansible` extensions with
the source file, line, column, play, section, task name, optional role, optional
import stack, and optional tags. These extensions are review/debug metadata and
do not define execution semantics.

For `--target-uws 1.5`, Ansible module calls are not source-bound. The emitted
operation keeps the same `request.body`, `outputs`, `successCriteria`, retries,
handlers, and workflow structure, but the module FQCN and argspec review
reference live under `x-uws-ansible-module`. UWS still owns orchestration; the
bound runtime owns module execution.

Unsupported or review-only constructs become diagnostics instead of guessed
workflow behavior, including:

- complex Jinja2 expressions, filters, and runtime facts
- dynamic includes, including `include_tasks`, `include_role`, and
  `include_vars`
- unknown modules or modules missing from supplied argspecs
- `delegate_to`, `run_once`, and target-changing directives
- block `rescue` or `always`
- non-static task-local `vars`
- `ignore_errors: true`
- unsupported `throttle`, multi-condition `changed_when`, and non-invertible
  or multi-condition `failed_when`
- notified handlers after `--inventory` host fan-out, because the current
  conversion does not lower per-host changed gates for handler execution
- inventory file expansion and connection behavior

This boundary keeps conversion review-first: unsupported behavior is visible in
diagnostics, and no converted artifact should be treated as approved for trusted
execution without human review.
