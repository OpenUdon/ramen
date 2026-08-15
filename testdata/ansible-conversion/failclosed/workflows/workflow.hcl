  uws = "1.5.0"
  info {
    title   = "Fail closed cases"
    version = "1.0.0"
  }
  operation "doubly_guarded_child" {
    description = "Doubly guarded child"
    outputs = {
      changed = "$response.body.changed"
    }
    request {
      body {
        name = "nginx"
        state = "stopped"
      }
    }
    successCriterion {
      condition = "$response.body.failed != true"
    }
    extensions {
      x-ramen-ansible-module {
        argspec {
          collection = "ansible.builtin"
          sourceId = "builtin"
          url = "testdata/argspec/ansible-builtin.argspec.json"
        }
        module = "ansible.builtin.service"
      }
      x-ramen-ansible-provenance {
        column = 11
        line = 17
        play = "Fail closed cases"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        task = "Doubly guarded child"
        version = "ramen.ansible.provenance.v1"
      }
      x-uws-operation-profile = "ramen.ansible-module-call.1.0"
    }
  }
  operation "singly_guarded_child" {
    description = "Singly guarded child"
    outputs = {
      changed = "$response.body.changed"
    }
    request {
      body {
        name = "nginx"
        state = "started"
      }
    }
    successCriterion {
      condition = "$response.body.failed != true"
    }
    extensions {
      x-ramen-ansible-module {
        argspec {
          collection = "ansible.builtin"
          sourceId = "builtin"
          url = "testdata/argspec/ansible-builtin.argspec.json"
        }
        module = "ansible.builtin.service"
      }
      x-ramen-ansible-provenance {
        column = 11
        line = 22
        play = "Fail closed cases"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        task = "Singly guarded child"
        version = "ramen.ansible.provenance.v1"
      }
      x-uws-operation-profile = "ramen.ansible-module-call.1.0"
    }
  }
  operation "consumer_of_skipped_producer" {
    description = "Consumer of skipped producer"
    outputs = {
      changed = "$response.body.changed"
    }
    request {
      body {
        cmd = "UWS-TODO({{ produced.stdout }})"
      }
    }
    successCriterion {
      condition = "$response.body.failed != true"
    }
    extensions {
      x-ramen-ansible-module {
        argspec {
          collection = "ansible.builtin"
          sourceId = "builtin"
          url = "testdata/argspec/ansible-builtin.argspec.json"
        }
        module = "ansible.builtin.shell"
      }
      x-ramen-ansible-provenance {
        column = 7
        line = 33
        play = "Fail closed cases"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        task = "Consumer of skipped producer"
        version = "ramen.ansible.provenance.v1"
      }
      x-uws-operation-profile = "ramen.ansible-module-call.1.0"
    }
  }
  operation "consumer_before_future_producer" {
    description = "Consumer before future producer"
    outputs = {
      changed = "$response.body.changed"
    }
    request {
      body {
        cmd = "UWS-TODO({{ future_result.stdout }})"
      }
    }
    successCriterion {
      condition = "$response.body.failed != true"
    }
    extensions {
      x-ramen-ansible-module {
        argspec {
          collection = "ansible.builtin"
          sourceId = "builtin"
          url = "testdata/argspec/ansible-builtin.argspec.json"
        }
        module = "ansible.builtin.shell"
      }
      x-ramen-ansible-provenance {
        column = 7
        line = 38
        play = "Fail closed cases"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        task = "Consumer before future producer"
        version = "ramen.ansible.provenance.v1"
      }
      x-uws-operation-profile = "ramen.ansible-module-call.1.0"
    }
  }
  operation "future_producer" {
    description = "Future producer"
    outputs = {
      changed = "$response.body.changed"
    }
    request {
      body {
        cmd = "echo future"
      }
    }
    successCriterion {
      condition = "$response.body.failed != true"
    }
    extensions {
      x-ramen-ansible-module {
        argspec {
          collection = "ansible.builtin"
          sourceId = "builtin"
          url = "testdata/argspec/ansible-builtin.argspec.json"
        }
        module = "ansible.builtin.shell"
      }
      x-ramen-ansible-provenance {
        column = 7
        line = 42
        play = "Fail closed cases"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        task = "Future producer"
        version = "ramen.ansible.provenance.v1"
      }
      x-uws-operation-profile = "ramen.ansible-module-call.1.0"
    }
  }
  operation "guarded_restart" {
    description = "guarded restart"
    outputs = {
      changed = "$response.body.changed"
    }
    request {
      body {
        name = "nginx"
        state = "restarted"
      }
    }
    successCriterion {
      condition = "$response.body.failed != true"
    }
    extensions {
      x-ramen-ansible-module {
        argspec {
          collection = "ansible.builtin"
          sourceId = "builtin"
          url = "testdata/argspec/ansible-builtin.argspec.json"
        }
        module = "ansible.builtin.service"
      }
      x-ramen-ansible-provenance {
        column = 7
        line = 58
        play = "Fail closed cases"
        section = "handlers"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        task = "guarded restart"
        version = "ramen.ansible.provenance.v1"
      }
      x-uws-operation-profile = "ramen.ansible-module-call.1.0"
    }
  }
  workflow "main" {
    type = "sequence"
    step "doubly_guarded_child_guard_1" {
      type = "switch"
      case "condition_1" {
        when = "$variables.env == \"prod\""
        step "doubly_guarded_child_guard_2" {
          type = "switch"
          case "condition_2" {
            when = "$variables.env == \"prod\""
            step "doubly_guarded_child" {
              operationRef = "doubly_guarded_child"
              extensions {
                x-ramen-ansible-provenance {
                  column = 11
                  line = 17
                  play = "Fail closed cases"
                  section = "tasks"
                  sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
                  task = "Doubly guarded child"
                  version = "ramen.ansible.provenance.v1"
                }
              }
            }
          }
          extensions {
            x-ramen-ansible-provenance {
              column = 11
              line = 17
              play = "Fail closed cases"
              section = "tasks"
              sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
              task = "Doubly guarded child"
              version = "ramen.ansible.provenance.v1"
            }
          }
        }
      }
      extensions {
        x-ramen-ansible-provenance {
          column = 11
          line = 17
          play = "Fail closed cases"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Doubly guarded child"
          version = "ramen.ansible.provenance.v1"
        }
      }
    }
    step "singly_guarded_child" {
      operationRef = "singly_guarded_child"
      when         = "$variables.env == \"prod\""
      extensions {
        x-ramen-ansible-provenance {
          column = 11
          line = 22
          play = "Fail closed cases"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Singly guarded child"
          version = "ramen.ansible.provenance.v1"
        }
      }
    }
    step "consumer_of_skipped_producer" {
      operationRef = "consumer_of_skipped_producer"
      extensions {
        x-ramen-ansible-provenance {
          column = 7
          line = 33
          play = "Fail closed cases"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Consumer of skipped producer"
          version = "ramen.ansible.provenance.v1"
        }
      }
    }
    step "consumer_before_future_producer" {
      operationRef = "consumer_before_future_producer"
      extensions {
        x-ramen-ansible-provenance {
          column = 7
          line = 38
          play = "Fail closed cases"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Consumer before future producer"
          version = "ramen.ansible.provenance.v1"
        }
      }
    }
    step "future_producer" {
      operationRef = "future_producer"
      extensions {
        x-ramen-ansible-provenance {
          column = 7
          line = 42
          play = "Fail closed cases"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Future producer"
          version = "ramen.ansible.provenance.v1"
        }
      }
    }
    step "guarded_restart_guard_1" {
      type = "switch"
      case "condition_1" {
        when = "$steps.consumer_of_skipped_producer.outputs.changed == true"
        step "guarded_restart_guard_2" {
          type = "switch"
          case "condition_2" {
            when = "$variables.env == \"prod\""
            step "guarded_restart" {
              operationRef = "guarded_restart"
              extensions {
                x-ramen-ansible-provenance {
                  column = 7
                  line = 58
                  play = "Fail closed cases"
                  section = "handlers"
                  sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
                  task = "guarded restart"
                  version = "ramen.ansible.provenance.v1"
                }
              }
            }
          }
          extensions {
            x-ramen-ansible-provenance {
              column = 7
              line = 58
              play = "Fail closed cases"
              section = "handlers"
              sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
              task = "guarded restart"
              version = "ramen.ansible.provenance.v1"
            }
          }
        }
      }
      extensions {
        x-ramen-ansible-provenance {
          column = 7
          line = 58
          play = "Fail closed cases"
          section = "handlers"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "guarded restart"
          version = "ramen.ansible.provenance.v1"
        }
      }
    }
  }
  components {
    variables {
      env = "prod"
    }
  }