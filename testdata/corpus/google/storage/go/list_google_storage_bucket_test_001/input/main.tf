resource "google_storage_bucket" "test" {
  name     = ramen-corpus
  location = "US"
  project  = ramen-corpus
}
