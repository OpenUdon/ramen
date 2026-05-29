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
        Bucket = "\"$${var.rName}-$${count.index}\""
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform "attributes" {
        tags = "var.resource_tags"
        bucket = "\"$${var.rName}-$${count.index}\""
      }
      x-ramen-terraform "object" {
        type = "aws_s3_bucket"
        address = "aws_s3_bucket.test"
        kind = "resource"
        name = "test"
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
          lifecycle = {

          }
          operations = {
            create = {
              source_id = "s3"
              source_path = "aws-smithy/s3.json"
              operation_id = "CreateBucket"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
            }
          }
          kind = "resource"
          credential_bindings = [
            "aws_hmac"
          ]
          attributes = {
            bucket = "\"$${var.rName}-$${count.index}\""
            tags = "var.resource_tags"
          }
          redaction = {

          }
          type = "aws_s3_bucket"
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          address = "aws_s3_bucket.test"
          name = "test"
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/s3/Bucket/list_include_resource/input"
        source = "ramen convert tf"
      }
    }
  }