terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

provider "aws" {
  region = "us-east-1"
}

variable "bucket" {
  type    = string
  default = "ramen-parity-w03-fixture-bucket"
}

resource "aws_s3_bucket_public_access_block" "test" {
  bucket                  = var.bucket
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
