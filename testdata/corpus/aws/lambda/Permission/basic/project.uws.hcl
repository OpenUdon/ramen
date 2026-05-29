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
      x-ramen-terraform {
        object {
          address = "aws_iam_role.test"
          kind = "resource"
          name = "test"
          type = "aws_iam_role"
        }
        attributes {
          assume_role_policy = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
          name = "var.rName"
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
            terraform_path = "name"
          }
        ]
      }
      body {
        RoleName = "var.rName"
        AssumeRolePolicyDocument = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
    }
  }
  operation "aws_lambda_function_test_create" {
    sourceDescription = "lambda"
    sourceOperationId = "CreateFunction"
    description       = "Review create create for Terraform resource aws_lambda_function.test"
    request {
      x-ramen-terraform "attributes" {
        function_name = "var.rName"
        handler = "\\\"exports.example\\\""
        role = "aws_iam_role.test.arn"
        runtime = "\\\"nodejs24.x\\\""
        filename = "\\\"test-fixtures/lambdatest.zip\\\""
      }
      x-ramen-terraform "object" {
        address = "aws_lambda_function.test"
        kind = "resource"
        name = "test"
        type = "aws_lambda_function"
      }
      body {
        Role = "aws_iam_role.test.arn"
        Runtime = "\\\"nodejs24.x\\\""
        FunctionName = "var.rName"
        Handler = "\\\"exports.example\\\""
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
    }
  }
  operation "aws_lambda_permission_test_create" {
    sourceDescription = "lambda"
    sourceOperationId = "AddPermission"
    description       = "Review create create for Terraform resource aws_lambda_permission.test"
    request {
      body {
        Action = "\\\"lambda:InvokeFunction\\\""
        EventSourceToken = "$${sensitive.event_source_token}"
        Principal = "\\\"events.amazonaws.com\\\""
        StatementId = "\\\"AllowExecutionFromCloudWatch\\\""
      }
      path {
        FunctionName = "aws_lambda_function.test.function_name"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          action = "\\\"lambda:InvokeFunction\\\""
          event_source_token = "$${sensitive.event_source_token}"
          function_name = "aws_lambda_function.test.function_name"
          principal = "\\\"events.amazonaws.com\\\""
          statement_id = "\\\"AllowExecutionFromCloudWatch\\\""
        }
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
            name = "statement_id"
            terraform_path = "statement_id"
            request_keys = [
              "StatementId"
            ]
            required = true
          }
        ]
        object {
          name = "test"
          type = "aws_lambda_permission"
          address = "aws_lambda_permission.test"
          kind = "resource"
        }
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
    step "aws_lambda_function_test_create" {
      operationRef = "aws_lambda_function_test_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "aws_lambda_function.test"
        terraform_type = "aws_lambda_function"
      }
    }
    step "aws_lambda_permission_test_create" {
      operationRef = "aws_lambda_permission_test_create"
      body {
        terraform_address = "aws_lambda_permission.test"
        terraform_type = "aws_lambda_permission"
        action = "create"
        purpose = "create"
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
        },
        {
          path = "aws-smithy/lambda.json"
          kind = "aws-smithy"
          id = "lambda"
        }
      ]
      resources = [
        {
          kind = "resource"
          type = "aws_iam_role"
          attributes = {
            assume_role_policy = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
            name = "var.rName"
          }
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          name = "test"
          lifecycle = {

          }
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
          redaction = {

          }
          identity_attributes = [
            {
              path = "name"
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
          address = "aws_iam_role.test"
        },
        {
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
          metadata = {
            terraform_address = "aws_lambda_function.test"
          }
          attributes = {
            filename = "\\\"test-fixtures/lambdatest.zip\\\""
            function_name = "var.rName"
            handler = "\\\"exports.example\\\""
            role = "aws_iam_role.test.arn"
            runtime = "\\\"nodejs24.x\\\""
          }
          redaction = {

          }
          type = "aws_lambda_function"
          dependencies = [
            "aws_iam_role.test"
          ]
          kind = "resource"
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_lambda_function.test"
          name = "test"
          lifecycle = {

          }
        },
        {
          dependencies = [
            "aws_lambda_function.test"
          ]
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_lambda_permission.test"
          }
          name = "test"
          attributes = {
            event_source_token = "$${sensitive.event_source_token}"
            function_name = "aws_lambda_function.test.function_name"
            principal = "\\\"events.amazonaws.com\\\""
            statement_id = "\\\"AllowExecutionFromCloudWatch\\\""
            action = "\\\"lambda:InvokeFunction\\\""
          }
          identity_attributes = [
            {
              required = true
              name = "function_name"
              path = "function_name"
              request_keys = [
                "FunctionName"
              ]
            },
            {
              required = true
              name = "statement_id"
              path = "statement_id"
              request_keys = [
                "StatementId"
              ]
            }
          ]
          redaction = {

          }
          address = "aws_lambda_permission.test"
          kind = "resource"
          lifecycle = {

          }
          operations = {
            create = {
              source_id = "lambda"
              source_path = "aws-smithy/lambda.json"
              operation_id = "AddPermission"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
            }
          }
          type = "aws_lambda_permission"
        }
      ]
      redaction {
        paths = [
          "aws_lambda_permission.test.event_source_token"
        ]
      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/lambda/Permission/basic/input"
        source = "ramen convert tf"
      }
    }
  }