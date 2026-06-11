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

variable "namespace_name" {
  type = string
}

variable "role_binding_name" {
  type = string
}

variable "role_name" {
  type = string
}

variable "service_account_name" {
  type = string
}

variable "phase" {
  type    = string
  default = "create"
}

provider "kubernetes" {
  config_path    = var.kubeconfig_path
  config_context = var.kube_context
}

resource "kubernetes_role_binding_v1" "k09_role_binding" {
  metadata {
    name      = var.role_binding_name
    namespace = var.namespace_name
    labels = {
      "app.kubernetes.io/managed-by" = "ramen-parity"
      "ramen.openudon.dev/lane"      = "k09"
      "ramen.openudon.dev/phase"     = var.phase
    }
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "Role"
    name      = var.role_name
  }

  subject {
    kind      = "ServiceAccount"
    name      = var.service_account_name
    namespace = var.namespace_name
  }
}
