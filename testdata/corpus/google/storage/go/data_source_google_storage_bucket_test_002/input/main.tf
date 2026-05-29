resource "google_storage_bucket" "foo" {
  project = "ramen-corpus-project"
  name     = "ramen-corpus-bucket"
  location = "US"
}

// The project argument here doesn't help the provider retrieve data about the bucket
// It only serves to stop the data source using the compute API to convert the project number to an id
data "google_storage_bucket" "bar" {
  project = "ramen-corpus-project"
  name = google_storage_bucket.foo.name
  depends_on = [
    google_storage_bucket.foo,
  ]
}
