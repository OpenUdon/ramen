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
      x-ramen-terraform "attributes" {
        region = "var.region"
        bucket = "var.rName"
      }
      x-ramen-terraform "object" {
        name = "test"
        type = "aws_s3_bucket"
        address = "aws_s3_bucket.test"
        kind = "resource"
      }
      path {
        Bucket = "var.rName"
      }
    }
  }
  operation "aws_s3_bucket_public_access_block_test_create" {
    sourceDescription = "s3"
    sourceOperationId = "PutPublicAccessBlock"
    description       = "Review create create for Terraform resource aws_s3_bucket_public_access_block.test"
    request {
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          block_public_acls = "false"
          block_public_policy = "false"
          bucket = "aws_s3_bucket.test.bucket"
          ignore_public_acls = "false"
          region = "var.region"
          restrict_public_buckets = "false"
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
          type = "aws_s3_bucket_public_access_block"
          address = "aws_s3_bucket_public_access_block.test"
          kind = "resource"
          name = "test"
        }
      }
      body "PublicAccessBlockConfiguration" {
        IgnorePublicAcls = "false"
        RestrictPublicBuckets = "false"
        BlockPublicAcls = "false"
        BlockPublicPolicy = "false"
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
        action = "create"
        purpose = "create"
        terraform_address = "aws_s3_bucket.test"
        terraform_type = "aws_s3_bucket"
      }
    }
    step "aws_s3_bucket_public_access_block_test_create" {
      operationRef = "aws_s3_bucket_public_access_block_test_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "aws_s3_bucket_public_access_block.test"
        terraform_type = "aws_s3_bucket_public_access_block"
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
          address = "aws_s3_bucket.test"
          kind = "resource"
          type = "aws_s3_bucket"
          name = "test"
          lifecycle = {

          }
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          attributes = {
            region = "var.region"
            bucket = "var.rName"
          }
          redaction = {

          }
        },
        {
          address = "aws_s3_bucket_public_access_block.test"
          kind = "resource"
          name = "test"
          lifecycle = {

          }
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
            terraform_address = "aws_s3_bucket_public_access_block.test"
          }
          operations = {
            create = {
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "s3"
              source_path = "aws-smithy/s3.json"
              operation_id = "PutPublicAccessBlock"
              credential_bindings = [
                "aws_hmac"
              ]
            }
          }
          redaction = {

          }
          dependencies = [
            "aws_s3_bucket.test"
          ]
          credential_bindings = [
            "aws_hmac"
          ]
          type = "aws_s3_bucket_public_access_block"
          attributes = {
            region = "var.region"
            restrict_public_buckets = "false"
            block_public_acls = "false"
            block_public_policy = "false"
            bucket = "aws_s3_bucket.test.bucket"
            ignore_public_acls = "false"
          }
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/aws/s3/BucketPublicAccessBlock/region_override/input"
        source = "ramen convert tf"
        action = "create"
      }
    }
  }