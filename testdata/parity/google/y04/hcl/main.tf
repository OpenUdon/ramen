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
  default = "ramen-parity-y04-fixture-project"
}

variable "bucket_name" {
  type    = string
  default = "ramen-parity-y04-static"
}

variable "location" {
  type    = string
  default = "US"
}

provider "google" {
  project = var.project
}

resource "google_storage_bucket" "bucket" {
  name                        = var.bucket_name
  location                    = var.location
  uniform_bucket_level_access = true
}
