  uws = "1.6.0"
  info {
    title   = "Configure nginx with multiple notifiers"
    version = "1.0.0"
  }
  sourceDescription "builtin" {
    url  = "testdata/argspec/ansible-builtin.argspec.json"
    type = "ansible-module"
  }
  operation "install_nginx" {
    sourceDescription = "builtin"
    sourceOperationId = "ansible.builtin.apt"
    description       = "Install nginx"
    outputs = {
      changed = "$response.body.changed"
    }
    request {
      body {
        name = "nginx"
        state = "present"
      }
    }
    successCriterion {
      condition = "$response.body.failed != true"
    }
  }
  operation "deploy_nginx_config" {
    sourceDescription = "builtin"
    sourceOperationId = "ansible.builtin.template"
    description       = "Deploy nginx config"
    outputs = {
      changed = "$response.body.changed"
    }
    request {
      body {
        dest = "/etc/nginx/nginx.conf"
        src = "nginx.conf.j2"
      }
    }
    successCriterion {
      condition = "$response.body.failed != true"
    }
  }
  operation "restart_nginx" {
    sourceDescription = "builtin"
    sourceOperationId = "ansible.builtin.service"
    description       = "restart nginx"
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
  }
  workflow "main" {
    type = "sequence"
    step "install_nginx" {
      operationRef = "install_nginx"
    }
    step "deploy_nginx_config" {
      operationRef = "deploy_nginx_config"
    }
    step "restart_nginx_notify" {
      type = "switch"
      case "notified_by_install_nginx" {
        when = "$steps.install_nginx.outputs.changed == true"
        step "restart_nginx_run_1" {
          operationRef = "restart_nginx"
        }
      }
      case "notified_by_deploy_nginx_config" {
        when = "$steps.deploy_nginx_config.outputs.changed == true"
        step "restart_nginx_run_2" {
          operationRef = "restart_nginx"
        }
      }
    }
  }