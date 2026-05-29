resource "google_storage_bucket" "foo" {
  name     = "ramen-corpus-bucket"
  location = "US"
}

data "google_storage_bucket" "bar" {
  name = google_storage_bucket.foo.name
  depends_on = [
    google_storage_bucket.foo,
  ]
}
