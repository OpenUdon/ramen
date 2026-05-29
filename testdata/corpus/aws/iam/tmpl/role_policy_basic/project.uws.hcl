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
          name = "test"
          type = "aws_iam_role"
          address = "aws_iam_role.test"
          kind = "resource"
        }
        attributes {
          assume_role_policy = "data.aws_iam_policy_document.assume_role.json"
          name = "var.rName"
        }
      }
      body {
        AssumeRolePolicyDocument = "data.aws_iam_policy_document.assume_role.json"
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
        kind = "resource"
        name = "test"
        type = "aws_iam_role_policy"
        address = "aws_iam_role_policy.test"
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
          kind = "aws-smithy"
          id = "iam"
          path = "aws-smithy/iam.json"
        }
      ]
      resources = [
        {
          dependencies = [
            "data.aws_iam_policy_document.assume_role"
          ]
          redaction = {

          }
          type = "aws_iam_role"
          attributes = {
            assume_role_policy = "data.aws_iam_policy_document.assume_role.json"
            name = "var.rName"
          }
          lifecycle = {

          }
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
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          name = "test"
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
          address = "aws_iam_role.test"
        },
        {
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
          type = "aws_iam_role_policy"
          name = "test"
          lifecycle = {

          }
          credential_bindings = [
            "aws_hmac"
          ]
          kind = "resource"
          attributes = {
            name = "var.rName"
            policy = "data.aws_iam_policy_document.test.json"
            role = "aws_iam_role.test.name"
          }
          metadata = {
            terraform_address = "aws_iam_role_policy.test"
          }
          dependencies = [
            "aws_iam_role.test",
            "data.aws_iam_policy_document.test"
          ]
          address = "aws_iam_role_policy.test"
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/aws/iam/tmpl/role_policy_basic/input"
        source = "ramen convert tf"
        action = "create"
      }
    }
  }