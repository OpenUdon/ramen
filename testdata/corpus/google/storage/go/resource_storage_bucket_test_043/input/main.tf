resource "google_storage_bucket" "website" {
  name          = "ramen-corpus.gcp.tfacc.hashicorptest.com"
  location      = "US"
  storage_class = "STANDARD"
  force_destroy = true

  website {
  }
}
