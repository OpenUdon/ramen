package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/state"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestCLIConvertTFHelpIncludesContract(t *testing.T) {
	cmd := helperCommand("convert", "tf", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert tf help failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{
		"Usage: ramen convert tf",
		"--config-dir",
		"--api-source",
		"--openapi",
		"--action",
		"--target",
		"--strict",
		"does not execute Terraform",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("convert tf help missing %q:\n%s", expected, text)
		}
	}
}

func TestCLIInitAndPlanHelpIncludesContracts(t *testing.T) {
	for _, tt := range []struct {
		command  []string
		expected []string
	}{
		{command: []string{"apply", "--help"}, expected: []string{"Usage: ramen apply", "--plan", "--config-dir", "--api-source", "--auto-approve", "--mock", "trusted executor"}},
		{command: []string{"destroy", "--help"}, expected: []string{"Usage: ramen destroy", "--plan", "--config-dir", "--api-source", "--auto-approve", "--mock", "trusted executor"}},
		{command: []string{"import", "--help"}, expected: []string{"Usage: ramen import", "--config-dir", "--api-source", "--identity", "plan-compatible desired hash"}},
		{command: []string{"init", "--help"}, expected: []string{"Usage: ramen init", "--config-dir", "--state", "does not execute Terraform"}},
		{command: []string{"plan", "--help"}, expected: []string{"Usage: ramen plan", "--config-dir", "--api-source", "--state", "--target", "--exclude", "--replace", "--destroy", "--out", "does not execute Terraform"}},
		{command: []string{"show", "--help"}, expected: []string{"Usage: ramen show", "--json", "without reading state"}},
		{command: []string{"state", "--help"}, expected: []string{"Usage: ramen state", "list", "show ADDRESS", "history", "Read-only"}},
	} {
		cmd := helperCommand(tt.command...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v help failed: %v\n%s", tt.command, err, output)
		}
		text := string(output)
		for _, expected := range tt.expected {
			if !strings.Contains(text, expected) {
				t.Fatalf("%v help missing %q:\n%s", tt.command, expected, text)
			}
		}
	}
}

func TestCLIImportReportsStableIdentityDiagnostics(t *testing.T) {
	for _, identity := range []string{"{", "null"} {
		cmd := helperCommand("import", "example_resource.test", "--type", "example_resource", "--identity", identity)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("import with identity %q unexpectedly succeeded:\n%s", identity, output)
		}
		if !strings.Contains(string(output), "import.identity_invalid") {
			t.Fatalf("import identity diagnostic missing stable code:\n%s", output)
		}
	}
}

func TestCLIVersionOutputsPlainTextJSONAndHelp(t *testing.T) {
	cmd := helperCommand("version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version failed: %v\n%s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != version {
		t.Fatalf("version output = %q, want %q", got, version)
	}

	cmd = helperCommand("version", "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version --json failed: %v\n%s", err, output)
	}
	var info versionInfo
	if err := json.Unmarshal(output, &info); err != nil {
		t.Fatalf("version JSON is not parseable: %v\n%s", err, output)
	}
	if info.Version != version {
		t.Fatalf("version JSON version = %q, want %q", info.Version, version)
	}
	if info.Module != "github.com/OpenUdon/ramen" {
		t.Fatalf("version JSON module = %q, want github.com/OpenUdon/ramen", info.Module)
	}

	cmd = helperCommand("version", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version help failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"Usage: ramen version", "--json", "does not check networks"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("version help missing %q:\n%s", expected, text)
		}
	}
}

