package apply

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/state"
)

func TestApplyAWSIAMRoleCreateThenNoOpWithMockExecutor(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, ".ramen", "state.db")
	writeApplyTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "apply-role"
  assume_role_policy = "{}"
}
`)
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	mock := &executor.MockExecutor{Results: map[string]executor.Result{
		"aws_iam_role.role": {
			Identity: map[string]any{"role_name": "apply-role", "secret_token": "should-not-persist"},
			Computed: map[string]any{"arn": "arn:aws:iam::123456789012:role/apply-role"},
		},
	}}

	result, err := Apply(context.Background(), Options{
		ConfigDir:   configDir,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		AutoApprove: true,
		OutDir:      filepath.Join(root, "out"),
		Executor:    mock,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Summary.Create != 1 || mock.RequestCount() != 1 {
		t.Fatalf("apply result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
	if len(result.GeneratedDocuments) != 1 {
		t.Fatalf("generated docs = %#v", result.GeneratedDocuments)
	}
	docText := readApplyTestFile(t, result.GeneratedDocuments[0])
	for _, expected := range []string{"ramen_apply_action", "CreateRole", "aws-smithy", "x-ramen-apply", "RoleName", "apply-role"} {
		if !strings.Contains(docText, expected) {
			t.Fatalf("generated UWS missing %q:\n%s", expected, docText)
		}
	}
	if strings.Contains(docText, "ramen-review-todo") || strings.Contains(docText, "x-ramen-terraform") {
		t.Fatalf("apply UWS leaked review scaffolding:\n%s", docText)
	}

	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	snap, err := store.CurrentResource(context.Background(), "aws_iam_role.role")
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close state: %v", err)
	}
	if snap == nil || snap.OperationID != "CreateRole" || !strings.Contains(snap.IdentityJSON, "role_name") {
		t.Fatalf("snapshot = %#v", snap)
	}
	if strings.Contains(snap.IdentityJSON, "should-not-persist") || !strings.Contains(snap.IdentityJSON, "${redacted}") {
		t.Fatalf("identity was not redacted: %s", snap.IdentityJSON)
	}

	result, err = Apply(context.Background(), Options{
		ConfigDir:   configDir,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		AutoApprove: true,
		Executor:    mock,
	})
	if err != nil {
		t.Fatalf("second Apply returned error: %v", err)
	}
	if result.Summary.NoOp != 1 || mock.RequestCount() != 1 {
		t.Fatalf("second apply result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
}

func TestApplyRecordsFailureAndBlocksDependentResources(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, ".ramen", "state.db")
	writeApplyTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "failed-role"
  assume_role_policy = "{}"
}

resource "aws_iam_role_policy" "policy" {
  name   = "policy"
  role   = aws_iam_role.role.name
  policy = "{}"
}
`)
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyFailureTest())
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		if req.Action.Address == "aws_iam_role.role" {
			return executor.Result{}, fmt.Errorf("token ABCDEFG should redact")
		}
		return executor.Result{Success: true}, nil
	}}
	result, err := Apply(context.Background(), Options{
		ConfigDir:   configDir,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		AutoApprove: true,
		Executor:    mock,
	})
	if err == nil {
		t.Fatalf("expected apply failure")
	}
	if result.Summary.Failed != 1 || result.Summary.Blocked != 1 || mock.RequestCount() != 1 {
		t.Fatalf("result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	revs, err := store.ListRevisions(context.Background(), "")
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revs) != 2 || revs[0].Action != "failed" || revs[1].Action != "blocked" {
		t.Fatalf("revisions = %#v", revs)
	}
	if strings.Contains(revs[0].DiffJSON, "ABCDEFG") || !strings.Contains(revs[0].DiffJSON, "${redacted}") {
		t.Fatalf("failure revision was not redacted: %s", revs[0].DiffJSON)
	}
	_ = store.Close()
}

