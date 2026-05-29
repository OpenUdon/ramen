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
        labels = "{\\\"mylabel\\\":\\\"label-value\\\"}"
        name = "\\\"terraform-example-namespace\\\""
        annotations = "{\\\"name\\\":\\\"example-annotation\\\"}"
      }
      x-ramen-terraform {
        attributes "metadata" {
          annotations = "{\\\"name\\\":\\\"example-annotation\\\"}"
          labels = "{\\\"mylabel\\\":\\\"label-value\\\"}"
          name = "\\\"terraform-example-namespace\\\""
        }
        identity_attributes = [
          {
            request_keys = [
              "name"
            ]
            response_paths = [
              "metadata.name"
            ]
            required = true
            name = "namespace_name"
            terraform_path = "metadata.name"
          }
        ]
        object {
          kind = "resource"
          name = "example"
          type = "kubernetes_namespace"
          address = "kubernetes_namespace.example"
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
        action = "create"
        purpose = "create"
        terraform_address = "kubernetes_namespace.example"
        terraform_type = "kubernetes_namespace"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      redaction {

      }
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/kubernetes/core/resources/namespace/example_1/input"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          path = "openapi/core.json"
          kind = "openapi"
          id = "core"
        }
      ]
      resources = [
        {
          lifecycle = {

          }
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
          redaction = {

          }
          metadata = {
            terraform_address = "kubernetes_namespace.example"
          }
          address = "kubernetes_namespace.example"
          operations = {
            create = {
              source_kind = "openapi"
              source_id = "core"
              source_path = "openapi/core.json"
              operation_id = "createCoreV1Namespace"
              purpose = "create"
            }
          }
          kind = "resource"
          type = "kubernetes_namespace"
          name = "example"
          attributes = {
            metadata = {
              annotations = "{\\\"name\\\":\\\"example-annotation\\\"}"
              labels = "{\\\"mylabel\\\":\\\"label-value\\\"}"
              name = "\\\"terraform-example-namespace\\\""
            }
          }
        }
      ]
    }
  }