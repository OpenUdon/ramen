terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

provider "aws" {
  region = var.region
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "user_name" {
  type    = string
  default = "ramen-parity-w01-static"
}

resource "aws_iam_user" "test" {
  name = var.user_name
}
