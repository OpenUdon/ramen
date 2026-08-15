# Ansible Conversion

`ramen convert ansible` statically converts a supported subset of Ansible
playbooks into reviewable UWS workflow artifacts. Ansible module leaves are
emitted as extension-owned operations carrying
`ramen.ansible-module-call.1.0`: the managed host does not expose a collection
module as a pre-existing named operation, so the control node supplies its
implementation and the argspec remains a client-side library manifest.
The profile, module extension, provenance extension, schemas, and typed helpers
are Ramen-owned internal conversion contracts; UWS owns only the generic
workflow model and `x-uws-operation-profile` selector used here.
Features that need unsupported semantics are strict diagnostics
rather than approximations:

```bash
ramen convert ansible --playbook FILE --argspec ID=PATH --argspec-dir DIR --project-dir DIR --roles-path DIR --collections-path DIR --inventory FILE --extra-var NAME=VALUE --target-uws 1.5 --out DIR --mode strict
```

The command does not execute Ansible, modules, inventory connections, API
source operations, or UWS workflows. It is a conversion and review aid only.

## Inputs

- `--playbook FILE` is required and must point to an Ansible playbook YAML
  file.
- `--argspec ID=PATH` supplies a collection argspec document. The flag may be
  repeated. `ID` is the raw argspec lookup key and is preserved in emitted
  module-call review references. Exact duplicate IDs are rejected, while IDs
  such as `acme.one` and `acme-one` remain distinct. `PATH` is recorded as the
  argspec URL.
- `--argspec-dir DIR` recursively discovers regular `*.argspec.json` files in
  deterministic path order. It is repeatable and bounded to 16 non-symlink
  directories, 256 documents, and 16 MiB per document. Each document is
  validated against the embedded schema before its declared collection becomes
  the source ID. Empty discovery, symlinks, malformed documents, duplicate
  collection IDs, and duplicate module FQCNs fail ingestion. Other filenames
  are ignored. Discovery never invokes `ansible-doc`, Python, Galaxy,
  collection plugins, or the network.
- `--project-dir DIR` records the static project root used for file
  resolution. It defaults to the playbook directory.
- `--roles-path DIR` and `--collections-path DIR` are repeatable static
  resolution inputs for play-level `roles` and task-level `import_role`.
  Literal `import_tasks` paths are resolved relative to the file containing the
  import. Play-level `roles` and task-level `import_role` resolve through
  `--roles-path` or, when no roles path is supplied, `PROJECT/roles`; FQCN
  collection roles resolve under
  `--collections-path DIR/ansible_collections/NAMESPACE/COLLECTION/roles/ROLE`.
- `--inventory FILE` is repeatable and reads bounded regular `.ini`, `.yaml`,
  `.yml`, or `.json` files. Each file is limited to 8 MiB; at most 32 inputs,
  4,096 hosts, and 1,024 groups are accepted. `all`, one exact host, or one
  exact group play target resolves to a sorted host-object collection. Exact
  group children are expanded recursively. Globs, unions/intersections,
  exclusions, ranges, templated targets, cycles, empty selections, executable
  inventory, directories, and symlinks fail closed. Ramen reads no inventory
  plugin and opens no connection.
- `--extra-var NAME=VALUE` or `--extra-var @file` is repeatable and enters
  static lowering at the highest precedence. File inputs must be bounded
  regular YAML/JSON maps; inline values are literal YAML values. Dynamic
  values, conflicting duplicates, malformed maps, directories, and symlinks
  fail closed. Secret-like literal variable names require a symbolic runtime
  credential binding. Reports retain names, safe filenames, and file digests,
  never inline values.
- Argspec documents must use the `ramen.ansible.1.0` shape:
  - top-level `argspec: ramen.ansible.1.0`
  - collection name, such as `ansible.builtin`
  - module entries keyed by FQCN, such as `ansible.builtin.file`
  - argument metadata for required fields and accepted enum values

  Ramen validates each file against its embedded Ansible argspec schema before
  decoding it. Unknown schema fields, missing required fields,
  module keys outside the declared collection, and parameter aliases claimed
  by more than one canonical parameter are argspec ingestion errors. The
  command exits `1` and writes no conversion/review artifacts.
- `--target-uws 1.5|1.6|1.7` selects only the `uws` version the emitted
  document declares; the default is `1.5`. The document shape is identical at
  every version, because module leaves are always extension-owned operations
  with `x-uws-operation-profile: ramen.ansible-module-call.1.0` and
  `x-ramen-ansible-module`. UWS 1.6 briefly offered an `ansible-module` source
  type and UWS 1.7 removed it.
- `--mode strict|partial` selects the common conversion gate. During the
  transition, omitted mode defaults to `strict`. `--strict` and
  `--ignore-unsupported` remain deprecated aliases for strict and partial.

