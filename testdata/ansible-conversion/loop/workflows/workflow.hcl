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
  }
  workflow "main" {
    type = "sequence"
    step "create_app_directories" {
      operationRef = "create_app_directories"
      forEach      = "$variables.create_app_directories_items"
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