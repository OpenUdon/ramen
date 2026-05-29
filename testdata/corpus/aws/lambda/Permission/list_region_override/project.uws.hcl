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
            terraform_path = "name"
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
        Role = "aws_iam_role.test.arn"
        Runtime = "\\\"nodejs24.x\\\""
        FunctionName = "var.rName"
        Handler = "\\\"exports.example\\\""
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform "attributes" {
        region = "var.region"
        role = "aws_iam_role.test.arn"
        runtime = "\\\"nodejs24.x\\\""
        filename = "\\\"test-fixtures/lambdatest.zip\\\""
        function_name = "var.rName"
        handler = "\\\"exports.example\\\""
      }
      x-ramen-terraform "object" {
        address = "aws_lambda_function.test"
        kind = "resource"
        name = "test"
        type = "aws_lambda_function"
      }
    }
  }
  operation "aws_lambda_permission_test_create" {
    sourceDescription = "lambda"
    sourceOperationId = "AddPermission"
    description       = "Review create create for Terraform resource aws_lambda_permission.test"
    request {
      body {
        Action = "\\\"lambda:InvokeFunction\\\""
        Principal = "\\\"events.amazonaws.com\\\""
        StatementId = "\\\"$${var.rName}-$${count.index}\\\""
      }
      path {
        FunctionName = "aws_lambda_function.test.function_name"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform {
        attributes {
          statement_id = "\\\"$${var.rName}-$${count.index}\\\""
          action = "\\\"lambda:InvokeFunction\\\""
          function_name = "aws_lambda_function.test.function_name"
          principal = "\\\"events.amazonaws.com\\\""
          region = "var.region"
        }
        identity_attributes = [
          {
            request_keys = [
              "FunctionName"
            ]
            required = true
            name = "function_name"
            terraform_path = "function_name"
          },
          {
            terraform_path = "statement_id"
            request_keys = [
              "StatementId"
            ]
            required = true
            name = "statement_id"
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
        purpose = "create"
        terraform_address = "aws_iam_role.test"
        terraform_type = "aws_iam_role"
        action = "create"
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
        terraform_type = "aws_lambda_permission"
        action = "create"
        purpose = "create"
        terraform_address = "aws_lambda_permission.test"
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
          id = "lambda"
          path = "aws-smithy/lambda.json"
          kind = "aws-smithy"
        }
      ]
      resources = [
        {
          attributes = {
            assume_role_policy = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
            name = "var.rName"
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
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          address = "aws_iam_role.test"
          identity_attributes = [
            {
              name = "role_name"
              path = "name"
              request_keys = [
                "RoleName"
              ]
              response_paths = [
                "Role.RoleName",
                "Role.Arn"
              ]
              required = true
            }
          ]
          kind = "resource"
          lifecycle = {

          }
          redaction = {

          }
          type = "aws_iam_role"
          name = "test"
        },
        {
          redaction = {

          }
          type = "aws_lambda_function"
          name = "test"
          dependencies = [
            "aws_iam_role.test"
          ]
          address = "aws_lambda_function.test"
          lifecycle = {

          }
          metadata = {
            terraform_address = "aws_lambda_function.test"
          }
          kind = "resource"
          attributes = {
            region = "var.region"
            role = "aws_iam_role.test.arn"
            runtime = "\\\"nodejs24.x\\\""
            filename = "\\\"test-fixtures/lambdatest.zip\\\""
            function_name = "var.rName"
            handler = "\\\"exports.example\\\""
          }
          operations = {
            create = {
              operation_id = "CreateFunction"
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
        },
        {
          attributes = {
            function_name = "aws_lambda_function.test.function_name"
            principal = "\\\"events.amazonaws.com\\\""
            region = "var.region"
            statement_id = "\\\"$${var.rName}-$${count.index}\\\""
            action = "\\\"lambda:InvokeFunction\\\""
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
          credential_bindings = [
            "aws_hmac"
          ]
          redaction = {

          }
          metadata = {
            terraform_address = "aws_lambda_permission.test"
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
          lifecycle = {

          }
          dependencies = [
            "aws_lambda_function.test"
          ]
          kind = "resource"
          type = "aws_lambda_permission"
          name = "test"
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/aws/lambda/Permission/list_region_override/input"
        source = "ramen convert tf"
        action = "create"
      }
    }
  }