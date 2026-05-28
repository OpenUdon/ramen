package plan

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/ramen/governance"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/state"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
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

func TestBuildNativeAWSIAMRoleProjectWithoutHCL(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	statePath := filepath.Join(projectDir, ".ramen", "state.db")
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())
	projectPath := writeNativeProjectForPlanTest(t, projectDir, project.Profile{
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
			Attributes: map[string]any{"name": "native-role", "assume_role_policy": "{}"},
			Operations: map[string]project.OperationRole{
				"create": {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "CreateRole"},
				"read":   {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "GetRole"},
				"update": {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "PutRolePolicy"},
				"delete": {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "DeleteRole"},
			},
			IdentityAttributes: []project.IdentityAttribute{{
				Name:          "role_name",
				Path:          "name",
				RequestKeys:   []string{"RoleName"},
				ResponsePaths: []string{"Role.RoleName", "Role.Arn"},
				Required:      true,
			}},
		}},
	})

	result, err := Build(context.Background(), Options{
		ProjectPath: projectPath,
		StatePath:   statePath,
		OutPath:     filepath.Join(root, "native-plan.json"),
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	assertPlanSummaryWithDiagnostics(t, result.Plan.Summary, result.Diagnostics, 1, 0, 0)
	role := result.Plan.Resources[0]
	if role.Action != "create" || role.Mapping == nil || role.Mapping.OperationID != "CreateRole" || role.Mapping.SourceKind != "aws-smithy" {
		t.Fatalf("unexpected native create plan resource: %#v", role)
	}
	if role.Mapping.SourcePath != sourcePath {
		t.Fatalf("native source path was not resolved relative to project: %q", role.Mapping.SourcePath)
	}
	if _, err := os.Stat(filepath.Join(root, "tf")); !os.IsNotExist(err) {
		t.Fatalf("test should not create HCL config dir, stat err=%v", err)
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
	noOp, err := Build(context.Background(), Options{ProjectPath: projectDir, StatePath: statePath})
	if err != nil {
		t.Fatalf("Build no-op returned error: %v", err)
	}
	assertPlanSummaryWithDiagnostics(t, noOp.Plan.Summary, noOp.Diagnostics, 0, 0, 1)
	role = noOp.Plan.Resources[0]
	if role.Action != "no-op" || role.Mapping == nil || role.Mapping.OperationID != "GetRole" {
		t.Fatalf("unexpected native no-op plan resource: %#v", role)
	}
}

func TestBuildNativeGoogleStorageProjectWithoutHCL(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "google-discovery", "storage.json")
	writePlanTestFile(t, sourcePath, minimalStorageDiscoveryForPlanTest())
	projectPath := writeNativeProjectForPlanTest(t, projectDir, project.Profile{
		Version: project.Version,
		APISources: []project.APISource{{
			Kind: "google-discovery",
			ID:   "storage",
			Path: "google-discovery/storage.json",
		}},
		Resources: []project.Resource{{
			Address:    "google_storage_bucket.bucket",
			Kind:       "resource",
			Type:       "google_storage_bucket",
			Name:       "bucket",
			Provider:   "provider.google",
			Attributes: map[string]any{"name": "native-bucket", "location": "US", "project": "review-project"},
			Operations: map[string]project.OperationRole{
				"create": {SourceKind: "google-discovery", SourceID: "storage", SourcePath: "google-discovery/storage.json", OperationID: "storage.buckets.insert"},
				"read":   {SourceKind: "google-discovery", SourceID: "storage", SourcePath: "google-discovery/storage.json", OperationID: "storage.buckets.get"},
				"update": {SourceKind: "google-discovery", SourceID: "storage", SourcePath: "google-discovery/storage.json", OperationID: "storage.buckets.patch"},
				"delete": {SourceKind: "google-discovery", SourceID: "storage", SourcePath: "google-discovery/storage.json", OperationID: "storage.buckets.delete"},
			},
			IdentityAttributes: []project.IdentityAttribute{{
				Name:          "bucket_name",
				Path:          "name",
				RequestKeys:   []string{"bucket", "name"},
				ResponsePaths: []string{"name", "id"},
				Required:      true,
			}},
		}},
	})

	result, err := Build(context.Background(), Options{
		ProjectPath: projectPath,
		StatePath:   filepath.Join(projectDir, ".ramen", "state.db"),
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	assertPlanSummaryWithDiagnostics(t, result.Plan.Summary, result.Diagnostics, 1, 0, 0)
	bucket := result.Plan.Resources[0]
	if bucket.Action != "create" || bucket.Mapping == nil || bucket.Mapping.OperationID != "storage.buckets.insert" || bucket.Mapping.SourceKind != "google-discovery" {
		t.Fatalf("unexpected native bucket create plan: %#v", bucket)
	}
}

func TestBuildProjectTargetExcludeAndConflictControls(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "iam.json")
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())
	projectPath := writeNativeProjectForPlanTest(t, filepath.Join(root, "project"), project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		Resources: []project.Resource{
			nativeIAMRoleResourceForPlanControl("aws_iam_role.db", nil),
			nativeIAMRoleResourceForPlanControl("aws_iam_role.app", []string{"aws_iam_role.db"}),
			nativeIAMRoleResourceForPlanControl("aws_iam_role.other", nil),
		},
	})

	targeted, err := Build(context.Background(), Options{ProjectPath: projectPath, StatePath: filepath.Join(root, "state.db"), Targets: []string{"aws_iam_role.app"}})
	if err != nil {
		t.Fatalf("targeted build: %v", err)
	}
	if targeted.Plan.Errored || targeted.Plan.Summary.Create != 2 || len(targeted.Plan.Resources) != 2 {
		t.Fatalf("targeted plan = %#v", targeted.Plan)
	}
	if targeted.Plan.Resources[0].Address != "aws_iam_role.db" || targeted.Plan.Resources[1].Address != "aws_iam_role.app" {
		t.Fatalf("targeted resources = %#v", targeted.Plan.Resources)
	}

	excluded, err := Build(context.Background(), Options{ProjectPath: projectPath, StatePath: filepath.Join(root, "state.db"), Excludes: []string{"aws_iam_role.db"}})
	if err != nil {
		t.Fatalf("excluded build: %v", err)
	}
	if excluded.Plan.Errored || excluded.Plan.Summary.Create != 1 || excluded.Plan.Resources[0].Address != "aws_iam_role.other" {
		t.Fatalf("excluded plan = %#v", excluded.Plan)
	}

	conflict, err := Build(context.Background(), Options{ProjectPath: projectPath, StatePath: filepath.Join(root, "state.db"), Targets: []string{"aws_iam_role.app"}, Excludes: []string{"aws_iam_role.db"}})
	if err != nil {
		t.Fatalf("conflict build: %v", err)
	}
	if !conflict.Plan.Errored || !hasPlanDiagnostic(conflict.Diagnostics, "plan.selection_conflict") {
		t.Fatalf("conflict plan = %#v diagnostics=%#v", conflict.Plan, conflict.Diagnostics)
	}
}

