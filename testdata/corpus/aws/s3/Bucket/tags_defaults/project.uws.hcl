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
      path {
        Bucket = "var.rName"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform "attributes" {
        bucket = "var.rName"
        tags = "var.resource_tags"
      }
      x-ramen-terraform "object" {
        kind = "resource"
        name = "test"
        type = "aws_s3_bucket"
        address = "aws_s3_bucket.test"
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "aws_s3_bucket_test_create" {
      operationRef = "aws_s3_bucket_test_create"
      body {
        terraform_type = "aws_s3_bucket"
        action = "create"
        purpose = "create"
        terraform_address = "aws_s3_bucket.test"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      api_sources = [
        {
          id = "s3"
          path = "aws-smithy/s3.json"
          kind = "aws-smithy"
        }
      ]
      resources = [
        {
          kind = "resource"
          name = "test"
          attributes = {
            bucket = "var.rName"
            tags = "var.resource_tags"
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
          redaction = {

          }
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          lifecycle = {

          }
          credential_bindings = [
            "aws_hmac"
          ]
          type = "aws_s3_bucket"
          address = "aws_s3_bucket.test"
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/s3/Bucket/tags_defaults/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
    }
  }