func TestApplyGoogleStorageBucketCreateThenNoOpWithMockExecutor(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "storage.json")
	statePath := filepath.Join(root, ".ramen", "state.db")
	writeApplyTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "google_storage_bucket" "bucket" {
  name     = "apply-bucket"
  location = "US"
  project  = "review-project"
}
`)
	writeApplyTestFile(t, sourcePath, minimalStorageDiscoveryForApplyTest())
	mock := &executor.MockExecutor{Results: map[string]executor.Result{
		"google_storage_bucket.bucket": {
			Identity: map[string]any{"bucket_name": "apply-bucket"},
			Computed: map[string]any{"id": "apply-bucket"},
		},
	}}
	result, err := Apply(context.Background(), Options{
		ConfigDir:   configDir,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "google-discovery", ID: "storage", Path: sourcePath}},
		AutoApprove: true,
		Executor:    mock,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Summary.Create != 1 || mock.RequestCount() != 1 {
		t.Fatalf("apply result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
	result, err = Apply(context.Background(), Options{
		ConfigDir:   configDir,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "google-discovery", ID: "storage", Path: sourcePath}},
		AutoApprove: true,
		Executor:    mock,
	})
	if err != nil {
		t.Fatalf("second Apply returned error: %v", err)
	}
	if result.Summary.NoOp != 1 || mock.RequestCount() != 1 {
		t.Fatalf("second apply result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
}

func TestApplyRequiresApprovalForMutations(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	writeApplyTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "approval-role"
  assume_role_policy = "{}"
}
`)
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	mock := &executor.MockExecutor{}
	_, err := Apply(context.Background(), Options{
		ConfigDir:  configDir,
		StatePath:  filepath.Join(root, ".ramen", "state.db"),
		APISources: []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		Executor:   mock,
	})
	if err == nil || !strings.Contains(err.Error(), "--auto-approve") {
		t.Fatalf("expected approval error, got %v", err)
	}
	if mock.RequestCount() != 0 {
		t.Fatalf("executor was called before approval")
	}
}

func writeApplyTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readApplyTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func minimalIAMSmithyForApplyTest() string {
	return `{
  "smithy": "2.0",
  "shapes": {
    "com.amazonaws.iam#IAM": {
      "type": "service",
      "version": "2010-05-08",
      "operations": [
        {"target": "com.amazonaws.iam#CreateRole"},
        {"target": "com.amazonaws.iam#GetRole"}
      ],
      "traits": {
        "aws.api#service": {"sdkId": "IAM", "endpointPrefix": "iam"},
        "aws.auth#sigv4": {"name": "iam"},
        "aws.protocols#awsQuery": {}
      }
    },
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#GetRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#GetRoleRequest"}, "output": {"target": "com.amazonaws.iam#GetRoleResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#GetRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#GetRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`
}

func minimalStorageDiscoveryForApplyTest() string {
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
      "properties": {"name": {"type": "string"}, "location": {"type": "string"}}
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

func minimalIAMSmithyForApplyFailureTest() string {
	return `{
  "smithy": "2.0",
  "shapes": {
    "com.amazonaws.iam#IAM": {
      "type": "service",
      "version": "2010-05-08",
      "operations": [
        {"target": "com.amazonaws.iam#CreateRole"},
        {"target": "com.amazonaws.iam#PutRolePolicy"}
      ],
      "traits": {
        "aws.api#service": {"sdkId": "IAM", "endpointPrefix": "iam"},
        "aws.auth#sigv4": {"name": "iam"},
        "aws.protocols#awsQuery": {}
      }
    },
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#PutRolePolicy": {"type": "operation", "input": {"target": "com.amazonaws.iam#PutRolePolicyRequest"}, "output": {"target": "com.amazonaws.iam#PutRolePolicyResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#PutRolePolicyRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "PolicyName": {"target": "com.amazonaws.iam#policyNameType"}, "PolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#PutRolePolicyResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`
}