func TestBuildProjectDestroyReplaceAndApprovalArtifact(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, "state.db")
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())
	resource := nativeIAMRoleResourceForPlanControl("aws_iam_role.app", nil)
	resource.AI = &project.AIMetadata{Confidence: &project.Confidence{Score: 0.82, Reason: "operation names match"}, Rationale: "IAM role create/read/update/delete operations are available."}
	resource.Operations["create"] = project.OperationRole{SourceKind: "aws-smithy", SourceID: "iam", OperationID: "CreateRole", AI: &project.AIMetadata{Confidence: &project.Confidence{Score: 0.91, Reason: "exact operation match"}}}
	projectPath := writeNativeProjectForPlanTest(t, filepath.Join(root, "project"), project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		Metadata:   map[string]any{"rationale": "replace app role after review"},
		Resources: []project.Resource{
			resource,
		},
	})
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{Address: "aws_iam_role.app", Type: "aws_iam_role", DesiredHash: "old", Status: "managed"}); err != nil {
		t.Fatalf("record resource: %v", err)
	}
	_ = store.Close()

	replaced, err := Build(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, Replaces: []string{"aws_iam_role.app"}})
	if err != nil {
		t.Fatalf("replace build: %v", err)
	}
	if replaced.Plan.Errored || replaced.Plan.Summary.Replace != 1 || replaced.Plan.Resources[0].Action != "replace" {
		t.Fatalf("replace plan = %#v", replaced.Plan)
	}
	if replaced.Plan.Resources[0].Mapping == nil || replaced.Plan.Resources[0].Mapping.Purpose != "create" || replaced.Plan.Resources[0].Mapping.OperationID != "CreateRole" {
		t.Fatalf("replace should use create mapping: %#v", replaced.Plan.Resources[0].Mapping)
	}
	if replaced.Plan.Approval == nil || replaced.Plan.Approval.Digest == "" || replaced.Plan.Approval.ProjectDigest == "" || replaced.Plan.Approval.StateDigest == "" {
		t.Fatalf("approval missing binding fields: %#v", replaced.Plan.Approval)
	}
	if replaced.Plan.Rationale != "replace app role after review" || replaced.Plan.Approval.Rationale != replaced.Plan.Rationale {
		t.Fatalf("rationale not carried into approval: plan=%q approval=%#v", replaced.Plan.Rationale, replaced.Plan.Approval)
	}
	if replaced.Plan.Resources[0].AI == nil || replaced.Plan.Resources[0].AI.Confidence.Score != 0.82 || replaced.Plan.Resources[0].Mapping.AI == nil || replaced.Plan.Resources[0].Mapping.AI.Confidence.Score != 0.91 {
		t.Fatalf("AI confidence metadata not carried into plan: %#v", replaced.Plan.Resources[0])
	}
	if err := VerifyApproval(replaced.Plan); err != nil {
		t.Fatalf("approval did not verify: %v", err)
	}
	replaced.Plan.Resources[0].Mapping.AI.Confidence.Score = 0.1
	if err := VerifyApproval(replaced.Plan); err == nil {
		t.Fatalf("tampered AI confidence unexpectedly verified")
	}
	replaced.Plan.Resources[0].Mapping.AI.Confidence.Score = 0.91
	replaced.Plan.Rationale = "tampered rationale"
	if err := VerifyApproval(replaced.Plan); err == nil {
		t.Fatalf("tampered rationale unexpectedly verified")
	}
	replaced.Plan.Rationale = "replace app role after review"
	replaced.Plan.Resources[0].Reason = "tampered"
	if err := VerifyApproval(replaced.Plan); err == nil {
		t.Fatalf("tampered approval unexpectedly verified")
	}

	destroyed, err := Build(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, Destroy: true})
	if err != nil {
		t.Fatalf("destroy build: %v", err)
	}
	if destroyed.Plan.Errored || destroyed.Plan.Action != "delete" || !destroyed.Plan.Controls.Destroy || destroyed.Plan.Summary.Delete != 1 {
		t.Fatalf("destroy plan = %#v", destroyed.Plan)
	}
}

