resource "google_storage_bucket" "bucket" {
  name     = "ramen-corpus"
  location = "us-central1"
  uniform_bucket_level_access = true
  force_destroy = true
}
