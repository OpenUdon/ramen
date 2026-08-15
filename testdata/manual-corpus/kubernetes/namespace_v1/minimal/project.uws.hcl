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
          address = "kubernetes_namespace_v1.manual"
          kind = "resource"
          name = "manual"
          type = "kubernetes_namespace_v1"
        }
        version = "ramen.terraform.provenance.v1"
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
      api_sources = [
        {
          id = "core"
          kind = "openapi"
          path = "openapi/core.json"
        }
      ]
      metadata {
        action = "create"
        config_dir = "testdata/manual-corpus/kubernetes/namespace_v1/minimal/input"
        source = "ramen convert tf"
      }
      redaction {

      }
      resources = [
        {
          address = "kubernetes_namespace_v1.manual"
          attributes = {
            metadata = {
              name = "\\\"ramen-manual-corpus\\\""
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
            terraform_address = "kubernetes_namespace_v1.manual"
          }
          name = "manual"
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
          type = "kubernetes_namespace_v1"
        }
      ]
      version = "ramen.project.v1"
    }
  }