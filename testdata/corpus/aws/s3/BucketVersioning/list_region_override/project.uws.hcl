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
        region = "var.region"
        bucket = "\\\"$${var.rName}-$${count.index}\\\""
      }
      x-ramen-terraform "object" {
        name = "test"
        type = "aws_s3_bucket"
        address = "aws_s3_bucket.test"
        kind = "resource"
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
        Bucket = "aws_s3_bucket.test[count.index].bucket"
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
          region = "var.region"
          versioning_configuration {
            status = "\\\"Enabled\\\""
          }
          bucket = "aws_s3_bucket.test[count.index].bucket"
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
          id = "s3"
          path = "aws-smithy/s3.json"
          kind = "aws-smithy"
        }
      ]
      resources = [
        {
          name = "test"
          attributes = {
            bucket = "\\\"$${var.rName}-$${count.index}\\\""
            region = "var.region"
          }
          credential_bindings = [
            "aws_hmac"
          ]
          redaction = {

          }
          address = "aws_s3_bucket.test"
          type = "aws_s3_bucket"
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
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
          kind = "resource"
          lifecycle = {

          }
        },
        {
          type = "aws_s3_bucket_versioning"
          kind = "resource"
          name = "test"
          attributes = {
            bucket = "aws_s3_bucket.test[count.index].bucket"
            region = "var.region"
            versioning_configuration = {
              status = "\\\"Enabled\\\""
            }
          }
          lifecycle = {

          }
          dependencies = [
            "aws_s3_bucket.test"
          ]
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_s3_bucket_versioning.test"
          operations = {
            create = {
              operation_id = "PutBucketVersioning"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "s3"
              source_path = "aws-smithy/s3.json"
            }
          }
          metadata = {
            terraform_address = "aws_s3_bucket_versioning.test"
          }
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
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/aws/s3/BucketVersioning/list_region_override/input"
        source = "ramen convert tf"
        action = "create"
      }
    }
  }