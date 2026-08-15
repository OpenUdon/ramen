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
          api_group = "\\\"rbac.authorization.k8s.io\\\""
          kind = "\\\"ClusterRole\\\""
          name = "var.cluster_role_name"
        }
        attributes "subject" {
          kind = "\\\"ServiceAccount\\\""
          name = "var.service_account_name"
          namespace = "var.service_account_namespace"
        }
        identity_attributes = [
          {
            name = "name"
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
      api_sources = [
        {
          id = "rbac"
          kind = "openapi"
          path = "openapi/rbac.json"
        }
      ]
      metadata {
        action = "create"
        config_dir = "testdata/corpus/kubernetes/core/resources/cluster_role_binding_v1/example_1/input"
        source = "ramen convert tf"
      }
      redaction {

      }
      resources = [
        {
          address = "kubernetes_cluster_role_binding_v1.k12_cluster_role_binding"
          attributes = {
            metadata = {
              labels = "{\\\"app.kubernetes.io/managed-by\\\":\\\"ramen-parity\\\",\\\"ramen.openudon.dev/lane\\\":\\\"k12\\\"}"
              name = "var.cluster_role_binding_name"
            }
            role_ref = {
              api_group = "\\\"rbac.authorization.k8s.io\\\""
              kind = "\\\"ClusterRole\\\""
              name = "var.cluster_role_name"
            }
            subject = {
              kind = "\\\"ServiceAccount\\\""
              name = "var.service_account_name"
              namespace = "var.service_account_namespace"
            }
          }
          identity_attributes = [
            {
              name = "name"
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
            terraform_address = "kubernetes_cluster_role_binding_v1.k12_cluster_role_binding"
          }
          name = "k12_cluster_role_binding"
          operations = {
            create = {
              operation_id = "createRbacAuthorizationV1ClusterRoleBinding"
              purpose = "create"
              source_id = "rbac"
              source_kind = "openapi"
              source_path = "openapi/rbac.json"
            }
          }
          redaction = {

          }
          type = "kubernetes_cluster_role_binding_v1"
        }
      ]
      version = "ramen.project.v1"
    }
  }