func TestBuildProjectVariablesAffectHashAndApproval(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(root, "iam.json")
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())
	writePlanTestFile(t, filepath.Join(projectDir, "values.json"), `{"role_name":"file-role","secret_policy":"hidden-value"}`)
	resource := nativeIAMRoleResourceForPlanControl("aws_iam_role.app", nil)
	resource.Attributes["name"] = "${var.role_name}"
	resource.Attributes["assume_role_policy"] = "${var.secret_policy}"
	projectPath := writeNativeProjectForPlanTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		Variables: []project.Variable{
			{Name: "role_name", Type: "string", Default: "default-role"},
			{Name: "secret_policy", Type: "string", Sensitive: true},
		},
		Resources: []project.Resource{resource},
	})
	first, err := Build(context.Background(), Options{ProjectPath: projectPath, StatePath: filepath.Join(root, "state.db"), VarFiles: []string{"values.json"}})
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	second, err := Build(context.Background(), Options{ProjectPath: projectPath, StatePath: filepath.Join(root, "state.db"), VarFiles: []string{"values.json"}, Vars: []string{"role_name=cli-role"}})
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	if first.Plan.Errored || second.Plan.Errored {
		t.Fatalf("plans errored: first=%#v second=%#v", first.Plan.Diagnostics, second.Plan.Diagnostics)
	}
	if first.Plan.Inputs.Version != project.InputsVersion || first.Plan.Inputs.Digest == "" || first.Plan.Inputs.Digest == second.Plan.Inputs.Digest {
		t.Fatalf("input digests = first=%#v second=%#v", first.Plan.Inputs, second.Plan.Inputs)
	}
	if first.Plan.Resources[0].DesiredHash == second.Plan.Resources[0].DesiredHash {
		t.Fatalf("desired hash did not change with variable input: %s", first.Plan.Resources[0].DesiredHash)
	}
	if err := VerifyApproval(first.Plan); err != nil {
		t.Fatalf("approval did not verify: %v", err)
	}
	first.Plan.Inputs.Values[0].Value = "tampered"
	if err := VerifyApproval(first.Plan); err == nil {
		t.Fatalf("tampered input unexpectedly verified")
	}
	for _, input := range second.Plan.Inputs.Values {
		if input.Name == "secret_policy" && (input.Value != "${redacted}" || input.Digest == "") {
			t.Fatalf("sensitive input not redacted/digested: %#v", input)
		}
	}
}

