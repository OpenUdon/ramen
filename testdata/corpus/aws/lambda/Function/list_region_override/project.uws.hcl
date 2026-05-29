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
            terraform_path = "name"
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
        object {
          address = "aws_iam_role.test"
          kind = "resource"
          name = "test"
          type = "aws_iam_role"
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
        name = "var.rName"
        policy = "<<EOF\\n{\\n  \\\"Version\\\": \\\"2012-10-17\\\",\\n  \\\"Statement\\\": [\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"logs:CreateLogGroup\\\",\\n        \\\"logs:CreateLogStream\\\",\\n        \\\"logs:PutLogEvents\\\"\\n      ],\\n      \\\"Resource\\\": \\\"arn:$${data.aws_partition.current.partition}:logs:*:*:*\\\"\\n    }\\n  ]\\n}\\nEOF"
        role = "aws_iam_role.test.id"
      }
      x-ramen-terraform "object" {
        name = "test"
        type = "aws_iam_role_policy"
        address = "aws_iam_role_policy.test"
        kind = "resource"
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
        runtime = "\\\"nodejs24.x\\\""
        filename = "\\\"test-fixtures/lambdatest.zip\\\""
        function_name = "\\\"$${var.rName}-$${count.index}\\\""
        handler = "\\\"exports.example\\\""
        region = "var.region"
        role = "aws_iam_role.test.arn"
      }
      x-ramen-terraform "object" {
        address = "aws_lambda_function.test"
        kind = "resource"
        name = "test"
        type = "aws_lambda_function"
      }
      body {
        FunctionName = "\\\"$${var.rName}-$${count.index}\\\""
        Handler = "\\\"exports.example\\\""
        Role = "aws_iam_role.test.arn"
        Runtime = "\\\"nodejs24.x\\\""
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "aws_iam_role_test_create" {
      operationRef = "aws_iam_role_test_create"
      body {
        terraform_type = "aws_iam_role"
        action = "create"
        purpose = "create"
        terraform_address = "aws_iam_role.test"
      }
    }
    step "aws_iam_role_policy_test_create" {
      operationRef = "aws_iam_role_policy_test_create"
      body {
        purpose = "create"
        terraform_address = "aws_iam_role_policy.test"
        terraform_type = "aws_iam_role_policy"
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
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          credential_bindings = [
            "aws_hmac"
          ]
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
          redaction = {

          }
          kind = "resource"
          name = "test"
          attributes = {
            name = "var.rName"
            assume_role_policy = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
          }
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
          address = "aws_iam_role.test"
          type = "aws_iam_role"
        },
        {
          credential_bindings = [
            "aws_hmac"
          ]
          redaction = {

          }
          kind = "resource"
          attributes = {
            role = "aws_iam_role.test.id"
            name = "var.rName"
            policy = "<<EOF\\n{\\n  \\\"Version\\\": \\\"2012-10-17\\\",\\n  \\\"Statement\\\": [\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"logs:CreateLogGroup\\\",\\n        \\\"logs:CreateLogStream\\\",\\n        \\\"logs:PutLogEvents\\\"\\n      ],\\n      \\\"Resource\\\": \\\"arn:$${data.aws_partition.current.partition}:logs:*:*:*\\\"\\n    }\\n  ]\\n}\\nEOF"
          }
          lifecycle = {

          }
          dependencies = [
            "aws_iam_role.test",
            "data.aws_partition.current"
          ]
          operations = {
            create = {
              source_path = "aws-smithy/iam.json"
              operation_id = "PutRolePolicy"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "iam"
            }
          }
          address = "aws_iam_role_policy.test"
          type = "aws_iam_role_policy"
          name = "test"
          metadata = {
            terraform_address = "aws_iam_role_policy.test"
          }
        },
        {
          type = "aws_lambda_function"
          attributes = {
            filename = "\\\"test-fixtures/lambdatest.zip\\\""
            function_name = "\\\"$${var.rName}-$${count.index}\\\""
            handler = "\\\"exports.example\\\""
            region = "var.region"
            role = "aws_iam_role.test.arn"
            runtime = "\\\"nodejs24.x\\\""
          }
          lifecycle = {

          }
          operations = {
            create = {
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "lambda"
              source_path = "aws-smithy/lambda.json"
              operation_id = "CreateFunction"
              credential_bindings = [
                "aws_hmac"
              ]
            }
          }
          kind = "resource"
          dependencies = [
            "aws_iam_role.test"
          ]
          redaction = {

          }
          credential_bindings = [
            "aws_hmac"
          ]
          name = "test"
          metadata = {
            terraform_address = "aws_lambda_function.test"
          }
          address = "aws_lambda_function.test"
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/lambda/Function/list_region_override/input"
        source = "ramen convert tf"
      }
    }
  }