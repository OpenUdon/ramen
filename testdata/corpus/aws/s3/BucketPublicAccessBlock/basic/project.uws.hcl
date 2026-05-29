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
        bucket = "var.rName"
      }
      x-ramen-terraform "object" {
        type = "aws_s3_bucket"
        address = "aws_s3_bucket.test"
        kind = "resource"
        name = "test"
      }
      path {
        Bucket = "var.rName"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
    }
  }
  operation "aws_s3_bucket_public_access_block_test_create" {
    sourceDescription = "s3"
    sourceOperationId = "PutPublicAccessBlock"
    description       = "Review create create for Terraform resource aws_s3_bucket_public_access_block.test"
    request {
      body "PublicAccessBlockConfiguration" {
        BlockPublicPolicy = "false"
        IgnorePublicAcls = "false"
        RestrictPublicBuckets = "false"
        BlockPublicAcls = "false"
      }
      path {
        Bucket = "aws_s3_bucket.test.bucket"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          ignore_public_acls = "false"
          restrict_public_buckets = "false"
          block_public_acls = "false"
          block_public_policy = "false"
          bucket = "aws_s3_bucket.test.bucket"
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
        object {
          address = "aws_s3_bucket_public_access_block.test"
          kind = "resource"
          name = "test"
          type = "aws_s3_bucket_public_access_block"
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
    step "aws_s3_bucket_public_access_block_test_create" {
      operationRef = "aws_s3_bucket_public_access_block_test_create"
      body {
        terraform_type = "aws_s3_bucket_public_access_block"
        action = "create"
        purpose = "create"
        terraform_address = "aws_s3_bucket_public_access_block.test"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/s3/BucketPublicAccessBlock/basic/input"
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
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_s3_bucket.test"
          kind = "resource"
          attributes = {
            bucket = "var.rName"
          }
          lifecycle = {

          }
          redaction = {

          }
          type = "aws_s3_bucket"
          name = "test"
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
        },
        {
          type = "aws_s3_bucket_public_access_block"
          lifecycle = {

          }
          dependencies = [
            "aws_s3_bucket.test"
          ]
          address = "aws_s3_bucket_public_access_block.test"
          kind = "resource"
          operations = {
            create = {
              source_kind = "aws-smithy"
              source_id = "s3"
              source_path = "aws-smithy/s3.json"
              operation_id = "PutPublicAccessBlock"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
            }
          }
          redaction = {

          }
          metadata = {
            terraform_address = "aws_s3_bucket_public_access_block.test"
          }
          attributes = {
            block_public_policy = "false"
            bucket = "aws_s3_bucket.test.bucket"
            ignore_public_acls = "false"
            restrict_public_buckets = "false"
            block_public_acls = "false"
          }
          name = "test"
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
          credential_bindings = [
            "aws_hmac"
          ]
        }
      ]
      redaction {

      }
    }
  }