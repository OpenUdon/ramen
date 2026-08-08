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
      x-uws-operation-profile = "uws.ansible-module-call.1.0"
      x-ansible {
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        task = "Doubly guarded child"
        version = "ramen.ansible.provenance.v1"
        column = 11
        line = 17
        play = "Fail closed cases"
      }
      x-uws-ansible-module {
        argspec {
          collection = "ansible.builtin"
          sourceId = "builtin"
          url = "testdata/argspec/ansible-builtin.argspec.json"
        }
        module = "ansible.builtin.service"
      }
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
      x-ansible {
        task = "Singly guarded child"
        version = "ramen.ansible.provenance.v1"
        column = 11
        line = 22
        play = "Fail closed cases"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
      }
      x-uws-ansible-module {
        argspec {
          collection = "ansible.builtin"
          sourceId = "builtin"
          url = "testdata/argspec/ansible-builtin.argspec.json"
        }
        module = "ansible.builtin.service"
      }
      x-uws-operation-profile = "uws.ansible-module-call.1.0"
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
      x-ansible {
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        task = "Consumer of skipped producer"
        version = "ramen.ansible.provenance.v1"
        column = 7
        line = 33
        play = "Fail closed cases"
        section = "tasks"
      }
      x-uws-ansible-module {
        argspec {
          collection = "ansible.builtin"
          sourceId = "builtin"
          url = "testdata/argspec/ansible-builtin.argspec.json"
        }
        module = "ansible.builtin.shell"
      }
      x-uws-operation-profile = "uws.ansible-module-call.1.0"
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
      x-uws-operation-profile = "uws.ansible-module-call.1.0"
      x-ansible {
        task = "Consumer before future producer"
        version = "ramen.ansible.provenance.v1"
        column = 7
        line = 38
        play = "Fail closed cases"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
      }
      x-uws-ansible-module {
        argspec {
          collection = "ansible.builtin"
          sourceId = "builtin"
          url = "testdata/argspec/ansible-builtin.argspec.json"
        }
        module = "ansible.builtin.shell"
      }
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
      x-ansible {
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        task = "Future producer"
        version = "ramen.ansible.provenance.v1"
        column = 7
        line = 42
        play = "Fail closed cases"
      }
      x-uws-ansible-module {
        argspec {
          sourceId = "builtin"
          url = "testdata/argspec/ansible-builtin.argspec.json"
          collection = "ansible.builtin"
        }
        module = "ansible.builtin.shell"
      }
      x-uws-operation-profile = "uws.ansible-module-call.1.0"
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
      x-ansible {
        version = "ramen.ansible.provenance.v1"
        column = 7
        line = 58
        play = "Fail closed cases"
        section = "handlers"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        task = "guarded restart"
      }
      x-uws-ansible-module {
        argspec {
          url = "testdata/argspec/ansible-builtin.argspec.json"
          collection = "ansible.builtin"
          sourceId = "builtin"
        }
        module = "ansible.builtin.service"
      }
      x-uws-operation-profile = "uws.ansible-module-call.1.0"
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
                x-ansible {
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
            x-ansible {
              line = 17
              play = "Fail closed cases"
              section = "tasks"
              sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
              task = "Doubly guarded child"
              version = "ramen.ansible.provenance.v1"
              column = 11
            }
          }
        }
      }
      extensions {
        x-ansible {
          line = 17
          play = "Fail closed cases"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Doubly guarded child"
          version = "ramen.ansible.provenance.v1"
          column = 11
        }
      }
    }
    step "singly_guarded_child" {
      operationRef = "singly_guarded_child"
      when         = "$variables.env == \"prod\""
      extensions {
        x-ansible {
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Singly guarded child"
          version = "ramen.ansible.provenance.v1"
          column = 11
          line = 22
          play = "Fail closed cases"
          section = "tasks"
        }
      }
    }
    step "consumer_of_skipped_producer" {
      operationRef = "consumer_of_skipped_producer"
      extensions {
        x-ansible {
          play = "Fail closed cases"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Consumer of skipped producer"
          version = "ramen.ansible.provenance.v1"
          column = 7
          line = 33
        }
      }
    }
    step "consumer_before_future_producer" {
      operationRef = "consumer_before_future_producer"
      extensions {
        x-ansible {
          play = "Fail closed cases"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Consumer before future producer"
          version = "ramen.ansible.provenance.v1"
          column = 7
          line = 38
        }
      }
    }
    step "future_producer" {
      operationRef = "future_producer"
      extensions {
        x-ansible {
          line = 42
          play = "Fail closed cases"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Future producer"
          version = "ramen.ansible.provenance.v1"
          column = 7
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
                x-ansible {
                  version = "ramen.ansible.provenance.v1"
                  column = 7
                  line = 58
                  play = "Fail closed cases"
                  section = "handlers"
                  sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
                  task = "guarded restart"
                }
              }
            }
          }
          extensions {
            x-ansible {
              sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
              task = "guarded restart"
              version = "ramen.ansible.provenance.v1"
              column = 7
              line = 58
              play = "Fail closed cases"
              section = "handlers"
            }
          }
        }
      }
      extensions {
        x-ansible {
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "guarded restart"
          version = "ramen.ansible.provenance.v1"
          column = 7
          line = 58
          play = "Fail closed cases"
          section = "handlers"
        }
      }
    }
  }
  components {
    variables {
      env = "prod"
    }
  }