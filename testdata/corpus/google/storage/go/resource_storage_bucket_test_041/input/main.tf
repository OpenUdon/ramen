resource "google_storage_bucket" "bucket" {
  name          = "ramen-corpus"
  location      = "US"
  force_destroy = true

  retention_policy {
    is_locked        = true
    retention_period = "10"
  }
}
