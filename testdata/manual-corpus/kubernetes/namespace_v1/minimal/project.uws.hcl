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
  operation "kubernetes_namespace_v1_manual_create" {
    sourceDescription = "core"
    sourceOperationId = "createCoreV1Namespace"
    description       = "Review create create for Terraform resource kubernetes_namespace_v1.manual"
    request {
      body "metadata" {
        name = "\\\"ramen-manual-corpus\\\""
      }
      x-ramen-terraform {
        attributes "metadata" {
          name = "\\\"ramen-manual-corpus\\\""
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
          address = "kubernetes_namespace_v1.manual"
          kind = "resource"
          name = "manual"
          type = "kubernetes_namespace_v1"
        }
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "kubernetes_namespace_v1_manual_create" {
      operationRef = "kubernetes_namespace_v1_manual_create"
      body {
        terraform_type = "kubernetes_namespace_v1"
        action = "create"
        purpose = "create"
        terraform_address = "kubernetes_namespace_v1.manual"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/manual-corpus/kubernetes/namespace_v1/minimal/input"
      }
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
          metadata = {
            terraform_address = "kubernetes_namespace_v1.manual"
          }
          lifecycle = {

          }
          name = "manual"
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
          attributes = {
            metadata = {
              name = "\\\"ramen-manual-corpus\\\""
            }
          }
          redaction = {

          }
          address = "kubernetes_namespace_v1.manual"
          kind = "resource"
          type = "kubernetes_namespace_v1"
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
    }
  }