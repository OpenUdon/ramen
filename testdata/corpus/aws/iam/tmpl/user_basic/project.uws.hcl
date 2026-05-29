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
  operation "aws_iam_user_test_create" {
    sourceDescription = "iam"
    sourceOperationId = "CreateUser"
    description       = "Review create create for Terraform resource aws_iam_user.test"
    request {
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          name = "var.rName"
        }
        identity_attributes = [
          {
            required = true
            name = "user_name"
            terraform_path = "name"
            request_keys = [
              "UserName"
            ]
            response_paths = [
              "User.UserName",
              "User.Arn",
              "User.UserId"
            ]
          }
        ]
        object {
          address = "aws_iam_user.test"
          kind = "resource"
          name = "test"
          type = "aws_iam_user"
        }
      }
      body {
        UserName = "var.rName"
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "aws_iam_user_test_create" {
      operationRef = "aws_iam_user_test_create"
      body {
        terraform_address = "aws_iam_user.test"
        terraform_type = "aws_iam_user"
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
          credential_bindings = [
            "aws_hmac"
          ]
          type = "aws_iam_user"
          lifecycle = {

          }
          identity_attributes = [
            {
              path = "name"
              request_keys = [
                "UserName"
              ]
              response_paths = [
                "User.UserName",
                "User.Arn",
                "User.UserId"
              ]
              required = true
              name = "user_name"
            }
          ]
          redaction = {

          }
          metadata = {
            terraform_address = "aws_iam_user.test"
          }
          attributes = {
            name = "var.rName"
          }
          operations = {
            create = {
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "iam"
              source_path = "aws-smithy/iam.json"
              operation_id = "CreateUser"
              credential_bindings = [
                "aws_hmac"
              ]
            }
          }
          address = "aws_iam_user.test"
          kind = "resource"
          name = "test"
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/aws/iam/tmpl/user_basic/input"
        source = "ramen convert tf"
        action = "create"
      }
      version = "ramen.project.v1"
    }
  }