func TestCLIValidateOutputsHumanJSONAndHelp(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	mustWriteCLIFile(t, sourcePath, []byte(`openapi: 3.0.0
info:
  title: Validate CLI
  version: v1
paths:
  /examples:
    post:
      operationId: createExample
      responses:
        "200":
          description: ok
`))
	projectPath := writeNativeProjectForCLITest(t, root, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: sourcePath}},
		Resources: []project.Resource{{
			Address:    "example_resource.test",
			Kind:       "resource",
			Type:       "example_resource",
			Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"}},
		}},
	})

	cmd := helperCommand("validate", "--project", projectPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validate failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "valid=true") {
		t.Fatalf("validate output missing success:\n%s", output)
	}

	cmd = helperCommand("validate", "--project", projectPath, "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validate --json failed: %v\n%s", err, output)
	}
	var result struct {
		Version string `json:"version"`
		Valid   bool   `json:"valid"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("validate JSON is not parseable: %v\n%s", err, output)
	}
	if result.Version != "ramen.validate.v1" || !result.Valid {
		t.Fatalf("validate JSON result = %#v\n%s", result, output)
	}

	cmd = helperCommand("validate", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validate help failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"Usage: ramen validate", "--project", "--api-source", "--json", "--strict", "without planning"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("validate help missing %q:\n%s", expected, text)
		}
	}

	missingPath := filepath.Join(root, "missing.yaml")
	badProjectPath := writeNativeProjectForCLITest(t, filepath.Join(root, "bad"), project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: missingPath}},
		Resources: []project.Resource{{
			Address:    "example_resource.test",
			Kind:       "resource",
			Type:       "example_resource",
			Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"}},
		}},
	})
	cmd = helperCommand("validate", "--project", badProjectPath, "--json")
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("validate unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), `"valid": false`) || !strings.Contains(string(output), "validate.api_source_document_read") {
		t.Fatalf("validate failure JSON missing diagnostics:\n%s", output)
	}
}

func TestCLIGraphOutputsDOTJSONAndOutFile(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	mustWriteCLIFile(t, sourcePath, []byte(`openapi: 3.0.0
info:
  title: Graph CLI
  version: v1
paths:
  /db:
    post:
      operationId: createDB
      responses:
        "200":
          description: ok
  /app:
    post:
      operationId: createApp
      responses:
        "200":
          description: ok
`))
	projectPath := writeNativeProjectForCLITest(t, root, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: sourcePath}},
		Resources: []project.Resource{
			{
				Address:      "example_resource.app",
				Kind:         "resource",
				Type:         "example_resource",
				Dependencies: []string{"example_resource.db"},
				Operations:   map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createApp"}},
			},
			{
				Address:    "example_resource.db",
				Kind:       "resource",
				Type:       "example_resource",
				Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createDB"}},
			},
		},
	})

	cmd := helperCommand("graph", "--project", projectPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("graph failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `digraph ramen`) || !strings.Contains(string(output), `"example_resource.db" -> "example_resource.app"`) {
		t.Fatalf("graph DOT missing expected edge:\n%s", output)
	}

	cmd = helperCommand("graph", "--project", projectPath, "--format", "json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("graph json failed: %v\n%s", err, output)
	}
	var result struct {
		Version string `json:"version"`
		Nodes   []struct {
			Address string `json:"address"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("graph JSON is not parseable: %v\n%s", err, output)
	}
	if result.Version != "ramen.graph.v1" || len(result.Nodes) != 2 || len(result.Edges) != 1 {
		t.Fatalf("graph JSON result = %#v\n%s", result, output)
	}

	outPath := filepath.Join(root, "graph.dot")
	cmd = helperCommand("graph", "--project", projectPath, "--out", outPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("graph --out failed: %v\n%s", err, output)
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("graph out file missing: %v", err)
	}
	if !strings.Contains(string(out), `"example_resource.db" -> "example_resource.app"`) {
		t.Fatalf("graph out file missing edge:\n%s", out)
	}
}

