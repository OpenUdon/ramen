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
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForDestroyVocabularyTest())
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

func TestImportValidatesNativeProjectMetadataAndStateConflicts(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	statePath := filepath.Join(projectDir, "state.db")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForRefreshTest())
	projectPath := writeReconcileProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
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
				Name:     "role_name",
				Path:     "name",
				Required: true,
			}},
		}},
	})
	base := ImportOptions{ProjectPath: projectPath, StatePath: statePath, Address: "aws_iam_role.role", Type: "aws_iam_role", Provider: "provider.aws"}
	if _, err := Import(context.Background(), base); err == nil || !strings.Contains(err.Error(), "import.identity_missing") {
		t.Fatalf("missing identity error = %v", err)
	}
	unknown := base
	unknown.Identity = map[string]any{"role_name": "imported-role", "extra": "value"}
	if _, err := Import(context.Background(), unknown); err == nil || !strings.Contains(err.Error(), "import.identity_unknown") {
		t.Fatalf("unknown identity error = %v", err)
	}
	mismatch := base
	mismatch.Address = "aws_iam_role.missing"
	mismatch.Identity = map[string]any{"role_name": "imported-role"}
	if _, err := Import(context.Background(), mismatch); err == nil || !strings.Contains(err.Error(), "import.address_unknown") {
		t.Fatalf("address mismatch error = %v", err)
	}
	typeMismatch := base
	typeMismatch.Type = "aws_iam_user"
	typeMismatch.Identity = map[string]any{"role_name": "imported-role"}
	if _, err := Import(context.Background(), typeMismatch); err == nil || !strings.Contains(err.Error(), "import.type_mismatch") {
		t.Fatalf("type mismatch error = %v", err)
	}
	noReadDir := filepath.Join(root, "no-read")
	noReadSourcePath := filepath.Join(noReadDir, "aws-smithy", "iam.json")
	writeReconcileTestFile(t, noReadSourcePath, minimalIAMSmithyForRefreshTest())
	noReadProjectPath := writeReconcileProjectForTest(t, noReadDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources: []project.Resource{{
			Address:    "aws_iam_role.role",
			Kind:       "resource",
			Type:       "aws_iam_role",
			Name:       "role",
			Provider:   "provider.aws",
			Attributes: map[string]any{"name": "imported-role", "assume_role_policy": "{}"},
			Operations: map[string]project.OperationRole{
				"create": {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "CreateRole"},
			},
			IdentityAttributes: []project.IdentityAttribute{{
				Name:     "role_name",
				Path:     "name",
				Required: true,
			}},
		}},
	})
	noRead := base
	noRead.ProjectPath = noReadProjectPath
	noRead.StatePath = filepath.Join(noReadDir, "state.db")
	noRead.Identity = map[string]any{"role_name": "imported-role"}
	if _, err := Import(context.Background(), noRead); err == nil || !strings.Contains(err.Error(), "import.operation_missing") {
		t.Fatalf("operation missing error = %v", err)
	}
	valid := base
	valid.Identity = map[string]any{"role_name": "imported-role"}
	if _, err := Import(context.Background(), valid); err != nil {
		t.Fatalf("valid import returned error: %v", err)
	}
	if _, err := Import(context.Background(), valid); err == nil || !strings.Contains(err.Error(), "import.state_conflict") {
		t.Fatalf("state conflict error = %v", err)
	}
}

