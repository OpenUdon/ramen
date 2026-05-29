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
  operation "aws_iam_user_expected_create" {
    sourceDescription = "iam"
    sourceOperationId = "CreateUser"
    description       = "Review create create for Terraform resource aws_iam_user.expected"
    request {
      x-ramen-terraform {
        identity_attributes = [
          {
            response_paths = [
              "User.UserName",
              "User.Arn",
              "User.UserId"
            ]
            required = true
            name = "user_name"
            terraform_path = "name"
            request_keys = [
              "UserName"
            ]
          }
        ]
        object {
          address = "aws_iam_user.expected"
          kind = "resource"
          name = "expected"
          type = "aws_iam_user"
        }
        attributes {
          name = "\\\"$${var.rName}-$${count.index}\\\""
          path = "var.expected_path_name"
        }
      }
      body {
        UserName = "\\\"$${var.rName}-$${count.index}\\\""
        Path = "var.expected_path_name"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
    }
  }
  operation "aws_iam_user_not_expected_create" {
    sourceDescription = "iam"
    sourceOperationId = "CreateUser"
    description       = "Review create create for Terraform resource aws_iam_user.not_expected"
    request {
      body {
        Path = "var.other_path_name"
        UserName = "\\\"$${var.rName}-other-$${count.index}\\\""
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          name = "\\\"$${var.rName}-other-$${count.index}\\\""
          path = "var.other_path_name"
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
          address = "aws_iam_user.not_expected"
          kind = "resource"
          name = "not_expected"
          type = "aws_iam_user"
        }
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "aws_iam_user_expected_create" {
      operationRef = "aws_iam_user_expected_create"
      body {
        terraform_type = "aws_iam_user"
        action = "create"
        purpose = "create"
        terraform_address = "aws_iam_user.expected"
      }
    }
    step "aws_iam_user_not_expected_create" {
      operationRef = "aws_iam_user_not_expected_create"
      body {
        purpose = "create"
        terraform_address = "aws_iam_user.not_expected"
        terraform_type = "aws_iam_user"
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
          credential_bindings = [
            "aws_hmac"
          ]
          kind = "resource"
          type = "aws_iam_user"
          lifecycle = {

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
          address = "aws_iam_user.expected"
          attributes = {
            name = "\\\"$${var.rName}-$${count.index}\\\""
            path = "var.expected_path_name"
          }
          redaction = {

          }
          metadata = {
            terraform_address = "aws_iam_user.expected"
          }
          name = "expected"
        },
        {
          kind = "resource"
          type = "aws_iam_user"
          name = "not_expected"
          attributes = {
            name = "\\\"$${var.rName}-other-$${count.index}\\\""
            path = "var.other_path_name"
          }
          lifecycle = {

          }
          credential_bindings = [
            "aws_hmac"
          ]
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
          identity_attributes = [
            {
              required = true
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
            }
          ]
          metadata = {
            terraform_address = "aws_iam_user.not_expected"
          }
          address = "aws_iam_user.not_expected"
          redaction = {

          }
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/iam/User/list_path_prefix/input"
        source = "ramen convert tf"
      }
    }
  }