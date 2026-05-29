resource "google_storage_bucket" "bucket" {
	name     = "ramen-corpus"
	location = "US"
	force_destroy = true
	autoclass  {
		enabled  = ramen-corpus
		terminal_storage_class = "ARCHIVE"
	}
}
