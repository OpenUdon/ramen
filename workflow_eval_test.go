package corpus

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

type workflowEvalManifest struct {
	Version string              `json:"version"`
	Entries []workflowEvalEntry `json:"entries"`
}

type workflowEvalEntry struct {
	ID              string `json:"id"`
	Source          string `json:"source"`
	Category        string `json:"category"`
	InputPath       string `json:"input_path"`
	WorkflowPath    string `json:"workflow_path"`
	HCLPath         string `json:"hcl_path"`
	DiagnosticsPath string `json:"diagnostics_path"`
	ReviewPath      string `json:"review_path"`
	StrictFailures  int    `json:"strict_failures"`
}

type workflowEvalDiagnostics struct {
	Version     string `json:"version"`
	Diagnostics []struct {
		StrictFailure bool `json:"strict_failure"`
	} `json:"diagnostics"`
}

func TestWorkflowEvalManifestCoversAnsibleConversionFixtures(t *testing.T) {
	manifest := loadWorkflowEvalManifest(t)
	if manifest.Version != "ramen.workflow-eval.v1" {
		t.Fatalf("workflow eval version = %q", manifest.Version)
	}
	wantIDs := map[string]bool{
		"ansible-failclosed":  false,
		"ansible-loop":        false,
		"ansible-multinotify": false,
		"ansible-nginx":       false,
	}
	for _, entry := range manifest.Entries {
		if _, ok := wantIDs[entry.ID]; !ok {
			t.Fatalf("unexpected workflow eval entry %s", entry.ID)
		}
		wantIDs[entry.ID] = true
		if entry.Source != "ramen convert ansible" || entry.Category != "ansible-conversion" {
			t.Fatalf("%s source/category = %q/%q", entry.ID, entry.Source, entry.Category)
		}
		for _, path := range []string{entry.InputPath, entry.WorkflowPath, entry.HCLPath, entry.DiagnosticsPath, entry.ReviewPath} {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("%s referenced path %s unavailable: %v", entry.ID, path, err)
			}
		}
		assertWorkflowEvalUWSValid(t, entry)
		assertWorkflowEvalDiagnostics(t, entry)
		assertWorkflowEvalReview(t, entry)
	}
	for id, seen := range wantIDs {
		if !seen {
			t.Fatalf("workflow eval manifest missing %s", id)
		}
	}
}

func loadWorkflowEvalManifest(t *testing.T) workflowEvalManifest {
	t.Helper()
	data, err := os.ReadFile("testdata/workflow-eval/manifest.json")
	if err != nil {
		t.Fatalf("read workflow eval manifest: %v", err)
	}
	var manifest workflowEvalManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse workflow eval manifest: %v", err)
	}
	if len(manifest.Entries) == 0 {
		t.Fatal("workflow eval manifest has no entries")
	}
	return manifest
}

func assertWorkflowEvalUWSValid(t *testing.T, entry workflowEvalEntry) {
	t.Helper()
	data, err := os.ReadFile(entry.WorkflowPath)
	if err != nil {
		t.Fatalf("read %s workflow: %v", entry.ID, err)
	}
	var doc uws1.Document
	if err := convert.UnmarshalYAML(data, &doc); err != nil {
		t.Fatalf("parse %s workflow: %v", entry.ID, err)
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("validate %s workflow: %v", entry.ID, err)
	}
}

func assertWorkflowEvalDiagnostics(t *testing.T, entry workflowEvalEntry) {
	t.Helper()
	data, err := os.ReadFile(entry.DiagnosticsPath)
	if err != nil {
		t.Fatalf("read %s diagnostics: %v", entry.ID, err)
	}
	var diags workflowEvalDiagnostics
	if err := json.Unmarshal(data, &diags); err != nil {
		t.Fatalf("parse %s diagnostics: %v", entry.ID, err)
	}
	if diags.Version != "ramen.ansible-convert.v1" {
		t.Fatalf("%s diagnostics version = %q", entry.ID, diags.Version)
	}
	strictFailures := 0
	for _, diag := range diags.Diagnostics {
		if diag.StrictFailure {
			strictFailures++
		}
	}
	if strictFailures != entry.StrictFailures {
		t.Fatalf("%s strict failures = %d, want %d", entry.ID, strictFailures, entry.StrictFailures)
	}
}

func assertWorkflowEvalReview(t *testing.T, entry workflowEvalEntry) {
	t.Helper()
	data, err := os.ReadFile(entry.ReviewPath)
	if err != nil {
		t.Fatalf("read %s review: %v", entry.ID, err)
	}
	text := string(data)
	for _, want := range []string{"# Ansible Conversion Review", "## Conversion Summary", "## Strict Gate"} {
		if !strings.Contains(text, want) {
			t.Fatalf("%s review missing %q", entry.ID, want)
		}
	}
}
