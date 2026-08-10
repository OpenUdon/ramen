package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/state"
)

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
	runID, err := store.StartRun(context.Background(), "apply")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := store.FinishRun(context.Background(), runID, "completed", `{"ok":true}`); err != nil {
		t.Fatalf("finish run: %v", err)
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
	cmd = helperCommand("state", "runs", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state runs failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "apply") || !strings.Contains(string(output), "completed") {
		t.Fatalf("state runs missing run summary:\n%s", output)
	}
	cmd = helperCommand("state", "export", "--state", statePath, "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state export failed: %v\n%s", err, output)
	}
	var exported state.ExportDocument
	if err := json.Unmarshal(output, &exported); err != nil || exported.Version != state.ExportVersion || len(exported.Resources) != 1 || len(exported.Revisions) != 1 || len(exported.Runs) != 1 {
		t.Fatalf("state export JSON invalid export=%#v err=%v\n%s", exported, err, output)
	}
	cmd = helperCommand("state", "audit", "--state", statePath, "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state audit failed: %v\n%s", err, output)
	}
	var audit state.AuditDocument
	if err := json.Unmarshal(output, &audit); err != nil || audit.Version != state.AuditVersion || !strings.HasPrefix(audit.Digest, "sha256:") || audit.Counts["resources"] != 1 {
		t.Fatalf("state audit JSON invalid audit=%#v err=%v\n%s", audit, err, output)
	}
	backupPath := filepath.Join(root, "backup.db")
	cmd = helperCommand("state", "backup", "--state", statePath, "--out", backupPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state backup failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "state backup written") {
		t.Fatalf("state backup output missing summary:\n%s", output)
	}
	restorePath := filepath.Join(root, "restored.db")
	cmd = helperCommand("state", "restore", "--state", restorePath, "--from", backupPath)
	output, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "--force") {
		t.Fatalf("state restore without force output: err=%v\n%s", err, output)
	}
	cmd = helperCommand("state", "restore", "--state", restorePath, "--from", backupPath, "--force")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state restore failed: %v\n%s", err, output)
	}
	cmd = helperCommand("state", "vacuum", "--state", restorePath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state vacuum failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "state vacuum completed") {
		t.Fatalf("state vacuum output missing summary:\n%s", output)
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

func TestCLIStateAsyncEvidenceJSON(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.db")
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	runID, err := store.StartRun(context.Background(), "apply")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := store.RecordAsyncEvidence(context.Background(), state.AsyncEvidenceRecord{
		RunID:           runID,
		ResourceAddress: "example.one",
		Action:          "create",
		OperationID:     "createOne",
		RecordKind:      "execution_request",
		Phase:           "submitted",
		EvidenceID:      "ev-cli",
		AttemptID:       "attempt-cli",
		Sequence:        1,
		RecordJSON:      `{"version":"evidence.async.execution-request.v1"}`,
	}); err != nil {
		t.Fatalf("record async evidence: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close state: %v", err)
	}

	cmd := helperCommand("state", "async-evidence", "--state", statePath, "--run", fmt.Sprint(runID), "--json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state async-evidence failed: %v\n%s", err, output)
	}
	var records []state.AsyncEvidenceRecord
	if err := json.Unmarshal(output, &records); err != nil {
		t.Fatalf("async evidence JSON parse: %v\n%s", err, output)
	}
	if len(records) != 1 || records[0].EvidenceID != "ev-cli" || records[0].RecordKind != "execution_request" {
		t.Fatalf("records = %#v", records)
	}
}

func TestCLIStateAsyncEvidenceDoesNotExposeResumeOrResubmit(t *testing.T) {
	for _, flag := range []string{"--resume", "--resubmit"} {
		cmd := helperCommand("state", "async-evidence", flag)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("state async-evidence %s unexpectedly succeeded:\n%s", flag, output)
		}
		if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
			t.Fatalf("state async-evidence %s exit = %v, output:\n%s", flag, err, output)
		}
		if !strings.Contains(string(output), strings.TrimPrefix(flag, "--")) {
			t.Fatalf("state async-evidence %s output missing rejected flag:\n%s", flag, output)
		}
	}

	cmd := helperCommand("state", "async-evidence", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state async-evidence help failed: %v\n%s", err, output)
	}
	for _, forbidden := range []string{"--resume", "--resubmit"} {
		if strings.Contains(string(output), forbidden) {
			t.Fatalf("state async-evidence help advertises %s:\n%s", forbidden, output)
		}
	}
}
