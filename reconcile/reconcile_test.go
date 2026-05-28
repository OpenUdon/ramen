package reconcile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/executor"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/state"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestImportRecordsRedactedIdentity(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.db")
	_, err := Import(context.Background(), ImportOptions{
		StatePath: statePath,
		Address:   "aws_iam_role.imported",
		Type:      "aws_iam_role",
		Provider:  "provider.aws",
		Identity: map[string]any{
			"role_name":    "imported",
			"secret_token": "do-not-store",
			"history":      []any{map[string]any{"access_key": "AKIA1234567890ABCDEF"}},
		},
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
	if snap == nil || snap.Status != "imported" || strings.Contains(snap.IdentityJSON, "do-not-store") || strings.Contains(snap.IdentityJSON, "AKIA1234567890ABCDEF") || !strings.Contains(snap.IdentityJSON, "${redacted}") {
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

func TestImportThenPlanNoOpForMatchingConfig(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	statePath := filepath.Join(root, "state.db")
	sourcePath := filepath.Join(root, "iam.json")
	writeReconcileTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "imported-role"
  assume_role_policy = "{}"
}
`)
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForRefreshTest())
	sources := []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}}
	if _, err := Import(context.Background(), ImportOptions{
		ConfigDir:  configDir,
		StatePath:  statePath,
		APISources: sources,
		Address:    "aws_iam_role.role",
		Type:       "aws_iam_role",
		Provider:   "provider.aws",
		Identity:   map[string]any{"role_name": "imported-role"},
	}); err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	result, err := tfplan.Build(context.Background(), tfplan.Options{
		ConfigDir:  configDir,
		StatePath:  statePath,
		APISources: []tfplan.APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if result.Plan.Errored || result.Plan.Summary.NoOp != 1 || result.Plan.Summary.Update != 0 {
		t.Fatalf("plan after import = %#v", result.Plan)
	}
}

func TestImportThenPlanNoOpForMatchingNativeProject(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	statePath := filepath.Join(projectDir, "state.db")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForRefreshTest())
	projectPath := writeReconcileProjectForTest(t, projectDir, project.Profile{
		Version: project.Version,
		APISources: []project.APISource{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: "aws-smithy/iam.json",
		}},
		Resources: []project.Resource{{
			Address:    "aws_iam_role.role",
			Kind:       "resource",
			Type:       "aws_iam_role",
			Name:       "role",
			Provider:   "provider.aws",
			Attributes: map[string]any{"name": "imported-role", "assume_role_policy": "{}"},
			Operations: map[string]project.OperationRole{
				"create": {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "CreateRole"},
				"read":   {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "GetRole"},
			},
			IdentityAttributes: []project.IdentityAttribute{{
				Name:        "role_name",
				Path:        "name",
				RequestKeys: []string{"RoleName"},
				Required:    true,
			}},
		}},
	})
	if _, err := Import(context.Background(), ImportOptions{
		ProjectPath: projectPath,
		StatePath:   statePath,
		Address:     "aws_iam_role.role",
		Type:        "aws_iam_role",
		Provider:    "provider.aws",
		Identity:    map[string]any{"role_name": "imported-role"},
	}); err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	result, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: projectPath,
		StatePath:   statePath,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if result.Plan.Errored || result.Plan.Summary.NoOp != 1 || result.Plan.Summary.Update != 0 {
		t.Fatalf("plan after native import = %#v", result.Plan)
	}
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
	revs, err := store.ListRevisions(context.Background(), "aws_iam_role.role")
	if err != nil {
		t.Fatalf("list revisions after destroy: %v", err)
	}
	if len(revs) != 1 || revs[0].Action != "delete" || revs[0].BeforeJSON == "" {
		t.Fatalf("destroy revisions = %#v", revs)
	}
	_ = store.Close()
}

func TestDestroyActionDocumentIncludesStateIdentity(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.db")
	sourcePath := filepath.Join(root, "iam.json")
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForReconcileTest())
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{
		Address:      "aws_iam_role.role",
		Type:         "aws_iam_role",
		Provider:     "provider.aws",
		DesiredHash:  "sha256:old",
		IdentityJSON: `{"role_name":"destroy-role"}`,
		Status:       "managed",
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	_ = store.Close()
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		data, _ := json.Marshal(req.Document)
		text := string(data)
		if !strings.Contains(text, "RoleName") || !strings.Contains(text, "destroy-role") {
			t.Fatalf("destroy UWS missing state identity binding: %s", text)
		}
		return executor.Result{Success: true}, nil
	}}
	if _, err := Destroy(context.Background(), Options{
		ConfigDir:   root,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		AutoApprove: true,
		Executor:    mock,
	}); err != nil {
		t.Fatalf("Destroy returned error: %v", err)
	}
	if mock.RequestCount() != 1 {
		t.Fatalf("requests = %d, want 1", mock.RequestCount())
	}
}

func TestDestroyUnsuccessfulExecutorResultPreservesStateAndRecordsFailure(t *testing.T) {
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
	mock := &executor.MockExecutor{ExecuteFn: func(context.Context, executor.Request) (executor.Result, error) {
		return executor.Result{Success: false}, nil
	}}
	result, err := Destroy(context.Background(), Options{
		ConfigDir:   root,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		AutoApprove: true,
		Executor:    mock,
	})
	if err == nil {
		t.Fatal("Destroy succeeded unexpectedly")
	}
	if result.Summary.Failed != 1 {
		t.Fatalf("summary = %#v", result.Summary)
	}
	store, _ = state.Open(context.Background(), statePath)
	snap, err := store.CurrentResource(context.Background(), "aws_iam_role.role")
	if err != nil {
		t.Fatalf("current after failed destroy: %v", err)
	}
	if snap == nil {
		t.Fatal("resource was deleted after unsuccessful executor result")
	}
	revs, err := store.ListRevisions(context.Background(), "aws_iam_role.role")
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revs) != 1 || revs[0].Action != "delete_failed" {
		t.Fatalf("revisions = %#v", revs)
	}
	_ = store.Close()
}

func TestRefreshActionDocumentIncludesStateIdentity(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	statePath := filepath.Join(root, "state.db")
	sourcePath := filepath.Join(root, "iam.json")
	writeReconcileTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "refresh-role"
  assume_role_policy = "{}"
}
`)
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForRefreshTest())
	planResult, err := tfplan.Build(context.Background(), tfplan.Options{
		ConfigDir:  configDir,
		StatePath:  statePath,
		APISources: []tfplan.APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	role := planResult.Plan.Resources[0]
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{
		Address:      role.Address,
		Type:         role.Type,
		Provider:     role.Provider,
		DesiredHash:  role.DesiredHash,
		IdentityJSON: `{"role_name":"refresh-role"}`,
		Status:       "managed",
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	_ = store.Close()
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		data, _ := json.Marshal(req.Document)
		text := string(data)
		if !strings.Contains(text, "RoleName") || !strings.Contains(text, "refresh-role") {
			t.Fatalf("refresh UWS missing state identity binding: %s", text)
		}
		return executor.Result{Success: true, Identity: map[string]any{"role_name": "refresh-role"}}, nil
	}}
	if _, err := Refresh(context.Background(), Options{
		ConfigDir:  configDir,
		StatePath:  statePath,
		APISources: []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		Executor:   mock,
	}); err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if mock.RequestCount() != 1 {
		t.Fatalf("requests = %d, want 1", mock.RequestCount())
	}
}

func TestRefreshUnsuccessfulExecutorResultRecordsFailure(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	statePath := filepath.Join(root, "state.db")
	sourcePath := filepath.Join(root, "iam.json")
	writeReconcileTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "role"
  assume_role_policy = "{}"
}
`)
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForRefreshTest())
	planResult, err := tfplan.Build(context.Background(), tfplan.Options{
		ConfigDir: configDir,
		StatePath: statePath,
		APISources: []tfplan.APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: sourcePath,
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	role := planResult.Plan.Resources[0]
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{Address: role.Address, Type: role.Type, Provider: role.Provider, DesiredHash: role.DesiredHash, Status: "managed"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	_ = store.Close()
	mock := &executor.MockExecutor{ExecuteFn: func(context.Context, executor.Request) (executor.Result, error) {
		return executor.Result{Success: false}, nil
	}}
	result, err := Refresh(context.Background(), Options{
		ConfigDir:  configDir,
		StatePath:  statePath,
		APISources: []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		Executor:   mock,
	})
	if err == nil {
		t.Fatal("Refresh succeeded unexpectedly")
	}
	if result.Summary.Failed != 1 || mock.RequestCount() != 1 {
		t.Fatalf("result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
	store, _ = state.Open(context.Background(), statePath)
	revs, err := store.ListRevisions(context.Background(), "aws_iam_role.role")
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revs) != 1 || revs[0].Action != "refresh_failed" {
		t.Fatalf("revisions = %#v", revs)
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

func writeReconcileProjectForTest(t *testing.T, dir string, profile project.Profile) string {
	t.Helper()
	doc := &uws1.Document{
		UWS: "1.4.0",
		Info: &uws1.Info{
			Title:   "reconcile_project_fixture",
			Version: "1.0.0",
		},
		Operations: []*uws1.Operation{{
			OperationID: "review",
			Request:     map[string]any{"x-test": true},
			Extensions:  map[string]any{uws1.ExtensionOperationProfile: "ramen-reconcile-test"},
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID: "main",
			Type:       uws1.WorkflowTypeSequence,
			Steps: []*uws1.Step{{
				StepID:       "review",
				OperationRef: "review",
			}},
		}},
		Extensions: map[string]any{
			project.ExtensionKey: profile,
		},
	}
	data, err := uwsconvert.MarshalJSONIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := filepath.Join(dir, project.DefaultJSON)
	writeReconcileTestFile(t, path, string(data))
	return path
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

func minimalIAMSmithyForRefreshTest() string {
	return `{
  "smithy": "2.0",
  "shapes": {
    "com.amazonaws.iam#IAM": {
      "type": "service",
      "version": "2010-05-08",
      "operations": [{"target": "com.amazonaws.iam#CreateRole"}, {"target": "com.amazonaws.iam#GetRole"}],
      "traits": {
        "aws.api#service": {"sdkId": "IAM", "endpointPrefix": "iam"},
        "aws.auth#sigv4": {"name": "iam"},
        "aws.protocols#awsQuery": {}
      }
    },
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#GetRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#GetRoleRequest"}, "output": {"target": "com.amazonaws.iam#GetRoleResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}},
    "com.amazonaws.iam#GetRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#GetRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`
}
