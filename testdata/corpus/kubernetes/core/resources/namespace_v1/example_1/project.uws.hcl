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
  operation "kubernetes_namespace_v1_example_create" {
    sourceDescription = "core"
    sourceOperationId = "createCoreV1Namespace"
    description       = "Review create create for Terraform resource kubernetes_namespace_v1.example"
    request {
      x-ramen-terraform {
        attributes "metadata" {
          annotations = "{\\\"name\\\":\\\"example-annotation\\\"}"
          labels = "{\\\"mylabel\\\":\\\"label-value\\\"}"
          name = "\\\"terraform-example-namespace\\\""
        }
        identity_attributes = [
          {
            name = "namespace_name"
            terraform_path = "metadata.name"
            request_keys = [
              "name"
            ]
            response_paths = [
              "metadata.name"
            ]
            required = true
          }
        ]
        object {
          name = "example"
          type = "kubernetes_namespace_v1"
          address = "kubernetes_namespace_v1.example"
          kind = "resource"
        }
      }
      body "metadata" {
        labels = "{\\\"mylabel\\\":\\\"label-value\\\"}"
        name = "\\\"terraform-example-namespace\\\""
        annotations = "{\\\"name\\\":\\\"example-annotation\\\"}"
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "kubernetes_namespace_v1_example_create" {
      operationRef = "kubernetes_namespace_v1_example_create"
      body {
        terraform_type = "kubernetes_namespace_v1"
        action = "create"
        purpose = "create"
        terraform_address = "kubernetes_namespace_v1.example"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      version = "ramen.project.v1"
      api_sources = [
        {
          kind = "openapi"
          id = "core"
          path = "openapi/core.json"
        }
      ]
      resources = [
        {
          kind = "resource"
          type = "kubernetes_namespace_v1"
          name = "example"
          attributes = {
            metadata = {
              annotations = "{\\\"name\\\":\\\"example-annotation\\\"}"
              labels = "{\\\"mylabel\\\":\\\"label-value\\\"}"
              name = "\\\"terraform-example-namespace\\\""
            }
          }
          operations = {
            create = {
              source_id = "core"
              source_path = "openapi/core.json"
              operation_id = "createCoreV1Namespace"
              purpose = "create"
              source_kind = "openapi"
            }
          }
          redaction = {

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
          metadata = {
            terraform_address = "kubernetes_namespace_v1.example"
          }
          lifecycle = {

          }
          address = "kubernetes_namespace_v1.example"
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/kubernetes/core/resources/namespace_v1/example_1/input"
        source = "ramen convert tf"
      }
    }
  }