# Client-Language Conversion Model

Terraform/OpenTofu and Ansible are client-side languages: they describe what a
client or peer wants to establish on remote systems. Ramen accepts bounded
static subsets as migration inputs, but neither language becomes Ramen's native
runtime or public semantic center.

Both adapters follow the same review lifecycle:

1. read bounded local inputs without executing their native toolchain;
2. extract only deterministic facts with an exact Ramen lowering;
3. validate the generated Ramen/UWS artifacts and embedded conversion metadata;
4. emit the common manifest, diagnostics, coverage, digests, and explicit
   `execution.performed: false` evidence;
5. withhold semantic output in strict mode when unsupported facts remain, or
   emit an explicitly partial review artifact when the operator selects partial
   mode.

That common lifecycle is not a shared semantic IR. Terraform conversion emits
native desired-state resources for Ramen reconciliation. Ansible conversion
emits a generic UWS workflow whose module leaves carry Ramen-owned conversion
metadata and still require a trusted Ansible-capable runtime.

## Ownership And Evidence

| Evidence or contract | What it says | Owner | Required? | It cannot do |
|---|---|---|---|---|
| Terraform/OpenTofu HCL | Client-side desired configuration and statically recoverable dependencies/instances/modules. | Input author; parsed through public `tfconfig`. | Terraform conversion input. | Define a server API operation, authorize execution, or provide Terraform runtime evaluation. |
| API source document | Server/peer operation inventory, request/response shapes, and security metadata. | API publisher and public `apitools` contracts. | Required for Terraform object-to-operation conversion. | Supply desired lifecycle or credentials, or prove provider behavior. |
| Ramen mapping/native profile | Desired-state identity, lifecycle roles, bindings, schema, provenance, and reconciliation metadata. | Ramen. | Generated/reviewed Terraform output. | Execute an operation without approval and a trusted executor. |
| Terraform provider-schema snapshot | Optional offline client-configuration shape evidence from standard format 1.0 JSON. | Operator-supplied evidence; read internally by Ramen. | Optional. API sources stay required. | Select operations, define auth/bindings/lifecycle, load a provider, or replace an API source. |
| Ansible playbook | Client-side ordered workflow intent, static guards, loops, roles/includes, and module arguments. | Input author; parsed by Ramen's internal adapter. | Ansible conversion input. | Define a server API contract or execute a module. |
| Ramen Ansible argspec | Client-side collection/module library metadata and argument validation. | Ramen-localized internal conversion contract (`ramen.ansible.1.0`). | Required for each lowered module. | Turn the managed host into a named API operation or grant runtime authority. |
| Ansible inventory/extra vars | Optional bounded client-selected hosts and literal non-connection variable facts. | Input author; read internally by Ramen. | Optional. | Open SSH, evaluate inventory plugins, decrypt vault, gather facts, or supply credentials. |
| UWS core document | Generic workflow structure, expressions, operations, and execution-policy vocabulary. | UWS. | Generated workflow foundation. | Define Terraform, provider, Ansible, inventory, or SSH semantics. |
| Ramen conversion extensions/reports | Adapter provenance, module-call references, loss diagnostics, coverage, and non-execution evidence. | Ramen. | Generated review metadata. | Broaden UWS core or approve/execute a workflow. |
| Trusted executor/runtime | Approved API calls, Ansible module implementation, connection/SSH behavior, and credential resolution. | Udon or another explicitly bound trusted runtime. | Required only after separate review/approval for execution. | Retroactively make unsupported conversion semantics exact. |

The practical distinction is deliberate: Terraform conversion needs a server
operation contract because Ramen will reconcile API-visible desired state.
Ansible modules are predominantly client-side library calls, so their argspec
and provenance stay inside Ramen's conversion adapter; SSH is runtime behavior,
not a reason to move an Ansible schema into UWS or Ramen's native state model.

## Promotion Rule For More Syntax

A language feature is lowerable only when all of these are true:

- the input fact is available statically from a bounded local reader;
- its meaning is deterministic without a plugin, provider, state, network,
  shell, Jinja/HCL evaluator, inventory connection, or module execution;
- existing native Ramen/UWS semantics represent it exactly;
- ordering, expansion limits, provenance, diagnostics, and partial-output
  behavior can be tested byte-for-byte;
- an unsupported or ambiguous form fails closed rather than silently changing
  target, lifecycle, dependency, condition, or credential behavior.

Current promoted examples include bounded literal Terraform instances and
exactly-once local modules, optional provider-schema validation, exact static
Ansible inventory targets, literal variable precedence, simple literal
includes, and literal list/dictionary loops. Composite Terraform expressions,
module outputs/instances/downloads, dynamic Ansible host patterns, plugins,
vault, complex Jinja, nested host/task loops, and per-host handler gates remain
explicit gaps.

## Operator Review

Before treating conversion output as a candidate project or workflow:

- confirm the manifest mode/status, every unsupported coverage item, and all
  strict diagnostics;
- verify API-source and mapping authority for Terraform, or argspec/module and
  static host selection for Ansible;
- inspect symbolic credentials and ensure no literal secret was embedded;
- compare deterministic YAML and HCL artifacts with the original intent;
- validate and plan native Terraform-derived projects, or run Ansible-derived
  workflows only through a separately approved trusted runtime.

See the [shared static conversion contract](conversion.md),
[Terraform/OpenTofu conversion contract](terraform-conversion.md), and
[Ansible conversion contract](ansible-conversion.md) for exact flags,
artifacts, supported subsets, and diagnostics.
