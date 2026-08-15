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
      x-ramen-terraform {
        attributes {
          bucket = "var.rName"
        }
        object {
          address = "aws_s3_bucket.test"
          kind = "resource"
          name = "test"
          type = "aws_s3_bucket"
        }
        version = "ramen.terraform.provenance.v1"
      }
    }
  }
  operation "aws_s3_bucket_public_access_block_test_create" {
    sourceDescription = "s3"
    sourceOperationId = "PutPublicAccessBlock"
    description       = "Review create create for Terraform resource aws_s3_bucket_public_access_block.test"
    request {
      body "PublicAccessBlockConfiguration" {
        BlockPublicAcls = "false"
        BlockPublicPolicy = "false"
        IgnorePublicAcls = "false"
        RestrictPublicBuckets = "false"
      }
      path {
        Bucket = "aws_s3_bucket.test.bucket"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          block_public_acls = "false"
          block_public_policy = "false"
          bucket = "aws_s3_bucket.test.bucket"
          ignore_public_acls = "false"
          restrict_public_buckets = "false"
        }
        identity_attributes = [
          {
            name = "bucket"
            request_keys = [
              "Bucket"
            ]
            required = true
            terraform_path = "bucket"
          }
        ]
        object {
          address = "aws_s3_bucket_public_access_block.test"
          kind = "resource"
          name = "test"
          type = "aws_s3_bucket_public_access_block"
        }
        version = "ramen.terraform.provenance.v1"
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
      api_sources = [
        {
          id = "s3"
          kind = "aws-smithy"
          path = "aws-smithy/s3.json"
        }
      ]
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/s3/BucketPublicAccessBlock/basic/input"
        source = "ramen convert tf"
      }
      redaction {

      }
      resources = [
        {
          address = "aws_s3_bucket.test"
          attributes = {
            bucket = "var.rName"
          }
          credential_bindings = [
            "aws_hmac"
          ]
          kind = "resource"
          lifecycle = {

          }
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
          name = "test"
          operations = {
            create = {
              credential_bindings = [
                "aws_hmac"
              ]
              operation_id = "CreateBucket"
              purpose = "create"
              source_id = "s3"
              source_kind = "aws-smithy"
              source_path = "aws-smithy/s3.json"
            }
          }
          redaction = {

          }
          type = "aws_s3_bucket"
        },
        {
          address = "aws_s3_bucket_public_access_block.test"
          attributes = {
            block_public_acls = "false"
            block_public_policy = "false"
            bucket = "aws_s3_bucket.test.bucket"
            ignore_public_acls = "false"
            restrict_public_buckets = "false"
          }
          credential_bindings = [
            "aws_hmac"
          ]
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
          kind = "resource"
          lifecycle = {

          }
          metadata = {
            terraform_address = "aws_s3_bucket_public_access_block.test"
          }
          name = "test"
          operations = {
            create = {
              credential_bindings = [
                "aws_hmac"
              ]
              operation_id = "PutPublicAccessBlock"
              purpose = "create"
              source_id = "s3"
              source_kind = "aws-smithy"
              source_path = "aws-smithy/s3.json"
            }
          }
          redaction = {

          }
          type = "aws_s3_bucket_public_access_block"
        }
      ]
      version = "ramen.project.v1"
    }
  }