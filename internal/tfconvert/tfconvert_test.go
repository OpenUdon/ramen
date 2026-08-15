package tfconvert

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestConvertWritesDraftArtifacts(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "openapi.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
provider "aws" {
  alias  = "west"
  region = "us-west-2"
}

data "aws_ami" "base" {
  provider = aws.west
  owners   = ["self"]
}

resource "aws_instance" "web" {
  provider = aws.west
  ami      = data.aws_ami.base.id
  name     = var.name
}

variable "name" {
  default = "web"
}

output "instance_id" {
  value = aws_instance.web.id
}
`)
	writeFileForTest(t, openAPIPath, `openapi: 3.0.0
info:
  title: AWS Test
  version: v1
paths:
  /amis/{id}:
    get:
      operationId: getAwsAmi
      responses:
        "200":
          description: ok
  /instances:
    post:
      operationId: createAwsInstance
      responses:
        "200":
          description: ok
`)

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "aws", Path: openAPIPath}},
		Action:    "create",
		OutDir:    filepath.Join(root, "out"),
	})
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	for _, path := range []string{
		result.ProjectPath,
		result.NativeProjectPath,
		result.UWSPath,
		result.ConversionPath,
		result.MappingsPath,
		result.PlanJSONPath,
		result.PlanMDPath,
		result.DiagnosticsJSON,
		result.DiagnosticsMD,
		result.ReviewPath,
		filepath.Join(result.OutDir, "openapi", "aws.yaml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("artifact %s was not written: %v", path, err)
		}
	}
	uwsText := readFileForTest(t, result.UWSPath)
	for _, expected := range []string{"createAwsInstance", "getAwsAmi", "openapi/aws.yaml", "terraform_conversion_draft", "x-ramen-terraform", TerraformProvenanceVersion, "attributes:", "ami:", "name:", "owners:"} {
		if !strings.Contains(uwsText, expected) {
			t.Fatalf("UWS missing %q:\n%s", expected, uwsText)
		}
	}
	uws := readUWSDocForTest(t, result.UWSPath)
	for _, operation := range uws.Operations {
		applicable, err := ValidateTerraformOperation(operation)
		if !applicable || err != nil {
			t.Fatalf("generated operation %q Terraform metadata: applicable=%t err=%v", operation.OperationID, applicable, err)
		}
		metadata, ok, err := ReadTerraformRequestMetadata(operation.Request)
		if err != nil || !ok || metadata.Provenance == nil || metadata.Provenance.Version != TerraformProvenanceVersion {
			t.Fatalf("generated operation %q metadata = %#v ok=%t err=%v", operation.OperationID, metadata, ok, err)
		}
	}
	if _, ok := operationBySourceIDForTest(t, uws, "createAwsInstance").Request["body"]; ok {
		t.Fatalf("unmapped OpenAPI operation should not put Terraform review attrs into request.body:\n%s", uwsText)
	}
	project := readFileForTest(t, result.ProjectPath)
	if !strings.Contains(project, "unapproved review scaffolding") || !strings.Contains(project, "aws_instance.web") {
		t.Fatalf("project did not summarize draft posture and resource:\n%s", project)
	}
}

func TestConvertDiagnosesAmbiguousOperation(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "openapi.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
data "aws_ami" "base" {
  owners = ["self"]
}
`)
	writeFileForTest(t, openAPIPath, `openapi: 3.0.0
info:
  title: AWS Test
  version: v1
paths:
  /amis/{id}:
    get:
      operationId: getAwsAmi
      responses:
        "200":
          description: ok
  /ami/{id}:
    get:
      operationId: readAwsAmi
      responses:
        "200":
          description: ok
`)

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "aws", Path: openAPIPath}},
		OutDir:    filepath.Join(root, "out"),
	})
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	if !hasDiagnostic(result.Diagnostics, "operation.ambiguous") {
		t.Fatalf("diagnostics missing operation.ambiguous: %#v", result.Diagnostics)
	}
	review := readFileForTest(t, result.ReviewPath)
	if !strings.Contains(review, "todo.data_aws_ami_base.read.read") {
		t.Fatalf("review missing deterministic TODO:\n%s", review)
	}
}

func TestConvertAWSProviderS3SingleOpenAPICorpus(t *testing.T) {
	tests := []struct {
		name                  string
		config                string
		expectedOperations    []string
		unexpectedDiagnostics []string
	}{
		{
			name: "bucket accelerate configuration",
			config: `
resource "aws_s3_bucket" "test" {
  bucket = "tf-acc-openudon-bucket"
}

resource "aws_s3_bucket_accelerate_configuration" "test" {
  bucket = aws_s3_bucket.test.bucket
  status = "Enabled"
}
`,
			expectedOperations: []string{"CreateBucket", "PutBucketAccelerateConfiguration"},
			unexpectedDiagnostics: []string{
				"operation.ambiguous",
				"operation.unresolved",
			},
		},
		{
			name: "bucket data source",
			config: `
resource "aws_s3_bucket" "test" {
  bucket = "tf-acc-openudon-bucket-ds"
}

data "aws_s3_bucket" "test" {
  bucket = aws_s3_bucket.test.id
}
`,
			expectedOperations: []string{"CreateBucket", "GetBucketLocation"},
			unexpectedDiagnostics: []string{
				"operation.ambiguous",
				"operation.unresolved",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			configDir := filepath.Join(root, "tf")
			openAPIPath := filepath.Join(root, "s3.yaml")
			writeFileForTest(t, filepath.Join(configDir, "main.tf"), tt.config)
			writeFileForTest(t, openAPIPath, s3OpenAPIForTest())

			result, err := Convert(context.Background(), Options{
				ConfigDir: configDir,
				OpenAPIs:  []OpenAPIInput{{ID: "s3", Path: openAPIPath}},
				Action:    "create",
				OutDir:    filepath.Join(root, "out"),
			})
			if err != nil {
				t.Fatalf("Convert returned error: %v", err)
			}
			for _, code := range tt.unexpectedDiagnostics {
				if hasDiagnostic(result.Diagnostics, code) {
					t.Fatalf("diagnostics should not contain %s: %#v", code, result.Diagnostics)
				}
			}
			intent := readFileForTest(t, result.UWSPath)
			workflow := readFileForTest(t, result.UWSPath)
			for _, operationID := range tt.expectedOperations {
				if !strings.Contains(intent, operationID) || !strings.Contains(workflow, operationID) {
					t.Fatalf("expected operation %q in intent and workflow\nintent:\n%s\nworkflow:\n%s", operationID, intent, workflow)
				}
			}
			for _, text := range []string{intent, workflow, readFileForTest(t, result.ReviewPath)} {
				if strings.Contains(text, "todo.") {
					t.Fatalf("known S3 corpus case should not emit operation TODOs:\n%s", text)
				}
			}
		})
	}
}

