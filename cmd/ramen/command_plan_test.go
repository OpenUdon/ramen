package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/state"
)

func TestCLIInitAndPlanHelpIncludesContracts(t *testing.T) {
	for _, tt := range []struct {
		command  []string
		expected []string
	}{
		{command: []string{"apply", "--help"}, expected: []string{"Usage: ramen apply", "--plan", "--config-dir", "--workspace", "--api-source", "--var-file", "--var", "--auto-approve", "--mock", "--executor", "--udon-output", "--json", "approved plan actions", "trusted executor"}},
		{command: []string{"import", "--help"}, expected: []string{"Usage: ramen import", "--config-dir", "--workspace", "--api-source", "--var-file", "--var", "--identity", "plan-compatible desired hash"}},
		{command: []string{"init", "--help"}, expected: []string{"Usage: ramen init", "--config-dir", "--state", "--workspace", "does not execute Terraform"}},
		{command: []string{"plan", "--help"}, expected: []string{"Usage: ramen plan", "--config-dir", "--workspace", "--api-source", "--var-file", "--var", "--policy-file", "--approved-by", "--approved-at", "--state", "--target", "--exclude", "--replace", "--out", "--hcl-out", "does not execute Terraform"}},
		{command: []string{"run", "--help"}, expected: []string{"Usage: ramen run", "--target", "--policy-file", "--check", "--approval-digest", "--auto-approve", "--mock", "trusted executor"}},
		{command: []string{"show", "--help"}, expected: []string{"Usage: ramen show", "--json", "without reading state"}},
		{command: []string{"state", "--help"}, expected: []string{"Usage: ramen state", "audit", "backup", "export", "list", "restore", "show ADDRESS", "history", "runs", "vacuum"}},
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
		if tt.command[0] == "plan" && strings.Contains(text, "--destroy") {
			t.Fatalf("plan help still advertises deprecated --destroy flag:\n%s", text)
		}
	}
}

func TestPlanHasChangesIncludesAPIActions(t *testing.T) {
	actions := []struct {
		name    string
		summary tfplan.Summary
	}{
		{name: "read", summary: tfplan.Summary{Read: 1}},
		{name: "post", summary: tfplan.Summary{Post: 1}},
		{name: "put", summary: tfplan.Summary{Put: 1}},
		{name: "patch", summary: tfplan.Summary{Patch: 1}},
	}
	for _, tt := range actions {
		if !planHasChanges(tfplan.Document{Summary: tt.summary}) {
			t.Fatalf("%s action should count as plan work", tt.name)
		}
	}
	if planHasChanges(tfplan.Document{Summary: tfplan.Summary{NoOp: 1}}) {
		t.Fatalf("no-op summary should not count as executable plan work")
	}
}

func TestCLINativeWidgetExampleGoldenPath(t *testing.T) {
	projectPath, err := filepath.Abs("../../examples/widget")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	statePath := filepath.Join(root, "state.db")
	graphPath := filepath.Join(root, "graph.json")
	planPath := filepath.Join(root, "plan.json")
	applyDir := filepath.Join(root, "apply")

	run := func(args ...string) []byte {
		t.Helper()
		output, runErr := helperCommand(args...).CombinedOutput()
		if runErr != nil {
			t.Fatalf("ramen %s failed: %v\n%s", strings.Join(args, " "), runErr, output)
		}
		return output
	}

	run("init", "--project", projectPath, "--state", statePath)
	if output := run("validate", "--project", projectPath, "--json"); !bytes.Contains(output, []byte(`"valid": true`)) {
		t.Fatalf("validate output did not report a valid project:\n%s", output)
	}
	run("graph", "--project", projectPath, "--format", "json", "--out", graphPath)
	run("plan", "--project", projectPath, "--state", statePath, "--out", planPath)
	run("apply", "--plan", planPath, "--state", statePath, "--auto-approve", "--mock", "--out", applyDir)

	var resources []state.ResourceSnapshot
	output := run("state", "list", "--state", statePath, "--json")
	if err := json.Unmarshal(output, &resources); err != nil {
		t.Fatalf("state list JSON is not parseable: %v\n%s", err, output)
	}
	if len(resources) != 1 || resources[0].Address != "widget.example" || resources[0].Status != "managed" {
		t.Fatalf("state resources = %#v", resources)
	}
	for _, path := range []string{graphPath, planPath, filepath.Join(applyDir, "actions", "widget_example.uws.json")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("golden-path artifact %s: %v", path, err)
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
	if !strings.Contains(string(output), "plan-hcl:") {
		t.Fatalf("plan output missing HCL path:\n%s", output)
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
	planHCLPath := strings.TrimSuffix(planPath, filepath.Ext(planPath)) + ".hcl"
	planHCL, err := os.ReadFile(planHCLPath)
	if err != nil {
		t.Fatalf("plan HCL missing: %v", err)
	}
	for _, expected := range []string{`"ramen.plan.v1"`, `resource "aws_iam_role.role"`, `operation_id = "CreateRole"`} {
		if !strings.Contains(string(planHCL), expected) {
			t.Fatalf("plan HCL missing %q:\n%s", expected, planHCL)
		}
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
