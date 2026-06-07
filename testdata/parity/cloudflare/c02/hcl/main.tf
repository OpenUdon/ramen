terraform {
  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.0"
    }
  }
}

variable "account_id" {
  type    = string
  default = "cloudflare-account-placeholder"
}

variable "bucket_name" {
  type    = string
  default = "ramen-parity-c02-static"
}

resource "cloudflare_r2_bucket" "bucket" {
  account_id = var.account_id
  name       = var.bucket_name
}
