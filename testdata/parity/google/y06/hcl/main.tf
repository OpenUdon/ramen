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
  default = "ramen-parity-y06-fixture-project"
}

variable "bucket_name" {
  type    = string
  default = "ramen-parity-y06-static"
}

provider "google" {
  project = var.project
}

resource "google_storage_managed_folder" "folder" {
  bucket        = var.bucket_name
  name          = "managed/y06/"
  force_destroy = false
}
