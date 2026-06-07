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

variable "database_name" {
  type    = string
  default = "ramen-parity-c04-static"
}

resource "cloudflare_d1_database" "database" {
  account_id = var.account_id
  name       = var.database_name
}