func TestBuildProjectPolicyDenyAndApprovalRouting(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(root, "iam.json")
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())
	resource := nativeIAMRoleResourceForPlanControl("aws_iam_role.app", nil)
	projectPath := writeNativeProjectForPlanTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		Resources:  []project.Resource{resource},
	})
	denyPath := filepath.Join(root, "deny.json")
	writePlanTestFile(t, denyPath, `{"version":"ramen.policy.v1","name":"deny-create","deny_actions":["create"]}`)
	denied, err := Build(context.Background(), Options{ProjectPath: projectPath, StatePath: filepath.Join(root, "state.db"), PolicyFiles: []string{denyPath}})
	if err != nil {
		t.Fatalf("denied build: %v", err)
	}
	if !denied.Plan.Errored || len(denied.Plan.Resources) != 0 {
		t.Fatalf("denied plan = errored:%t resources:%#v diagnostics:%#v", denied.Plan.Errored, denied.Plan.Resources, denied.Plan.Diagnostics)
	}
	if len(denied.Plan.Governance.Decisions) != 1 || denied.Plan.Governance.Decisions[0].Code != "policy.deny" {
		t.Fatalf("deny governance = %#v", denied.Plan.Governance)
	}

	requirePath := filepath.Join(root, "require.json")
	writePlanTestFile(t, requirePath, `{"version":"ramen.policy.v1","name":"approval","require_approval_actions":["create"],"required_approver_roles":["admin"]}`)
	unapproved, err := Build(context.Background(), Options{ProjectPath: projectPath, StatePath: filepath.Join(root, "state.db"), PolicyFiles: []string{requirePath}})
	if err != nil {
		t.Fatalf("unapproved build: %v", err)
	}
	if unapproved.Plan.Errored || len(unapproved.Plan.Governance.ApprovalRequirements) != 1 {
		t.Fatalf("unapproved plan = errored:%t governance:%#v diagnostics:%#v", unapproved.Plan.Errored, unapproved.Plan.Governance, unapproved.Plan.Diagnostics)
	}
	if err := VerifyApproval(unapproved.Plan); err == nil {
		t.Fatalf("unapproved policy requirement unexpectedly verified")
	}
	approved, err := Build(context.Background(), Options{
		ProjectPath: projectPath,
		StatePath:   filepath.Join(root, "state.db"),
		PolicyFiles: []string{requirePath},
		Approvers:   []governance.Approver{{Identity: "alice@example.com", Role: "admin", Context: "change-123", ApprovedAt: time.Unix(1700000000, 0)}},
		Workspace:   "prod",
	})
	if err != nil {
		t.Fatalf("approved build: %v", err)
	}
	if approved.Plan.Workspace != "prod" || len(approved.Plan.Approval.Approvers) != 1 {
		t.Fatalf("approved plan workspace/approvers = workspace:%q approval:%#v", approved.Plan.Workspace, approved.Plan.Approval)
	}
	if err := VerifyApproval(approved.Plan); err != nil {
		t.Fatalf("approved policy requirement did not verify: %v", err)
	}
	approved.Plan.Approval.Approvers[0].Role = "viewer"
	if err := VerifyApproval(approved.Plan); err == nil {
		t.Fatalf("tampered approver unexpectedly verified")
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

func TestBuildWritesErroredPlanWithoutResourcesOnError(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "missing.json")
	outPath := filepath.Join(root, "plan.json")
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "role"
  assume_role_policy = "{}"
}
`)

	result, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: filepath.Join(root, "state.db"),
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: sourcePath,
		}},
		OutPath: outPath,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if !result.Plan.Errored || len(result.Plan.Resources) != 0 || result.Plan.Summary.Diagnostics == 0 {
		t.Fatalf("errored plan = %#v", result.Plan)
	}
	var written Document
	if err := json.Unmarshal([]byte(readPlanTestFile(t, outPath)), &written); err != nil {
		t.Fatalf("decode written plan: %v", err)
	}
	if !written.Errored || len(written.Resources) != 0 {
		t.Fatalf("written plan should be non-actionable: %#v", written)
	}
}

func TestBuildDiagnosesUnsupportedStaticOpenTofuBlocks(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "role"
  assume_role_policy = "{}"
}

moved {
  from = aws_iam_role.old
  to   = aws_iam_role.role
}

import {
  to = aws_iam_role.role
  id = "role"
}

removed {
  from = aws_iam_role.gone
}
`)
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())

	result, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: filepath.Join(root, "state.db"),
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: sourcePath,
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	for _, code := range []string{"plan.moved_unsupported", "plan.import_unsupported", "plan.removed_unsupported"} {
		if !hasPlanDiagnostic(result.Diagnostics, code) {
			t.Fatalf("missing %s in %#v", code, result.Diagnostics)
		}
	}
	if !result.Plan.Errored || len(result.Plan.Resources) != 0 {
		t.Fatalf("plan should be errored and non-actionable: %#v", result.Plan)
	}
}

