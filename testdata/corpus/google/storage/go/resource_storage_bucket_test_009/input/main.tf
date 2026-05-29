resource "google_storage_bucket" "bucket" {
  name          = "ramen-corpus"
  location      = "ASIA"
  force_destroy = true
  custom_placement_config {
    data_locations = ["asia-east1", "asia-southeast1"]
  }
}
