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
  sourceDescription "lambda" {
    url  = "aws-smithy/lambda.json"
    type = "aws-smithy"
  }
  operation "aws_iam_role_test_create" {
    sourceDescription = "iam"
    sourceOperationId = "CreateRole"
    description       = "Review create create for Terraform resource aws_iam_role.test"
    request {
      body {
        AssumeRolePolicyDocument = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
        RoleName = "var.rName"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          assume_role_policy = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
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
          address = "aws_iam_role.test"
          kind = "resource"
          name = "test"
          type = "aws_iam_role"
        }
      }
    }
  }
  operation "aws_lambda_function_test_create" {
    sourceDescription = "lambda"
    sourceOperationId = "CreateFunction"
    description       = "Review create create for Terraform resource aws_lambda_function.test"
    request {
      body {
        FunctionName = "var.rName"
        Handler = "\\\"exports.example\\\""
        Role = "aws_iam_role.test.arn"
        Runtime = "\\\"nodejs24.x\\\""
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform "attributes" {
        filename = "\\\"test-fixtures/lambdatest.zip\\\""
        function_name = "var.rName"
        handler = "\\\"exports.example\\\""
        role = "aws_iam_role.test.arn"
        runtime = "\\\"nodejs24.x\\\""
      }
      x-ramen-terraform "object" {
        kind = "resource"
        name = "test"
        type = "aws_lambda_function"
        address = "aws_lambda_function.test"
      }
    }
  }
  operation "aws_lambda_permission_test_create" {
    sourceDescription = "lambda"
    sourceOperationId = "AddPermission"
    description       = "Review create create for Terraform resource aws_lambda_permission.test"
    request {
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        identity_attributes = [
          {
            name = "function_name"
            terraform_path = "function_name"
            request_keys = [
              "FunctionName"
            ]
            required = true
          },
          {
            required = true
            name = "statement_id"
            terraform_path = "statement_id"
            request_keys = [
              "StatementId"
            ]
          }
        ]
        object {
          address = "aws_lambda_permission.test"
          kind = "resource"
          name = "test"
          type = "aws_lambda_permission"
        }
        attributes {
          action = "\\\"lambda:InvokeFunction\\\""
          function_name = "aws_lambda_function.test.function_name"
          principal = "\\\"events.amazonaws.com\\\""
          statement_id = "\\\"$${var.rName}-$${count.index}\\\""
        }
      }
      body {
        Principal = "\\\"events.amazonaws.com\\\""
        StatementId = "\\\"$${var.rName}-$${count.index}\\\""
        Action = "\\\"lambda:InvokeFunction\\\""
      }
      path {
        FunctionName = "aws_lambda_function.test.function_name"
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "aws_iam_role_test_create" {
      operationRef = "aws_iam_role_test_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "aws_iam_role.test"
        terraform_type = "aws_iam_role"
      }
    }
    step "aws_lambda_function_test_create" {
      operationRef = "aws_lambda_function_test_create"
      body {
        terraform_type = "aws_lambda_function"
        action = "create"
        purpose = "create"
        terraform_address = "aws_lambda_function.test"
      }
    }
    step "aws_lambda_permission_test_create" {
      operationRef = "aws_lambda_permission_test_create"
      body {
        purpose = "create"
        terraform_address = "aws_lambda_permission.test"
        terraform_type = "aws_lambda_permission"
        action = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      redaction {

      }
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/aws/lambda/Permission/list_include_resource/input"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          kind = "aws-smithy"
          id = "iam"
          path = "aws-smithy/iam.json"
        },
        {
          id = "lambda"
          path = "aws-smithy/lambda.json"
          kind = "aws-smithy"
        }
      ]
      resources = [
        {
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
          name = "test"
          attributes = {
            assume_role_policy = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
            name = "var.rName"
          }
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          identity_attributes = [
            {
              response_paths = [
                "Role.RoleName",
                "Role.Arn"
              ]
              required = true
              name = "role_name"
              path = "name"
              request_keys = [
                "RoleName"
              ]
            }
          ]
          credential_bindings = [
            "aws_hmac"
          ]
          redaction = {

          }
          address = "aws_iam_role.test"
          kind = "resource"
          type = "aws_iam_role"
          lifecycle = {

          }
        },
        {
          name = "test"
          attributes = {
            filename = "\\\"test-fixtures/lambdatest.zip\\\""
            function_name = "var.rName"
            handler = "\\\"exports.example\\\""
            role = "aws_iam_role.test.arn"
            runtime = "\\\"nodejs24.x\\\""
          }
          dependencies = [
            "aws_iam_role.test"
          ]
          metadata = {
            terraform_address = "aws_lambda_function.test"
          }
          address = "aws_lambda_function.test"
          operations = {
            create = {
              source_kind = "aws-smithy"
              source_id = "lambda"
              source_path = "aws-smithy/lambda.json"
              operation_id = "CreateFunction"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
            }
          }
          credential_bindings = [
            "aws_hmac"
          ]
          lifecycle = {

          }
          type = "aws_lambda_function"
          redaction = {

          }
          kind = "resource"
        },
        {
          metadata = {
            terraform_address = "aws_lambda_permission.test"
          }
          type = "aws_lambda_permission"
          name = "test"
          lifecycle = {

          }
          attributes = {
            action = "\\\"lambda:InvokeFunction\\\""
            function_name = "aws_lambda_function.test.function_name"
            principal = "\\\"events.amazonaws.com\\\""
            statement_id = "\\\"$${var.rName}-$${count.index}\\\""
          }
          operations = {
            create = {
              source_path = "aws-smithy/lambda.json"
              operation_id = "AddPermission"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "lambda"
            }
          }
          identity_attributes = [
            {
              name = "function_name"
              path = "function_name"
              request_keys = [
                "FunctionName"
              ]
              required = true
            },
            {
              name = "statement_id"
              path = "statement_id"
              request_keys = [
                "StatementId"
              ]
              required = true
            }
          ]
          address = "aws_lambda_permission.test"
          dependencies = [
            "aws_lambda_function.test"
          ]
          kind = "resource"
          credential_bindings = [
            "aws_hmac"
          ]
          redaction = {

          }
        }
      ]
    }
  }