func TestBuildLifecyclePreventDestroyBlocksDelete(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, "state.db")
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "role"
  assume_role_policy = "{}"

  lifecycle {
    prevent_destroy = true
  }
}
`)
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{
		Address:     "aws_iam_role.role",
		Type:        "aws_iam_role",
		Provider:    "provider.aws",
		DesiredHash: "sha256:old",
		Status:      "managed",
	}); err != nil {
		t.Fatalf("record resource: %v", err)
	}
	_ = store.Close()

	result, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: statePath,
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: sourcePath,
		}},
		Action: "delete",
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if !result.Plan.Errored || !hasPlanDiagnostic(result.Diagnostics, "plan.prevent_destroy") {
		t.Fatalf("prevent_destroy was not enforced: %#v", result)
	}
}

func TestBuildLifecycleIgnoreChangesSuppressesHashChanges(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, "state.db")
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "role"
  description = "old"
  assume_role_policy = "{}"

  lifecycle {
    ignore_changes = [description]
  }
}
`)
	createResult, err := Build(context.Background(), Options{
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
	role := createResult.Plan.Resources[0]
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{Address: role.Address, Type: role.Type, Provider: role.Provider, DesiredHash: role.DesiredHash, Status: "managed"}); err != nil {
		t.Fatalf("record resource: %v", err)
	}
	_ = store.Close()
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "role"
  description = "new"
  assume_role_policy = "{}"

  lifecycle {
    ignore_changes = [description]
  }
}
`)

	noOpResult, err := Build(context.Background(), Options{
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
	if noOpResult.Plan.Errored || noOpResult.Plan.Summary.NoOp != 1 {
		t.Fatalf("ignore_changes did not suppress update: %#v", noOpResult.Plan)
	}
}

func TestBuildDiagnosesUnsupportedReplaceTriggeredBy(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "role"
  assume_role_policy = "{}"

  lifecycle {
    replace_triggered_by = [aws_iam_role.other]
  }
}

resource "aws_iam_role" "other" {
  name = "other"
  assume_role_policy = "{}"
}
`)
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())
	result, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: filepath.Join(root, "state.db"),
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: sourcePath,
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if !result.Plan.Errored || !hasPlanDiagnostic(result.Diagnostics, "plan.replace_triggered_by_unsupported") {
		t.Fatalf("replace_triggered_by was not diagnosed: %#v", result.Diagnostics)
	}
}

