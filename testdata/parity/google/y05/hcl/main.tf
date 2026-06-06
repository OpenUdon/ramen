terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
  }
}

variable "project" {
  type    = string
  default = "ramen-parity-y05-fixture-project"
}

variable "bucket_name" {
  type    = string
  default = "ramen-parity-y05-static"
}

provider "google" {
  project = var.project
}

resource "google_storage_bucket_object" "object" {
  bucket       = var.bucket_name
  name         = "ramen-parity-y05-object.txt"
  content      = "ramen-y05-static-fixture"
  content_type = "text/plain"

  metadata = {
    ramen_parity_lane  = "y05"
    ramen_parity_phase = "update"
  }
}