This namespace change is a hard break. Ramen rejects
`uws.ansible.1.0`, `uws.ansible-module-call.1.0`, and
`x-uws-ansible-module`; it does not translate them and provides no legacy
decoder or artifact migration command. Historical artifacts remain available
from repository history but are not accepted as current Ramen contracts.

## Outputs

With at least one lowered task, `DIR` contains:

- `workflows/workflow.uws.yaml`
- `workflows/workflow.hcl`
- `expected/diagnostics.json`
- `expected/diagnostics.md`
- `expected/review.md`
- `expected/manifest.json`

If unsupported constructs produce strict-failure diagnostics, or if no task can
be lowered, the workflow files are not written by default, but
`expected/diagnostics.*` and `expected/review.md` are still written. Rerun with
`--mode partial` to write a partial workflow that omits unsupported constructs.

`expected/review.md` includes the conversion summary, artifact paths, lowered
operation table, diagnostics summary, and strict-gate status.

## Unsupported Constructs

By default, unsupported constructs are explicit conversion failures. The CLI
prints each strict-failure diagnostic, writes diagnostics and review markdown,
does not write UWS/HCL workflow artifacts, and exits with code `3`. Usage
errors exit with code `2`; unexpected converter errors exit with code `1`.

Use `--mode partial` only when a partial workflow is acceptable. In that
mode, Ramen still records diagnostics, but writes UWS/HCL artifacts with the
unsupported tasks, handlers, or control-flow constructs omitted.
Task-level argspec and `noLog` failures always omit their affected task from
partial output. Partial mode does not restore an invalid operation.

The JSON diagnostics and manifest use the shared embedded contracts documented
in the [static conversion contract](conversion.md). The manifest records input
and artifact digests, lowering coverage, the effective mode, and explicit
non-execution evidence without storing raw inline extra-variable values.

## Supported Subset

The current subset is intentionally fail-closed. It can lower simple static
tasks, literal and variable-backed arguments, simple loops, `when` comparisons
and condition lists, block/task guard combinations through nested `switch`
steps, registers that refer to already-lowered producers, and notified
handlers. Static `pre_tasks`, `tasks`, and `post_tasks` lower in play order.
Literal `import_tasks` and simple literal `include_tasks` files are expanded in
place, including nested content. Literal `include_role` with only a role name
reuses static role resolution. Include templating, apply/extra options, loops,
register/notify, and `include_vars` remain strict.
Static roles load `tasks/main.yml`, `handlers/main.yml`, `defaults/main.yml`,
and `vars/main.yml`; missing role tasks, missing imported files, cycles,
ambiguous role matches, and templated paths are strict diagnostics. Handler
`listen` aliases can be notified by name, and duplicate handler names or
aliases in one resolved play are strict diagnostics. Static values use this
bounded precedence from low to high: role defaults; inventory `all` vars;
equal-precedence inventory group vars; inventory host vars; vars files; play
vars; role vars; task vars; extra vars. Higher layers override lower layers.
Conflicting values within one layer fail closed rather than relying on group
priority or plugin behavior. Static task-local `vars` lower into task step
`inputs` and resolve as `$inputs.<name>`. With `--inventory`, non-local steps
fan out over a deterministic `components.variables` host-object list, bind
`inputs.host` to `$item.host`, and resolve unshadowed inventory variables as
`$item.vars.<name>`. Variables beginning `ansible_` remain connection/runtime
facts and are not embedded in those host objects.

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
matches UWS fail-fast behavior and emits no field.

Argspec parameter aliases are normalized to their canonical request-body keys
after value lowering. One alias spelling emits only its canonical key, and
equal canonical/alias duplicates collapse to one value. Conflicting values
through canonical and alias spellings emit
`ansible.argspec_violation` and omit the affected task. Unknown parameters,
missing required parameters, invalid choices, and literal values for `noLog`
parameters follow the same task-omission rule.

## Lowering Contract

Ramen owns Ansible playbook lowering and its conversion-only metadata. UWS owns
the resulting generic orchestration objects and the bound runtime owns Ansible
module execution. All `--target-uws` values share the same lowering; only the
declared `uws` version differs.

