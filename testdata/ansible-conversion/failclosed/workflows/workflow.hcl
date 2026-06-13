  uws = "1.6.0"
  info {
    title   = "Fail closed cases"
    version = "1.0.0"
  }
  sourceDescription "builtin" {
    url  = "testdata/argspec/ansible-builtin.argspec.json"
    type = "ansible-module"
  }
  operation "doubly_guarded_child" {
    sourceDescription = "builtin"
    sourceOperationId = "ansible.builtin.service"
    description       = "Doubly guarded child"
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
      x-ansible {
        version = "ramen.ansible.provenance.v1"
        column = 11
        line = 17
        play = "Fail closed cases"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        task = "Doubly guarded child"
      }
    }
  }
  operation "singly_guarded_child" {
    sourceDescription = "builtin"
    sourceOperationId = "ansible.builtin.service"
    description       = "Singly guarded child"
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
  operation "consumer_of_skipped_producer" {
    sourceDescription = "builtin"
    sourceOperationId = "ansible.builtin.shell"
    description       = "Consumer of skipped producer"
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
        line = 33
        play = "Fail closed cases"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        task = "Consumer of skipped producer"
        version = "ramen.ansible.provenance.v1"
        column = 7
      }
    }
  }
  operation "consumer_before_future_producer" {
    sourceDescription = "builtin"
    sourceOperationId = "ansible.builtin.shell"
    description       = "Consumer before future producer"
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
      x-ansible {
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        task = "Consumer before future producer"
        version = "ramen.ansible.provenance.v1"
        column = 7
        line = 38
        play = "Fail closed cases"
      }
    }
  }
  operation "future_producer" {
    sourceDescription = "builtin"
    sourceOperationId = "ansible.builtin.shell"
    description       = "Future producer"
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
        task = "Future producer"
        version = "ramen.ansible.provenance.v1"
        column = 7
        line = 42
        play = "Fail closed cases"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
      }
    }
  }
  operation "guarded_restart" {
    sourceDescription = "builtin"
    sourceOperationId = "ansible.builtin.service"
    description       = "guarded restart"
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
              version = "ramen.ansible.provenance.v1"
              column = 11
              line = 17
              play = "Fail closed cases"
              section = "tasks"
              sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
              task = "Doubly guarded child"
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
          play = "Fail closed cases"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Singly guarded child"
          version = "ramen.ansible.provenance.v1"
          column = 11
          line = 22
        }
      }
    }
    step "consumer_of_skipped_producer" {
      operationRef = "consumer_of_skipped_producer"
      extensions {
        x-ansible {
          version = "ramen.ansible.provenance.v1"
          column = 7
          line = 33
          play = "Fail closed cases"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Consumer of skipped producer"
        }
      }
    }
    step "consumer_before_future_producer" {
      operationRef = "consumer_before_future_producer"
      extensions {
        x-ansible {
          line = 38
          play = "Fail closed cases"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Consumer before future producer"
          version = "ramen.ansible.provenance.v1"
          column = 7
        }
      }
    }
    step "future_producer" {
      operationRef = "future_producer"
      extensions {
        x-ansible {
          version = "ramen.ansible.provenance.v1"
          column = 7
          line = 42
          play = "Fail closed cases"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
          task = "Future producer"
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
                  section = "handlers"
                  sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
                  task = "guarded restart"
                  version = "ramen.ansible.provenance.v1"
                  column = 7
                  line = 58
                  play = "Fail closed cases"
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
          task = "guarded restart"
          version = "ramen.ansible.provenance.v1"
          column = 7
          line = 58
          play = "Fail closed cases"
          section = "handlers"
          sourceFile = "../../testdata/ansible-conversion/failclosed/input/playbook.yml"
        }
      }
    }
  }
  components {
    variables {
      env = "prod"
    }
  }