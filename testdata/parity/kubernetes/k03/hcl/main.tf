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

variable "config_map_name" {
  type = string
}

variable "mode" {
  type = string
}

variable "payload" {
  type = string
}

provider "kubernetes" {
  config_path    = var.kubeconfig_path
  config_context = var.kube_context
}

resource "kubernetes_config_map_v1" "k03_config_map" {
  metadata {
    name      = var.config_map_name
    namespace = var.namespace_name
    labels = {
      "app.kubernetes.io/managed-by" = "ramen-parity"
      "ramen.openudon.dev/lane"      = "k03"
    }
  }

  data = {
    mode  = var.mode
    owner = "ramen"
  }

  binary_data = {
    payload = var.payload
  }
}
