resource "google_storage_bucket" "bucket" {
  name                      = "ramen-corpus"
  location                  = "US"
  public_access_prevention  = "ramen-corpus"
  force_destroy             = true
}
