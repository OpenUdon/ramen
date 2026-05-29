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
        object {
          kind = "resource"
          name = "test"
          type = "aws_s3_bucket_versioning"
          address = "aws_s3_bucket_versioning.test"
        }
        attributes {
          bucket = "aws_s3_bucket.test.bucket"
          versioning_configuration {
            status = "\\\"Enabled\\\""
          }
        }
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
          redaction = {

          }
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          address = "aws_s3_bucket.test"
          type = "aws_s3_bucket"
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
          credential_bindings = [
            "aws_hmac"
          ]
          kind = "resource"
          name = "test"
          attributes = {
            bucket = "var.rName"
          }
        },
        {
          name = "test"
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
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_s3_bucket_versioning.test"
          }
          kind = "resource"
          type = "aws_s3_bucket_versioning"
          attributes = {
            bucket = "aws_s3_bucket.test.bucket"
            versioning_configuration = {
              status = "\\\"Enabled\\\""
            }
          }
          redaction = {

          }
          address = "aws_s3_bucket_versioning.test"
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/aws/s3/tmpl/bucket_versioning_basic/input"
        source = "ramen convert tf"
        action = "create"
      }
    }
  }