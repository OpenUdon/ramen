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
        AssumeRolePolicyDocument = "data.aws_iam_policy_document.assume_role.json"
        RoleName = "var.rName"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          assume_role_policy = "data.aws_iam_policy_document.assume_role.json"
          name = "var.rName"
        }
        identity_attributes = [
          {
            response_paths = [
              "Role.RoleName",
              "Role.Arn"
            ]
            required = true
            name = "role_name"
            terraform_path = "name"
            request_keys = [
              "RoleName"
            ]
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
  operation "aws_iam_role_policy_test_create" {
    sourceDescription = "iam"
    sourceOperationId = "PutRolePolicy"
    description       = "Review create create for Terraform resource aws_iam_role_policy.test"
    request {
      body {
        RoleName = "aws_iam_role.test.name"
        PolicyDocument = "data.aws_iam_policy_document.test.json"
        PolicyName = "var.rName"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform "attributes" {
        role = "aws_iam_role.test.name"
        name = "var.rName"
        policy = "data.aws_iam_policy_document.test.json"
      }
      x-ramen-terraform "object" {
        address = "aws_iam_role_policy.test"
        kind = "resource"
        name = "test"
        type = "aws_iam_role_policy"
      }
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
        purpose = "create"
        terraform_address = "aws_iam_role_policy.test"
        terraform_type = "aws_iam_role_policy"
        action = "create"
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
          attributes = {
            assume_role_policy = "data.aws_iam_policy_document.assume_role.json"
            name = "var.rName"
          }
          dependencies = [
            "data.aws_iam_policy_document.assume_role"
          ]
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          lifecycle = {

          }
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_iam_role.test"
          kind = "resource"
          operations = {
            create = {
              source_id = "iam"
              source_path = "aws-smithy/iam.json"
              operation_id = "CreateRole"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
            }
          }
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
              path = "name"
            }
          ]
          type = "aws_iam_role"
          name = "test"
          redaction = {

          }
        },
        {
          attributes = {
            name = "var.rName"
            policy = "data.aws_iam_policy_document.test.json"
            role = "aws_iam_role.test.name"
          }
          lifecycle = {

          }
          operations = {
            create = {
              source_id = "iam"
              source_path = "aws-smithy/iam.json"
              operation_id = "PutRolePolicy"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
            }
          }
          kind = "resource"
          name = "test"
          metadata = {
            terraform_address = "aws_iam_role_policy.test"
          }
          type = "aws_iam_role_policy"
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_iam_role_policy.test"
          dependencies = [
            "aws_iam_role.test",
            "data.aws_iam_policy_document.test"
          ]
          redaction = {

          }
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/iam/RolePolicy/basic_v6.0.0/input"
        source = "ramen convert tf"
      }
    }
  }