func TestConvertAWSProviderLambdaFunctionURLMultiOpenAPICorpus(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	iamOpenAPI := filepath.Join(root, "iam.yaml")
	lambdaOpenAPI := filepath.Join(root, "lambda.yaml")
	stsOpenAPI := filepath.Join(root, "sts.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
data "aws_partition" "current" {}

resource "aws_iam_role_policy" "iam_policy_for_lambda" {
  name = "tf-acc-openudon-lambda-policy"
  role = aws_iam_role.iam_for_lambda.id

  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["logs:CreateLogGroup"],
    "Resource": "arn:${data.aws_partition.current.partition}:logs:*:*:*"
  }]
}
EOF
}

resource "aws_iam_role" "iam_for_lambda" {
  name = "tf-acc-openudon-lambda-role"

  assume_role_policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [{
    "Action": "sts:AssumeRole",
    "Principal": {"Service": "lambda.amazonaws.com"},
    "Effect": "Allow"
  }]
}
EOF
}

resource "aws_lambda_function" "test" {
  filename      = "test-fixtures/lambdatest.zip"
  function_name = "tf-acc-openudon-lambda"
  role          = aws_iam_role.iam_for_lambda.arn
  handler       = "exports.example"
  runtime       = "nodejs24.x"
}

resource "aws_lambda_function_url" "test" {
  function_name      = aws_lambda_function.test.function_name
  authorization_type = "NONE"
}
`)
	writeFileForTest(t, iamOpenAPI, iamOpenAPIForTest())
	writeFileForTest(t, lambdaOpenAPI, lambdaOpenAPIForTest())
	writeFileForTest(t, stsOpenAPI, stsOpenAPIForTest())

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs: []OpenAPIInput{
			{ID: "iam", Path: iamOpenAPI},
			{ID: "lambda", Path: lambdaOpenAPI},
			{ID: "sts", Path: stsOpenAPI},
		},
		Action: "create",
		OutDir: filepath.Join(root, "out"),
		Strict: true,
	})
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	for _, code := range []string{"operation.ambiguous", "operation.unresolved"} {
		if hasDiagnostic(result.Diagnostics, code) {
			t.Fatalf("diagnostics should not contain %s: %#v", code, result.Diagnostics)
		}
	}

	intent := readFileForTest(t, result.UWSPath)
	workflow := readFileForTest(t, result.UWSPath)
	review := readFileForTest(t, result.ReviewPath)
	project := readFileForTest(t, result.ProjectPath)
	for _, expected := range []string{
		"POST_CreateRole",
		"POST_PutRolePolicy",
		"CreateFunction",
		"CreateFunctionUrlConfig",
		"openapi/iam.yaml",
		"openapi/lambda.yaml",
		"aws_hmac",
		"Action",
		"Version",
		"CreateRole",
		"PutRolePolicy",
		"2010-05-08",
	} {
		if !strings.Contains(intent, expected) || !strings.Contains(workflow, expected) {
			t.Fatalf("expected %q in intent and workflow\nintent:\n%s\nworkflow:\n%s", expected, intent, workflow)
		}
	}
	if !strings.Contains(workflow, "FunctionName") || !strings.Contains(workflow, "aws_lambda_function.test.function_name") {
		t.Fatalf("workflow should bind Lambda FunctionName path parameter from function_name:\n%s", workflow)
	}
	uws := readUWSDocForTest(t, result.UWSPath)
	iamRole := operationBySourceIDForTest(t, uws, "POST_CreateRole")
	assertRequestValueForTest(t, iamRole, "query", "Action", "CreateRole")
	assertRequestValueForTest(t, iamRole, "query", "Version", "2010-05-08")
	rolePolicy := operationBySourceIDForTest(t, uws, "POST_PutRolePolicy")
	assertRequestValueForTest(t, rolePolicy, "query", "Action", "PutRolePolicy")
	assertRequestStringContainsForTest(t, rolePolicy, "body", "PolicyDocument", "logs:CreateLogGroup")
	createFunction := operationBySourceIDForTest(t, uws, "CreateFunction")
	assertRequestValueForTest(t, createFunction, "body", "FunctionName", `"tf-acc-openudon-lambda"`)
	assertRequestValueForTest(t, createFunction, "body", "Role", "aws_iam_role.iam_for_lambda.arn")
	lambdaURL := operationBySourceIDForTest(t, uws, "CreateFunctionUrlConfig")
	assertRequestValueForTest(t, lambdaURL, "path", "FunctionName", "aws_lambda_function.test.function_name")
	assertRequestValueForTest(t, lambdaURL, "body", "authorization_type", `"NONE"`)
	for _, text := range []string{intent, workflow, review} {
		if strings.Contains(text, "todo.") || strings.Contains(text, "aws_partition.current_read") {
			t.Fatalf("known Lambda/IAM corpus case should not emit operation TODOs or partition operations:\n%s", text)
		}
	}
	if !strings.Contains(project, "data.aws_partition.current") || !strings.Contains(project, "provider-local metadata") {
		t.Fatalf("project should classify aws_partition as provider-local metadata:\n%s", project)
	}
}

func TestConvertAWSCredentialBindingPreservesProviderAlias(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "s3.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
provider "aws" {
  alias  = "west"
  region = "us-west-2"
}

resource "aws_s3_bucket" "test" {
  provider = aws.west
  bucket   = "tf-acc-openudon-alias"
}
`)
	writeFileForTest(t, openAPIPath, s3OpenAPIWithHMACForTest())

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "s3", Path: openAPIPath}},
		Action:    "create",
		OutDir:    filepath.Join(root, "out"),
		Strict:    true,
	})
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	intent := readFileForTest(t, result.UWSPath)
	workflow := readFileForTest(t, result.UWSPath)
	project := readFileForTest(t, result.ProjectPath)
	for _, expected := range []string{"aws_west_hmac", "aws_west"} {
		if !strings.Contains(intent, expected) || !strings.Contains(workflow, expected) || !strings.Contains(project, expected) {
			t.Fatalf("expected aliased credential binding %q\nintent:\n%s\nworkflow:\n%s\nproject:\n%s", expected, intent, workflow, project)
		}
	}
	if strings.Contains(workflow, `Authorization = "aws_hmac"`) {
		t.Fatalf("workflow collapsed aliased AWS credential to default aws_hmac:\n%s", workflow)
	}
}

