package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/state"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestRunCheckModeDoesNotCallExecutorOrWriteState(t *testing.T) {
	root := t.TempDir()
	docPath := writeRunTestUWS(t, root)
	mock := &executor.MockExecutor{}
	result, err := Execute(context.Background(), Options{DocumentPath: docPath, StatePath: filepath.Join(root, "state.db"), Targets: []string{"a", "b"}, Check: true, Executor: mock})
	if err != nil {
		t.Fatalf("check run: %v", err)
	}
	if !result.Check || result.Summary.Skipped != 2 || result.ApprovalDigest == "" || mock.RequestCount() != 0 {
		t.Fatalf("result=%#v requests=%d", result, mock.RequestCount())
	}
	if store, err := state.OpenReadOnly(context.Background(), filepath.Join(root, "state.db")); err != nil || store != nil {
		t.Fatalf("state should not exist after check mode: store=%#v err=%v", store, err)
	}
}

func TestRunRequiresApprovalAndRecordsHistory(t *testing.T) {
	root := t.TempDir()
	docPath := writeRunTestUWS(t, root)
	statePath := filepath.Join(root, "state.db")
	preview, err := Execute(context.Background(), Options{DocumentPath: docPath, StatePath: statePath, Targets: []string{"b", "a"}, Check: true})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	_, err = Execute(context.Background(), Options{DocumentPath: docPath, StatePath: statePath, Targets: []string{"a"}, Executor: &executor.MockExecutor{}})
	if err == nil || !strings.Contains(err.Error(), "run approval required") {
		t.Fatalf("approval error = %v", err)
	}
	result, err := Execute(context.Background(), Options{DocumentPath: docPath, StatePath: statePath, Targets: []string{"b", "a"}, ApprovalDigest: preview.ApprovalDigest, Executor: &executor.MockExecutor{}})
	if err != nil {
		t.Fatalf("approved run: %v", err)
	}
	if result.Summary.Executed != 2 || result.RunID == 0 || len(result.Executed) != 2 {
		t.Fatalf("result = %#v", result)
	}
	store, err := state.OpenReadOnly(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	runs, err := store.ListRuns(context.Background(), "")
	if err != nil || len(runs) != 1 || runs[0].Command != "run" || runs[0].Status != "completed" {
		t.Fatalf("runs=%#v err=%v", runs, err)
	}
	events, err := store.ListRunEvents(context.Background(), result.RunID)
	if err != nil || len(events) == 0 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	_ = store.Close()
}

func writeRunTestUWS(t *testing.T, dir string) string {
	t.Helper()
	doc := &uws1.Document{
		UWS:  "1.4.0",
		Info: &uws1.Info{Title: "run_fixture", Version: "1.0.0"},
		Operations: []*uws1.Operation{{
			OperationID: "do",
			Request:     map[string]any{"x-test": true},
			Extensions:  map[string]any{uws1.ExtensionOperationProfile: "ramen-run-test"},
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID: "main",
			Type:       uws1.WorkflowTypeSequence,
			Steps: []*uws1.Step{{
				StepID:       "do",
				OperationRef: "do",
			}},
		}},
	}
	data, err := uwsconvert.MarshalJSONIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "run.uws.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
