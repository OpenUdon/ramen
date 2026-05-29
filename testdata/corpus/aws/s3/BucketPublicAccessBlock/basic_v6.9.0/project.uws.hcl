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
        bucket = "var.rName"
      }
      x-ramen-terraform "object" {
        address = "aws_s3_bucket.test"
        kind = "resource"
        name = "test"
        type = "aws_s3_bucket"
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
      path {
        Bucket = "aws_s3_bucket.test.bucket"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        object {
          type = "aws_s3_bucket_public_access_block"
          address = "aws_s3_bucket_public_access_block.test"
          kind = "resource"
          name = "test"
        }
        attributes {
          block_public_policy = "false"
          bucket = "aws_s3_bucket.test.bucket"
          ignore_public_acls = "false"
          restrict_public_buckets = "false"
          block_public_acls = "false"
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
      body "PublicAccessBlockConfiguration" {
        RestrictPublicBuckets = "false"
        BlockPublicAcls = "false"
        BlockPublicPolicy = "false"
        IgnorePublicAcls = "false"
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
        terraform_address = "aws_s3_bucket_public_access_block.test"
        terraform_type = "aws_s3_bucket_public_access_block"
        action = "create"
        purpose = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/s3/BucketPublicAccessBlock/basic_v6.9.0/input"
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
          name = "test"
          attributes = {
            bucket = "var.rName"
          }
          redaction = {

          }
          lifecycle = {

          }
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
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_s3_bucket.test"
          kind = "resource"
        },
        {
          credential_bindings = [
            "aws_hmac"
          ]
          type = "aws_s3_bucket_public_access_block"
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
          name = "test"
          attributes = {
            restrict_public_buckets = "false"
            block_public_acls = "false"
            block_public_policy = "false"
            bucket = "aws_s3_bucket.test.bucket"
            ignore_public_acls = "false"
          }
          redaction = {

          }
          lifecycle = {

          }
          dependencies = [
            "aws_s3_bucket.test"
          ]
          metadata = {
            terraform_address = "aws_s3_bucket_public_access_block.test"
          }
          address = "aws_s3_bucket_public_access_block.test"
          kind = "resource"
          operations = {
            create = {
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "s3"
              source_path = "aws-smithy/s3.json"
              operation_id = "PutPublicAccessBlock"
            }
          }
        }
      ]
    }
  }