func TestConvertAWSCallerIdentityDataSourceUsesSTS(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	stsOpenAPI := filepath.Join(root, "sts.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
data "aws_caller_identity" "current" {}
`)
	writeFileForTest(t, stsOpenAPI, stsOpenAPIForTest())

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "sts", Path: stsOpenAPI}},
		OutDir:    filepath.Join(root, "out"),
		Strict:    true,
	})
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	if hasDiagnostic(result.Diagnostics, "operation.ambiguous") || hasDiagnostic(result.Diagnostics, "operation.unresolved") {
		t.Fatalf("caller identity should map to STS without operation diagnostics: %#v", result.Diagnostics)
	}
	intent := readFileForTest(t, result.UWSPath)
	workflow := readFileForTest(t, result.UWSPath)
	for _, expected := range []string{"POST_GetCallerIdentity", "openapi/sts.yaml", "GetCallerIdentity", "2011-06-15", "aws_hmac"} {
		if !strings.Contains(intent, expected) || !strings.Contains(workflow, expected) {
			t.Fatalf("expected %q in caller identity intent and workflow\nintent:\n%s\nworkflow:\n%s", expected, intent, workflow)
		}
	}
	project := readFileForTest(t, result.ProjectPath)
	if strings.Contains(project, "data.aws_caller_identity.current` is provider-local metadata") {
		t.Fatalf("caller identity should not be classified as provider-local metadata:\n%s", project)
	}
}

func TestConvertAWSIAMRoleUsesNativeSmithySource(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	smithyPath := filepath.Join(root, "iam.json")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "tf-acc-openudon-role"
  assume_role_policy = "{}"
}
`)
	writeFileForTest(t, smithyPath, minimalIAMSmithyForTest())

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: smithyPath,
		}},
		Action: "create",
		OutDir: filepath.Join(root, "out"),
		Strict: true,
	})
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	intent := readFileForTest(t, result.UWSPath)
	workflow := readFileForTest(t, result.UWSPath)
	for _, expected := range []string{"aws-smithy/iam.json", "CreateRole", "RoleName", "AssumeRolePolicyDocument"} {
		if !strings.Contains(intent, expected) || !strings.Contains(workflow, expected) {
			t.Fatalf("expected native Smithy mapping %q\nintent:\n%s\nworkflow:\n%s", expected, intent, workflow)
		}
	}
	if !strings.Contains(intent, "aws_hmac") {
		t.Fatalf("intent missing symbolic AWS credential binding:\n%s", intent)
	}
	uws := readUWSDocForTest(t, result.UWSPath)
	role := operationBySourceIDForTest(t, uws, "CreateRole")
	assertRequestValueForTest(t, role, "body", "RoleName", `"tf-acc-openudon-role"`)
	assertRequestValueForTest(t, role, "body", "AssumeRolePolicyDocument", `"{}"`)
	if _, err := os.Stat(filepath.Join(result.OutDir, "openapi")); !os.IsNotExist(err) {
		t.Fatalf("native Smithy conversion should not stage OpenAPI fallback, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(result.OutDir, "aws-smithy", "iam.json")); err != nil {
		t.Fatalf("staged Smithy source missing: %v", err)
	}
	if _, err := os.Stat(result.NativeProjectPath); err != nil {
		t.Fatalf("native project missing: %v", err)
	}
	nativeDoc, err := project.Load(result.NativeProjectPath)
	if err != nil {
		t.Fatalf("load native project: %v", err)
	}
	if len(nativeDoc.Profile.Resources) != 1 || nativeDoc.Profile.Resources[0].Operations["create"].OperationID != "CreateRole" {
		t.Fatalf("unexpected native project profile: %#v", nativeDoc.Profile)
	}
	planResult, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: result.NativeProjectPath,
		StatePath:   filepath.Join(root, "state.db"),
	})
	if err != nil {
		t.Fatalf("plan native project: %v", err)
	}
	if planResult.Plan.Errored || planResult.Plan.Summary.Create != 1 || planResult.Plan.Resources[0].Mapping.OperationID != "CreateRole" {
		t.Fatalf("converted native project did not plan as create: %#v", planResult.Plan)
	}
	mappings := readMappingsForTest(t, result.MappingsPath)
	identity := identityForTest(t, mappings, "aws_iam_role.role", "role_name")
	if identity.TerraformPath != "name" || !equalStringsForTest(identity.RequestKeys, []string{"RoleName"}) || !equalStringsForTest(identity.ResponsePaths, []string{"Role.RoleName", "Role.Arn"}) {
		t.Fatalf("unexpected IAM role identity metadata: %#v", identity)
	}
	review := readFileForTest(t, result.ReviewPath)
	if !strings.Contains(review, "Identity `role_name`") || !strings.Contains(review, "Role.Arn") {
		t.Fatalf("review missing IAM identity metadata:\n%s", review)
	}
}

func TestConvertGitHubRepositoryDerivesOwnerFromProviderConfig(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "github.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
provider "github" {
  owner = var.github_owner
}

variable "github_owner" {
  default = "github-owner-placeholder"
}

resource "github_repository" "repo" {
  name        = "ramen-parity-h01-static"
  description = "Ramen GitHub H01 parity fixture"
  visibility  = "private"
  auto_init   = true
}
`)
	writeFileForTest(t, openAPIPath, `openapi: 3.0.3
info:
  title: GitHub Test
  version: v1
paths:
  /orgs/{org}/repos:
    post:
      operationId: repos/create-in-org
      parameters:
        - name: org
          in: path
          required: true
          schema:
            type: string
      requestBody:
        content:
          application/json:
            schema:
              type: object
      responses:
        "201":
          description: created
`)

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "github", Path: openAPIPath}},
		Action:    "create",
		OutDir:    filepath.Join(root, "out"),
		Strict:    true,
	})
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	nativeDoc, err := project.Load(result.NativeProjectPath)
	if err != nil {
		t.Fatalf("load native project: %v", err)
	}
	if len(nativeDoc.Profile.Resources) != 1 {
		t.Fatalf("native project resources = %#v", nativeDoc.Profile.Resources)
	}
	repo := nativeDoc.Profile.Resources[0]
	if got := repo.Attributes["owner"]; got != "var.github_owner" {
		t.Fatalf("derived GitHub owner attribute = %#v, want provider expression", got)
	}
	if repo.Operations["create"].OperationID != "repos/create-in-org" {
		t.Fatalf("GitHub create operation = %#v", repo.Operations["create"])
	}
	var foundOwnerBinding bool
	for _, binding := range repo.RequestBindings {
		if binding.OperationRole == "create" && binding.Path == "owner" && binding.RequestPath == "org" && binding.Location == "path" {
			foundOwnerBinding = true
		}
	}
	if !foundOwnerBinding {
		t.Fatalf("native project request bindings missing GitHub owner/org binding: %#v", repo.RequestBindings)
	}
}

