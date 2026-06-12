resource "kubernetes_namespace_v1" "manual" {
  metadata {
    name = "ramen-manual-corpus"
  }
}
