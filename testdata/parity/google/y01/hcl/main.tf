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
  default = "ramen-parity-y01-fixture-project"
}

provider "google" {
  project = var.project
}

resource "google_storage_bucket" "bucket" {
  name                        = "ramen-parity-y01-static"
  location                    = "US"
  uniform_bucket_level_access = true
}
