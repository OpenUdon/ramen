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

variable "role_name" {
  type = string
}

provider "kubernetes" {
  config_path    = var.kubeconfig_path
  config_context = var.kube_context
}

resource "kubernetes_role_v1" "k05_role" {
  metadata {
    name      = var.role_name
    namespace = var.namespace_name
    labels = {
      "app.kubernetes.io/managed-by" = "ramen-parity"
      "ramen.openudon.dev/lane"      = "k05"
    }
  }

  rule {
    api_groups = [""]
    resources  = ["configmaps", "secrets"]
    verbs      = ["get", "list", "watch"]
  }
}
