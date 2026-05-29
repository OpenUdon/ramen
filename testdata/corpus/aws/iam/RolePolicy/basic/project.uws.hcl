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
          name = "var.rName"
          assume_role_policy = "data.aws_iam_policy_document.assume_role.json"
        }
        identity_attributes = [
          {
            required = true
            name = "role_name"
            terraform_path = "name"
            request_keys = [
              "RoleName"
            ]
            response_paths = [
              "Role.RoleName",
              "Role.Arn"
            ]
          }
        ]
        object {
          name = "test"
          type = "aws_iam_role"
          address = "aws_iam_role.test"
          kind = "resource"
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
        PolicyDocument = "data.aws_iam_policy_document.test.json"
        PolicyName = "var.rName"
        RoleName = "aws_iam_role.test.name"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform "attributes" {
        name = "var.rName"
        policy = "data.aws_iam_policy_document.test.json"
        role = "aws_iam_role.test.name"
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
        terraform_type = "aws_iam_role"
        action = "create"
        purpose = "create"
        terraform_address = "aws_iam_role.test"
      }
    }
    step "aws_iam_role_policy_test_create" {
      operationRef = "aws_iam_role_policy_test_create"
      body {
        terraform_address = "aws_iam_role_policy.test"
        terraform_type = "aws_iam_role_policy"
        action = "create"
        purpose = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
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
            name = "var.rName"
            assume_role_policy = "data.aws_iam_policy_document.assume_role.json"
          }
          lifecycle = {

          }
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
          credential_bindings = [
            "aws_hmac"
          ]
          type = "aws_iam_role"
          name = "test"
          redaction = {

          }
          address = "aws_iam_role.test"
          kind = "resource"
          dependencies = [
            "data.aws_iam_policy_document.assume_role"
          ]
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
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
        },
        {
          dependencies = [
            "aws_iam_role.test",
            "data.aws_iam_policy_document.test"
          ]
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
          credential_bindings = [
            "aws_hmac"
          ]
          redaction = {

          }
          metadata = {
            terraform_address = "aws_iam_role_policy.test"
          }
          name = "test"
          attributes = {
            name = "var.rName"
            policy = "data.aws_iam_policy_document.test.json"
            role = "aws_iam_role.test.name"
          }
          lifecycle = {

          }
          address = "aws_iam_role_policy.test"
          kind = "resource"
          type = "aws_iam_role_policy"
        }
      ]
      redaction {

      }
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/aws/iam/RolePolicy/basic/input"
      }
      version = "ramen.project.v1"
    }
  }