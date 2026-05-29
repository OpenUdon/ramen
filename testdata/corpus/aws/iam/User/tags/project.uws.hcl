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
            terraform_path = "name"
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
        object {
          kind = "resource"
          name = "test"
          type = "aws_iam_user"
          address = "aws_iam_user.test"
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
        action = "create"
        purpose = "create"
        terraform_address = "aws_iam_user.test"
        terraform_type = "aws_iam_user"
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
          name = "test"
          redaction = {

          }
          metadata = {
            terraform_address = "aws_iam_user.test"
          }
          attributes = {
            name = "var.rName"
            tags = "var.resource_tags"
          }
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
          credential_bindings = [
            "aws_hmac"
          ]
          type = "aws_iam_user"
          lifecycle = {

          }
          address = "aws_iam_user.test"
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
          kind = "resource"
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/iam/User/tags/input"
        source = "ramen convert tf"
      }
    }
  }