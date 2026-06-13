  uws = "1.6.0"
  info {
    title   = "Configure nginx"
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
        name = "$variables.pkg"
        state = "present"
      }
    }
    successCriterion {
      condition = "$response.body.failed != true"
    }
    extensions {
      x-ansible {
        play = "Configure nginx"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/nginx/input/playbook.yml"
        task = "Install nginx"
        version = "ramen.ansible.provenance.v1"
        column = 7
        line = 6
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
        line = 11
        play = "Configure nginx"
        section = "tasks"
        sourceFile = "../../testdata/ansible-conversion/nginx/input/playbook.yml"
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
        task = "restart nginx"
        version = "ramen.ansible.provenance.v1"
        column = 7
        line = 18
        play = "Configure nginx"
        section = "handlers"
        sourceFile = "../../testdata/ansible-conversion/nginx/input/playbook.yml"
      }
    }
  }
  workflow "main" {
    type = "sequence"
    step "install_nginx" {
      operationRef = "install_nginx"
      extensions {
        x-ansible {
          column = 7
          line = 6
          play = "Configure nginx"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/nginx/input/playbook.yml"
          task = "Install nginx"
          version = "ramen.ansible.provenance.v1"
        }
      }
    }
    step "deploy_nginx_config" {
      operationRef = "deploy_nginx_config"
      extensions {
        x-ansible {
          version = "ramen.ansible.provenance.v1"
          column = 7
          line = 11
          play = "Configure nginx"
          section = "tasks"
          sourceFile = "../../testdata/ansible-conversion/nginx/input/playbook.yml"
          task = "Deploy nginx config"
        }
      }
    }
    step "restart_nginx" {
      operationRef = "restart_nginx"
      when         = "$steps.deploy_nginx_config.outputs.changed == true"
      extensions {
        x-ansible {
          sourceFile = "../../testdata/ansible-conversion/nginx/input/playbook.yml"
          task = "restart nginx"
          version = "ramen.ansible.provenance.v1"
          column = 7
          line = 18
          play = "Configure nginx"
          section = "handlers"
        }
      }
    }
  }
  components {
    variables {
      pkg = "nginx"
    }
  }