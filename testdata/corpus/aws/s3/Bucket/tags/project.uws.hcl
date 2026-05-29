  uws = "1.4.0"
  info {
    title       = "ramen_native_project"
    description = "Native UWS/Ramen desired-state project generated from static Terraform/OpenTofu configuration."
    version     = "1.0.0"
  }
  sourceDescription "s3" {
    url  = "aws-smithy/s3.json"
    type = "aws-smithy"
  }
  operation "aws_s3_bucket_test_create" {
    sourceDescription = "s3"
    sourceOperationId = "CreateBucket"
    description       = "Review create create for Terraform resource aws_s3_bucket.test"
    request {
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform "attributes" {
        tags = "var.resource_tags"
        bucket = "var.rName"
      }
      x-ramen-terraform "object" {
        name = "test"
        type = "aws_s3_bucket"
        address = "aws_s3_bucket.test"
        kind = "resource"
      }
      path {
        Bucket = "var.rName"
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "aws_s3_bucket_test_create" {
      operationRef = "aws_s3_bucket_test_create"
      body {
        purpose = "create"
        terraform_address = "aws_s3_bucket.test"
        terraform_type = "aws_s3_bucket"
        action = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      version = "ramen.project.v1"
      api_sources = [
        {
          kind = "aws-smithy"
          id = "s3"
          path = "aws-smithy/s3.json"
        }
      ]
      resources = [
        {
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_s3_bucket.test"
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          name = "test"
          attributes = {
            bucket = "var.rName"
            tags = "var.resource_tags"
          }
          lifecycle = {

          }
          operations = {
            create = {
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "s3"
              source_path = "aws-smithy/s3.json"
              operation_id = "CreateBucket"
              credential_bindings = [
                "aws_hmac"
              ]
            }
          }
          type = "aws_s3_bucket"
          redaction = {

          }
          kind = "resource"
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/s3/Bucket/tags/input"
        source = "ramen convert tf"
      }
    }
  }