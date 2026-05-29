resource "google_storage_bucket" "bucket" {
  name                      = "ramen-corpus"
  location                  = "US"
  force_destroy             = true
  uniform_bucket_level_access = true
  encryption  {
	google_managed_encryption_enforcement_config {
      restriction_mode = "ramen-corpus"
    }
  }
}
