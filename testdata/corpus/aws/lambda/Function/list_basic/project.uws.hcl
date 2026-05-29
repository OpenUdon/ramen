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
        attributes {
          assume_role_policy = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
          name = "var.rName"
        }
      }
    }
  }
  operation "aws_iam_role_policy_test_create" {
    sourceDescription = "iam"
    sourceOperationId = "PutRolePolicy"
    description       = "Review create create for Terraform resource aws_iam_role_policy.test"
    request {
      body {
        RoleName = "aws_iam_role.test.id"
        PolicyDocument = "<<EOF\\n{\\n  \\\"Version\\\": \\\"2012-10-17\\\",\\n  \\\"Statement\\\": [\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"logs:CreateLogGroup\\\",\\n        \\\"logs:CreateLogStream\\\",\\n        \\\"logs:PutLogEvents\\\"\\n      ],\\n      \\\"Resource\\\": \\\"arn:$${data.aws_partition.current.partition}:logs:*:*:*\\\"\\n    }\\n  ]\\n}\\nEOF"
        PolicyName = "var.rName"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform "attributes" {
        role = "aws_iam_role.test.id"
        name = "var.rName"
        policy = "<<EOF\\n{\\n  \\\"Version\\\": \\\"2012-10-17\\\",\\n  \\\"Statement\\\": [\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"logs:CreateLogGroup\\\",\\n        \\\"logs:CreateLogStream\\\",\\n        \\\"logs:PutLogEvents\\\"\\n      ],\\n      \\\"Resource\\\": \\\"arn:$${data.aws_partition.current.partition}:logs:*:*:*\\\"\\n    }\\n  ]\\n}\\nEOF"
      }
      x-ramen-terraform "object" {
        type = "aws_iam_role_policy"
        address = "aws_iam_role_policy.test"
        kind = "resource"
        name = "test"
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
        function_name = "\\\"$${var.rName}-$${count.index}\\\""
        handler = "\\\"exports.example\\\""
        role = "aws_iam_role.test.arn"
      }
      x-ramen-terraform "object" {
        address = "aws_lambda_function.test"
        kind = "resource"
        name = "test"
        type = "aws_lambda_function"
      }
      body {
        Handler = "\\\"exports.example\\\""
        Role = "aws_iam_role.test.arn"
        Runtime = "\\\"nodejs24.x\\\""
        FunctionName = "\\\"$${var.rName}-$${count.index}\\\""
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
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
    step "aws_iam_role_policy_test_create" {
      operationRef = "aws_iam_role_policy_test_create"
      body {
        terraform_type = "aws_iam_role_policy"
        action = "create"
        purpose = "create"
        terraform_address = "aws_iam_role_policy.test"
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
  }
  extensions {
    x-ramen-desired-state {
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
          attributes = {
            assume_role_policy = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
            name = "var.rName"
          }
          operations = {
            create = {
              operation_id = "CreateRole"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "iam"
              source_path = "aws-smithy/iam.json"
            }
          }
          redaction = {

          }
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          name = "test"
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
          kind = "resource"
          address = "aws_iam_role.test"
          type = "aws_iam_role"
          lifecycle = {

          }
          credential_bindings = [
            "aws_hmac"
          ]
        },
        {
          lifecycle = {

          }
          operations = {
            create = {
              operation_id = "PutRolePolicy"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "iam"
              source_path = "aws-smithy/iam.json"
            }
          }
          credential_bindings = [
            "aws_hmac"
          ]
          attributes = {
            name = "var.rName"
            policy = "<<EOF\\n{\\n  \\\"Version\\\": \\\"2012-10-17\\\",\\n  \\\"Statement\\\": [\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"logs:CreateLogGroup\\\",\\n        \\\"logs:CreateLogStream\\\",\\n        \\\"logs:PutLogEvents\\\"\\n      ],\\n      \\\"Resource\\\": \\\"arn:$${data.aws_partition.current.partition}:logs:*:*:*\\\"\\n    }\\n  ]\\n}\\nEOF"
            role = "aws_iam_role.test.id"
          }
          dependencies = [
            "aws_iam_role.test",
            "data.aws_partition.current"
          ]
          metadata = {
            terraform_address = "aws_iam_role_policy.test"
          }
          address = "aws_iam_role_policy.test"
          kind = "resource"
          type = "aws_iam_role_policy"
          name = "test"
          redaction = {

          }
        },
        {
          kind = "resource"
          attributes = {
            runtime = "\\\"nodejs24.x\\\""
            filename = "\\\"test-fixtures/lambdatest.zip\\\""
            function_name = "\\\"$${var.rName}-$${count.index}\\\""
            handler = "\\\"exports.example\\\""
            role = "aws_iam_role.test.arn"
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
          type = "aws_lambda_function"
          lifecycle = {

          }
          dependencies = [
            "aws_iam_role.test"
          ]
          metadata = {
            terraform_address = "aws_lambda_function.test"
          }
          address = "aws_lambda_function.test"
          redaction = {

          }
          name = "test"
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/aws/lambda/Function/list_basic/input"
        source = "ramen convert tf"
        action = "create"
      }
      version = "ramen.project.v1"
    }
  }