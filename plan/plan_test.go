package plan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/state"
)

func TestBuildAWSIAMRoleCreateAndNoOpPlans(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, ".ramen", "state.db")
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "tf-acc-ramen-role"
  assume_role_policy = "{}"
}
`)
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())

	createResult, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: statePath,
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: sourcePath,
		}},
		OutPath: filepath.Join(root, "plan-create.json"),
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("plan should not create missing state, stat err=%v", err)
	}
	assertPlanSummary(t, createResult.Plan.Summary, 1, 0, 0)
	role := createResult.Plan.Resources[0]
	if role.Action != "create" || role.Mapping == nil || role.Mapping.OperationID != "CreateRole" || role.Mapping.SourceKind != "aws-smithy" {
		t.Fatalf("unexpected create plan resource: %#v", role)
	}

	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{
		Address:     role.Address,
		Type:        role.Type,
		Provider:    role.Provider,
		DesiredHash: role.DesiredHash,
		Status:      "managed",
		SourceKind:  role.Mapping.SourceKind,
		SourceID:    role.Mapping.SourceID,
		OperationID: role.Mapping.OperationID,
	}); err != nil {
		t.Fatalf("record resource: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close state: %v", err)
	}

	noOpResult, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: statePath,
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: sourcePath,
		}},
		OutPath: filepath.Join(root, "plan-noop.json"),
	})
	if err != nil {
		t.Fatalf("Build no-op returned error: %v", err)
	}
	assertPlanSummary(t, noOpResult.Plan.Summary, 0, 0, 1)
	role = noOpResult.Plan.Resources[0]
	if role.Action != "no-op" || role.Mapping == nil || role.Mapping.OperationID != "GetRole" {
		t.Fatalf("unexpected no-op plan resource: %#v", role)
	}
	first := readPlanTestFile(t, createResult.OutPath)
	second, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: statePath,
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: sourcePath,
		}},
		OutPath: filepath.Join(root, "plan-noop-2.json"),
	})
	if err != nil {
		t.Fatalf("Build second no-op returned error: %v", err)
	}
	if got := readPlanTestFile(t, second.OutPath); got != readPlanTestFile(t, noOpResult.OutPath) {
		t.Fatalf("no-op plan output is not deterministic:\nfirst:\n%s\nsecond:\n%s", readPlanTestFile(t, noOpResult.OutPath), got)
	}
	if !strings.Contains(first, `"action": "create"`) {
		t.Fatalf("create plan JSON missing create action:\n%s", first)
	}
}

func TestBuildGoogleStorageBucketCreateAndNoOpPlans(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "storage.json")
	statePath := filepath.Join(root, ".ramen", "state.db")
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "google_storage_bucket" "bucket" {
  name     = "openudon-bucket"
  location = "US"
  project  = "review-project"
}
`)
	writePlanTestFile(t, sourcePath, minimalStorageDiscoveryForPlanTest())

	result, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: statePath,
		APISources: []APISourceInput{{
			Kind: "google-discovery",
			ID:   "storage",
			Path: sourcePath,
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	assertPlanSummary(t, result.Plan.Summary, 1, 0, 0)
	bucket := result.Plan.Resources[0]
	if bucket.Action != "create" || bucket.Mapping == nil || bucket.Mapping.OperationID != "storage.buckets.insert" || bucket.Mapping.SourceKind != "google-discovery" {
		t.Fatalf("unexpected bucket create plan: %#v", bucket)
	}

	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{
		Address:     bucket.Address,
		Type:        bucket.Type,
		Provider:    bucket.Provider,
		DesiredHash: bucket.DesiredHash,
		Status:      "managed",
		SourceKind:  bucket.Mapping.SourceKind,
		SourceID:    bucket.Mapping.SourceID,
		OperationID: bucket.Mapping.OperationID,
	}); err != nil {
		t.Fatalf("record resource: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close state: %v", err)
	}
	result, err = Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: statePath,
		APISources: []APISourceInput{{
			Kind: "google-discovery",
			ID:   "storage",
			Path: sourcePath,
		}},
	})
	if err != nil {
		t.Fatalf("Build no-op returned error: %v", err)
	}
	assertPlanSummary(t, result.Plan.Summary, 0, 0, 1)
	bucket = result.Plan.Resources[0]
	if bucket.Action != "no-op" || bucket.Mapping == nil || bucket.Mapping.OperationID != "storage.buckets.get" {
		t.Fatalf("unexpected bucket no-op plan: %#v", bucket)
	}
}

