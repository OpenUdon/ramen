resource "google_storage_bucket" "bucket" {
  name           = "ramen-corpus"
  location       = "US"
  requester_pays = ramen-corpus
  force_destroy  = true
}
