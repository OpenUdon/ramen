  uws = "1.4.0"
  info {
    title       = "ramen_native_project"
    description = "Native UWS/Ramen desired-state project generated from static Terraform/OpenTofu configuration."
    version     = "1.0.0"
  }
  sourceDescription "rbac" {
    url  = "openapi/rbac.json"
    type = "openapi"
  }
  operation "kubernetes_role_binding_v1_basic_create" {
    sourceDescription = "rbac"
    sourceOperationId = "createRbacAuthorizationV1NamespacedRoleBinding"
    description       = "Review create create for Terraform resource kubernetes_role_binding_v1.basic"
    request {
      body "metadata" {
        labels = "{\\\"app.kubernetes.io/managed-by\\\":\\\"ramen\\\",\\\"ramen.openudon.dev/corpus\\\":\\\"role-binding\\\"}"
        name = "\\\"ramen-corpus-role-binding\\\""
        namespace = "\\\"ramen-corpus\\\""
      }
      path {
        namespace = "\\\"ramen-corpus\\\""
      }
      x-ramen-terraform {
        attributes "metadata" {
          labels = "{\\\"app.kubernetes.io/managed-by\\\":\\\"ramen\\\",\\\"ramen.openudon.dev/corpus\\\":\\\"role-binding\\\"}"
          name = "\\\"ramen-corpus-role-binding\\\""
          namespace = "\\\"ramen-corpus\\\""
        }
        attributes "role_ref" {
          api_group = "\\\"rbac.authorization.k8s.io\\\""
          kind = "\\\"Role\\\""
          name = "\\\"ramen-corpus-role\\\""
        }
        attributes "subject" {
          namespace = "\\\"ramen-corpus\\\""
          kind = "\\\"ServiceAccount\\\""
          name = "\\\"ramen-corpus-service-account\\\""
        }
        identity_attributes = [
          {
            response_paths = [
              "metadata.name"
            ]
            required = true
            name = "name"
            terraform_path = "metadata.name"
            request_keys = [
              "name"
            ]
          },
          {
            request_keys = [
              "namespace"
            ]
            response_paths = [
              "metadata.namespace"
            ]
            required = true
            name = "namespace"
            terraform_path = "metadata.namespace"
          }
        ]
        object {
          kind = "resource"
          name = "basic"
          type = "kubernetes_role_binding_v1"
          address = "kubernetes_role_binding_v1.basic"
        }
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "kubernetes_role_binding_v1_basic_create" {
      operationRef = "kubernetes_role_binding_v1_basic_create"
      body {
        purpose = "create"
        terraform_address = "kubernetes_role_binding_v1.basic"
        terraform_type = "kubernetes_role_binding_v1"
        action = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      version = "ramen.project.v1"
      api_sources = [
        {
          kind = "openapi"
          id = "rbac"
          path = "openapi/rbac.json"
        }
      ]
      resources = [
        {
          lifecycle = {

          }
          redaction = {

          }
          address = "kubernetes_role_binding_v1.basic"
          kind = "resource"
          type = "kubernetes_role_binding_v1"
          identity_attributes = [
            {
              name = "name"
              path = "metadata.name"
              request_keys = [
                "name"
              ]
              response_paths = [
                "metadata.name"
              ]
              required = true
            },
            {
              name = "namespace"
              path = "metadata.namespace"
              request_keys = [
                "namespace"
              ]
              response_paths = [
                "metadata.namespace"
              ]
              required = true
            }
          ]
          attributes = {
            metadata = {
              name = "\\\"ramen-corpus-role-binding\\\""
              namespace = "\\\"ramen-corpus\\\""
              labels = "{\\\"app.kubernetes.io/managed-by\\\":\\\"ramen\\\",\\\"ramen.openudon.dev/corpus\\\":\\\"role-binding\\\"}"
            }
            role_ref = {
              api_group = "\\\"rbac.authorization.k8s.io\\\""
              kind = "\\\"Role\\\""
              name = "\\\"ramen-corpus-role\\\""
            }
            subject = {
              kind = "\\\"ServiceAccount\\\""
              name = "\\\"ramen-corpus-service-account\\\""
              namespace = "\\\"ramen-corpus\\\""
            }
          }
          metadata = {
            terraform_address = "kubernetes_role_binding_v1.basic"
          }
          name = "basic"
          operations = {
            create = {
              purpose = "create"
              source_kind = "openapi"
              source_id = "rbac"
              source_path = "openapi/rbac.json"
              operation_id = "createRbacAuthorizationV1NamespacedRoleBinding"
            }
          }
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/kubernetes/rbac/role_binding_v1/basic/input"
        source = "ramen convert tf"
      }
    }
  }