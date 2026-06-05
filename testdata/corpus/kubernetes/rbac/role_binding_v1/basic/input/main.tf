terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "3.1.0"
    }
  }
}

resource "kubernetes_role_binding_v1" "basic" {
  metadata {
    name      = "ramen-corpus-role-binding"
    namespace = "ramen-corpus"
    labels = {
      "app.kubernetes.io/managed-by" = "ramen"
      "ramen.openudon.dev/corpus"    = "role-binding"
    }
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "Role"
    name      = "ramen-corpus-role"
  }

  subject {
    kind      = "ServiceAccount"
    name      = "ramen-corpus-service-account"
    namespace = "ramen-corpus"
  }
}
