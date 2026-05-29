resource "google_storage_bucket" "bucket" {
  name          = "ramen-corpus"
  location      = "US"
  force_destroy = true

  retention_policy {
    retention_period = "3600"
  }
}
