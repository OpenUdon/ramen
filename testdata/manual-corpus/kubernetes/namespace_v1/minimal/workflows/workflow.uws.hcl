  uws = "1.4.0"
  info {
    title       = "terraform_conversion_draft"
    description = "Draft review scaffold generated from static Terraform/OpenTofu configuration."
    version     = "1.0.0"
  }
  sourceDescription "core" {
    url  = "openapi/core.json"
    type = "openapi"
  }
  operation "kubernetes_namespace_v1_manual_create" {
    sourceDescription = "core"
    sourceOperationId = "createCoreV1Namespace"
    description       = "Review create create for Terraform resource kubernetes_namespace_v1.manual"
    request {
      body "metadata" {
        name = "\\\"ramen-manual-corpus\\\""
      }
      x-ramen-terraform {
        object {
          type = "kubernetes_namespace_v1"
          address = "kubernetes_namespace_v1.manual"
          kind = "resource"
          name = "manual"
        }
        attributes "metadata" {
          name = "\\\"ramen-manual-corpus\\\""
        }
        identity_attributes = [
          {
            required = true
            name = "namespace_name"
            terraform_path = "metadata.name"
            request_keys = [
              "name"
            ]
            response_paths = [
              "metadata.name"
            ]
          }
        ]
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "kubernetes_namespace_v1_manual_create" {
      operationRef = "kubernetes_namespace_v1_manual_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "kubernetes_namespace_v1.manual"
        terraform_type = "kubernetes_namespace_v1"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      redaction {

      }
      metadata {
        config_dir = "testdata/manual-corpus/kubernetes/namespace_v1/minimal/input"
        source = "ramen convert tf"
        action = "create"
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
              required = true
              name = "namespace_name"
              path = "metadata.name"
              request_keys = [
                "name"
              ]
              response_paths = [
                "metadata.name"
              ]
            }
          ]
          operations = {
            create = {
              source_id = "core"
              source_path = "openapi/core.json"
              operation_id = "createCoreV1Namespace"
              purpose = "create"
              source_kind = "openapi"
            }
          }
          metadata = {
            terraform_address = "kubernetes_namespace_v1.manual"
          }
          kind = "resource"
          type = "kubernetes_namespace_v1"
          attributes = {
            metadata = {
              name = "\\\"ramen-manual-corpus\\\""
            }
          }
          redaction = {

          }
          address = "kubernetes_namespace_v1.manual"
          name = "manual"
        }
      ]
    }
  }