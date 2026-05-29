resource "google_storage_bucket" "bucket" {
  name          = "ramen-corpus"
  location      = "US"
  force_destroy = true
  labels = {
    my-label = "my-label-value"
  }
}