func TestConvertAWSS3BucketVersioningNestsDottedTerraformAttributes(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "s3.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "aws_s3_bucket" "test" {
  bucket = "tf-acc-openudon-bucket-versioning"
}

resource "aws_s3_bucket_versioning" "test" {
  bucket = aws_s3_bucket.test.bucket

  versioning_configuration {
    status = "Enabled"
  }
}
`)
	writeFileForTest(t, openAPIPath, s3OpenAPIForTest())

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "s3", Path: openAPIPath}},
		Action:    "create",
		OutDir:    filepath.Join(root, "out"),
		Strict:    true,
	})
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	uws := readUWSDocForTest(t, result.UWSPath)
	op := operationBySourceIDForTest(t, uws, "PutBucketVersioning")
	body, ok := op.Request["body"].(map[string]any)
	if !ok {
		t.Fatalf("PutBucketVersioning body missing: %#v", op.Request)
	}
	config, ok := body["VersioningConfiguration"].(map[string]any)
	if !ok {
		t.Fatalf("PutBucketVersioning body did not nest versioning configuration: %#v", body)
	}
	if got := config["Status"]; got != `"Enabled"` {
		t.Fatalf("unexpected versioning status binding: %#v", got)
	}
	nativeDoc, err := project.Load(result.NativeProjectPath)
	if err != nil {
		t.Fatalf("load native project: %v", err)
	}
	var versioning *project.Resource
	for i := range nativeDoc.Profile.Resources {
		if nativeDoc.Profile.Resources[i].Type == "aws_s3_bucket_versioning" {
			versioning = &nativeDoc.Profile.Resources[i]
			break
		}
	}
	if versioning == nil {
		t.Fatalf("native project missing bucket versioning resource: %#v", nativeDoc.Profile.Resources)
	}
	nested, ok := versioning.Attributes["versioning_configuration"].(map[string]any)
	if !ok || nested["status"] != `"Enabled"` {
		t.Fatalf("native project did not nest dotted Terraform attributes: %#v", versioning.Attributes)
	}
}

func TestSetRequestBindingNestsDottedBodyKeys(t *testing.T) {
	path := map[string]any{}
	query := map[string]any{}
	header := map[string]any{}
	cookie := map[string]any{}
	body := map[string]any{}

	setRequestBinding("body", "PublicAccessBlockConfiguration.BlockPublicPolicy", false, path, query, header, cookie, body)

	config, ok := body["PublicAccessBlockConfiguration"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested body configuration, got %#v", body)
	}
	if got := config["BlockPublicPolicy"]; got != false {
		t.Fatalf("unexpected nested body value: %#v", got)
	}
	if _, ok := body["PublicAccessBlockConfiguration.BlockPublicPolicy"]; ok {
		t.Fatalf("body retained flat dotted key: %#v", body)
	}
}

func TestConvertGoogleStorageBucketUsesNativeDiscoverySource(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	discoveryPath := filepath.Join(root, "storage.json")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "google_storage_bucket" "bucket" {
  name     = "openudon-bucket"
  location = "US"
  project  = "review-project"
}
`)
	writeFileForTest(t, discoveryPath, minimalStorageDiscoveryForTest())

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		APISources: []APISourceInput{{
			Kind: "google-discovery",
			ID:   "storage",
			Path: discoveryPath,
		}},
		Action: "create",
		OutDir: filepath.Join(root, "out"),
		Strict: true,
	})
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	intent := readFileForTest(t, result.UWSPath)
	workflow := readFileForTest(t, result.UWSPath)
	for _, expected := range []string{"google-discovery/storage.json", "storage.buckets.insert", "project", "location"} {
		if !strings.Contains(intent, expected) || !strings.Contains(workflow, expected) {
			t.Fatalf("expected native Discovery mapping %q\nintent:\n%s\nworkflow:\n%s", expected, intent, workflow)
		}
	}
	if !strings.Contains(intent, "google_oauth2") {
		t.Fatalf("intent missing symbolic Google credential binding:\n%s", intent)
	}
	uws := readUWSDocForTest(t, result.UWSPath)
	insert := operationBySourceIDForTest(t, uws, "storage.buckets.insert")
	assertRequestValueForTest(t, insert, "query", "project", `"review-project"`)
	assertRequestValueForTest(t, insert, "body", "name", `"openudon-bucket"`)
	assertRequestValueForTest(t, insert, "body", "location", `"US"`)
	if _, err := os.Stat(filepath.Join(result.OutDir, "openapi")); !os.IsNotExist(err) {
		t.Fatalf("native Discovery conversion should not stage OpenAPI fallback, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(result.OutDir, "google-discovery", "storage.json")); err != nil {
		t.Fatalf("staged Discovery source missing: %v", err)
	}
	mappings := readMappingsForTest(t, result.MappingsPath)
	identity := identityForTest(t, mappings, "google_storage_bucket.bucket", "bucket_name")
	if identity.TerraformPath != "name" || !equalStringsForTest(identity.RequestKeys, []string{"name"}) || !equalStringsForTest(identity.ResponsePaths, []string{"name", "id"}) {
		t.Fatalf("unexpected Google bucket identity metadata: %#v", identity)
	}
	review := readFileForTest(t, result.ReviewPath)
	if !strings.Contains(review, "Identity `bucket_name`") || !strings.Contains(review, "name, id") {
		t.Fatalf("review missing Google identity metadata:\n%s", review)
	}
}

