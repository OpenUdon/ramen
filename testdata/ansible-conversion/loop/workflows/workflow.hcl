  uws = "1.6.0"
  info {
    title   = "Create application directories"
    version = "1.0.0"
  }
  sourceDescription "builtin" {
    url  = "testdata/argspec/ansible-builtin.argspec.json"
    type = "ansible-module"
  }
  operation "create_app_directories" {
    sourceDescription = "builtin"
    sourceOperationId = "ansible.builtin.file"
    description       = "Create app directories"
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
        version = "ramen.ansible.provenance.v1"
        column = 7
        line = 4
        play = "Create application directories"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/loop/input/playbook.yml"
        task = "Create app directories"
      }
    }
  }
  workflow "main" {
    type = "sequence"
    step "create_app_directories" {
      operationRef = "create_app_directories"
      forEach      = "$variables.create_app_directories_items"
      extensions {
        x-ansible {
          task = "Create app directories"
          version = "ramen.ansible.provenance.v1"
          column = 7
          line = 4
          play = "Create application directories"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/loop/input/playbook.yml"
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