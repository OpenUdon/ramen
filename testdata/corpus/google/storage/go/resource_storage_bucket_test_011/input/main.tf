resource "google_storage_bucket" "bucket" {
  name          = "ramen-corpus"
  location      = "EU"
  force_destroy = "true"
}
