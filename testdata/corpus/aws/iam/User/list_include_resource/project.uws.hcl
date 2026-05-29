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
        UserName = "\\\"$${var.rName}-$${count.index}\\\""
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          name = "\\\"$${var.rName}-$${count.index}\\\""
          tags = "var.resource_tags"
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
          address = "aws_iam_user.test"
          kind = "resource"
          name = "test"
          type = "aws_iam_user"
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
      resources = [
        {
          attributes = {
            name = "\\\"$${var.rName}-$${count.index}\\\""
            tags = "var.resource_tags"
          }
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_iam_user.test"
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
          type = "aws_iam_user"
          kind = "resource"
          name = "test"
          lifecycle = {

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
              path = "name"
            }
          ]
          redaction = {

          }
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/iam/User/list_include_resource/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          id = "iam"
          path = "aws-smithy/iam.json"
          kind = "aws-smithy"
        }
      ]
    }
  }