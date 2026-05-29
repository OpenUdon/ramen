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
        RoleName = "var.rName"
        AssumeRolePolicyDocument = "jsonencode({\\n    Version = \\\"2012-10-17\\\"\\n    Statement = [{\\n      Action = \\\"sts:AssumeRole\\\",\\n      Principal = {\\n        Service = \\\"ec2.$${data.aws_partition.current.dns_suffix}\\\",\\n      }\\n      Effect = \\\"Allow\\\"\\n      Sid    = \\\"\\\"\\n    }]\\n  })"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          assume_role_policy = "jsonencode({\\n    Version = \\\"2012-10-17\\\"\\n    Statement = [{\\n      Action = \\\"sts:AssumeRole\\\",\\n      Principal = {\\n        Service = \\\"ec2.$${data.aws_partition.current.dns_suffix}\\\",\\n      }\\n      Effect = \\\"Allow\\\"\\n      Sid    = \\\"\\\"\\n    }]\\n  })"
          name = "var.rName"
        }
        identity_attributes = [
          {
            name = "role_name"
            terraform_path = "name"
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
        object {
          kind = "resource"
          name = "test"
          type = "aws_iam_role"
          address = "aws_iam_role.test"
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
          kind = "aws-smithy"
          id = "iam"
          path = "aws-smithy/iam.json"
        }
      ]
      resources = [
        {
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
          attributes = {
            assume_role_policy = "jsonencode({\\n    Version = \\\"2012-10-17\\\"\\n    Statement = [{\\n      Action = \\\"sts:AssumeRole\\\",\\n      Principal = {\\n        Service = \\\"ec2.$${data.aws_partition.current.dns_suffix}\\\",\\n      }\\n      Effect = \\\"Allow\\\"\\n      Sid    = \\\"\\\"\\n    }]\\n  })"
            name = "var.rName"
          }
          lifecycle = {

          }
          name = "test"
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
          address = "aws_iam_role.test"
          kind = "resource"
          type = "aws_iam_role"
          credential_bindings = [
            "aws_hmac"
          ]
          redaction = {

          }
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          dependencies = [
            "data.aws_partition.current"
          ]
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/iam/tmpl/role_basic/input"
        source = "ramen convert tf"
      }
    }
  }