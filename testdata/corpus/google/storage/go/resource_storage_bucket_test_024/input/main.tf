resource "google_storage_bucket" "bucket" {
  name                    = "ramen-corpus"
  location                = "US"
  force_destroy           = "true"
  enable_object_retention = "ramen-corpus"
}