func TestConvertRedactsSensitiveCandidate(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "openapi.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "example_secret" "main" {
  api_token = "do-not-emit"
  config = {
    password = "nested-secret-do-not-emit"
  }
}
`)
	writeFileForTest(t, openAPIPath, `openapi: 3.0.0
info:
  title: Secret Test
  version: v1
paths:
  /secrets:
    post:
      operationId: createExampleSecret
      responses:
        "200":
          description: ok
`)

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "secrets", Path: openAPIPath}},
		Action:    "create",
		OutDir:    filepath.Join(root, "out"),
	})
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	for _, path := range []string{result.ProjectPath, result.UWSPath, result.DiagnosticsJSON, result.DiagnosticsMD, result.ReviewPath} {
		text := readFileForTest(t, path)
		for _, leaked := range []string{"do-not-emit", "nested-secret-do-not-emit"} {
			if strings.Contains(text, leaked) {
				t.Fatalf("%s leaked sensitive literal %q:\n%s", path, leaked, text)
			}
		}
	}
	if !hasDiagnostic(result.Diagnostics, "redaction.review_required") {
		t.Fatalf("diagnostics missing redaction review: %#v", result.Diagnostics)
	}
}

func TestConvertRejectsOpenAPIInputInsideStagingDir(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "openapi", "app.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "example_resource" "main" {
  name = "web"
}
`)
	writeFileForTest(t, openAPIPath, `openapi: 3.0.0
info:
  title: Staging Safety
  version: v1
paths:
  /resources:
    post:
      operationId: createExampleResource
      responses:
        "200":
          description: ok
`)

	_, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "app", Path: openAPIPath}},
		Action:    "create",
		OutDir:    root,
	})
	if err == nil {
		t.Fatal("Convert returned nil error for OpenAPI input inside staging directory")
	}
	if !strings.Contains(err.Error(), "staging directory") {
		t.Fatalf("error did not describe staging overlap: %v", err)
	}
	if text := readFileForTest(t, openAPIPath); !strings.Contains(text, "Staging Safety") {
		t.Fatalf("OpenAPI source was modified or deleted:\n%s", text)
	}
}

func TestConvertRejectsMalformedOpenAPIInputInsideStagingDirWithoutDeletingIt(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "openapi", "app.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "example_resource" "main" {
  name = "web"
}
`)
	writeFileForTest(t, openAPIPath, `not: openapi
`)

	_, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "app", Path: openAPIPath}},
		Action:    "create",
		OutDir:    root,
	})
	if err == nil {
		t.Fatal("Convert returned nil error for malformed OpenAPI input inside staging directory")
	}
	if !strings.Contains(err.Error(), "staging directory") {
		t.Fatalf("error did not describe staging overlap: %v", err)
	}
	if text := readFileForTest(t, openAPIPath); !strings.Contains(text, "not: openapi") {
		t.Fatalf("malformed OpenAPI source was modified or deleted:\n%s", text)
	}
}

func TestConvertRejectsUnownedPreexistingAPISourceStagingDir(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "source.yaml")
	outDir := filepath.Join(root, "out")
	unownedPath := filepath.Join(outDir, "openapi", "keep.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "example_resource" "main" {
  name = "web"
}
`)
	writeFileForTest(t, openAPIPath, `openapi: 3.0.0
info:
  title: Source
  version: v1
paths:
  /resources:
    post:
      operationId: createExampleResource
      responses:
        "200":
          description: ok
`)
	writeFileForTest(t, unownedPath, "do not delete\n")

	_, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "source", Path: openAPIPath}},
		Action:    "create",
		OutDir:    outDir,
	})
	if err == nil {
		t.Fatal("Convert returned nil error for unowned pre-existing API source staging directory")
	}
	if !strings.Contains(err.Error(), "not marked as owned") {
		t.Fatalf("error did not describe ownership marker failure: %v", err)
	}
	if got := readFileForTest(t, unownedPath); got != "do not delete\n" {
		t.Fatalf("unowned API source staging content was modified or deleted: %q", got)
	}
}

func TestConvertDiagnosesOpenAPIPackagePathCollision(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	firstOpenAPIPath := filepath.Join(root, "first.yaml")
	secondOpenAPIPath := filepath.Join(root, "second.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "example_resource" "main" {
  name = "web"
}
`)
	writeFileForTest(t, firstOpenAPIPath, `openapi: 3.0.0
info:
  title: First
  version: v1
paths:
  /resources:
    post:
      operationId: createExampleResource
      responses:
        "200":
          description: ok
`)
	writeFileForTest(t, secondOpenAPIPath, `openapi: 3.0.0
info:
  title: Second
  version: v1
paths:
  /other:
    post:
      operationId: createOther
      responses:
        "200":
          description: ok
`)

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs: []OpenAPIInput{
			{ID: "a-b", Path: firstOpenAPIPath},
			{ID: "a_b", Path: secondOpenAPIPath},
		},
		Action: "create",
		OutDir: filepath.Join(root, "out"),
		Strict: true,
	})
	if !IsStrictFailure(err) {
		t.Fatalf("Convert error = %v, want strict failure", err)
	}
	if result == nil || !hasDiagnostic(result.Diagnostics, "api_source.package_path_collision") {
		t.Fatalf("diagnostics missing package path collision: result=%#v", result)
	}
	if staged := readFileForTest(t, filepath.Join(result.OutDir, "openapi", "a_b.yaml")); !strings.Contains(staged, "First") || strings.Contains(staged, "Second") {
		t.Fatalf("staged OpenAPI was overwritten:\n%s", staged)
	}
}

func TestConvertStrictMissingAPISourceReturnsStrictFailureBeforeSynthesis(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "aws_instance" "web" {
  name = "web"
}
`)

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		Action:    "create",
		OutDir:    filepath.Join(root, "out"),
		Strict:    true,
	})
	if !IsStrictFailure(err) {
		t.Fatalf("Convert error = %T %v, want strict failure", err, err)
	}
	if result == nil || !result.StrictFailed || !hasDiagnostic(result.Diagnostics, "api_source.missing") {
		t.Fatalf("result missing strict api_source.missing diagnostic: %#v", result)
	}
	if _, statErr := os.Stat(result.DiagnosticsJSON); statErr != nil {
		t.Fatalf("strict conversion did not write diagnostics: %v", statErr)
	}
}

