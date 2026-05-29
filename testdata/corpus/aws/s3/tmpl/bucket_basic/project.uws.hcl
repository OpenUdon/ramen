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
      }
      x-ramen-terraform "object" {
        address = "aws_s3_bucket.test"
        kind = "resource"
        name = "test"
        type = "aws_s3_bucket"
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "aws_s3_bucket_test_create" {
      operationRef = "aws_s3_bucket_test_create"
      body {
        terraform_address = "aws_s3_bucket.test"
        terraform_type = "aws_s3_bucket"
        action = "create"
        purpose = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      resources = [
        {
          name = "test"
          operations = {
            create = {
              source_kind = "aws-smithy"
              source_id = "s3"
              source_path = "aws-smithy/s3.json"
              operation_id = "CreateBucket"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
            }
          }
          redaction = {

          }
          kind = "resource"
          type = "aws_s3_bucket"
          lifecycle = {

          }
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_s3_bucket.test"
          attributes = {
            bucket = "var.rName"
          }
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/aws/s3/tmpl/bucket_basic/input"
        source = "ramen convert tf"
        action = "create"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          id = "s3"
          path = "aws-smithy/s3.json"
          kind = "aws-smithy"
        }
      ]
    }
  }