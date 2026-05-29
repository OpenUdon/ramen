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
        RoleName = "var.rName"
        AssumeRolePolicyDocument = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
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
          name = "test"
          type = "aws_iam_role"
          address = "aws_iam_role.test"
          kind = "resource"
        }
      }
    }
  }
  operation "aws_lambda_function_test_create" {
    sourceDescription = "lambda"
    sourceOperationId = "CreateFunction"
    description       = "Review create create for Terraform resource aws_lambda_function.test"
    request {
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform "attributes" {
        handler = "\\\"exports.example\\\""
        role = "aws_iam_role.test.arn"
        runtime = "\\\"nodejs24.x\\\""
        filename = "\\\"test-fixtures/lambdatest.zip\\\""
        function_name = "var.rName"
      }
      x-ramen-terraform "object" {
        address = "aws_lambda_function.test"
        kind = "resource"
        name = "test"
        type = "aws_lambda_function"
      }
      body {
        FunctionName = "var.rName"
        Handler = "\\\"exports.example\\\""
        Role = "aws_iam_role.test.arn"
        Runtime = "\\\"nodejs24.x\\\""
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
        object {
          address = "aws_lambda_permission.test"
          kind = "resource"
          name = "test"
          type = "aws_lambda_permission"
        }
        attributes {
          event_source_token = "$${sensitive.event_source_token}"
          function_name = "aws_lambda_function.test.function_name"
          principal = "\\\"events.amazonaws.com\\\""
          statement_id = "\\\"AllowExecutionFromCloudWatch\\\""
          action = "\\\"lambda:InvokeFunction\\\""
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
            request_keys = [
              "StatementId"
            ]
            required = true
            name = "statement_id"
            terraform_path = "statement_id"
          }
        ]
      }
      body {
        Action = "\\\"lambda:InvokeFunction\\\""
        EventSourceToken = "$${sensitive.event_source_token}"
        Principal = "\\\"events.amazonaws.com\\\""
        StatementId = "\\\"AllowExecutionFromCloudWatch\\\""
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
        purpose = "create"
        terraform_address = "aws_lambda_function.test"
        terraform_type = "aws_lambda_function"
        action = "create"
      }
    }
    step "aws_lambda_permission_test_create" {
      operationRef = "aws_lambda_permission_test_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "aws_lambda_permission.test"
        terraform_type = "aws_lambda_permission"
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
          kind = "resource"
          name = "test"
          lifecycle = {

          }
          redaction = {

          }
          type = "aws_iam_role"
          address = "aws_iam_role.test"
          attributes = {
            assume_role_policy = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
            name = "var.rName"
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
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          operations = {
            create = {
              source_path = "aws-smithy/iam.json"
              operation_id = "CreateRole"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "iam"
            }
          }
        },
        {
          name = "test"
          operations = {
            create = {
              source_id = "lambda"
              source_path = "aws-smithy/lambda.json"
              operation_id = "CreateFunction"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
            }
          }
          redaction = {

          }
          attributes = {
            runtime = "\\\"nodejs24.x\\\""
            filename = "\\\"test-fixtures/lambdatest.zip\\\""
            function_name = "var.rName"
            handler = "\\\"exports.example\\\""
            role = "aws_iam_role.test.arn"
          }
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_lambda_function.test"
          }
          address = "aws_lambda_function.test"
          kind = "resource"
          type = "aws_lambda_function"
          lifecycle = {

          }
          dependencies = [
            "aws_iam_role.test"
          ]
        },
        {
          attributes = {
            statement_id = "\\\"AllowExecutionFromCloudWatch\\\""
            action = "\\\"lambda:InvokeFunction\\\""
            event_source_token = "$${sensitive.event_source_token}"
            function_name = "aws_lambda_function.test.function_name"
            principal = "\\\"events.amazonaws.com\\\""
          }
          operations = {
            create = {
              operation_id = "AddPermission"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "lambda"
              source_path = "aws-smithy/lambda.json"
            }
          }
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_lambda_permission.test"
          name = "test"
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
              name = "statement_id"
              path = "statement_id"
              request_keys = [
                "StatementId"
              ]
              required = true
            }
          ]
          kind = "resource"
          lifecycle = {

          }
          dependencies = [
            "aws_lambda_function.test"
          ]
          metadata = {
            terraform_address = "aws_lambda_permission.test"
          }
          redaction = {

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
        config_dir = "testdata/corpus/aws/lambda/tmpl/permission_basic/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          kind = "aws-smithy"
          id = "iam"
          path = "aws-smithy/iam.json"
        },
        {
          kind = "aws-smithy"
          id = "lambda"
          path = "aws-smithy/lambda.json"
        }
      ]
    }
  }