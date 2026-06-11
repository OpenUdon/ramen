  uws = "1.6.0"
  info {
    title   = "Fail closed cases"
    version = "1.0.0"
  }
  sourceDescription "builtin" {
    url  = "testdata/argspec/ansible-builtin.argspec.json"
    type = "ansible-module"
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
  }
  workflow "main" {
    type = "sequence"
    step "singly_guarded_child" {
      operationRef = "singly_guarded_child"
      when         = "$variables.env == \"prod\""
    }
    step "consumer_of_skipped_producer" {
      operationRef = "consumer_of_skipped_producer"
    }
    step "consumer_before_future_producer" {
      operationRef = "consumer_before_future_producer"
    }
    step "future_producer" {
      operationRef = "future_producer"
    }
  }
  components {
    variables {
      env = "prod"
    }
  }