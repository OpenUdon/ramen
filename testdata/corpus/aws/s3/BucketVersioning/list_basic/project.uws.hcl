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
        Bucket = "\\\"$${var.rName}-$${count.index}\\\""
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform "attributes" {
        bucket = "\\\"$${var.rName}-$${count.index}\\\""
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
          kind = "resource"
          name = "test"
          type = "aws_s3_bucket_versioning"
          address = "aws_s3_bucket_versioning.test"
        }
        attributes {
          bucket = "aws_s3_bucket.test[count.index].bucket"
          versioning_configuration {
            status = "\\\"Enabled\\\""
          }
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
        Bucket = "aws_s3_bucket.test[count.index].bucket"
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
      version = "ramen.project.v1"
      api_sources = [
        {
          path = "aws-smithy/s3.json"
          kind = "aws-smithy"
          id = "s3"
        }
      ]
      resources = [
        {
          credential_bindings = [
            "aws_hmac"
          ]
          redaction = {

          }
          attributes = {
            bucket = "\\\"$${var.rName}-$${count.index}\\\""
          }
          lifecycle = {

          }
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          address = "aws_s3_bucket.test"
          kind = "resource"
          type = "aws_s3_bucket"
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
        },
        {
          lifecycle = {

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
          redaction = {

          }
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
          credential_bindings = [
            "aws_hmac"
          ]
          type = "aws_s3_bucket_versioning"
          attributes = {
            bucket = "aws_s3_bucket.test[count.index].bucket"
            versioning_configuration = {
              status = "\\\"Enabled\\\""
            }
          }
          metadata = {
            terraform_address = "aws_s3_bucket_versioning.test"
          }
          name = "test"
          address = "aws_s3_bucket_versioning.test"
          kind = "resource"
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/aws/s3/BucketVersioning/list_basic/input"
        source = "ramen convert tf"
        action = "create"
      }
    }
  }