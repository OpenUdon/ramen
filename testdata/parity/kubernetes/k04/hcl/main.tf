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

variable "service_account_name" {
  type = string
}

provider "kubernetes" {
  config_path    = var.kubeconfig_path
  config_context = var.kube_context
}

resource "kubernetes_service_account_v1" "k04_service_account" {
  metadata {
    name      = var.service_account_name
    namespace = var.namespace_name
    labels = {
      "app.kubernetes.io/managed-by" = "ramen-parity"
      "ramen.openudon.dev/lane"      = "k04"
    }
  }

  automount_service_account_token = true
}
