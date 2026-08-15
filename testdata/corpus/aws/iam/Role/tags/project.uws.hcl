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
        AssumeRolePolicyDocument = "jsonencode({\\n    Version = \\\"2012-10-17\\\"\\n    Statement = [{\\n      Action = \\\"sts:AssumeRole\\\",\\n      Principal = {\\n        Service = \\\"ec2.$${data.aws_partition.current.dns_suffix}\\\",\\n      }\\n      Effect = \\\"Allow\\\"\\n      Sid    = \\\"\\\"\\n    }]\\n  })"
        RoleName = "var.rName"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          assume_role_policy = "jsonencode({\\n    Version = \\\"2012-10-17\\\"\\n    Statement = [{\\n      Action = \\\"sts:AssumeRole\\\",\\n      Principal = {\\n        Service = \\\"ec2.$${data.aws_partition.current.dns_suffix}\\\",\\n      }\\n      Effect = \\\"Allow\\\"\\n      Sid    = \\\"\\\"\\n    }]\\n  })"
          name = "var.rName"
          tags = "var.resource_tags"
        }
        identity_attributes = [
          {
            name = "role_name"
            request_keys = [
              "RoleName"
            ]
            required = true
            response_paths = [
              "Role.Arn",
              "Role.RoleName"
            ]
            terraform_path = "name"
          }
        ]
        object {
          address = "aws_iam_role.test"
          kind = "resource"
          name = "test"
          type = "aws_iam_role"
        }
        version = "ramen.terraform.provenance.v1"
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
      api_sources = [
        {
          id = "iam"
          kind = "aws-smithy"
          path = "aws-smithy/iam.json"
        }
      ]
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/iam/Role/tags/input"
        source = "ramen convert tf"
      }
      redaction {

      }
      resources = [
        {
          address = "aws_iam_role.test"
          attributes = {
            assume_role_policy = "jsonencode({\\n    Version = \\\"2012-10-17\\\"\\n    Statement = [{\\n      Action = \\\"sts:AssumeRole\\\",\\n      Principal = {\\n        Service = \\\"ec2.$${data.aws_partition.current.dns_suffix}\\\",\\n      }\\n      Effect = \\\"Allow\\\"\\n      Sid    = \\\"\\\"\\n    }]\\n  })"
            name = "var.rName"
            tags = "var.resource_tags"
          }
          credential_bindings = [
            "aws_hmac"
          ]
          dependencies = [
            "data.aws_partition.current"
          ]
          identity_attributes = [
            {
              name = "role_name"
              path = "name"
              request_keys = [
                "RoleName"
              ]
              required = true
              response_paths = [
                "Role.RoleName",
                "Role.Arn"
              ]
            }
          ]
          kind = "resource"
          lifecycle = {

          }
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          name = "test"
          operations = {
            create = {
              credential_bindings = [
                "aws_hmac"
              ]
              operation_id = "CreateRole"
              purpose = "create"
              source_id = "iam"
              source_kind = "aws-smithy"
              source_path = "aws-smithy/iam.json"
            }
          }
          redaction = {

          }
          type = "aws_iam_role"
        }
      ]
      version = "ramen.project.v1"
    }
  }