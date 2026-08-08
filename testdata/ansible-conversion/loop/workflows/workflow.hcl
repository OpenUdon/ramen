  uws = "1.5.0"
  info {
    title   = "Create application directories"
    version = "1.0.0"
  }
  operation "create_app_directories" {
    description = "Create app directories"
    outputs = {
      changed = "$response.body.changed"
    }
    request {
      body {
        path = "$item"
        state = "directory"
      }
    }
    successCriterion {
      condition = "$response.body.failed != true"
    }
    extensions {
      x-ansible {
        column = 7
        line = 4
        play = "Create application directories"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/loop/input/playbook.yml"
        task = "Create app directories"
        version = "ramen.ansible.provenance.v1"
      }
      x-uws-ansible-module {
        argspec {
          collection = "ansible.builtin"
          sourceId = "builtin"
          url = "testdata/argspec/ansible-builtin.argspec.json"
        }
        module = "ansible.builtin.file"
      }
      x-uws-operation-profile = "uws.ansible-module-call.1.0"
    }
  }
  workflow "main" {
    type = "sequence"
    step "create_app_directories" {
      operationRef = "create_app_directories"
      forEach      = "$variables.create_app_directories_items"
      extensions {
        x-ansible {
          column = 7
          line = 4
          play = "Create application directories"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/loop/input/playbook.yml"
          task = "Create app directories"
          version = "ramen.ansible.provenance.v1"
        }
      }
    }
  }
  components {
    variables {
      create_app_directories_items = [
        "/etc/app",
        "/var/log/app"
      ]
    }
  }