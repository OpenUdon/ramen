resource "google_storage_bucket" "bucket" {
  name     = "ramen-corpus"
  location = "US"
  uniform_bucket_level_access = true
  hierarchical_namespace {
    enabled = ramen-corpus
  }
  force_destroy= true
}
