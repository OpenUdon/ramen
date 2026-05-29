resource "google_storage_bucket" "bucket" {
  name          = "ramen-corpus"
  location      = "US"
  force_destroy = true

  soft_delete_policy {
    retention_duration_seconds = 1
  }
}
