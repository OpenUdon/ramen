resource "google_storage_bucket" "bucket" {
  name          = "ramen-corpus"
  location      = "eu"
  force_destroy = true
}