func TestCLIGraphReportsValidationDiagnostics(t *testing.T) {
	root := t.TempDir()
	projectPath := writeNativeProjectForCLITest(t, root, project.Profile{
		Version: project.Version,
		Resources: []project.Resource{
			{Address: "a", Kind: "resource", Type: "example", Dependencies: []string{"b"}, Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createA"}}},
			{Address: "b", Kind: "resource", Type: "example", Dependencies: []string{"a"}, Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createB"}}},
		},
	})
	cmd := helperCommand("graph", "--project", projectPath, "--format", "json")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("graph unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), `"diagnostics"`) || !strings.Contains(string(output), "validate.dependency_cycle") {
		t.Fatalf("graph JSON missing diagnostics:\n%s", output)
	}
}

func TestCLIForceUnlockRequiresExactHolder(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.db")
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.AcquireLock(context.Background(), "state", "holder-1", time.Minute); err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	_ = store.Close()

	cmd := helperCommand("force-unlock", "wrong-holder", "--state", statePath)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("force-unlock unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), "holder-1") || !strings.Contains(string(output), "wrong-holder") {
		t.Fatalf("force-unlock mismatch output missing holder detail:\n%s", output)
	}

	cmd = helperCommand("force-unlock", "holder-1", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("force-unlock failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "force-unlocked state held by holder-1") {
		t.Fatalf("force-unlock output missing summary:\n%s", output)
	}
	verify, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("reopen state: %v", err)
	}
	if lock, err := verify.CurrentLock(context.Background(), "state"); err != nil || lock != nil {
		t.Fatalf("lock after force unlock = %#v err=%v", lock, err)
	}
	_ = verify.Close()

	cmd = helperCommand("force-unlock", "holder-1", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("force-unlock missing lock unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), `state lock "state" is not held`) {
		t.Fatalf("force-unlock missing output:\n%s", output)
	}

	expired, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open expired state: %v", err)
	}
	if err := expired.AcquireLock(context.Background(), "state", "expired-holder", time.Nanosecond); err != nil {
		t.Fatalf("acquire expired lock: %v", err)
	}
	_ = expired.Close()
	time.Sleep(time.Millisecond)
	cmd = helperCommand("force-unlock", "expired-holder", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("force-unlock expired unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), `state lock "state" is not held`) {
		t.Fatalf("force-unlock expired output:\n%s", output)
	}

	cmd = helperCommand("force-unlock", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("force-unlock malformed args unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), "Usage: ramen force-unlock") {
		t.Fatalf("force-unlock malformed output missing usage:\n%s", output)
	}
}

func TestCLIApplyMockWritesState(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, "state.db")
	outDir := filepath.Join(root, "apply")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_iam_role" "role" {
  name = "cli-apply-role"
  assume_role_policy = "{}"
}
`))
	mustWriteCLIFile(t, sourcePath, []byte(`{
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
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`))
	cmd := helperCommand("apply", "--config-dir", configDir, "--state", statePath, "--api-source", "aws-smithy:iam="+sourcePath, "--auto-approve", "--mock", "--out", outDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("apply failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "executed=1") {
		t.Fatalf("apply output missing summary:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(outDir, "actions", "aws_iam_role_role.uws.json")); err != nil {
		t.Fatalf("apply UWS not written: %v", err)
	}
	cmd = helperCommand("apply", "--config-dir", configDir, "--state", statePath, "--api-source", "aws-smithy:iam="+sourcePath, "--auto-approve", "--mock")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("second apply failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "no-op=1") || !strings.Contains(string(output), "executed=0") {
		t.Fatalf("second apply output missing no-op summary:\n%s", output)
	}
}

func TestCLIConvertTFWritesDraftArtifacts(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "openapi.yaml")
	outDir := filepath.Join(root, "out")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_instance" "web" {
  name = "web"
}
`))
	mustWriteCLIFile(t, openAPIPath, []byte(`openapi: 3.0.0
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
`))
	cmd := helperCommand("convert", "tf", "--config-dir", configDir, "--openapi", "aws="+openAPIPath, "--action", "create", "--out", outDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert tf failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "ramen: convert tf wrote") {
		t.Fatalf("convert output missing summary:\n%s", output)
	}
	for _, rel := range []string{"project.md", "project.uws.yaml", "workflows/workflow.uws.yaml", "expected/conversion.json", "expected/mappings.json", "expected/diagnostics.json", "expected/diagnostics.md", "expected/review.md"} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	planPath := filepath.Join(root, "native-plan.json")
	cmd = helperCommand("plan", "--project", outDir, "--state", filepath.Join(root, "state.db"), "--out", planPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native project plan failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "create=1") {
		t.Fatalf("native project plan output missing summary:\n%s", output)
	}
}

func TestCLIInitAndPlanWritesStaticPlan(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, "state.db")
	planPath := filepath.Join(root, "plan.json")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_iam_role" "role" {
  name = "cli-role"
  assume_role_policy = "{}"
}
`))
	mustWriteCLIFile(t, sourcePath, []byte(`{
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
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`))
	cmd := helperCommand("init", "--config-dir", configDir, "--state", statePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state was not created: %v", err)
	}
	cmd = helperCommand("plan", "--config-dir", configDir, "--state", statePath, "--api-source", "aws-smithy:iam="+sourcePath, "--out", planPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plan failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "create=1") {
		t.Fatalf("plan output missing summary:\n%s", output)
	}
	planText, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("plan JSON missing: %v", err)
	}
	for _, expected := range []string{`"version": "ramen.plan.v1"`, `"address": "aws_iam_role.role"`, `"operation_id": "CreateRole"`} {
		if !strings.Contains(string(planText), expected) {
			t.Fatalf("plan JSON missing %q:\n%s", expected, planText)
		}
	}
}

func TestCLIShowAndStateInspectReadOnly(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	statePath := filepath.Join(root, "state.db")
	planPath := filepath.Join(root, "plan.json")
	mustWriteCLIFile(t, sourcePath, []byte(`openapi: 3.0.0
info:
  title: Show CLI
  version: v1
paths:
  /examples:
    post:
      operationId: createExample
      responses:
        "200":
          description: ok
`))
	projectPath := writeNativeProjectForCLITest(t, root, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: sourcePath}},
		Resources: []project.Resource{{
			Address:    "example_resource.test",
			Kind:       "resource",
			Type:       "example_resource",
			Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"}},
		}},
	})
	cmd := helperCommand("plan", "--project", projectPath, "--state", statePath, "--out", planPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plan failed: %v\n%s", err, output)
	}
	cmd = helperCommand("show", planPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("show failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "ramen: show") || !strings.Contains(string(output), "approval:") {
		t.Fatalf("show output missing summary:\n%s", output)
	}
	cmd = helperCommand("show", planPath, "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("show --json failed: %v\n%s", err, output)
	}
	var shown tfplan.Document
	if err := json.Unmarshal(output, &shown); err != nil || shown.Version != tfplan.Version {
		t.Fatalf("show JSON invalid doc=%#v err=%v\n%s", shown, err, output)
	}
	badPath := filepath.Join(root, "bad.json")
	mustWriteCLIFile(t, badPath, []byte(`{"version":"bad"}`))
	cmd = helperCommand("show", badPath)
	output, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "show.plan_version_invalid") {
		t.Fatalf("bad show output: err=%v\n%s", err, output)
	}

	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{Address: "example_resource.test", Type: "example_resource", DesiredHash: "hash", IdentityJSON: `{"id":"test"}`, Status: "managed"}); err != nil {
		t.Fatalf("record resource: %v", err)
	}
	if err := store.RecordRevision(context.Background(), state.Revision{ResourceAddress: "example_resource.test", Action: "import", AfterJSON: `{"status":"managed"}`}); err != nil {
		t.Fatalf("record revision: %v", err)
	}
	_ = store.Close()
	cmd = helperCommand("state", "list", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state list failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "example_resource.test") {
		t.Fatalf("state list missing address:\n%s", output)
	}
	cmd = helperCommand("state", "show", "example_resource.test", "--state", statePath, "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state show failed: %v\n%s", err, output)
	}
	var resource state.ResourceSnapshot
	if err := json.Unmarshal(output, &resource); err != nil || resource.Address != "example_resource.test" {
		t.Fatalf("state show JSON invalid resource=%#v err=%v\n%s", resource, err, output)
	}
	cmd = helperCommand("state", "history", "example_resource.test", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state history failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "action=import") {
		t.Fatalf("state history missing revision:\n%s", output)
	}
	cmd = helperCommand("state", "show", "missing.address", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "state.resource_not_found") {
		t.Fatalf("missing state show output: err=%v\n%s", err, output)
	}
}

func TestCLIInitValidatesStaticConfigBeforeCreatingState(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	statePath := filepath.Join(root, "state.db")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_iam_role" "role" {
  name = "broken"
`))
	cmd := helperCommand("init", "--config-dir", configDir, "--state", statePath)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("init succeeded unexpectedly:\n%s", output)
	}
	if _, statErr := os.Stat(statePath); !os.IsNotExist(statErr) {
		t.Fatalf("state should not be created after invalid init, stat err=%v", statErr)
	}
}

