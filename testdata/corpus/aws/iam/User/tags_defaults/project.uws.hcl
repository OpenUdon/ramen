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
      body {
        UserName = "var.rName"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          tags = "var.resource_tags"
          name = "var.rName"
        }
        identity_attributes = [
          {
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
            terraform_path = "name"
          }
        ]
        object {
          name = "test"
          type = "aws_iam_user"
          address = "aws_iam_user.test"
          kind = "resource"
        }
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "aws_iam_user_test_create" {
      operationRef = "aws_iam_user_test_create"
      body {
        terraform_type = "aws_iam_user"
        action = "create"
        purpose = "create"
        terraform_address = "aws_iam_user.test"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      resources = [
        {
          credential_bindings = [
            "aws_hmac"
          ]
          type = "aws_iam_user"
          operations = {
            create = {
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "iam"
              source_path = "aws-smithy/iam.json"
              operation_id = "CreateUser"
            }
          }
          redaction = {

          }
          metadata = {
            terraform_address = "aws_iam_user.test"
          }
          kind = "resource"
          name = "test"
          lifecycle = {

          }
          identity_attributes = [
            {
              name = "user_name"
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
            }
          ]
          address = "aws_iam_user.test"
          attributes = {
            name = "var.rName"
            tags = "var.resource_tags"
          }
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/iam/User/tags_defaults/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          path = "aws-smithy/iam.json"
          kind = "aws-smithy"
          id = "iam"
        }
      ]
    }
  }