func TestConvertWritesReviewArtifactsForUnresolvedTODOs(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "openapi.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
data "aws_ami" "base" {
  owners = ["self"]
}
`)
	writeFileForTest(t, openAPIPath, `openapi: 3.0.0
info:
  title: Unrelated
  version: v1
paths:
  /users:
    get:
      operationId: getUser
      responses:
        "200":
          description: ok
`)

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "users", Path: openAPIPath}},
		OutDir:    filepath.Join(root, "out"),
	})
	if err != nil {
		t.Fatalf("Convert returned error outside strict mode: %v", err)
	}
	if _, err := os.Stat(result.UWSPath); err != nil {
		t.Fatalf("UWS should still be generated for review: %v", err)
	}
	review := readFileForTest(t, result.ReviewPath)
	if !strings.Contains(review, "todo.") {
		t.Fatalf("review should include unresolved TODO:\n%s", review)
	}
	uws := readUWSDocForTest(t, result.UWSPath)
	if len(uws.Operations) == 0 {
		t.Fatal("unresolved conversion emitted no operations")
	}
	for _, operation := range uws.Operations {
		if operation.ExtensionProfile() != TerraformReviewTODOProfile {
			t.Fatalf("unresolved operation profile = %q", operation.ExtensionProfile())
		}
		metadata, ok, err := ReadTerraformRequestMetadata(operation.Request)
		if err != nil || !ok || metadata.TODO == "" {
			t.Fatalf("unresolved operation metadata = %#v ok=%t err=%v", metadata, ok, err)
		}
	}
	if text := readFileForTest(t, result.UWSPath); strings.Contains(text, retiredTerraformReviewTODOProfile) {
		t.Fatalf("unresolved workflow retained retired profile:\n%s", text)
	}
}

func TestConvertStrictModeFailsUnresolvedTODOs(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "openapi.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
data "aws_ami" "base" {
  owners = ["self"]
}
`)
	writeFileForTest(t, openAPIPath, `openapi: 3.0.0
info:
  title: Unrelated
  version: v1
paths:
  /users:
    get:
      operationId: getUser
      responses:
        "200":
          description: ok
`)

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "users", Path: openAPIPath}},
		OutDir:    filepath.Join(root, "out"),
		Strict:    true,
	})
	if err == nil {
		t.Fatal("Convert succeeded in strict mode with unresolved TODO")
	}
	if !IsStrictFailure(err) {
		t.Fatalf("error is not strict failure: %T %v", err, err)
	}
	if result == nil || !result.StrictFailed {
		t.Fatalf("result did not report strict failure: %#v", result)
	}
	if !hasDiagnostic(result.Diagnostics, "mapping.unsupported_type") {
		t.Fatalf("diagnostics missing public mapping code: %#v", result.Diagnostics)
	}
	if _, statErr := os.Stat(result.DiagnosticsJSON); statErr != nil {
		t.Fatalf("strict conversion did not write diagnostics: %v", statErr)
	}
}

func TestConvertOutputIsDeterministic(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "openapi.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "aws_instance" "web" {
  name = "web"
}
`)
	writeFileForTest(t, openAPIPath, `openapi: 3.0.0
info:
  title: AWS Test
  version: v1
paths:
  /instances:
    post:
      operationId: createAwsInstance
      responses:
        "200":
          description: ok
