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
        region = "var.region"
        bucket = "var.rName"
      }
      x-ramen-terraform "object" {
        name = "test"
        type = "aws_s3_bucket"
        address = "aws_s3_bucket.test"
        kind = "resource"
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "aws_s3_bucket_test_create" {
      operationRef = "aws_s3_bucket_test_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "aws_s3_bucket.test"
        terraform_type = "aws_s3_bucket"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      api_sources = [
        {
          kind = "aws-smithy"
          id = "s3"
          path = "aws-smithy/s3.json"
        }
      ]
      resources = [
        {
          address = "aws_s3_bucket.test"
          kind = "resource"
          type = "aws_s3_bucket"
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          name = "test"
          lifecycle = {

          }
          attributes = {
            bucket = "var.rName"
            region = "var.region"
          }
          operations = {
            create = {
              operation_id = "CreateBucket"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "s3"
              source_path = "aws-smithy/s3.json"
            }
          }
          redaction = {

          }
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/aws/s3/Bucket/region_override/input"
        source = "ramen convert tf"
        action = "create"
      }
      version = "ramen.project.v1"
    }
  }