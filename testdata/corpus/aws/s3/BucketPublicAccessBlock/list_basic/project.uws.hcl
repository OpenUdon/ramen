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
        Bucket = "aws_s3_bucket.test[count.index].bucket"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          restrict_public_buckets = "false"
          block_public_acls = "false"
          block_public_policy = "false"
          bucket = "aws_s3_bucket.test[count.index].bucket"
          ignore_public_acls = "false"
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
        action = "create"
        purpose = "create"
        terraform_address = "aws_s3_bucket.test"
        terraform_type = "aws_s3_bucket"
      }
    }
    step "aws_s3_bucket_public_access_block_test_create" {
      operationRef = "aws_s3_bucket_public_access_block_test_create"
      body {
        purpose = "create"
        terraform_address = "aws_s3_bucket_public_access_block.test"
        terraform_type = "aws_s3_bucket_public_access_block"
        action = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      api_sources = [
        {
          path = "aws-smithy/s3.json"
          kind = "aws-smithy"
          id = "s3"
        }
      ]
      resources = [
        {
          type = "aws_s3_bucket"
          name = "test"
          lifecycle = {

          }
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
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
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_s3_bucket.test"
          kind = "resource"
          attributes = {
            bucket = "\\\"$${var.rName}-$${count.index}\\\""
          }
          redaction = {

          }
        },
        {
          lifecycle = {

          }
          dependencies = [
            "aws_s3_bucket.test"
          ]
          credential_bindings = [
            "aws_hmac"
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
          address = "aws_s3_bucket_public_access_block.test"
          kind = "resource"
          attributes = {
            block_public_policy = "false"
            bucket = "aws_s3_bucket.test[count.index].bucket"
            ignore_public_acls = "false"
            restrict_public_buckets = "false"
            block_public_acls = "false"
          }
          type = "aws_s3_bucket_public_access_block"
          redaction = {

          }
          name = "test"
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/s3/BucketPublicAccessBlock/list_basic/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
    }
  }