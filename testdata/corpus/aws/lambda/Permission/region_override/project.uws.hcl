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
            required = true
            name = "role_name"
            terraform_path = "name"
            request_keys = [
              "RoleName"
            ]
            response_paths = [
              "Role.RoleName",
              "Role.Arn"
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
      x-ramen-terraform "attributes" {
        runtime = "\\\"nodejs24.x\\\""
        filename = "\\\"test-fixtures/lambdatest.zip\\\""
        function_name = "var.rName"
        handler = "\\\"exports.example\\\""
        region = "var.region"
        role = "aws_iam_role.test.arn"
      }
      x-ramen-terraform "object" {
        name = "test"
        type = "aws_lambda_function"
        address = "aws_lambda_function.test"
        kind = "resource"
      }
      body {
        FunctionName = "var.rName"
        Handler = "\\\"exports.example\\\""
        Role = "aws_iam_role.test.arn"
        Runtime = "\\\"nodejs24.x\\\""
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
          region = "var.region"
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
            request_keys = [
              "StatementId"
            ]
            required = true
            name = "statement_id"
            terraform_path = "statement_id"
          }
        ]
        object {
          type = "aws_lambda_permission"
          address = "aws_lambda_permission.test"
          kind = "resource"
          name = "test"
        }
      }
      body {
        Action = "\\\"lambda:InvokeFunction\\\""
        EventSourceToken = "$${sensitive.event_source_token}"
        Principal = "\\\"events.amazonaws.com\\\""
        StatementId = "\\\"AllowExecutionFromCloudWatch\\\""
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
        action = "create"
        purpose = "create"
        terraform_address = "aws_lambda_permission.test"
        terraform_type = "aws_lambda_permission"
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
          kind = "aws-smithy"
          id = "lambda"
          path = "aws-smithy/lambda.json"
        }
      ]
      resources = [
        {
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          redaction = {

          }
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_iam_role.test"
          kind = "resource"
          name = "test"
          lifecycle = {

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
          type = "aws_iam_role"
          attributes = {
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
              path = "name"
            }
          ]
        },
        {
          type = "aws_lambda_function"
          lifecycle = {

          }
          dependencies = [
            "aws_iam_role.test"
          ]
          metadata = {
            terraform_address = "aws_lambda_function.test"
          }
          operations = {
            create = {
              source_path = "aws-smithy/lambda.json"
              operation_id = "CreateFunction"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "lambda"
            }
          }
          address = "aws_lambda_function.test"
          name = "test"
          attributes = {
            function_name = "var.rName"
            handler = "\\\"exports.example\\\""
            region = "var.region"
            role = "aws_iam_role.test.arn"
            runtime = "\\\"nodejs24.x\\\""
            filename = "\\\"test-fixtures/lambdatest.zip\\\""
          }
          credential_bindings = [
            "aws_hmac"
          ]
          redaction = {

          }
          kind = "resource"
        },
        {
          lifecycle = {

          }
          attributes = {
            region = "var.region"
            statement_id = "\\\"AllowExecutionFromCloudWatch\\\""
            action = "\\\"lambda:InvokeFunction\\\""
            event_source_token = "$${sensitive.event_source_token}"
            function_name = "aws_lambda_function.test.function_name"
            principal = "\\\"events.amazonaws.com\\\""
          }
          dependencies = [
            "aws_lambda_function.test"
          ]
          identity_attributes = [
            {
              request_keys = [
                "FunctionName"
              ]
              required = true
              name = "function_name"
              path = "function_name"
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
          name = "test"
          operations = {
            create = {
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "lambda"
              source_path = "aws-smithy/lambda.json"
              operation_id = "AddPermission"
              credential_bindings = [
                "aws_hmac"
              ]
            }
          }
          credential_bindings = [
            "aws_hmac"
          ]
          redaction = {

          }
          metadata = {
            terraform_address = "aws_lambda_permission.test"
          }
          address = "aws_lambda_permission.test"
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
        config_dir = "testdata/corpus/aws/lambda/Permission/region_override/input"
        source = "ramen convert tf"
      }
    }
  }