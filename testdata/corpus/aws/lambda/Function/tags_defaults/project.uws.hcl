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
          name = "test"
          type = "aws_iam_role"
          address = "aws_iam_role.test"
          kind = "resource"
        }
      }
      body {
        AssumeRolePolicyDocument = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
        RoleName = "var.rName"
      }
    }
  }
  operation "aws_iam_role_policy_test_create" {
    sourceDescription = "iam"
    sourceOperationId = "PutRolePolicy"
    description       = "Review create create for Terraform resource aws_iam_role_policy.test"
    request {
      x-ramen-credential-bindings = [
        "aws_hmac"
      ]
      x-ramen-terraform "attributes" {
        role = "aws_iam_role.test.id"
        name = "var.rName"
        policy = "<<EOF\\n{\\n  \\\"Version\\\": \\\"2012-10-17\\\",\\n  \\\"Statement\\\": [\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"logs:CreateLogGroup\\\",\\n        \\\"logs:CreateLogStream\\\",\\n        \\\"logs:PutLogEvents\\\"\\n      ],\\n      \\\"Resource\\\": \\\"arn:$${data.aws_partition.current.partition}:logs:*:*:*\\\"\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"ec2:CreateNetworkInterface\\\",\\n        \\\"ec2:DescribeNetworkInterfaces\\\",\\n        \\\"ec2:DeleteNetworkInterface\\\",\\n        \\\"ec2:AssignPrivateIpAddresses\\\",\\n        \\\"ec2:UnassignPrivateIpAddresses\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"SNS:Publish\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"xray:PutTraceSegments\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    }\\n  ]\\n}\\nEOF"
      }
      x-ramen-terraform "object" {
        address = "aws_iam_role_policy.test"
        kind = "resource"
        name = "test"
        type = "aws_iam_role_policy"
      }
      body {
        PolicyDocument = "<<EOF\\n{\\n  \\\"Version\\\": \\\"2012-10-17\\\",\\n  \\\"Statement\\\": [\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"logs:CreateLogGroup\\\",\\n        \\\"logs:CreateLogStream\\\",\\n        \\\"logs:PutLogEvents\\\"\\n      ],\\n      \\\"Resource\\\": \\\"arn:$${data.aws_partition.current.partition}:logs:*:*:*\\\"\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"ec2:CreateNetworkInterface\\\",\\n        \\\"ec2:DescribeNetworkInterfaces\\\",\\n        \\\"ec2:DeleteNetworkInterface\\\",\\n        \\\"ec2:AssignPrivateIpAddresses\\\",\\n        \\\"ec2:UnassignPrivateIpAddresses\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"SNS:Publish\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"xray:PutTraceSegments\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    }\\n  ]\\n}\\nEOF"
        PolicyName = "var.rName"
        RoleName = "aws_iam_role.test.id"
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
        tags = "var.resource_tags"
      }
      x-ramen-terraform "object" {
        address = "aws_lambda_function.test"
        kind = "resource"
        name = "test"
        type = "aws_lambda_function"
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
    step "aws_iam_role_policy_test_create" {
      operationRef = "aws_iam_role_policy_test_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "aws_iam_role_policy.test"
        terraform_type = "aws_iam_role_policy"
      }
    }
    step "aws_lambda_function_test_create" {
      operationRef = "aws_lambda_function_test_create"
      body {
        terraform_address = "aws_lambda_function.test"
        terraform_type = "aws_lambda_function"
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
          type = "aws_iam_role"
          attributes = {
            assume_role_policy = "\\\"{\\\\n  \\\\\"Version\\\\\": \\\\\"2012-10-17\\\\\",\\\\n  \\\\\"Statement\\\\\": [\\\\n    {\\\\n      \\\\\"Action\\\\\": \\\\\"sts:AssumeRole\\\\\",\\\\n      \\\\\"Principal\\\\\": {\\\\n        \\\\\"Service\\\\\": \\\\\"lambda.amazonaws.com\\\\\"\\\\n      },\\\\n      \\\\\"Effect\\\\\": \\\\\"Allow\\\\\",\\\\n      \\\\\"Sid\\\\\": \\\\\"\\\\\"\\\\n    }\\\\n  ]\\\\n}\\\\n\\\""
            name = "var.rName"
          }
          lifecycle = {

          }
          address = "aws_iam_role.test"
          kind = "resource"
          name = "test"
          operations = {
            create = {
              source_kind = "aws-smithy"
              source_id = "iam"
              source_path = "aws-smithy/iam.json"
              operation_id = "CreateRole"
              credential_bindings = [
                "aws_hmac"
              ]
              purpose = "create"
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
        },
        {
          attributes = {
            name = "var.rName"
            policy = "<<EOF\\n{\\n  \\\"Version\\\": \\\"2012-10-17\\\",\\n  \\\"Statement\\\": [\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"logs:CreateLogGroup\\\",\\n        \\\"logs:CreateLogStream\\\",\\n        \\\"logs:PutLogEvents\\\"\\n      ],\\n      \\\"Resource\\\": \\\"arn:$${data.aws_partition.current.partition}:logs:*:*:*\\\"\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"ec2:CreateNetworkInterface\\\",\\n        \\\"ec2:DescribeNetworkInterfaces\\\",\\n        \\\"ec2:DeleteNetworkInterface\\\",\\n        \\\"ec2:AssignPrivateIpAddresses\\\",\\n        \\\"ec2:UnassignPrivateIpAddresses\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"SNS:Publish\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    },\\n    {\\n      \\\"Effect\\\": \\\"Allow\\\",\\n      \\\"Action\\\": [\\n        \\\"xray:PutTraceSegments\\\"\\n      ],\\n      \\\"Resource\\\": [\\n        \\\"*\\\"\\n      ]\\n    }\\n  ]\\n}\\nEOF"
            role = "aws_iam_role.test.id"
          }
          credential_bindings = [
            "aws_hmac"
          ]
          lifecycle = {

          }
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
          redaction = {

          }
          address = "aws_iam_role_policy.test"
          kind = "resource"
          dependencies = [
            "aws_iam_role.test",
            "data.aws_partition.current"
          ]
          metadata = {
            terraform_address = "aws_iam_role_policy.test"
          }
          type = "aws_iam_role_policy"
          name = "test"
        },
        {
          name = "test"
          attributes = {
            handler = "\\\"exports.example\\\""
            role = "aws_iam_role.test.arn"
            runtime = "\\\"nodejs24.x\\\""
            tags = "var.resource_tags"
            filename = "\\\"test-fixtures/lambdatest.zip\\\""
            function_name = "var.rName"
          }
          metadata = {
            terraform_address = "aws_lambda_function.test"
          }
          lifecycle = {

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
          redaction = {

          }
          kind = "resource"
          dependencies = [
            "aws_iam_role.test"
          ]
          credential_bindings = [
            "aws_hmac"
          ]
          address = "aws_lambda_function.test"
          type = "aws_lambda_function"
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/aws/lambda/Function/tags_defaults/input"
        source = "ramen convert tf"
      }
    }
  }