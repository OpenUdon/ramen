resource "cloudflare_r2_bucket" "ramen_corpus" {
    account_id    = "023e105f4ecef8ad9ca31a8372d0c353"
    name          = "ramen_corpus"
    location      = "ENAM"
    storage_class = "InfrequentAccess"
  }
