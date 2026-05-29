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
          name = "var.rName"
          assume_role_policy = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
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
  operation "aws_iam_role_policy_test_create" {
    sourceDescription = "iam"
    sourceOperationId = "PutRolePolicy"
    description       = "Review create create for Terraform resource aws_iam_role_policy.test"
    request {
      body {
        PolicyDocument = "<<EOF\\n{\\n  \\\"Version\\\": \\\"2012-10-17\\\",\\n  \\\"Statement\\\": [\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"logs:CreateLogGroup\\\",\\n        \\\"logs:CreateLogStream\\\",\\n        \\\"logs:PutLogEvents\\\"\\n      ],\\n      \\\"Resource\\\": \\\"arn:$${data.aws_partition.current.partition}:logs:*:*:*\\\"\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"ec2:CreateNetworkInterface\\\",\\n        \\\"ec2:DescribeNetworkInterfaces\\\",\\n        \\\"ec2:DeleteNetworkInterface\\\",\\n        \\\"ec2:AssignPrivateIpAddresses\\\",\\n        \\\"ec2:UnassignPrivateIpAddresses\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"SNS:Publish\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"xray:PutTraceSegments\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    }\\n  ]\\n}\\nEOF"
        PolicyName = "var.rName"
        RoleName = "aws_iam_role.test.id"
      }
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform "attributes" {
        name = "var.rName"
        policy = "<<EOF\\n{\\n  \\\"Version\\\": \\\"2012-10-17\\\",\\n  \\\"Statement\\\": [\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"logs:CreateLogGroup\\\",\\n        \\\"logs:CreateLogStream\\\",\\n        \\\"logs:PutLogEvents\\\"\\n      ],\\n      \\\"Resource\\\": \\\"arn:$${data.aws_partition.current.partition}:logs:*:*:*\\\"\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"ec2:CreateNetworkInterface\\\",\\n        \\\"ec2:DescribeNetworkInterfaces\\\",\\n        \\\"ec2:DeleteNetworkInterface\\\",\\n        \\\"ec2:AssignPrivateIpAddresses\\\",\\n        \\\"ec2:UnassignPrivateIpAddresses\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"SNS:Publish\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"xray:PutTraceSegments\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    }\\n  ]\\n}\\nEOF"
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
        function_name = "var.rName"
        handler = "\\\"exports.example\\\""
        role = "aws_iam_role.test.arn"
        runtime = "\\\"nodejs24.x\\\""
        filename = "\\\"test-fixtures/lambdatest.zip\\\""
      }
      x-ramen-terraform "object" {
        name = "test"
        type = "aws_lambda_function"
        address = "aws_lambda_function.test"
        kind = "resource"
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
        terraform_type = "aws_iam_role_policy"
        action = "create"
        purpose = "create"
        terraform_address = "aws_iam_role_policy.test"
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
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/lambda/Function/basic_v6.8.0/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          id = "iam"
          path = "aws-smithy/iam.json"
          kind = "aws-smithy"
        },
        {
          path = "aws-smithy/lambda.json"
          kind = "aws-smithy"
          id = "lambda"
        }
      ]
      resources = [
        {
          type = "aws_iam_role"
          lifecycle = {

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
          attributes = {
            assume_role_policy = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
            name = "var.rName"
          }
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
          credential_bindings = [
            "aws_hmac"
          ]
          redaction = {

          }
          metadata = {
            terraform_address = "aws_iam_role.test"
          }
          address = "aws_iam_role.test"
          name = "test"
          kind = "resource"
        },
        {
          kind = "resource"
          type = "aws_iam_role_policy"
          lifecycle = {

          }
          name = "test"
          attributes = {
            name = "var.rName"
            policy = "<<EOF\\n{\\n  \\\"Version\\\": \\\"2012-10-17\\\",\\n  \\\"Statement\\\": [\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"logs:CreateLogGroup\\\",\\n        \\\"logs:CreateLogStream\\\",\\n        \\\"logs:PutLogEvents\\\"\\n      ],\\n      \\\"Resource\\\": \\\"arn:$${data.aws_partition.current.partition}:logs:*:*:*\\\"\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"ec2:CreateNetworkInterface\\\",\\n        \\\"ec2:DescribeNetworkInterfaces\\\",\\n        \\\"ec2:DeleteNetworkInterface\\\",\\n        \\\"ec2:AssignPrivateIpAddresses\\\",\\n        \\\"ec2:UnassignPrivateIpAddresses\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"SNS:Publish\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"xray:PutTraceSegments\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    }\\n  ]\\n}\\nEOF"
            role = "aws_iam_role.test.id"
          }
          operations = {
            create = {
              purpose = "create"
              source_kind = "aws-smithy"
              source_id = "iam"
              source_path = "aws-smithy/iam.json"
              operation_id = "PutRolePolicy"
              credential_bindings = [
                "aws_hmac"
              ]
            }
          }
          dependencies = [
            "aws_iam_role.test",
            "data.aws_partition.current"
          ]
          credential_bindings = [
            "aws_hmac"
          ]
          metadata = {
            terraform_address = "aws_iam_role_policy.test"
          }
          redaction = {

          }
          address = "aws_iam_role_policy.test"
        },
        {
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
          metadata = {
            terraform_address = "aws_lambda_function.test"
          }
          address = "aws_lambda_function.test"
          attributes = {
            handler = "\\\"exports.example\\\""
            role = "aws_iam_role.test.arn"
            runtime = "\\\"nodejs24.x\\\""
            filename = "\\\"test-fixtures/lambdatest.zip\\\""
            function_name = "var.rName"
          }
          dependencies = [
            "aws_iam_role.test"
          ]
          kind = "resource"
          type = "aws_lambda_function"
          redaction = {

          }
          name = "test"
          lifecycle = {

          }
        }
      ]
      redaction {

      }
    }
  }