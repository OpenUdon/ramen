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
  operation "kubernetes_namespace_example_create" {
    sourceDescription = "core"
    sourceOperationId = "createCoreV1Namespace"
    description       = "Review create create for Terraform resource kubernetes_namespace.example"
    request {
      body "metadata" {
        name = "\\\"my-first-namespace\\\""
      }
      x-ramen-terraform {
        identity_attributes = [
          {
            response_paths = [
              "metadata.name"
            ]
            required = true
            name = "namespace_name"
            terraform_path = "metadata.name"
            request_keys = [
              "name"
            ]
          }
        ]
        object {
          kind = "resource"
          name = "example"
          type = "kubernetes_namespace"
          address = "kubernetes_namespace.example"
        }
        attributes "metadata" {
          name = "\\\"my-first-namespace\\\""
        }
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "kubernetes_namespace_example_create" {
      operationRef = "kubernetes_namespace_example_create"
      body {
        terraform_address = "kubernetes_namespace.example"
        terraform_type = "kubernetes_namespace"
        action = "create"
        purpose = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      resources = [
        {
          redaction = {

          }
          attributes = {
            metadata = {
              name = "\\\"my-first-namespace\\\""
            }
          }
          lifecycle = {

          }
          metadata = {
            terraform_address = "kubernetes_namespace.example"
          }
          name = "example"
          address = "kubernetes_namespace.example"
          identity_attributes = [
            {
              name = "namespace_name"
              path = "metadata.name"
              request_keys = [
                "name"
              ]
              response_paths = [
                "metadata.name"
              ]
              required = true
            }
          ]
          kind = "resource"
          type = "kubernetes_namespace"
          operations = {
            create = {
              purpose = "create"
              source_kind = "openapi"
              source_id = "core"
              source_path = "openapi/core.json"
              operation_id = "createCoreV1Namespace"
            }
          }
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/kubernetes/core/example_1/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          id = "core"
          path = "openapi/core.json"
          kind = "openapi"
        }
      ]
    }
  }