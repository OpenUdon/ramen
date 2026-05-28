package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
		{command: []string{"apply", "--help"}, expected: []string{"Usage: ramen apply", "--config-dir", "--api-source", "--auto-approve", "--mock", "trusted executor"}},
		{command: []string{"init", "--help"}, expected: []string{"Usage: ramen init", "--config-dir", "--state", "does not execute Terraform"}},
		{command: []string{"plan", "--help"}, expected: []string{"Usage: ramen plan", "--config-dir", "--api-source", "--state", "--out", "does not execute Terraform"}},
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
	for _, rel := range []string{"project.md", "workflows/workflow.uws.yaml", "expected/conversion.json", "expected/mappings.json", "expected/diagnostics.json", "expected/diagnostics.md", "expected/review.md"} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
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