func TestImportAcceptsSchemaAndResponseDerivedIdentity(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	statePath := filepath.Join(projectDir, "state.db")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForRefreshTest())
	projectPath := writeReconcileProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources: []project.Resource{{
			Address:    "aws_iam_role.role",
			Kind:       "resource",
			Type:       "aws_iam_role",
			Name:       "role",
			Provider:   "provider.aws",
			Attributes: map[string]any{"name": "imported-role", "assume_role_policy": "{}"},
			Schema: []project.SchemaPath{
				{Path: "name", Type: "string"},
				{Path: "assume_role_policy", Type: "string"},
				{Path: "url", Type: "string", Identity: true, ResponseDerivedIdentity: true},
			},
			ResponseBindings: []project.ResponseBinding{{OperationRole: "read", ResponsePath: "Role.Arn", StatePath: "url", Identity: true, ResponseDerivedIdentity: true}},
			Operations: map[string]project.OperationRole{
				"read": {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "GetRole"},
			},
		}},
	})
	if _, err := Import(context.Background(), ImportOptions{
		ProjectPath: projectPath,
		StatePath:   statePath,
		Address:     "aws_iam_role.role",
		Type:        "aws_iam_role",
		Provider:    "provider.aws",
		Identity:    map[string]any{"url": "arn:aws:iam::123456789012:role/imported-role"},
	}); err != nil {
		t.Fatalf("schema identity import returned error: %v", err)
	}

	noIdentityDir := filepath.Join(root, "no-identity")
	noIdentitySource := filepath.Join(noIdentityDir, "aws-smithy", "iam.json")
	writeReconcileTestFile(t, noIdentitySource, minimalIAMSmithyForRefreshTest())
	noIdentityProject := writeReconcileProjectForTest(t, noIdentityDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources: []project.Resource{{
			Address:    "aws_iam_role.role",
			Kind:       "resource",
			Type:       "aws_iam_role",
			Attributes: map[string]any{"name": "imported-role"},
			Operations: map[string]project.OperationRole{"read": {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "GetRole"}},
		}},
	})
	if _, err := Import(context.Background(), ImportOptions{
		ProjectPath: noIdentityProject,
		StatePath:   filepath.Join(noIdentityDir, "state.db"),
		Address:     "aws_iam_role.role",
		Type:        "aws_iam_role",
		Identity:    map[string]any{"url": "value"},
	}); err == nil || !strings.Contains(err.Error(), "import.identity_schema_missing") {
		t.Fatalf("identity schema missing error = %v", err)
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

func TestDestroyUsesExplicitRemoveConfigRole(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	statePath := filepath.Join(projectDir, "state.db")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForDestroyVocabularyTest())
	projectPath := writeReconcileProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources: []project.Resource{{
			Address:    "aws_iam_role.role",
			Kind:       "resource",
			Type:       "aws_iam_role",
			Name:       "role",
			Provider:   "provider.aws",
			Attributes: map[string]any{"name": "remove-config-role", "assume_role_policy": "{}"},
			Operations: map[string]project.OperationRole{
				"create":        {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "CreateRole"},
				"read":          {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "GetRole"},
				"remove_config": {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "DeleteRole"},
			},
			IdentityAttributes: []project.IdentityAttribute{{Name: "role_name", Path: "name", RequestKeys: []string{"RoleName"}, Required: true}},
		}},
	})
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{Address: "aws_iam_role.role", Type: "aws_iam_role", Provider: "provider.aws", IdentityJSON: `{"role_name":"remove-config-role"}`, Status: "managed"}); err != nil {
		t.Fatalf("record resource: %v", err)
	}
	_ = store.Close()
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		if req.Action.Mapping.OperationID != "DeleteRole" {
			t.Fatalf("destroy used operation %s, want DeleteRole", req.Action.Mapping.OperationID)
		}
		return executor.Result{Success: true}, nil
	}}
	planned, err := tfplan.Build(context.Background(), tfplan.Options{ProjectPath: projectPath, StatePath: statePath, Action: "delete"})
	if err != nil {
		t.Fatalf("Build destroy plan returned error: %v", err)
	}
	if planned.Plan.Errored {
		t.Fatalf("destroy plan diagnostics = %#v", planned.Diagnostics)
	}
	result, err := Destroy(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err != nil {
		t.Fatalf("Destroy returned error: %v", err)
	}
	if result.Summary.Delete != 1 || mock.RequestCount() != 1 {
		t.Fatalf("result=%#v requests=%d", result.Summary, mock.RequestCount())
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

func TestDestroyExecutesVerifiedPlanArtifactInReverseDependencyOrder(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	statePath := filepath.Join(projectDir, "state.db")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	planPath := filepath.Join(root, "destroy.json")
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForReconcileTest())
	projectPath := writeReconcileProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources: []project.Resource{
			destroyProjectRole("aws_iam_role.db", "db-role", nil),
			destroyProjectRole("aws_iam_role.app", "app-role", []string{"aws_iam_role.db"}),
		},
	})
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	for _, snap := range []state.ResourceSnapshot{
		{Address: "aws_iam_role.db", Type: "aws_iam_role", Provider: "provider.aws", IdentityJSON: `{"role_name":"db-role"}`, Status: "managed"},
		{Address: "aws_iam_role.app", Type: "aws_iam_role", Provider: "provider.aws", IdentityJSON: `{"role_name":"app-role"}`, Status: "managed"},
	} {
		if err := store.RecordResource(context.Background(), snap); err != nil {
			t.Fatalf("record %s: %v", snap.Address, err)
		}
	}
	_ = store.Close()
	if _, err := tfplan.Build(context.Background(), tfplan.Options{ProjectPath: projectPath, StatePath: statePath, Action: "delete", OutPath: planPath}); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	var order []string
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		order = append(order, req.Action.Address)
		if req.Action.Mapping.OperationID != "DeleteRole" {
			t.Fatalf("destroy used %s, want DeleteRole", req.Action.Mapping.OperationID)
		}
		return executor.Result{Success: true}, nil
	}}
	result, err := Destroy(context.Background(), Options{PlanPath: planPath, AutoApprove: true, Executor: mock})
	if err != nil {
		t.Fatalf("Destroy returned error: %v", err)
	}
	if result.Summary.Delete != 2 || strings.Join(order, ",") != "aws_iam_role.app,aws_iam_role.db" {
		t.Fatalf("summary=%#v order=%v", result.Summary, order)
	}
}

