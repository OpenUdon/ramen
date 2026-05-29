resource "google_storage_bucket" "bucket" {
  name                     = "ramen-corpus"
  location                 = "US"
  default_event_based_hold = true
  force_destroy            = true
}