func TestBuildDiagnosesUnsupportedLifecycleFacts(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "role"
  assume_role_policy = "{}"

  lifecycle {
    create_before_destroy = true

    precondition {
      condition     = true
      error_message = "precondition"
    }

    postcondition {
      condition     = true
      error_message = "postcondition"
    }
  }
}
`)
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())
	result, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: filepath.Join(root, "state.db"),
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: sourcePath,
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	for _, code := range []string{"plan.create_before_destroy_unsupported", "plan.precondition_unsupported", "plan.postcondition_unsupported"} {
		if !hasPlanDiagnostic(result.Diagnostics, code) {
			t.Fatalf("missing %s in %#v", code, result.Diagnostics)
		}
	}
	if !result.Plan.Errored || len(result.Plan.Resources) != 0 {
		t.Fatalf("unsupported lifecycle facts did not block the plan: %#v", result.Plan)
	}
}

func TestBuildDiagnosesUnsupportedComplexIgnoreChanges(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
variable "ignored" {}

resource "aws_iam_role" "role" {
  name = "role"
  assume_role_policy = "{}"

  lifecycle {
    ignore_changes = var.ignored
  }
}
`)
	writePlanTestFile(t, sourcePath, minimalIAMSmithyForPlanTest())
	result, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: filepath.Join(root, "state.db"),
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: sourcePath,
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if !result.Plan.Errored || !hasPlanDiagnostic(result.Diagnostics, "plan.ignore_changes_unsupported") {
		t.Fatalf("complex ignore_changes was not diagnosed: %#v", result.Diagnostics)
	}
}

func TestBuildDetectsAmbiguousOperationMatches(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	firstSource := filepath.Join(root, "iam.json")
	secondSource := filepath.Join(root, "aws-iam.json")
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "role"
  assume_role_policy = "{}"
}
`)
	writePlanTestFile(t, firstSource, minimalIAMSmithyForPlanTest())
	writePlanTestFile(t, secondSource, minimalIAMSmithyForPlanTest())

	result, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: filepath.Join(root, "state.db"),
		APISources: []APISourceInput{
			{Kind: "aws-smithy", ID: "iam", Path: firstSource},
			{Kind: "aws-smithy", ID: "aws-iam", Path: secondSource},
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if !result.Plan.Errored || !hasPlanDiagnostic(result.Diagnostics, "mapping.operation_ambiguous") {
		t.Fatalf("ambiguous operation was not diagnosed: %#v", result.Diagnostics)
	}
}

func TestBuildDesiredHashIncludesAPISourceDigest(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	firstSource := filepath.Join(root, "iam-a.json")
	secondSource := filepath.Join(root, "iam-b.json")
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "role"
  assume_role_policy = "{}"
}
`)
	writePlanTestFile(t, firstSource, minimalIAMSmithyForPlanTest())
	writePlanTestFile(t, secondSource, strings.Replace(minimalIAMSmithyForPlanTest(), `"version": "2010-05-08"`, `"version": "2010-05-08", "documentation": "changed"`, 1))

	first, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: filepath.Join(root, "state.db"),
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: firstSource,
		}},
	})
	if err != nil {
		t.Fatalf("Build first returned error: %v", err)
	}
	second, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: filepath.Join(root, "state.db"),
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: secondSource,
		}},
	})
	if err != nil {
		t.Fatalf("Build second returned error: %v", err)
	}
	if first.Plan.Resources[0].DesiredHash == second.Plan.Resources[0].DesiredHash {
		t.Fatalf("desired hash did not change with API source digest: %s", first.Plan.Resources[0].DesiredHash)
	}
}

