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
            name = "role_name"
            request_keys = [
              "RoleName"
            ]
            required = true
            response_paths = [
              "Role.Arn",
              "Role.RoleName"
            ]
            terraform_path = "name"
          }
        ]
        object {
          address = "aws_iam_role.test"
          kind = "resource"
          name = "test"
          type = "aws_iam_role"
        }
        version = "ramen.terraform.provenance.v1"
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
      x-ramen-terraform {
        attributes {
          filename = "\\\"test-fixtures/lambdatest.zip\\\""
          function_name = "var.rName"
          handler = "\\\"exports.example\\\""
          role = "aws_iam_role.test.arn"
          runtime = "\\\"nodejs24.x\\\""
        }
        object {
          address = "aws_lambda_function.test"
          kind = "resource"
          name = "test"
          type = "aws_lambda_function"
        }
        version = "ramen.terraform.provenance.v1"
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
            request_keys = [
              "FunctionName"
            ]
            required = true
            terraform_path = "function_name"
          },
          {
            name = "statement_id"
            request_keys = [
              "StatementId"
            ]
            required = true
            terraform_path = "statement_id"
          }
        ]
        object {
          address = "aws_lambda_permission.test"
          kind = "resource"
          name = "test"
          type = "aws_lambda_permission"
        }
        version = "ramen.terraform.provenance.v1"
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
        action = "create"
        purpose = "create"
        terraform_address = "aws_lambda_function.test"
        terraform_type = "aws_lambda_function"
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
      api_sources = [
        {
          id = "iam"
          kind = "aws-smithy"
          path = "aws-smithy/iam.json"
        },
        {
          id = "lambda"
          kind = "aws-smithy"
          path = "aws-smithy/lambda.json"
        }
      ]
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/lambda/tmpl/permission_basic/input"
        source = "ramen convert tf"
      }
      redaction {
        paths = [
          "aws_lambda_permission.test.event_source_token"
        ]
      }
      resources = [
        {
          address = "aws_iam_role.test"
          attributes = {
            assume_role_policy = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
            name = "var.rName"
          }
          credential_bindings = [
            "aws_hmac"
          ]
          identity_attributes = [
            {
              name = "role_name"
              path = "name"
              request_keys = [
                "RoleName"
              ]
              required = true
              response_paths = [
                "Role.RoleName",
                "Role.Arn"
              ]
            }
          ]
          kind = "resource"
          lifecycle = {

          }
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          name = "test"
          operations = {
            create = {
              credential_bindings = [
                "aws_hmac"
              ]
              operation_id = "CreateRole"
              purpose = "create"
              source_id = "iam"
              source_kind = "aws-smithy"
              source_path = "aws-smithy/iam.json"
            }
          }
          redaction = {

          }
          type = "aws_iam_role"
        },
        {
          address = "aws_lambda_function.test"
          attributes = {
            filename = "\\\"test-fixtures/lambdatest.zip\\\""
            function_name = "var.rName"
            handler = "\\\"exports.example\\\""
            role = "aws_iam_role.test.arn"
            runtime = "\\\"nodejs24.x\\\""
          }
          credential_bindings = [
            "aws_hmac"
          ]
          dependencies = [
            "aws_iam_role.test"
          ]
          kind = "resource"
          lifecycle = {

          }
          metadata = {
            terraform_address = "aws_lambda_function.test"
          }
          name = "test"
          operations = {
            create = {
              credential_bindings = [
                "aws_hmac"
              ]
              operation_id = "CreateFunction"
              purpose = "create"
              source_id = "lambda"
              source_kind = "aws-smithy"
              source_path = "aws-smithy/lambda.json"
            }
          }
          redaction = {

          }
          type = "aws_lambda_function"
        },
        {
          address = "aws_lambda_permission.test"
          attributes = {
            action = "\\\"lambda:InvokeFunction\\\""
            event_source_token = "$${sensitive.event_source_token}"
            function_name = "aws_lambda_function.test.function_name"
            principal = "\\\"events.amazonaws.com\\\""
            statement_id = "\\\"AllowExecutionFromCloudWatch\\\""
          }
          credential_bindings = [
            "aws_hmac"
          ]
          dependencies = [
            "aws_lambda_function.test"
          ]
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
          kind = "resource"
          lifecycle = {

          }
          metadata = {
            terraform_address = "aws_lambda_permission.test"
          }
          name = "test"
          operations = {
            create = {
              credential_bindings = [
                "aws_hmac"
              ]
              operation_id = "AddPermission"
              purpose = "create"
              source_id = "lambda"
              source_kind = "aws-smithy"
              source_path = "aws-smithy/lambda.json"
            }
          }
          redaction = {

          }
          type = "aws_lambda_permission"
        }
      ]
      version = "ramen.project.v1"
    }
  }