`)
	opts := Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "aws", Path: openAPIPath}},
		Action:    "create",
		OutDir:    filepath.Join(root, "out"),
	}
	first, err := Convert(context.Background(), opts)
	if err != nil {
		t.Fatalf("first Convert returned error: %v", err)
	}
	firstIntent := readFileForTest(t, first.UWSPath)
	firstDiagnostics := readFileForTest(t, first.DiagnosticsJSON)
	second, err := Convert(context.Background(), opts)
	if err != nil {
		t.Fatalf("second Convert returned error: %v", err)
	}
	if got := readFileForTest(t, second.UWSPath); got != firstIntent {
		t.Fatalf("UWS output was not deterministic:\nfirst:\n%s\nsecond:\n%s", firstIntent, got)
	}
	if got := readFileForTest(t, second.DiagnosticsJSON); got != firstDiagnostics {
		t.Fatalf("diagnostics output was not deterministic:\nfirst:\n%s\nsecond:\n%s", firstDiagnostics, got)
	}
}

func TestConvertPrunesStaleStagedOpenAPIs(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	firstOpenAPI := filepath.Join(root, "first.yaml")
	secondOpenAPI := filepath.Join(root, "second.yaml")
	outDir := filepath.Join(root, "out")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "aws_instance" "web" {
  name = "web"
}
`)
	writeFileForTest(t, firstOpenAPI, `openapi: 3.0.0
info:
  title: First
  version: v1
paths:
  /instances:
    post:
      operationId: createAwsInstance
      responses:
        "200":
          description: ok
`)
	writeFileForTest(t, secondOpenAPI, `openapi: 3.0.0
info:
  title: Second
  version: v1
paths:
  /instances:
    post:
      operationId: createAwsInstance
      responses:
        "200":
          description: ok
`)

	if _, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "first", Path: firstOpenAPI}},
		Action:    "create",
		OutDir:    outDir,
	}); err != nil {
		t.Fatalf("first Convert returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "openapi", "first.yaml")); err != nil {
		t.Fatalf("first staged OpenAPI was not written: %v", err)
	}

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "second", Path: secondOpenAPI}},
		Action:    "create",
		OutDir:    outDir,
	})
	if err != nil {
		t.Fatalf("second Convert returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "openapi", "first.yaml")); !os.IsNotExist(err) {
		t.Fatalf("stale staged OpenAPI should have been pruned, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(result.OutDir, "openapi", "second.yaml")); err != nil {
		t.Fatalf("current staged OpenAPI missing: %v", err)
	}
}

func TestConvertNamespacesDuplicateOperationIDsAcrossOpenAPIs(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	firstOpenAPI := filepath.Join(root, "first.yaml")
	secondOpenAPI := filepath.Join(root, "second.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "aws_instance" "web" {
  name = "web"
}
`)
	spec := `openapi: 3.0.0
info:
  title: AWS Test
  version: v1
paths:
  /instances:
    post:
      operationId: createAwsInstance
      responses:
        "200":
          description: ok
`
	writeFileForTest(t, firstOpenAPI, spec)
	writeFileForTest(t, secondOpenAPI, spec)
	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs:  []OpenAPIInput{{ID: "first", Path: firstOpenAPI}, {ID: "second", Path: secondOpenAPI}},
		Action:    "create",
		OutDir:    filepath.Join(root, "out"),
	})
	if err != nil {
		t.Fatalf("Convert returned error for duplicate cross-document operation IDs: %v", err)
	}
	if hasDiagnostic(result.Diagnostics, "openapi.index_error") {
		t.Fatalf("cross-document duplicate operation IDs produced index error: %#v", result.Diagnostics)
	}
}

func TestConvertAWSHardcodedMappingPrefersExpectedSourceID(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	wrongOpenAPI := filepath.Join(root, "aaa.yaml")
	iamOpenAPI := filepath.Join(root, "iam.yaml")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "tf-acc-openudon-role"
}
`)
	writeFileForTest(t, wrongOpenAPI, `openapi: 3.0.0
info:
  title: Wrong Service
  version: v1
paths:
  /wrong:
    post:
      operationId: POST_CreateRole
      responses:
        "200":
          description: ok
`)
	writeFileForTest(t, iamOpenAPI, iamOpenAPIForTest())

	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir,
		OpenAPIs: []OpenAPIInput{
			{ID: "aaa", Path: wrongOpenAPI},
			{ID: "iam", Path: iamOpenAPI},
		},
		Action: "create",
		OutDir: filepath.Join(root, "out"),
		Strict: true,
	})
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	if hasDiagnostic(result.Diagnostics, "operation.ambiguous") || hasDiagnostic(result.Diagnostics, "operation.unresolved") {
		t.Fatalf("expected IAM source ID to disambiguate duplicate operation ID: %#v", result.Diagnostics)
	}
	intent := readFileForTest(t, result.UWSPath)
	if !strings.Contains(intent, `url: openapi/iam.yaml`) {
		t.Fatalf("IAM operation did not bind to iam source:\n%s", intent)
	}
}

func hasDiagnostic(diags []Diagnostic, code string) bool {
	for _, diag := range diags {
		if diag.Code == code {
			return true
		}
	}
	return false
}

type mappingArtifactForTest struct {
	Address            string                     `json:"address"`
	IdentityAttributes []identityAttributeForTest `json:"identity_attributes"`
}

type identityAttributeForTest struct {
	Name          string   `json:"name"`
	TerraformPath string   `json:"terraform_path"`
	RequestKeys   []string `json:"request_keys"`
	ResponsePaths []string `json:"response_paths"`
	Required      bool     `json:"required"`
}

func readMappingsForTest(t *testing.T, path string) []mappingArtifactForTest {
	t.Helper()
	var mappings []mappingArtifactForTest
	if err := json.Unmarshal([]byte(readFileForTest(t, path)), &mappings); err != nil {
		t.Fatalf("failed to parse mappings artifact: %v", err)
	}
	return mappings
}

func identityForTest(t *testing.T, mappings []mappingArtifactForTest, address, name string) identityAttributeForTest {
	t.Helper()
	for _, mapping := range mappings {
		if mapping.Address != address {
			continue
		}
		for _, identity := range mapping.IdentityAttributes {
			if identity.Name == name {
				return identity
			}
		}
		t.Fatalf("mapping %s missing identity %s: %#v", address, name, mapping.IdentityAttributes)
	}
	t.Fatalf("mapping for %s not found: %#v", address, mappings)
	return identityAttributeForTest{}
}

