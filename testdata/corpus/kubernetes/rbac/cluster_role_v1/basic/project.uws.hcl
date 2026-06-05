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
  operation "kubernetes_cluster_role_v1_basic_create" {
    sourceDescription = "rbac"
    sourceOperationId = "createRbacAuthorizationV1ClusterRole"
    description       = "Review create create for Terraform resource kubernetes_cluster_role_v1.basic"
    request {
      body "metadata" {
        labels = "{\\\"app.kubernetes.io/managed-by\\\":\\\"ramen\\\",\\\"ramen.openudon.dev/corpus\\\":\\\"cluster-role\\\"}"
        name = "\\\"ramen-corpus-cluster-role\\\""
      }
      x-ramen-terraform {
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
          }
        ]
        object {
          address = "kubernetes_cluster_role_v1.basic"
          kind = "resource"
          name = "basic"
          type = "kubernetes_cluster_role_v1"
        }
        attributes "metadata" {
          labels = "{\\\"app.kubernetes.io/managed-by\\\":\\\"ramen\\\",\\\"ramen.openudon.dev/corpus\\\":\\\"cluster-role\\\"}"
          name = "\\\"ramen-corpus-cluster-role\\\""
        }
        attributes "rule" {
          resources = "[\\\"configmaps\\\",\\\"secrets\\\"]"
          verbs = "[\\\"get\\\",\\\"list\\\",\\\"watch\\\"]"
          api_groups = "[\\\"\\\"]"
        }
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "kubernetes_cluster_role_v1_basic_create" {
      operationRef = "kubernetes_cluster_role_v1_basic_create"
      body {
        terraform_type = "kubernetes_cluster_role_v1"
        action = "create"
        purpose = "create"
        terraform_address = "kubernetes_cluster_role_v1.basic"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      metadata {
        action = "create"
        config_dir = "testdata/corpus/kubernetes/rbac/cluster_role_v1/basic/input"
        source = "ramen convert tf"
      }
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
          identity_attributes = [
            {
              path = "metadata.name"
              request_keys = [
                "name"
              ]
              response_paths = [
                "metadata.name"
              ]
              required = true
              name = "name"
            }
          ]
          redaction = {

          }
          kind = "resource"
          attributes = {
            rule = {
              verbs = "[\\\"get\\\",\\\"list\\\",\\\"watch\\\"]"
              api_groups = "[\\\"\\\"]"
              resources = "[\\\"configmaps\\\",\\\"secrets\\\"]"
            }
            metadata = {
              labels = "{\\\"app.kubernetes.io/managed-by\\\":\\\"ramen\\\",\\\"ramen.openudon.dev/corpus\\\":\\\"cluster-role\\\"}"
              name = "\\\"ramen-corpus-cluster-role\\\""
            }
          }
          operations = {
            create = {
              source_kind = "openapi"
              source_id = "rbac"
              source_path = "openapi/rbac.json"
              operation_id = "createRbacAuthorizationV1ClusterRole"
              purpose = "create"
            }
          }
          address = "kubernetes_cluster_role_v1.basic"
          name = "basic"
          metadata = {
            terraform_address = "kubernetes_cluster_role_v1.basic"
          }
          type = "kubernetes_cluster_role_v1"
        }
      ]
      redaction {

      }
    }
  }