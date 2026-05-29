resource "google_storage_bucket" "bucket" {
  name          = "ramen-corpus"
  location      = "EU"
  force_destroy = "true"
  lifecycle_rule {
    action {
      type = "Delete"
    }
    condition {
      send_age_if_zero = true
      age = 0
    }
  }
}