func TestBuildDesiredHashIgnoresUnselectedAPISources(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	iamSource := filepath.Join(root, "iam.json")
	storageSource := filepath.Join(root, "storage.json")
	writePlanTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "role"
  assume_role_policy = "{}"
}
`)
	writePlanTestFile(t, iamSource, minimalIAMSmithyForPlanTest())
	writePlanTestFile(t, storageSource, minimalStorageDiscoveryForPlanTest())
	first, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: filepath.Join(root, "state.db"),
		APISources: []APISourceInput{{
			Kind: "aws-smithy",
			ID:   "iam",
			Path: iamSource,
		}},
	})
	if err != nil {
		t.Fatalf("Build first returned error: %v", err)
	}
	second, err := Build(context.Background(), Options{
		ConfigDir: configDir,
		StatePath: filepath.Join(root, "state.db"),
		APISources: []APISourceInput{
			{Kind: "aws-smithy", ID: "iam", Path: iamSource},
			{Kind: "google-discovery", ID: "storage", Path: storageSource},
		},
	})
	if err != nil {
		t.Fatalf("Build second returned error: %v", err)
	}
	if first.Plan.Resources[0].DesiredHash != second.Plan.Resources[0].DesiredHash {
		t.Fatalf("desired hash changed for unrelated source: first=%s second=%s", first.Plan.Resources[0].DesiredHash, second.Plan.Resources[0].DesiredHash)
	}
}

func TestDesiredHashIgnoresDiagnosticText(t *testing.T) {
	input := DesiredHashInput{
		Address:    "aws_iam_role.role",
		Type:       "aws_iam_role",
		Provider:   "provider.aws",
		Attributes: map[string]string{"name": `"role"`},
		Lifecycle:  map[string]any{},
		Mapping: &MappingHashInput{
			Purpose:     "create",
			SourceKind:  "aws-smithy",
			SourceID:    "iam",
			OperationID: "CreateRole",
		},
		APISourceDigest: "sha256:source",
	}
	first := DesiredHash(input)
	second := DesiredHash(input)
	if first != second {
		t.Fatalf("desired hash was unstable: first=%s second=%s", first, second)
	}
}

func assertPlanSummary(t *testing.T, got Summary, create, update, noOp int) {
	t.Helper()
	if got.Create != create || got.Update != update || got.NoOp != noOp || got.Delete != 0 {
		t.Fatalf("summary = %#v, want create=%d update=%d no-op=%d", got, create, update, noOp)
	}
}

func assertPlanSummaryWithDiagnostics(t *testing.T, got Summary, diagnostics []Diagnostic, create, update, noOp int) {
	t.Helper()
	if got.Create != create || got.Update != update || got.NoOp != noOp || got.Delete != 0 {
		t.Fatalf("summary = %#v, diagnostics = %#v, want create=%d update=%d no-op=%d", got, diagnostics, create, update, noOp)
	}
}

func hasPlanDiagnostic(diags []Diagnostic, code string) bool {
	for _, diag := range diags {
		if diag.Code == code {
			return true
		}
	}
	return false
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

func writeNativeProjectForPlanTest(t *testing.T, dir string, profile project.Profile) string {
	t.Helper()
	doc := &uws1.Document{
		UWS: "1.4.0",
		Info: &uws1.Info{
			Title:       "native_project_fixture",
			Description: "Native Ramen project fixture.",
			Version:     "1.0.0",
		},
		Operations: []*uws1.Operation{{
			OperationID: "review",
			Description: "Review native desired-state metadata.",
			Request:     map[string]any{"x-ramen-test": true},
			Extensions:  map[string]any{uws1.ExtensionOperationProfile: "ramen-project-fixture"},
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
		t.Fatalf("marshal native project: %v", err)
	}
	data = append(data, '\n')
	path := filepath.Join(dir, project.DefaultJSON)
	writePlanTestFile(t, path, string(data))
	return path
}

func nativeIAMRoleResourceForPlanControl(address string, dependencies []string) project.Resource {
	name := strings.TrimPrefix(address, "aws_iam_role.")
	return project.Resource{
		Address:      address,
		Kind:         "resource",
		Type:         "aws_iam_role",
		Name:         name,
		Provider:     "provider.aws",
		Attributes:   map[string]any{"name": name, "assume_role_policy": "{}"},
		Dependencies: slicesCloneForPlanTest(dependencies),
		Operations: map[string]project.OperationRole{
			"create": {SourceKind: "aws-smithy", SourceID: "iam", OperationID: "CreateRole"},
			"read":   {SourceKind: "aws-smithy", SourceID: "iam", OperationID: "GetRole"},
			"update": {SourceKind: "aws-smithy", SourceID: "iam", OperationID: "PutRolePolicy"},
			"delete": {SourceKind: "aws-smithy", SourceID: "iam", OperationID: "DeleteRole"},
		},
		IdentityAttributes: []project.IdentityAttribute{{Name: "role_name", Path: "name", RequestKeys: []string{"RoleName"}, ResponsePaths: []string{"Role.RoleName"}, Required: true}},
	}
}

func slicesCloneForPlanTest(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
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
