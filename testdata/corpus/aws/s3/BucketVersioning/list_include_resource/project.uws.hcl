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
      x-ramen-terraform "attributes" {
        bucket = "\\\"$${var.rName}-$${count.index}\\\""
      }
      x-ramen-terraform "object" {
        kind = "resource"
        name = "test"
        type = "aws_s3_bucket"
        address = "aws_s3_bucket.test"
      }
      path {
        Bucket = "\\\"$${var.rName}-$${count.index}\\\""
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
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
        Bucket = "aws_s3_bucket.test[count.index].bucket"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
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
          name = "test"
          type = "aws_s3_bucket_versioning"
          address = "aws_s3_bucket_versioning.test"
          kind = "resource"
        }
        attributes {
          bucket = "aws_s3_bucket.test[count.index].bucket"
          versioning_configuration {
            status = "\\\"Enabled\\\""
          }
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
        action = "create"
        purpose = "create"
        terraform_address = "aws_s3_bucket.test"
        terraform_type = "aws_s3_bucket"
      }
    }
    step "aws_s3_bucket_versioning_test_create" {
      operationRef = "aws_s3_bucket_versioning_test_create"
      body {
        terraform_type = "aws_s3_bucket_versioning"
        action = "create"
        purpose = "create"
        terraform_address = "aws_s3_bucket_versioning.test"
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
          type = "aws_s3_bucket"
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          address = "aws_s3_bucket.test"
          kind = "resource"
          operations = {
            create = {
              source_path = "aws-smithy/s3.json"
              operation_id = "CreateBucket"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "s3"
            }
          }
          attributes = {
            bucket = "\\\"$${var.rName}-$${count.index}\\\""
          }
          redaction = {

          }
          name = "test"
        },
        {
          address = "aws_s3_bucket_versioning.test"
          type = "aws_s3_bucket_versioning"
          name = "test"
          lifecycle = {

          }
          kind = "resource"
          attributes = {
            bucket = "aws_s3_bucket.test[count.index].bucket"
            versioning_configuration = {
              status = "\\\"Enabled\\\""
            }
          }
          dependencies = [
            "aws_s3_bucket.test"
          ]
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
          credential_bindings = [
            "aws_hmac"
          ]
          identity_attributes = [
            {
              path = "bucket"
              request_keys = [
                "Bucket"
              ]
              required = true
              name = "bucket"
            }
          ]
          redaction = {

          }
          metadata = {
            terraform_address = "aws_s3_bucket_versioning.test"
          }
        }
      ]
      redaction {

      }
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/aws/s3/BucketVersioning/list_include_resource/input"
      }
    }
  }