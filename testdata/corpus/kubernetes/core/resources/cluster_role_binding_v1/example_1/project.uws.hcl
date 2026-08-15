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
  operation "kubernetes_cluster_role_binding_v1_k12_cluster_role_binding_create" {
    sourceDescription = "rbac"
    sourceOperationId = "createRbacAuthorizationV1ClusterRoleBinding"
    description       = "Review create create for Terraform resource kubernetes_cluster_role_binding_v1.k12_cluster_role_binding"
    request {
      body "metadata" {
        labels = "{\\\"app.kubernetes.io/managed-by\\\":\\\"ramen-parity\\\",\\\"ramen.openudon.dev/lane\\\":\\\"k12\\\"}"
        name = "var.cluster_role_binding_name"
      }
      x-ramen-terraform {
        attributes "metadata" {
          labels = "{\\\"app.kubernetes.io/managed-by\\\":\\\"ramen-parity\\\",\\\"ramen.openudon.dev/lane\\\":\\\"k12\\\"}"
          name = "var.cluster_role_binding_name"
        }
        attributes "role_ref" {
          kind = "\\\"ClusterRole\\\""
          name = "var.cluster_role_name"
          api_group = "\\\"rbac.authorization.k8s.io\\\""
        }
        attributes "subject" {
          kind = "\\\"ServiceAccount\\\""
          name = "var.service_account_name"
          namespace = "var.service_account_namespace"
        }
        identity_attributes = [
          {
            name = "name"
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
          address = "kubernetes_cluster_role_binding_v1.k12_cluster_role_binding"
          kind = "resource"
          name = "k12_cluster_role_binding"
          type = "kubernetes_cluster_role_binding_v1"
        }
        version = "ramen.terraform.provenance.v1"
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "kubernetes_cluster_role_binding_v1_k12_cluster_role_binding_create" {
      operationRef = "kubernetes_cluster_role_binding_v1_k12_cluster_role_binding_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "kubernetes_cluster_role_binding_v1.k12_cluster_role_binding"
        terraform_type = "kubernetes_cluster_role_binding_v1"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      version = "ramen.project.v1"
      api_sources = [
        {
          id = "rbac"
          path = "openapi/rbac.json"
          kind = "openapi"
        }
      ]
      resources = [
        {
          redaction = {

          }
          metadata = {
            terraform_address = "kubernetes_cluster_role_binding_v1.k12_cluster_role_binding"
          }
          type = "kubernetes_cluster_role_binding_v1"
          attributes = {
            subject = {
              kind = "\\\"ServiceAccount\\\""
              name = "var.service_account_name"
              namespace = "var.service_account_namespace"
            }
            metadata = {
              name = "var.cluster_role_binding_name"
              labels = "{\\\"app.kubernetes.io/managed-by\\\":\\\"ramen-parity\\\",\\\"ramen.openudon.dev/lane\\\":\\\"k12\\\"}"
            }
            role_ref = {
              kind = "\\\"ClusterRole\\\""
              name = "var.cluster_role_name"
              api_group = "\\\"rbac.authorization.k8s.io\\\""
            }
          }
          operations = {
            create = {
              source_id = "rbac"
              source_path = "openapi/rbac.json"
              operation_id = "createRbacAuthorizationV1ClusterRoleBinding"
              purpose = "create"
              source_kind = "openapi"
            }
          }
          address = "kubernetes_cluster_role_binding_v1.k12_cluster_role_binding"
          lifecycle = {

          }
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
            }
          ]
          kind = "resource"
          name = "k12_cluster_role_binding"
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/kubernetes/core/resources/cluster_role_binding_v1/example_1/input"
        source = "ramen convert tf"
      }
    }
  }
