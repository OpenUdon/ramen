# Ansible Conversion Diagnostics

| Code | Severity | Task | Message |
|---|---|---|---|
| `ansible.delegate_unsupported` | error | Delegated block | directive "delegate_to" on a block changes the execution target for all of its tasks; the block was not lowered |
| `ansible.jinja_unsupported` | error | Consumer before future producer | argument "cmd": unknown dotted reference "future_result.stdout" |
| `ansible.jinja_unsupported` | error | Consumer of skipped producer | argument "cmd": unknown dotted reference "produced.stdout" |
| `ansible.jinja_unsupported` | error | Doubly guarded child | a block-level when and a task-level when combine with AND in Ansible; UWS core supports a single comparison — the guarded task was not lowered |
| `ansible.jinja_unsupported` | error | Guarded producer | expression "ansible_facts['os_family']" uses Jinja2 features (filters, lookups, math, or indexing) outside UWS core; the guarded task was not lowered |
| `ansible.jinja_unsupported` | error | guarded restart | a handler-level when combines with the notifier changed gate using AND; UWS core supports a single comparison — the handler was not lowered |
| `ansible.rescue_todo` | error | Block with rescue | block rescue tasks are not lowered yet; the block was not lowered |
