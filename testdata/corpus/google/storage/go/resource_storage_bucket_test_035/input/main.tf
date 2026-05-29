resource "google_storage_bucket" "bucket" {
  name                        = "ramen-corpus"
  location                    = "US"
  uniform_bucket_level_access = ramen-corpus
  force_destroy               = true
}
