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
  default = "ramen-parity-w04-fixture-bucket"
}

resource "aws_s3_bucket_versioning" "test" {
  bucket = var.bucket

  versioning_configuration {
    status = "Enabled"
  }
}
