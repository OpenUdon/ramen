terraform {
  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.0"
    }
  }
}

variable "github_owner" {
  type    = string
  default = "github-owner-placeholder"
}

variable "repository_name" {
  type    = string
  default = "ramen-parity-h02-static"
}

provider "github" {
  owner = var.github_owner
}

resource "github_issue_label" "label" {
  repository  = var.repository_name
  name        = "ramen-parity-h02"
  color       = "0e8a16"
  description = "Ramen GitHub H02 parity fixture"
}
