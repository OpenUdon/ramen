resource "google_storage_bucket" "bucket" {
  name                        = "ramen-corpus"
  location                    = "EU"
  uniform_bucket_level_access = true
  hierarchical_namespace {
	enabled = ramen-corpus
  }
  force_destroy = ramen-corpus
}