func TestBuildOrdersResourcesByStaticDependencies(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role_policy" "policy" {
  name   = "policy"
  role   = aws_iam_role.role.name
  policy = "{}"
}

resource "aws_iam_role" "role" {
  name = "role"
  assume_role_policy = "{}"
}
`)
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())
	result, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: filepath.Join(root, ".ramen", "state.db"),
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: sourcePath,
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(result.Plan.Resources) != 2 {
		t.Fatalf("resources = %#v", result.Plan.Resources)
	}
	if result.Plan.Resources[0].Address != "aws_iam_role.role" || result.Plan.Resources[1].Address != "aws_iam_role_policy.policy" {
		t.Fatalf("resources not dependency ordered: %#v", result.Plan.Resources)
	}
	if got := result.Plan.Resources[1].Dependencies; len(got) != 1 || got[0] != "aws_iam_role.role" {
		t.Fatalf("policy dependencies = %#v", got)
	}
}

func TestBuildPlansDeleteForStateOnlyResource(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, ".ramen", "state.db")
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), "\n")
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{
		Address:     "aws_iam_role.old",
		Type:        "aws_iam_role",
		Provider:    "provider.aws",
		DesiredHash: "sha256:old",
		Status:      "managed",
	}); err != nil {
		t.Fatalf("record resource: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close state: %v", err)
	}
	result, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: statePath,
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: sourcePath,
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if result.Plan.Summary.Delete != 1 || len(result.Plan.Resources) != 1 {
		t.Fatalf("plan = %#v", result.Plan)
	}
	if got := result.Plan.Resources[0]; got.Action != "delete" || got.Address != "aws_iam_role.old" {
		t.Fatalf("delete resource = %#v", got)
	}
}

func assertPlanSummary(t *testing.T, got Summary, create, update, noOp int) {
	t.Helper()
	if got.Create != create || got.Update != update || got.NoOp != noOp || got.Delete != 0 {
		t.Fatalf("summary = %#v, want create=%d update=%d no-op=%d", got, create, update, noOp)
	}
}

func writePlanTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readPlanTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func minimalIAMSmithyForPlanTest() string {
	return `{
  "smithy": "2.0",
  "shapes": {
    "com.amazonaws.iam#IAM": {
      "type": "service",
      "version": "2010-05-08",
      "operations": [
        {"target": "com.amazonaws.iam#CreateRole"},
        {"target": "com.amazonaws.iam#GetRole"},
        {"target": "com.amazonaws.iam#PutRolePolicy"},
        {"target": "com.amazonaws.iam#DeleteRole"}
      ],
      "traits": {
        "aws.api#service": {"sdkId": "IAM", "endpointPrefix": "iam"},
        "aws.auth#sigv4": {"name": "iam"},
        "aws.protocols#awsQuery": {}
      }
    },
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#GetRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#GetRoleRequest"}, "output": {"target": "com.amazonaws.iam#GetRoleResponse"}},
    "com.amazonaws.iam#PutRolePolicy": {"type": "operation", "input": {"target": "com.amazonaws.iam#PutRolePolicyRequest"}, "output": {"target": "com.amazonaws.iam#PutRolePolicyResponse"}},
    "com.amazonaws.iam#DeleteRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#DeleteRoleRequest"}, "output": {"target": "com.amazonaws.iam#DeleteRoleResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType", "traits": {"smithy.api#required": {}}}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType", "traits": {"smithy.api#required": {}}}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#GetRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType", "traits": {"smithy.api#required": {}}}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#PutRolePolicyRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType", "traits": {"smithy.api#required": {}}}, "PolicyName": {"target": "com.amazonaws.iam#policyNameType", "traits": {"smithy.api#required": {}}}, "PolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType", "traits": {"smithy.api#required": {}}}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#DeleteRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType", "traits": {"smithy.api#required": {}}}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#GetRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#PutRolePolicyResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#DeleteRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`
}

func minimalStorageDiscoveryForPlanTest() string {
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
          "parameters": {"project": {"type": "string", "required": true, "location": "query"}},
          "request": {"$ref": "Bucket"},
          "response": {"$ref": "Bucket"}
        },
        "get": {
          "id": "storage.buckets.get",
          "path": "b/{bucket}",
          "httpMethod": "GET",
          "parameters": {"bucket": {"type": "string", "required": true, "location": "path"}},
          "response": {"$ref": "Bucket"}
        }
      }
    }
  }
}`
}