func TestCLIPlanErroredOutAndDetailedExitCode(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	missingSource := filepath.Join(root, "missing.json")
	planPath := filepath.Join(root, "plan.json")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_iam_role" "role" {
  name = "cli-role"
  assume_role_policy = "{}"
}
`))
	cmd := helperCommand("plan", "--config-dir", configDir, "--state", filepath.Join(root, "state.db"), "--api-source", "aws-smithy:iam="+missingSource, "--out", planPath, "--detailed-exitcode")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("plan succeeded unexpectedly:\n%s", output)
	}
	planText, readErr := os.ReadFile(planPath)
	if readErr != nil {
		t.Fatalf("errored plan JSON missing: %v\n%s", readErr, output)
	}
	if !strings.Contains(string(planText), `"errored": true`) || !strings.Contains(string(planText), `"resources": null`) {
		t.Fatalf("plan output should be non-actionable:\n%s", planText)
	}
	if !strings.Contains(string(output), "api_source.load_error") {
		t.Fatalf("plan output missing diagnostic:\n%s", output)
	}
}

func TestCLIPlanDetailedExitCodeReturnsTwoForChanges(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_iam_role" "role" {
  name = "cli-role"
  assume_role_policy = "{}"
}
`))
	mustWriteCLIFile(t, sourcePath, []byte(`{
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
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`))
	cmd := helperCommand("plan", "--config-dir", configDir, "--state", filepath.Join(root, "state.db"), "--api-source", "aws-smithy:iam="+sourcePath, "--detailed-exitcode")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("plan should exit 2 for changes but succeeded:\n%s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
		t.Fatalf("plan exit = %v, output:\n%s", err, output)
	}
}

func helperCommand(args ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=TestHelperProcess", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	os.Args = append([]string{"ramen"}, args[1:]...)
	main()
	os.Exit(0)
}

func mustWriteCLIFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeNativeProjectForCLITest(t *testing.T, dir string, profile project.Profile) string {
	t.Helper()
	doc := &uws1.Document{
		UWS: "1.4.0",
		Info: &uws1.Info{
			Title:   "cli_validate_fixture",
			Version: "1.0.0",
		},
		Operations: []*uws1.Operation{{
			OperationID: "review",
			Request:     map[string]any{"x-test": true},
			Extensions:  map[string]any{uws1.ExtensionOperationProfile: "ramen-cli-test"},
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID: "main",
			Type:       uws1.WorkflowTypeSequence,
			Steps: []*uws1.Step{{
				StepID:       "review",
				OperationRef: "review",
			}},
		}},
		Extensions: map[string]any{project.ExtensionKey: profile},
	}
	data, err := uwsconvert.MarshalJSONIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, project.DefaultJSON)
	mustWriteCLIFile(t, path, data)
	return path
}