| Ansible construct | Ramen lowering |
| --- | --- |
| Ordered `pre_tasks`, `tasks`, `post_tasks` | One UWS `sequence` workflow preserving resolved play order. |
| Module task | One UWS operation plus one operation step. The operation is extension-owned, carrying `ramen.ansible-module-call.1.0` with the module FQCN and argspec review reference. |
| Literal or whole-reference args | `request.body` values, with `{{ var }}` lowered to `$variables.*`, `$inputs.*`, `$item`, or `$steps.*.outputs.*` when safe. |
| `when: expr` | Step `when` for one condition; nested `switch` guards for conjunctions that need more than one condition. |
| `when: a and b` | DNF conjunction lowered as nested `switch` guards so both conditions must pass. |
| `when: a or b` | DNF disjunction lowered as a `switch` with one case per disjunct when the task does not need a stable original step output for `register` or `notify`. |
| `not`, `is defined`, `is not defined` | Lowered to simple UWS comparisons when the operand is a lowerable reference. |
| `loop` / `with_items` / `with_list` | Step `forEach`; literal lists are hoisted to `components.variables`, and `$item`, dotted item fields, and `$index` are available inside the task. |
| Literal `with_dict` | Mapping keys sort lexically into `{key,value}` items, then lower through `forEach` with `$item.key` and `$item.value`. |
| `register` field reads | Later references like `result.rc` become `$steps.<producer>.outputs.rc`; the producer operation exposes the requested response path. |
| `changed_when` | A single lowerable condition replaces the operation's `outputs.changed`. |
| `failed_when` | A single mechanically invertible condition replaces the default failure criterion in `successCriteria`. |
| `until` + `retries` + `delay` | The `until` conditions append to `successCriteria`; static `retries` emits `onFailure` retry with `retryLimit: retries - 1`; static `delay` becomes `retryAfter`. |
| `notify` / handlers | One notifier gates the handler step on `$steps.<notifier>.outputs.changed == true`; multiple notifiers lower to one `switch` that runs the handler at most once. |
| Static import/include tasks and roles | Literal import forms plus bounded literal `include_tasks`/`include_role` resolve before lowering; supported wrapper guards, tags, vars, retry, and condition directives are inherited. |
| Static inventory host fan-out | Non-local tasks get `forEach: $variables.inventory_*_hosts`, `inputs.host: $item.host`, and unshadowed host variables under `$item.vars.*`; connection behavior stays runtime-owned. |

## Support Matrix

| Category | Constructs | Behavior |
| --- | --- | --- |
| Supported | Static module tasks, simple args, simple `when`, `and`/`or`/`not`, literal loops, register field reads, handlers, bounded static imports/includes/roles, `changed_when`, `failed_when`, `until`/`retries`/`delay` | Lowered into UWS core workflow objects and validated argspec-bound module leaves. |
| Partially supported | OR-guarded tasks, bounded static inventory fan-out, `throttle`, retries without `until`, and layered static variables | Lowered only when a stable UWS meaning exists; otherwise emits diagnostics. |
| Runtime-owned | `become`, `become_user`, `become_method`, `environment`, `no_log`, inventory connection behavior, module invocation, credentials | Recorded as informational diagnostics or provenance; not emitted as UWS execution policy. |
| Review-only | `x-ramen-ansible-provenance`, argspec references, and safe project/role/collection/inventory/extra-vars input evidence | Included for review and reproducibility; evidence metadata does not define UWS execution semantics. |
| Unsupported / fail-closed | Complex Jinja2, runtime facts, dynamic includes, unknown modules, `delegate_to`, `run_once`, `rescue`, `always`, non-static task vars, `ignore_errors`, unsafe handler/host fan-out combinations | Emits strict diagnostics and omits the affected task/handler unless `--mode partial` allows a partial artifact. |

Lowered operations and steps carry provenance-only `x-ramen-ansible-provenance` extensions with
the source file, line, column, play, section, task name, optional role, optional
import stack, and optional tags. These extensions are review/debug metadata and
do not define execution semantics.

Ansible module calls are not source-bound. The emitted operation carries
`request.body`, `outputs`, `successCriteria`, retries, handlers, and workflow
structure as usual, with the module FQCN and argspec review reference under
`x-ramen-ansible-module`. The Ramen profile selects that metadata through the
UWS core `x-uws-operation-profile` key; the bound runtime owns module
execution. When an argspec reference is present, its source ID and URL must be
non-blank and its collection must match the `namespace.collection` portion of
the module FQCN.

Unsupported or review-only constructs become diagnostics instead of guessed
workflow behavior, including:

- complex Jinja2 expressions, filters, and runtime facts
- templated or option-bearing `include_tasks`/`include_role`, include loops,
  and `include_vars`
- unknown modules or modules missing from supplied argspecs
- `delegate_to`, `run_once`, and target-changing directives
- block `rescue` or `always`
- non-static task-local `vars`
- `ignore_errors: true`
- unsupported `throttle`, multi-condition `changed_when`, and non-invertible
  or multi-condition `failed_when`
- simultaneous `loop` and `with_items`, including when either key has an empty
  value, or any mixed supported loop directives, because precedence would be
  ambiguous
- dynamic/plugin-backed legacy `with_*` forms and templates nested inside
  literal loop items
- notified handlers after `--inventory` host fan-out, because the current
  conversion does not lower per-host changed gates for handler execution
- inventory plugins, executable/directory inventory, dynamic host patterns,
  group-priority ambiguity, vault, and all inventory connection behavior

This boundary keeps conversion review-first: unsupported behavior is visible in
diagnostics, and no converted artifact should be treated as approved for trusted
execution without human review.
