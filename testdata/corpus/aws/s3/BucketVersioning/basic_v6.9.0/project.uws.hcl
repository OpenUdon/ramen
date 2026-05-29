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
      body "VersioningConfiguration" {
        Status = "\\\"Enabled\\\""
      }
      path {
        Bucket = "aws_s3_bucket.test.bucket"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          bucket = "aws_s3_bucket.test.bucket"
          versioning_configuration {
            status = "\\\"Enabled\\\""
          }
        }
        identity_attributes = [
          {
            required = true
            name = "bucket"
            terraform_path = "bucket"
            request_keys = [
              "Bucket"
            ]
          }
        ]
        object {
          address = "aws_s3_bucket_versioning.test"
          kind = "resource"
          name = "test"
          type = "aws_s3_bucket_versioning"
        }
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
          kind = "resource"
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          attributes = {
            bucket = "var.rName"
          }
          credential_bindings = [
            "aws_hmac"
          ]
          redaction = {

          }
          type = "aws_s3_bucket"
          address = "aws_s3_bucket.test"
          name = "test"
          lifecycle = {

          }
        },
        {
          attributes = {
            bucket = "aws_s3_bucket.test.bucket"
            versioning_configuration = {
              status = "\\\"Enabled\\\""
            }
          }
          address = "aws_s3_bucket_versioning.test"
          type = "aws_s3_bucket_versioning"
          lifecycle = {

          }
          dependencies = [
            "aws_s3_bucket.test"
          ]
          identity_attributes = [
            {
              name = "bucket"
              path = "bucket"
              request_keys = [
                "Bucket"
              ]
              required = true
            }
          ]
          metadata = {
            terraform_address = "aws_s3_bucket_versioning.test"
          }
          kind = "resource"
          name = "test"
          operations = {
            create = {
              source_id = "s3"
              source_path = "aws-smithy/s3.json"
              operation_id = "PutBucketVersioning"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
            }
          }
          redaction = {

          }
          credential_bindings = [
            "aws_hmac"
          ]
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/s3/BucketVersioning/basic_v6.9.0/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
    }
  }