resource "google_storage_bucket" "bucket" {
  name          = "ramen-corpus"
  location      = "US"
  force_destroy = true
  logging {
    log_bucket        = "ramen-corpus"
    log_object_prefix = "ramen-corpus"
  }
}
