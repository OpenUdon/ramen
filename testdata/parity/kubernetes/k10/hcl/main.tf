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

variable "cluster_role_name" {
  type = string
}

variable "phase" {
  type    = string
  default = "create"
}

variable "include_pods" {
  type    = bool
  default = false
}

provider "kubernetes" {
  config_path    = var.kubeconfig_path
  config_context = var.kube_context
}

resource "kubernetes_cluster_role_v1" "k10_cluster_role" {
  metadata {
    name = var.cluster_role_name
    labels = {
      "app.kubernetes.io/managed-by" = "ramen-parity"
      "ramen.openudon.dev/lane"      = "k10"
      "ramen.openudon.dev/phase"     = var.phase
    }
  }

  rule {
    api_groups = [""]
    resources  = var.include_pods ? ["configmaps", "pods", "secrets"] : ["configmaps", "secrets"]
    verbs      = ["get", "list", "watch"]
  }
}
