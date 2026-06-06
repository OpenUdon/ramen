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
  default = "ramen-parity-y02-fixture-project"
}

variable "bucket_name" {
  type    = string
  default = "ramen-parity-y02-existing"
}

provider "google" {
  project = var.project
}

data "google_storage_bucket" "bucket" {
  name = var.bucket_name
}
