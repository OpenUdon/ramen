resource "google_storage_bucket" "bucket" {
  name          = "ramen-corpus"
  storage_class = "ramen-corpus"
  location      = "ramen-corpus"
  force_destroy = true
}
