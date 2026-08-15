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
        annotations = "{\\\"name\\\":\\\"example-annotation\\\"}"
        labels = "{\\\"mylabel\\\":\\\"label-value\\\"}"
        name = "\\\"terraform-example-namespace\\\""
      }
      x-ramen-terraform {
        attributes "metadata" {
          annotations = "{\\\"name\\\":\\\"example-annotation\\\"}"
          labels = "{\\\"mylabel\\\":\\\"label-value\\\"}"
          name = "\\\"terraform-example-namespace\\\""
        }
        identity_attributes = [
          {
            name = "namespace_name"
            request_keys = [
              "name"
            ]
            required = true
            response_paths = [
              "metadata.name"
            ]
            terraform_path = "metadata.name"
          }
        ]
        object {
          address = "kubernetes_namespace.example"
          kind = "resource"
          name = "example"
          type = "kubernetes_namespace"
        }
        version = "ramen.terraform.provenance.v1"
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
      api_sources = [
        {
          id = "core"
          kind = "openapi"
          path = "openapi/core.json"
        }
      ]
      metadata {
        action = "create"
        config_dir = "testdata/corpus/kubernetes/core/resources/namespace/example_1/input"
        source = "ramen convert tf"
      }
      redaction {

      }
      resources = [
        {
          address = "kubernetes_namespace.example"
          attributes = {
            metadata = {
              annotations = "{\\\"name\\\":\\\"example-annotation\\\"}"
              labels = "{\\\"mylabel\\\":\\\"label-value\\\"}"
              name = "\\\"terraform-example-namespace\\\""
            }
          }
          identity_attributes = [
            {
              name = "namespace_name"
              path = "metadata.name"
              request_keys = [
                "name"
              ]
              required = true
              response_paths = [
                "metadata.name"
              ]
            }
          ]
          kind = "resource"
          lifecycle = {

          }
          metadata = {
            terraform_address = "kubernetes_namespace.example"
          }
          name = "example"
          operations = {
            create = {
              operation_id = "createCoreV1Namespace"
              purpose = "create"
              source_id = "core"
              source_kind = "openapi"
              source_path = "openapi/core.json"
            }
          }
          redaction = {

          }
          type = "kubernetes_namespace"
        }
      ]
      version = "ramen.project.v1"
    }
  }