func equalStringsForTest(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func writeFileForTest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFileForTest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readUWSDocForTest(t *testing.T, path string) *uws1.Document {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc uws1.Document
	if err := uwsconvert.UnmarshalYAML(data, &doc); err != nil {
		t.Fatalf("failed to parse UWS %s: %v\n%s", path, err, data)
	}
	return &doc
}

func operationBySourceIDForTest(t *testing.T, doc *uws1.Document, sourceOperationID string) *uws1.Operation {
	t.Helper()
	for _, op := range doc.Operations {
		if op != nil && op.SourceOperationID == sourceOperationID {
			return op
		}
	}
	t.Fatalf("operation with sourceOperationId %q not found", sourceOperationID)
	return nil
}

func assertRequestValueForTest(t *testing.T, op *uws1.Operation, location, key string, want any) {
	t.Helper()
	locationMap, ok := op.Request[location].(map[string]any)
	if !ok {
		t.Fatalf("%s request for %s missing or wrong type: %#v", location, op.OperationID, op.Request)
	}
	if got := locationMap[key]; got != want {
		t.Fatalf("%s request %s.%s = %#v, want %#v; request=%#v", op.OperationID, location, key, got, want, op.Request)
	}
}

func assertRequestStringContainsForTest(t *testing.T, op *uws1.Operation, location, key, wantSubstring string) {
	t.Helper()
	locationMap, ok := op.Request[location].(map[string]any)
	if !ok {
		t.Fatalf("%s request for %s missing or wrong type: %#v", location, op.OperationID, op.Request)
	}
	got, ok := locationMap[key].(string)
	if !ok || !strings.Contains(got, wantSubstring) {
		t.Fatalf("%s request %s.%s = %#v, want string containing %q; request=%#v", op.OperationID, location, key, locationMap[key], wantSubstring, op.Request)
	}
}

func s3OpenAPIForTest() string {
	return `openapi: 3.0.0
info:
  title: Amazon Simple Storage Service
  version: "2006-03-01"
paths:
  /:
    get:
      operationId: ListBuckets
      responses:
        "200":
          description: ok
  /{Bucket}:
    put:
      operationId: CreateBucket
      parameters:
        - name: Bucket
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
    head:
      operationId: HeadBucket
      parameters:
        - name: Bucket
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
  /{Bucket}#accelerate:
    get:
      operationId: GetBucketAccelerateConfiguration
      parameters:
        - name: Bucket
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
    put:
      operationId: PutBucketAccelerateConfiguration
      parameters:
        - name: Bucket
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
  /{Bucket}#versioning:
    get:
      operationId: GetBucketVersioning
      parameters:
        - name: Bucket
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
    put:
      operationId: PutBucketVersioning
      parameters:
        - name: Bucket
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
  /{Bucket}#location:
    get:
      operationId: GetBucketLocation
      parameters:
        - name: Bucket
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
`
}

func s3OpenAPIWithHMACForTest() string {
	return strings.Replace(s3OpenAPIForTest(), "paths:\n", `security:
  - hmac: []
paths:
`, 1) + `components:
  securitySchemes:
    hmac:
      type: apiKey
      name: Authorization
      in: header
`
}

func iamOpenAPIForTest() string {
	return `openapi: 3.0.0
info:
  title: AWS Identity and Access Management
  version: "2010-05-08"
security:
  - hmac: []
paths:
  /#Action=CreateRole:
    post:
      operationId: POST_CreateRole
      parameters:
        - name: Action
          in: query
          required: true
          schema:
            type: string
            enum: [CreateRole]
        - name: Version
          in: query
          required: true
          schema:
            type: string
            enum: ["2010-05-08"]
      responses:
        "200":
          description: ok
  /#Action=PutRolePolicy:
    post:
      operationId: POST_PutRolePolicy
      parameters:
        - name: Action
          in: query
          required: true
          schema:
            type: string
            enum: [PutRolePolicy]
        - name: Version
          in: query
          required: true
          schema:
            type: string
            enum: ["2010-05-08"]
      responses:
        "200":
          description: ok
components:
  securitySchemes:
    hmac:
      type: apiKey
      name: Authorization
      in: header
`
}

func lambdaOpenAPIForTest() string {
	return `openapi: 3.0.0
info:
  title: AWS Lambda
  version: "2015-03-31"
security:
  - hmac: []
paths:
  /2015-03-31/functions:
    post:
      operationId: CreateFunction
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                FunctionName:
                  type: string
                Role:
                  type: string
                Runtime:
                  type: string
                Handler:
                  type: string
      responses:
        "201":
          description: ok
  /2021-10-31/functions/{FunctionName}/url:
    post:
      operationId: CreateFunctionUrlConfig
      parameters:
        - name: FunctionName
          in: path
          required: true
          schema:
            type: string
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                authorization_type:
                  type: string
      responses:
        "201":
          description: ok
components:
  securitySchemes:
    hmac:
      type: apiKey
      name: Authorization
      in: header
`
}

func stsOpenAPIForTest() string {
	return `openapi: 3.0.0
info:
  title: AWS Security Token Service
  version: "2011-06-15"
security:
  - hmac: []
paths:
  /assume-role:
    post:
      operationId: POST_AssumeRole
      responses:
        "200":
          description: ok
  /#Action=GetCallerIdentity:
    post:
      operationId: POST_GetCallerIdentity
      parameters:
        - name: Action
          in: query
          required: true
          schema:
            type: string
            enum: [GetCallerIdentity]
        - name: Version
          in: query
          required: true
          schema:
            type: string
            enum: ["2011-06-15"]
      responses:
        "200":
          description: ok
components:
  securitySchemes:
    hmac:
      type: apiKey
      name: Authorization
      in: header
`
}

func minimalIAMSmithyForTest() string {
	return `{
  "smithy": "2.0",
  "shapes": {
    "com.amazonaws.iam#IAM": {
      "type": "service",
      "version": "2010-05-08",
      "operations": [{"target": "com.amazonaws.iam#CreateRole"}],
      "traits": {
        "aws.api#service": {"sdkId": "IAM", "endpointPrefix": "iam"},
        "aws.auth#sigv4": {"name": "iam"},
        "aws.protocols#awsQuery": {}
      }
    },
    "com.amazonaws.iam#CreateRole": {
      "type": "operation",
      "input": {"target": "com.amazonaws.iam#CreateRoleRequest"},
      "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}
    },
    "com.amazonaws.iam#CreateRoleRequest": {
      "type": "structure",
      "members": {
        "RoleName": {
          "target": "com.amazonaws.iam#roleNameType",
          "traits": {"smithy.api#required": {}}
        },
        "AssumeRolePolicyDocument": {
          "target": "com.amazonaws.iam#policyDocumentType",
          "traits": {"smithy.api#required": {}}
        }
      },
      "traits": {"smithy.api#input": {}}
    },
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`
}

func minimalStorageDiscoveryForTest() string {
	return `{
  "discoveryVersion": "v1",
  "name": "storage",
  "version": "v1",
  "rootUrl": "https://storage.googleapis.com/",
  "servicePath": "storage/v1/",
  "schemas": {
    "Bucket": {
      "id": "Bucket",
      "type": "object",
      "properties": {
        "name": {"type": "string"},
        "location": {"type": "string"}
      }
    }
  },
  "resources": {
    "buckets": {
      "methods": {
        "insert": {
          "id": "storage.buckets.insert",
          "path": "b",
          "httpMethod": "POST",
          "parameters": {
            "project": {
              "type": "string",
              "required": true,
              "location": "query"
            }
          },
          "request": {"$ref": "Bucket"},
          "response": {"$ref": "Bucket"},
          "scopes": ["https://www.googleapis.com/auth/devstorage.full_control"]
        }
      }
    }
  }
}`
}
