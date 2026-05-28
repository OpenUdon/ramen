package reconcile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/state"
)

func TestImportRecordsRedactedIdentity(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.db")
	_, err := Import(context.Background(), ImportOptions{
		StatePath: statePath,
		Address:   "aws_iam_role.imported",
		Type:      "aws_iam_role",
		Provider:  "provider.aws",
		Identity:  map[string]any{"role_name": "imported", "secret_token": "do-not-store"},
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	snap, err := store.CurrentResource(context.Background(), "aws_iam_role.imported")
	if err != nil {
		t.Fatalf("current resource: %v", err)
	}
	if snap == nil || snap.Status != "imported" || strings.Contains(snap.IdentityJSON, "do-not-store") || !strings.Contains(snap.IdentityJSON, "${redacted}") {
		t.Fatalf("snapshot = %#v", snap)
	}
	revs, err := store.ListRevisions(context.Background(), "aws_iam_role.imported")
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revs) != 1 || revs[0].Action != "import" {
		t.Fatalf("revisions = %#v", revs)
	}
	_ = store.Close()
}

func TestDestroyDeletesTrackedResourceWithMockExecutor(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.db")
	sourcePath := filepath.Join(root, "iam.json")
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForReconcileTest())
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{Address: "aws_iam_role.role", Type: "aws_iam_role", Provider: "provider.aws", DesiredHash: "sha256:old", Status: "managed"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	_ = store.Close()
	mock := &executor.MockExecutor{}
	result, err := Destroy(context.Background(), Options{
		ConfigDir:   root,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		AutoApprove: true,
		Executor:    mock,
	})
	if err != nil {
		t.Fatalf("Destroy returned error: %v", err)
	}
	if result.Summary.Delete != 1 || mock.RequestCount() != 1 {
		t.Fatalf("result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
	store, _ = state.Open(context.Background(), statePath)
	snap, err := store.CurrentResource(context.Background(), "aws_iam_role.role")
	if err != nil {
		t.Fatalf("current after destroy: %v", err)
	}
	if snap != nil {
		data, _ := json.Marshal(snap)
		t.Fatalf("resource still present: %s", data)
	}
	_ = store.Close()
}

func writeReconcileTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func minimalIAMSmithyForReconcileTest() string {
	return `{
  "smithy": "2.0",
  "shapes": {
    "com.amazonaws.iam#IAM": {
      "type": "service",
      "version": "2010-05-08",
      "operations": [{"target": "com.amazonaws.iam#DeleteRole"}],
      "traits": {
        "aws.api#service": {"sdkId": "IAM", "endpointPrefix": "iam"},
        "aws.auth#sigv4": {"name": "iam"},
        "aws.protocols#awsQuery": {}
      }
    },
    "com.amazonaws.iam#DeleteRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#DeleteRoleRequest"}, "output": {"target": "com.amazonaws.iam#DeleteRoleResponse"}},
    "com.amazonaws.iam#DeleteRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}}},
    "com.amazonaws.iam#DeleteRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"}
  }
}`
}
