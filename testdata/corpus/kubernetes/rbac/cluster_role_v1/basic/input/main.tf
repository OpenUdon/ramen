terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "3.1.0"
    }
  }
}

resource "kubernetes_cluster_role_v1" "basic" {
  metadata {
    name = "ramen-corpus-cluster-role"
    labels = {
      "app.kubernetes.io/managed-by" = "ramen"
      "ramen.openudon.dev/corpus"    = "cluster-role"
    }
  }

  rule {
    api_groups = [""]
    resources  = ["configmaps", "secrets"]
    verbs      = ["get", "list", "watch"]
  }
}
