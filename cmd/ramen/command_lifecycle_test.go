package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/project"
	ramenrun "github.com/OpenUdon/ramen/run"
)

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

func TestCLIRunCheckAndApprovedExecution(t *testing.T) {
	root := t.TempDir()
	docPath := writeNativeProjectForCLITest(t, root, project.Profile{Version: project.Version})
	statePath := filepath.Join(root, "run-state.db")
	cmd := helperCommand("run", docPath, "--target", "a", "--target", "b", "--state", statePath, "--check", "--json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run check failed: %v\n%s", err, output)
	}
	var preview ramenrun.Result
	if err := json.Unmarshal(output, &preview); err != nil || preview.Version != ramenrun.Version || !preview.Check || preview.Summary.Skipped != 2 || preview.ApprovalDigest == "" {
		t.Fatalf("preview invalid: %#v err=%v\n%s", preview, err, output)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("check mode created state: %v", err)
	}
	cmd = helperCommand("run", docPath, "--target", "a", "--target", "b", "--state", statePath, "--approval-digest", preview.ApprovalDigest, "--mock", "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("approved run failed: %v\n%s", err, output)
	}
	var executed ramenrun.Result
	if err := json.Unmarshal(output, &executed); err != nil || executed.RunID == 0 || executed.Summary.Executed != 2 {
		t.Fatalf("executed invalid: %#v err=%v\n%s", executed, err, output)
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

func TestCLIApplyPlanReplayUsesArtifactStatePath(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	planPath := filepath.Join(root, "read-plan.json")
	outDir := filepath.Join(root, "apply")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_iam_role" "role" {
  name = "cli-apply-replay-role"
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
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}, "traits": {"aws.api#input": {}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`))
	cmd := helperCommand("plan", "--config-dir", configDir, "--state", filepath.Join(root, "plan-state.db"), "--api-source", "aws-smithy:iam="+sourcePath, "--out", planPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("plan for replay test failed: %v\n%s", err, output)
	}
	cmd = helperCommand("apply", "--plan", planPath, "--auto-approve", "--mock", "--out", outDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("apply plan replay failed: %v\n%s", err, output)
	}
	if strings.Contains(string(output), "apply.approval_mismatch") {
		t.Fatalf("apply plan replay used wrong default state path:\n%s", output)
	}
	if !strings.Contains(string(output), "executed=1") {
		t.Fatalf("apply output missing execution summary:\n%s", output)
	}
}
