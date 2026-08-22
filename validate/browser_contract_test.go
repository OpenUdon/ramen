package validate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenUdon/ramen/project"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestRunValidatesBrowserProfileContract(t *testing.T) {
	root := t.TempDir()
	projectPath := writeValidateBrowserProject(t, root, "read_status")
	result, err := Run(context.Background(), Options{ProjectPath: projectPath, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Summary.Diagnostics != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunReportsBrowserCrossDocumentMismatch(t *testing.T) {
	root := t.TempDir()
	projectPath := writeValidateBrowserProject(t, root, "another_action")
	result, err := Run(context.Background(), Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || !hasValidateCode(result, "validate.browser_contract_invalid") {
		t.Fatalf("result = %#v", result)
	}
}

func writeValidateBrowserProject(t *testing.T, root, selectedAction string) string {
	t.Helper()
	browserPath := filepath.Join(root, "browser.yaml")
	if err := os.WriteFile(browserPath, []byte(`profile: uws.browser.1.7
info:
  title: Reviewed status
  origin: https://example.test
  loginStateRequired: false
observationKind: accessibility_snapshot
evidence:
  learnedAt: "2026-08-20T00:00:00Z"
  source: reviewed_synthetic_fixture
confidence: high
expiresAfter: P30D
verification:
  lastVerifiedAt: "2026-08-20T00:00:00Z"
  successfulRuns: 1
actions:
  read_status:
    sequence:
      - navigate: /status
    outputs:
      count:
        type: integer
        source: a11y
        locator: {role: status, name: Count}
    sideEffects: [read_only]
    confirmationPolicy: {required: false}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "browser-profile", ID: "browser", Path: "browser.yaml"}},
		Resources: []project.Resource{{
			Address: "example.browser",
			Kind:    "resource",
			Type:    "example_browser",
			Operations: map[string]project.OperationRole{
				"read": {SourceKind: "browser-profile", SourceID: "browser", SourcePath: "browser.yaml", OperationID: "read_status", UWSOperationRef: "read_status_uws"},
			},
		}},
	}
	doc := &uws1.Document{
		UWS:  "1.9.0",
		Info: &uws1.Info{Title: "browser_validate_fixture", Version: "1.0.0"},
		SourceDescriptions: []*uws1.SourceDescription{{
			Name: "browser", URL: "browser.yaml", Type: uws1.SourceDescriptionTypeBrowserProfile,
		}},
		Operations: []*uws1.Operation{{
			OperationID:       "read_status_uws",
			SourceDescription: "browser",
			SourceOperationID: selectedAction,
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID: "main",
			Type:       uws1.WorkflowTypeSequence,
			Steps:      []*uws1.Step{{StepID: "read", OperationRef: "read_status_uws"}},
		}},
		Extensions: map[string]any{project.ExtensionKey: profile},
	}
	data, err := uwsconvert.MarshalJSONIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, project.DefaultJSON)
	if err := os.WriteFile(projectPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return projectPath
}
