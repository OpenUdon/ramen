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
        region = "var.region"
      }
      x-ramen-terraform "object" {
        address = "aws_s3_bucket.test"
        kind = "resource"
        name = "test"
        type = "aws_s3_bucket"
      }
      path {
        Bucket = "\\\"$${var.rName}-$${count.index}\\\""
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
        IgnorePublicAcls = "false"
        RestrictPublicBuckets = "false"
        BlockPublicAcls = "false"
        BlockPublicPolicy = "false"
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
            request_keys = [
              "Bucket"
            ]
            required = true
            name = "bucket"
            terraform_path = "bucket"
          }
        ]
        object {
          name = "test"
          type = "aws_s3_bucket_public_access_block"
          address = "aws_s3_bucket_public_access_block.test"
          kind = "resource"
        }
        attributes {
          block_public_acls = "false"
          block_public_policy = "false"
          bucket = "aws_s3_bucket.test[count.index].bucket"
          ignore_public_acls = "false"
          region = "var.region"
          restrict_public_buckets = "false"
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
        terraform_address = "aws_s3_bucket_public_access_block.test"
        terraform_type = "aws_s3_bucket_public_access_block"
        action = "create"
        purpose = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      resources = [
        {
          type = "aws_s3_bucket"
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_s3_bucket.test"
          lifecycle = {

          }
          redaction = {

          }
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
          kind = "resource"
          name = "test"
          attributes = {
            bucket = "\\\"$${var.rName}-$${count.index}\\\""
            region = "var.region"
          }
          metadata = {
            terraform_address = "aws_s3_bucket.test"
          }
        },
        {
          type = "aws_s3_bucket_public_access_block"
          lifecycle = {

          }
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_s3_bucket_public_access_block.test"
          }
          address = "aws_s3_bucket_public_access_block.test"
          name = "test"
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
          attributes = {
            block_public_policy = "false"
            bucket = "aws_s3_bucket.test[count.index].bucket"
            ignore_public_acls = "false"
            region = "var.region"
            restrict_public_buckets = "false"
            block_public_acls = "false"
          }
          redaction = {

          }
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/aws/s3/BucketPublicAccessBlock/list_region_override/input"
        source = "ramen convert tf"
        action = "create"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          path = "aws-smithy/s3.json"
          kind = "aws-smithy"
          id = "s3"
        }
      ]
    }
  }