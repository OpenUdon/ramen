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
      x-ramen-terraform {
        identity_attributes = [
          {
            request_keys = [
              "RoleName"
            ]
            response_paths = [
              "Role.RoleName",
              "Role.Arn"
            ]
            required = true
            name = "role_name"
            terraform_path = "name"
          }
        ]
        object {
          address = "aws_iam_role.test"
          kind = "resource"
          name = "test"
          type = "aws_iam_role"
        }
        attributes {
          name = "var.rName"
          assume_role_policy = "jsonencode({\n    Version = \"2012-10-17\"\n    Statement = [\n      {\n        Action = \"sts:AssumeRole\"\n        Effect = \"Allow\"\n        Principal = {\n          Service = \"ec2.amazonaws.com\"\n        }\n      }\n    ]\n  })"
        }
      }
      body {
        AssumeRolePolicyDocument = "jsonencode({\n    Version = \"2012-10-17\"\n    Statement = [\n      {\n        Action = \"sts:AssumeRole\"\n        Effect = \"Allow\"\n        Principal = {\n          Service = \"ec2.amazonaws.com\"\n        }\n      }\n    ]\n  })"
        RoleName = "var.rName"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
    }
  }
  operation "aws_iam_role_policy_test_create" {
    sourceDescription = "iam"
    sourceOperationId = "PutRolePolicy"
    description       = "Review create create for Terraform resource aws_iam_role_policy.test"
    request {
      x-ramen-terraform "attributes" {
        policy = "jsonencode({\n    Version = \"2012-10-17\"\n    Statement = [\n      {\n        Action = [\n          \"s3:GetObject\",\n        ]\n        Effect   = \"Allow\"\n        Resource = \"*\"\n      }\n    ]\n  })"
        role = "aws_iam_role.test.name"
        name = "\"$${var.rName}-$${count.index}\""
      }
      x-ramen-terraform "object" {
        type = "aws_iam_role_policy"
        address = "aws_iam_role_policy.test"
        kind = "resource"
        name = "test"
      }
      body {
        PolicyDocument = "jsonencode({\n    Version = \"2012-10-17\"\n    Statement = [\n      {\n        Action = [\n          \"s3:GetObject\",\n        ]\n        Effect   = \"Allow\"\n        Resource = \"*\"\n      }\n    ]\n  })"
        PolicyName = "\"$${var.rName}-$${count.index}\""
        RoleName = "aws_iam_role.test.name"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "aws_iam_role_test_create" {
      operationRef = "aws_iam_role_test_create"
      body {
        terraform_address = "aws_iam_role.test"
        terraform_type = "aws_iam_role"
        action = "create"
        purpose = "create"
      }
    }
    step "aws_iam_role_policy_test_create" {
      operationRef = "aws_iam_role_policy_test_create"
      body {
        terraform_type = "aws_iam_role_policy"
        action = "create"
        purpose = "create"
        terraform_address = "aws_iam_role_policy.test"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      version = "ramen.project.v1"
      api_sources = [
        {
          path = "aws-smithy/iam.json"
          kind = "aws-smithy"
          id = "iam"
        }
      ]
      resources = [
        {
          lifecycle = {

          }
          identity_attributes = [
            {
              required = true
              name = "role_name"
              path = "name"
              request_keys = [
                "RoleName"
              ]
              response_paths = [
                "Role.RoleName",
                "Role.Arn"
              ]
            }
          ]
          redaction = {

          }
          attributes = {
            assume_role_policy = "jsonencode({\n    Version = \"2012-10-17\"\n    Statement = [\n      {\n        Action = \"sts:AssumeRole\"\n        Effect = \"Allow\"\n        Principal = {\n          Service = \"ec2.amazonaws.com\"\n        }\n      }\n    ]\n  })"
            name = "var.rName"
          }
          kind = "resource"
          type = "aws_iam_role"
          name = "test"
          operations = {
            create = {
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "iam"
              source_path = "aws-smithy/iam.json"
              operation_id = "CreateRole"
              credential_bindings = [
                "aws_hmac"
              ]
            }
          }
          address = "aws_iam_role.test"
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
        },
        {
          type = "aws_iam_role_policy"
          attributes = {
            policy = "jsonencode({\n    Version = \"2012-10-17\"\n    Statement = [\n      {\n        Action = [\n          \"s3:GetObject\",\n        ]\n        Effect   = \"Allow\"\n        Resource = \"*\"\n      }\n    ]\n  })"
            role = "aws_iam_role.test.name"
            name = "\"$${var.rName}-$${count.index}\""
          }
          lifecycle = {

          }
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_iam_role_policy.test"
          }
          address = "aws_iam_role_policy.test"
          name = "test"
          dependencies = [
            "aws_iam_role.test"
          ]
          kind = "resource"
          operations = {
            create = {
              source_kind = "aws-smithy"
              source_id = "iam"
              source_path = "aws-smithy/iam.json"
              operation_id = "PutRolePolicy"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
            }
          }
          redaction = {

          }
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/iam/RolePolicy/list_include_resource/input"
        source = "ramen convert tf"
      }
    }
  }