func TestDestroyRejectsStaleNonDestroyOrTamperedPlanArtifactBeforeExecutor(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.db")
	sourcePath := filepath.Join(root, "iam.json")
	planPath := filepath.Join(root, "destroy.json")
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForReconcileTest())
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{Address: "aws_iam_role.role", Type: "aws_iam_role", Provider: "provider.aws", IdentityJSON: `{"role_name":"destroy-role"}`, Status: "managed"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	_ = store.Close()
	if _, err := tfplan.Build(context.Background(), tfplan.Options{ConfigDir: root, StatePath: statePath, APISources: []tfplan.APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}}, Action: "delete", OutPath: planPath}); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	store, err = state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("reopen state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{Address: "aws_iam_role.extra", Type: "aws_iam_role", Status: "managed"}); err != nil {
		t.Fatalf("record extra: %v", err)
	}
	_ = store.Close()
	mock := &executor.MockExecutor{}
	_, err = Destroy(context.Background(), Options{PlanPath: planPath, AutoApprove: true, Executor: mock})
	if err == nil || !strings.Contains(err.Error(), "destroy.approval_mismatch") {
		t.Fatalf("stale artifact error = %v", err)
	}
	if mock.RequestCount() != 0 {
		t.Fatalf("executor was called for stale artifact")
	}
	var doc tfplan.Document
	if err := json.Unmarshal([]byte(readReconcileTestFile(t, planPath)), &doc); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}
	doc.Resources[0].DesiredHash = "tampered"
	tamperedPath := filepath.Join(root, "tampered.json")
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal tampered plan: %v", err)
	}
	writeReconcileTestFile(t, tamperedPath, string(append(data, '\n')))
	_, err = Destroy(context.Background(), Options{PlanPath: tamperedPath, AutoApprove: true, Executor: mock})
	if err == nil || !strings.Contains(err.Error(), "destroy.approval_invalid") {
		t.Fatalf("tampered artifact error = %v", err)
	}
	createPlanPath := filepath.Join(root, "create.json")
	if _, err := tfplan.Build(context.Background(), tfplan.Options{ConfigDir: root, StatePath: filepath.Join(root, "create-state.db"), APISources: []tfplan.APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}}, OutPath: createPlanPath}); err != nil {
		t.Fatalf("Build create returned error: %v", err)
	}
	_, err = Destroy(context.Background(), Options{PlanPath: createPlanPath, AutoApprove: true, Executor: mock})
	if err == nil || !strings.Contains(err.Error(), "destroy.plan_action_invalid") {
		t.Fatalf("non-destroy artifact error = %v", err)
	}
	if mock.RequestCount() != 0 {
		t.Fatalf("executor was called for rejected artifact")
	}
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

