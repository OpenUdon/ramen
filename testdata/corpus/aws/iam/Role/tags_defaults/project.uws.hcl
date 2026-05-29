  uws = "1.4.0"
  info {
    title       = "ramen_native_project"
    description = "Native UWS/Ramen desired-state project generated from static Terraform/OpenTofu configuration."
    version     = "1.0.0"
  }
  sourceDescription "iam" {
    url  = "aws-smithy/iam.json"
    type = "aws-smithy"
  }
  operation "aws_iam_role_test_create" {
    sourceDescription = "iam"
    sourceOperationId = "CreateRole"
    description       = "Review create create for Terraform resource aws_iam_role.test"
    request {
      body {
        AssumeRolePolicyDocument = "jsonencode({\n    Version = \"2012-10-17\"\n    Statement = [{\n      Action = \"sts:AssumeRole\",\n      Principal = {\n        Service = \"ec2.$${data.aws_partition.current.dns_suffix}\",\n      }\n      Effect = \"Allow\"\n      Sid    = \"\"\n    }]\n  })"
        RoleName = "var.rName"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        identity_attributes = [
          {
            terraform_path = "name"
            request_keys = [
              "RoleName"
            ]
            response_paths = [
              "Role.RoleName",
              "Role.Arn"
            ]
            required = true
            name = "role_name"
          }
        ]
        object {
          kind = "resource"
          name = "test"
          type = "aws_iam_role"
          address = "aws_iam_role.test"
        }
        attributes {
          tags = "var.resource_tags"
          assume_role_policy = "jsonencode({\n    Version = \"2012-10-17\"\n    Statement = [{\n      Action = \"sts:AssumeRole\",\n      Principal = {\n        Service = \"ec2.$${data.aws_partition.current.dns_suffix}\",\n      }\n      Effect = \"Allow\"\n      Sid    = \"\"\n    }]\n  })"
          name = "var.rName"
        }
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "aws_iam_role_test_create" {
      operationRef = "aws_iam_role_test_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "aws_iam_role.test"
        terraform_type = "aws_iam_role"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      version = "ramen.project.v1"
      api_sources = [
        {
          id = "iam"
          path = "aws-smithy/iam.json"
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
              source_id = "iam"
              source_path = "aws-smithy/iam.json"
              operation_id = "CreateRole"
            }
          }
          attributes = {
            name = "var.rName"
            tags = "var.resource_tags"
            assume_role_policy = "jsonencode({\n    Version = \"2012-10-17\"\n    Statement = [{\n      Action = \"sts:AssumeRole\",\n      Principal = {\n        Service = \"ec2.$${data.aws_partition.current.dns_suffix}\",\n      }\n      Effect = \"Allow\"\n      Sid    = \"\"\n    }]\n  })"
          }
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          kind = "resource"
          lifecycle = {

          }
          identity_attributes = [
            {
              name = "role_name"
              path = "name"
              request_keys = [
                "RoleName"
              ]
              response_paths = [
                "Role.RoleName",
                "Role.Arn"
              ]
              required = true
            }
          ]
          address = "aws_iam_role.test"
          type = "aws_iam_role"
          redaction = {

          }
          name = "test"
          dependencies = [
            "data.aws_partition.current"
          ]
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/aws/iam/Role/tags_defaults/input"
        source = "ramen convert tf"
        action = "create"
      }
    }
  }