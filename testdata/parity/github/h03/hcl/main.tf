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
  default = "ramen-parity-h03-static"
}

provider "github" {
  owner = var.github_owner
}

resource "github_repository_file" "file" {
  repository     = var.repository_name
  file           = "ramen-parity-h03.txt"
  content        = "ramen h03 create"
  branch         = "main"
  commit_message = "Create Ramen H03 parity file"
}
