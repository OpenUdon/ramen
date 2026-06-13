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
    extensions {
      x-ansible {
        column = 7
        line = 4
        play = "Configure nginx with multiple notifiers"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/multinotify/input/playbook.yml"
        task = "Install nginx"
        version = "ramen.ansible.provenance.v1"
      }
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
    extensions {
      x-ansible {
        task = "Deploy nginx config"
        version = "ramen.ansible.provenance.v1"
        column = 7
        line = 10
        play = "Configure nginx with multiple notifiers"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/multinotify/input/playbook.yml"
      }
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
    extensions {
      x-ansible {
        column = 7
        line = 17
        play = "Configure nginx with multiple notifiers"
        section = "handlers"
        sourceFile = "../../testdata/ansible-conversion/multinotify/input/playbook.yml"
        task = "restart nginx"
        version = "ramen.ansible.provenance.v1"
      }
    }
  }
  workflow "main" {
    type = "sequence"
    step "install_nginx" {
      operationRef = "install_nginx"
      extensions {
        x-ansible {
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/multinotify/input/playbook.yml"
          task = "Install nginx"
          version = "ramen.ansible.provenance.v1"
          column = 7
          line = 4
          play = "Configure nginx with multiple notifiers"
        }
      }
    }
    step "deploy_nginx_config" {
      operationRef = "deploy_nginx_config"
      extensions {
        x-ansible {
          version = "ramen.ansible.provenance.v1"
          column = 7
          line = 10
          play = "Configure nginx with multiple notifiers"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/multinotify/input/playbook.yml"
          task = "Deploy nginx config"
        }
      }
    }
    step "restart_nginx_notify" {
      type = "switch"
      case "notified_by_install_nginx" {
        when = "$steps.install_nginx.outputs.changed == true"
        step "restart_nginx_run_1" {
          operationRef = "restart_nginx"
          extensions {
            x-ansible {
              line = 17
              play = "Configure nginx with multiple notifiers"
              section = "handlers"
              sourceFile = "../../testdata/ansible-conversion/multinotify/input/playbook.yml"
              task = "restart nginx"
              version = "ramen.ansible.provenance.v1"
              column = 7
            }
          }
        }
      }
      case "notified_by_deploy_nginx_config" {
        when = "$steps.deploy_nginx_config.outputs.changed == true"
        step "restart_nginx_run_2" {
          operationRef = "restart_nginx"
          extensions {
            x-ansible {
              line = 17
              play = "Configure nginx with multiple notifiers"
              section = "handlers"
              sourceFile = "../../testdata/ansible-conversion/multinotify/input/playbook.yml"
              task = "restart nginx"
              version = "ramen.ansible.provenance.v1"
              column = 7
            }
          }
        }
      }
      extensions {
        x-ansible {
          version = "ramen.ansible.provenance.v1"
          column = 7
          line = 17
          play = "Configure nginx with multiple notifiers"
          section = "handlers"
          sourceFile = "../../testdata/ansible-conversion/multinotify/input/playbook.yml"
          task = "restart nginx"
        }
      }
    }
  }