func TestRefreshNativeReadRolesAndDriftSummary(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	statePath := filepath.Join(projectDir, "state.db")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForRefreshTest())
	resources := []project.Resource{
		refreshProjectRole("aws_iam_role.changed", "changed-role"),
		refreshProjectRole("aws_iam_role.same", "same-role"),
		refreshProjectRole("aws_iam_role.missing", "missing-role"),
		refreshProjectRole("aws_iam_role.skipped", "skipped-role"),
	}
	projectPath := writeReconcileProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources:  resources,
	})
	planResult, err := tfplan.Build(context.Background(), tfplan.Options{ProjectPath: projectPath, StatePath: statePath})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	hashes := map[string]string{}
	for _, resource := range planResult.Plan.Resources {
		hashes[resource.Address] = resource.DesiredHash
	}
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	for _, snap := range []state.ResourceSnapshot{
		{Address: "aws_iam_role.changed", Type: "aws_iam_role", Provider: "provider.aws", DesiredHash: hashes["aws_iam_role.changed"], IdentityJSON: mustReconcileJSON(t, map[string]any{"role_name": "changed-role"}), AttributesJSON: mustReconcileJSON(t, map[string]any{"arn": "old"}), Status: "managed"},
		{Address: "aws_iam_role.same", Type: "aws_iam_role", Provider: "provider.aws", DesiredHash: hashes["aws_iam_role.same"], IdentityJSON: mustReconcileJSON(t, map[string]any{"role_name": "same-role"}), AttributesJSON: mustReconcileJSON(t, map[string]any{"arn": "same"}), Status: "managed"},
		{Address: "aws_iam_role.missing", Type: "aws_iam_role", Provider: "provider.aws", DesiredHash: hashes["aws_iam_role.missing"], IdentityJSON: mustReconcileJSON(t, map[string]any{"role_name": "missing-role"}), AttributesJSON: mustReconcileJSON(t, map[string]any{"arn": "missing"}), Status: "managed"},
	} {
		if err := store.RecordResource(context.Background(), snap); err != nil {
			t.Fatalf("record %s: %v", snap.Address, err)
		}
	}
	_ = store.Close()
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		if req.Action.Mapping.OperationID != "GetRole" {
			t.Fatalf("refresh used %s, want GetRole", req.Action.Mapping.OperationID)
		}
		switch req.Action.Address {
		case "aws_iam_role.changed":
			return executor.Result{Success: true, Identity: map[string]any{"role_name": "changed-role"}, Computed: map[string]any{"arn": "new"}}, nil
		case "aws_iam_role.same":
			return executor.Result{Success: true, Identity: map[string]any{"role_name": "same-role"}, Computed: map[string]any{"arn": "same"}}, nil
		case "aws_iam_role.missing":
			return executor.Result{Success: true, Missing: true}, nil
		default:
			t.Fatalf("unexpected refresh request for %s", req.Action.Address)
			return executor.Result{}, nil
		}
	}}
	result, err := Refresh(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, Executor: mock})
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if result.Summary.Read != 3 || result.Summary.Changed != 1 || result.Summary.Unchanged != 1 || result.Summary.Missing != 1 || result.Summary.Skipped != 1 || result.Summary.Failed != 0 {
		t.Fatalf("summary = %#v", result.Summary)
	}
	store, _ = state.Open(context.Background(), statePath)
	changed, err := store.CurrentResource(context.Background(), "aws_iam_role.changed")
	if err != nil {
		t.Fatalf("current changed: %v", err)
	}
	if changed == nil || !strings.Contains(changed.AttributesJSON, `"new"`) {
		t.Fatalf("changed state = %#v", changed)
	}
	revs, err := store.ListRevisions(context.Background(), "aws_iam_role.missing")
	if err != nil {
		t.Fatalf("missing revisions: %v", err)
	}
	if len(revs) != 1 || revs[0].Action != "refresh_missing" {
		t.Fatalf("missing revisions = %#v", revs)
	}
	_ = store.Close()
}

