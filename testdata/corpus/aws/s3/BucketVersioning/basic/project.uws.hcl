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
  operation "aws_s3_bucket_versioning_test_create" {
    sourceDescription = "s3"
    sourceOperationId = "PutBucketVersioning"
    description       = "Review create create for Terraform resource aws_s3_bucket_versioning.test"
    request {
      path {
        Bucket = "aws_s3_bucket.test.bucket"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        identity_attributes = [
          {
            name = "bucket"
            terraform_path = "bucket"
            request_keys = [
              "Bucket"
            ]
            required = true
          }
        ]
        object {
          name = "test"
          type = "aws_s3_bucket_versioning"
          address = "aws_s3_bucket_versioning.test"
          kind = "resource"
        }
        attributes {
          versioning_configuration {
            status = "\\\"Enabled\\\""
          }
          bucket = "aws_s3_bucket.test.bucket"
        }
      }
      body "VersioningConfiguration" {
        Status = "\\\"Enabled\\\""
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
    step "aws_s3_bucket_versioning_test_create" {
      operationRef = "aws_s3_bucket_versioning_test_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "aws_s3_bucket_versioning.test"
        terraform_type = "aws_s3_bucket_versioning"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/s3/BucketVersioning/basic/input"
        source = "ramen convert tf"
      }
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
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          address = "aws_s3_bucket.test"
          name = "test"
          redaction = {

          }
          kind = "resource"
          type = "aws_s3_bucket"
          lifecycle = {

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
          attributes = {
            bucket = "var.rName"
          }
          credential_bindings = [
            "aws_hmac"
          ]
        },
        {
          operations = {
            create = {
              source_path = "aws-smithy/s3.json"
              operation_id = "PutBucketVersioning"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "s3"
            }
          }
          address = "aws_s3_bucket_versioning.test"
          type = "aws_s3_bucket_versioning"
          attributes = {
            bucket = "aws_s3_bucket.test.bucket"
            versioning_configuration = {
              status = "\\\"Enabled\\\""
            }
          }
          name = "test"
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
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_s3_bucket_versioning.test"
          }
          kind = "resource"
          dependencies = [
            "aws_s3_bucket.test"
          ]
          redaction = {

          }
          lifecycle = {

          }
        }
      ]
    }
  }