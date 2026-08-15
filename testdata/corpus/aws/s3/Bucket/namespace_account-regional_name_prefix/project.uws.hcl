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
      x-ramen-terraform {
        attributes {
          bucket_namespace = "\\\"account-regional\\\""
          bucket_prefix = "var.rName"
        }
        object {
          address = "aws_s3_bucket.test"
          kind = "resource"
          name = "test"
          type = "aws_s3_bucket"
        }
        version = "ramen.terraform.provenance.v1"
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
          id = "s3"
          kind = "aws-smithy"
          path = "aws-smithy/s3.json"
        }
      ]
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/s3/Bucket/namespace_account-regional_name_prefix/input"
        source = "ramen convert tf"
      }
      redaction {

      }
      resources = [
        {
          address = "aws_s3_bucket.test"
          attributes = {
            bucket_namespace = "\\\"account-regional\\\""
            bucket_prefix = "var.rName"
          }
          credential_bindings = [
            "aws_hmac"
          ]
          kind = "resource"
          lifecycle = {

          }
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          name = "test"
          operations = {
            create = {
              credential_bindings = [
                "aws_hmac"
              ]
              operation_id = "CreateBucket"
              purpose = "create"
              source_id = "s3"
              source_kind = "aws-smithy"
              source_path = "aws-smithy/s3.json"
            }
          }
          redaction = {

          }
          type = "aws_s3_bucket"
        }
      ]
      version = "ramen.project.v1"
    }
  }