func TestRecordRefreshUsesResponseBindingsNormalizersAndRedaction(t *testing.T) {
	ctx := context.Background()
	statePath := filepath.Join(t.TempDir(), "state.db")
	store, err := state.Open(ctx, statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	defer store.Close()
	if err := store.RecordResource(ctx, state.ResourceSnapshot{
		Address:        "aws_sqs_queue.queue",
		Type:           "aws_sqs_queue",
		Provider:       "provider.aws",
		IdentityJSON:   `{"url":"queue-url"}`,
		AttributesJSON: `{"policy":"{\"a\":1,\"b\":2}","secret":"${redacted}","status":"active"}`,
		Status:         "managed",
	}); err != nil {
		t.Fatalf("record resource: %v", err)
	}
	resource := tfplan.ResourcePlan{
		Address: "aws_sqs_queue.queue",
		Type:    "aws_sqs_queue",
		Mapping: &tfplan.MappingPlan{
			ResponseBindings: []project.ResponseBinding{
				{OperationRole: "read", ResponsePath: "QueueUrl", StatePath: "url", Identity: true, ResponseDerivedIdentity: true},
				{OperationRole: "read", ResponsePath: "Secret", StatePath: "secret", Computed: true, Sensitive: true},
			},
			Normalizers: []project.Normalizer{
				{Path: "policy", Kind: "json_semantic"},
				{Path: "status", Kind: "case_fold"},
			},
		},
	}
	changed, err := recordRefresh(ctx, store, 7, resource, executor.Result{
		Success: true,
		Computed: map[string]any{
			"QueueUrl": "queue-url",
			"Secret":   "do-not-store",
			"policy":   `{"b":2,"a":1}`,
			"status":   "ACTIVE",
		},
	})
	if err != nil {
		t.Fatalf("recordRefresh returned error: %v", err)
	}
	if changed {
		t.Fatalf("refresh should be unchanged after projection and normalization")
	}
	snap, err := store.CurrentResource(ctx, "aws_sqs_queue.queue")
	if err != nil {
		t.Fatalf("current resource: %v", err)
	}
	if snap == nil || strings.Contains(snap.AttributesJSON, "do-not-store") || !strings.Contains(snap.AttributesJSON, `"secret":"${redacted}"`) {
		t.Fatalf("snapshot was not redacted: %#v", snap)
	}
}

func TestRefreshRequiresNativeReadRole(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	writeReconcileTestFile(t, sourcePath, minimalIAMSmithyForRefreshTest())
	projectPath := writeReconcileProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources: []project.Resource{{
			Address:    "aws_iam_role.role",
			Kind:       "resource",
			Type:       "aws_iam_role",
			Name:       "role",
			Provider:   "provider.aws",
			Attributes: map[string]any{"name": "role", "assume_role_policy": "{}"},
			Operations: map[string]project.OperationRole{
				"create": {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "CreateRole"},
			},
		}},
	})
	statePath := filepath.Join(projectDir, "state.db")
	planResult, err := tfplan.Build(context.Background(), tfplan.Options{ProjectPath: projectPath, StatePath: statePath})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{Address: "aws_iam_role.role", Type: "aws_iam_role", Provider: "provider.aws", DesiredHash: planResult.Plan.Resources[0].DesiredHash, Status: "managed"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	_ = store.Close()
	_, err = Refresh(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, Executor: &executor.MockExecutor{}})
	if err == nil || !strings.Contains(err.Error(), "refresh.read_operation_missing") {
		t.Fatalf("read role error = %v", err)
	}
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

func readReconcileTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
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

func destroyProjectRole(address, name string, dependencies []string) project.Resource {
	return project.Resource{
		Address:      address,
		Kind:         "resource",
		Type:         "aws_iam_role",
		Name:         strings.TrimPrefix(address, "aws_iam_role."),
		Provider:     "provider.aws",
		Attributes:   map[string]any{"name": name},
		Dependencies: dependencies,
		Operations: map[string]project.OperationRole{
			"delete": {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "DeleteRole"},
		},
		IdentityAttributes: []project.IdentityAttribute{{
			Name:        "role_name",
			Path:        "name",
			RequestKeys: []string{"RoleName"},
			Required:    true,
		}},
	}
}

func refreshProjectRole(address, name string) project.Resource {
	return project.Resource{
		Address:    address,
		Kind:       "resource",
		Type:       "aws_iam_role",
		Name:       strings.TrimPrefix(address, "aws_iam_role."),
		Provider:   "provider.aws",
		Attributes: map[string]any{"name": name, "assume_role_policy": "{}"},
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
	}
}

func mustReconcileJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
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

func minimalIAMSmithyForDestroyVocabularyTest() string {
	return `{
  "smithy": "2.0",
  "shapes": {
    "com.amazonaws.iam#IAM": {
      "type": "service",
      "version": "2010-05-08",
      "operations": [{"target": "com.amazonaws.iam#CreateRole"}, {"target": "com.amazonaws.iam#GetRole"}, {"target": "com.amazonaws.iam#DeleteRole"}],
      "traits": {
        "aws.api#service": {"sdkId": "IAM", "endpointPrefix": "iam"},
        "aws.auth#sigv4": {"name": "iam"},
        "aws.protocols#awsQuery": {}
      }
    },
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#GetRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#GetRoleRequest"}, "output": {"target": "com.amazonaws.iam#GetRoleResponse"}},
    "com.amazonaws.iam#DeleteRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#DeleteRoleRequest"}, "output": {"target": "com.amazonaws.iam#DeleteRoleResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}},
    "com.amazonaws.iam#GetRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}}},
    "com.amazonaws.iam#DeleteRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#GetRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#DeleteRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`
}
