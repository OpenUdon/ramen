  uws = "1.4.0"
  info {
    title       = "ramen_native_project"
    description = "Native UWS/Ramen desired-state project generated from static Terraform/OpenTofu configuration."
    version     = "1.0.0"
  }
  sourceDescription "core" {
    url  = "openapi/core.json"
    type = "openapi"
  }
  operation "kubernetes_cluster_role_v1_example_create" {
    sourceDescription = "core"
    sourceOperationId = "createRbacAuthorizationV1ClusterRole"
    description       = "Review create create for Terraform resource kubernetes_cluster_role_v1.example"
    request {
      body "metadata" {
        name = "\\\"terraform-example\\\""
      }
      x-ramen-terraform {
        attributes "metadata" {
          name = "\\\"terraform-example\\\""
        }
        attributes "rule" {
          api_groups = "[\\\"\\\"]"
          resources = "[\\\"namespaces\\\",\\\"pods\\\"]"
          verbs = "[\\\"get\\\",\\\"list\\\",\\\"watch\\\"]"
        }
        identity_attributes = [
          {
            required = true
            name = "name"
            terraform_path = "metadata.name"
            request_keys = [
              "name"
            ]
            response_paths = [
              "metadata.name"
            ]
          }
        ]
        object {
          kind = "resource"
          name = "example"
          type = "kubernetes_cluster_role_v1"
          address = "kubernetes_cluster_role_v1.example"
        }
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "kubernetes_cluster_role_v1_example_create" {
      operationRef = "kubernetes_cluster_role_v1_example_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "kubernetes_cluster_role_v1.example"
        terraform_type = "kubernetes_cluster_role_v1"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      api_sources = [
        {
          kind = "openapi"
          id = "core"
          path = "openapi/core.json"
        }
      ]
      resources = [
        {
          identity_attributes = [
            {
              response_paths = [
                "metadata.name"
              ]
              required = true
              name = "name"
              path = "metadata.name"
              request_keys = [
                "name"
              ]
            }
          ]
          redaction = {

          }
          attributes = {
            metadata = {
              name = "\\\"terraform-example\\\""
            }
            rule = {
              api_groups = "[\\\"\\\"]"
              resources = "[\\\"namespaces\\\",\\\"pods\\\"]"
              verbs = "[\\\"get\\\",\\\"list\\\",\\\"watch\\\"]"
            }
          }
          lifecycle = {

          }
          address = "kubernetes_cluster_role_v1.example"
          kind = "resource"
          type = "kubernetes_cluster_role_v1"
          operations = {
            create = {
              source_kind = "openapi"
              source_id = "core"
              source_path = "openapi/core.json"
              operation_id = "createRbacAuthorizationV1ClusterRole"
              purpose = "create"
            }
          }
          metadata = {
            terraform_address = "kubernetes_cluster_role_v1.example"
          }
          name = "example"
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/kubernetes/core/resources/cluster_role_v1/example_1/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
    }
  }