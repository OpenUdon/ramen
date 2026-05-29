resource "google_storage_bucket" "bucket" {
  name          = "ramen-corpus"
  location      = "US"
  force_destroy = true
  lifecycle_rule {
    action {
      type = "Delete"
    }

    condition {
      age        = 10
      with_state = "ANY"
    }
  }
}
