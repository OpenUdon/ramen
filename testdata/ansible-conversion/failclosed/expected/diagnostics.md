# Ansible Conversion Diagnostics

| Code | Severity | Task | Message |
|---|---|---|---|
| `ansible.delegate_unsupported` | error | Delegated block | directive "delegate_to" on a block changes the execution target for all of its tasks; the block was not lowered |
| `ansible.jinja_unsupported` | error | Consumer before future producer | argument "cmd": unknown dotted reference "future_result.stdout" |
| `ansible.jinja_unsupported` | error | Consumer of skipped producer | argument "cmd": unknown dotted reference "produced.stdout" |
| `ansible.jinja_unsupported` | error | Guarded producer | guarded task: expression "ansible_facts['os_family']" uses Jinja2 features (filters, lookups, math, or indexing) outside UWS core; the task was not lowered |
| `ansible.rescue_todo` | error | Block with rescue | block rescue tasks are not lowered yet; the block was not lowered |
