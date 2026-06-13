terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "3.1.0"
    }
  }
}

variable "kubeconfig_path" {
  type = string
}

variable "kube_context" {
  type = string
}

variable "cluster_role_binding_name" {
  type    = string
  default = "ramen-parity-k13-cluster-role-binding"
}

variable "cluster_role_name" {
  type    = string
  default = "ramen-parity-k13-cluster-role"
}

variable "service_account_name" {
  type    = string
  default = "ramen-parity-k13-service-account"
}

variable "service_account_namespace" {
  type    = string
  default = "ramen-parity-k13-subjects"
}

variable "phase" {
  type    = string
  default = "create"
}

provider "kubernetes" {
  config_path    = var.kubeconfig_path
  config_context = var.kube_context
}

resource "kubernetes_cluster_role_binding_v1" "k13_cluster_role_binding" {
  metadata {
    name = var.cluster_role_binding_name
    labels = {
      "app.kubernetes.io/managed-by" = "ramen-parity"
      "ramen.openudon.dev/lane"      = "k13"
      "ramen.openudon.dev/phase"     = var.phase
    }
  }

  # role_ref/subject intentionally reference an externally-managed ClusterRole
  # and ServiceAccount; this lane asserts the binding object's own fields
  # (roleRef, subjects, labels) round-trip, not referential integrity.
  # Kubernetes permits a ClusterRoleBinding to bind not-yet-existing identities.
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = var.cluster_role_name
  }

  subject {
    kind      = "ServiceAccount"
    name      = var.service_account_name
    namespace = var.service_account_namespace
  }
}
