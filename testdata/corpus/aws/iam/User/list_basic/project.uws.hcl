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
        }
        identity_attributes = [
          {
            name = "user_name"
            request_keys = [
              "UserName"
            ]
            required = true
            response_paths = [
              "User.Arn",
              "User.UserId",
              "User.UserName"
            ]
            terraform_path = "name"
          }
        ]
        object {
          address = "aws_iam_user.test"
          kind = "resource"
          name = "test"
          type = "aws_iam_user"
        }
        version = "ramen.terraform.provenance.v1"
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
      api_sources = [
        {
          id = "iam"
          kind = "aws-smithy"
          path = "aws-smithy/iam.json"
        }
      ]
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/iam/User/list_basic/input"
        source = "ramen convert tf"
      }
      redaction {

      }
      resources = [
        {
          address = "aws_iam_user.test"
          attributes = {
            name = "\\\"$${var.rName}-$${count.index}\\\""
          }
          credential_bindings = [
            "aws_hmac"
          ]
          identity_attributes = [
            {
              name = "user_name"
              path = "name"
              request_keys = [
                "UserName"
              ]
              required = true
              response_paths = [
                "User.UserName",
                "User.Arn",
                "User.UserId"
              ]
            }
          ]
          kind = "resource"
          lifecycle = {

          }
          metadata = {
            terraform_address = "aws_iam_user.test"
          }
          name = "test"
          operations = {
            create = {
              credential_bindings = [
                "aws_hmac"
              ]
              operation_id = "CreateUser"
              purpose = "create"
              source_id = "iam"
              source_kind = "aws-smithy"
              source_path = "aws-smithy/iam.json"
            }
          }
          redaction = {

          }
          type = "aws_iam_user"
        }
      ]
      version = "ramen.project.v1"
    }
  }