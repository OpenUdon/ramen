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
        region = "var.region"
      }
      x-ramen-terraform "object" {
        address = "aws_s3_bucket.test"
        kind = "resource"
        name = "test"
        type = "aws_s3_bucket"
      }
    }
  }
  operation "aws_s3_bucket_versioning_test_create" {
    sourceDescription = "s3"
    sourceOperationId = "PutBucketVersioning"
    description       = "Review create create for Terraform resource aws_s3_bucket_versioning.test"
    request {
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        object {
          type = "aws_s3_bucket_versioning"
          address = "aws_s3_bucket_versioning.test"
          kind = "resource"
          name = "test"
        }
        attributes {
          versioning_configuration {
            status = "\\\"Enabled\\\""
          }
          bucket = "aws_s3_bucket.test.bucket"
          region = "var.region"
        }
        identity_attributes = [
          {
            terraform_path = "bucket"
            request_keys = [
              "Bucket"
            ]
            required = true
            name = "bucket"
          }
        ]
      }
      body "VersioningConfiguration" {
        Status = "\\\"Enabled\\\""
      }
      path {
        Bucket = "aws_s3_bucket.test.bucket"
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
    step "aws_s3_bucket_versioning_test_create" {
      operationRef = "aws_s3_bucket_versioning_test_create"
      body {
        purpose = "create"
        terraform_address = "aws_s3_bucket_versioning.test"
        terraform_type = "aws_s3_bucket_versioning"
        action = "create"
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
          redaction = {

          }
          operations = {
            create = {
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "s3"
              source_path = "aws-smithy/s3.json"
              operation_id = "CreateBucket"
            }
          }
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_s3_bucket.test"
          type = "aws_s3_bucket"
          lifecycle = {

          }
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          kind = "resource"
          name = "test"
          attributes = {
            bucket = "var.rName"
            region = "var.region"
          }
        },
        {
          name = "test"
          credential_bindings = [
            "aws_hmac"
          ]
          attributes = {
            versioning_configuration = {
              status = "\\\"Enabled\\\""
            }
            bucket = "aws_s3_bucket.test.bucket"
            region = "var.region"
          }
          identity_attributes = [
            {
              required = true
              name = "bucket"
              path = "bucket"
              request_keys = [
                "Bucket"
              ]
            }
          ]
          address = "aws_s3_bucket_versioning.test"
          dependencies = [
            "aws_s3_bucket.test"
          ]
          operations = {
            create = {
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "s3"
              source_path = "aws-smithy/s3.json"
              operation_id = "PutBucketVersioning"
              credential_bindings = [
                "aws_hmac"
              ]
            }
          }
          metadata = {
            terraform_address = "aws_s3_bucket_versioning.test"
          }
          type = "aws_s3_bucket_versioning"
          lifecycle = {

          }
          redaction = {

          }
          kind = "resource"
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/s3/BucketVersioning/region_override/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
    }
  }