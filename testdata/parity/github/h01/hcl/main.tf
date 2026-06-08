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
  default = "ramen-parity-h01-static"
}

provider "github" {
  owner = var.github_owner
}

resource "github_repository" "repo" {
  name        = var.repository_name
  description = "Ramen GitHub H01 parity fixture"
  visibility  = "private"
  has_issues  = true
  auto_init   = true
}
