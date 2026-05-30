package authoring

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenUdon/authoring/promptcontext"
	sharedreport "github.com/OpenUdon/authoring/report"
	"github.com/OpenUdon/ramen/project"
)

func TestDraftProjectWritesAndValidatesSkeleton(t *testing.T) {
	root := t.TempDir()
	writeTestOpenAPI(t, filepath.Join(root, "api.yaml"), "createWidget")
	result, err := DraftProject(context.Background(), Options{
		Goal:        "Manage a widget through the local API source.",
		ProjectName: "Widget Manager",
		OutDir:      root,
		Validate:    true,
		Context: promptcontext.Context{
			Sources: []promptcontext.SourceDocument{{
				ID:    "widgets",
				Kind:  "openapi",
				URI:   "api.yaml",
				Title: "Widget API",
			}},
			Operations: []promptcontext.OperationCandidate{{
				ID:          "widgets#createWidget",
				SourceID:    "widgets",
				OperationID: "createWidget",
				Verb:        "POST",
				Path:        "/widgets",
				Summary:     "Create a widget.",
			}},
		},
	})
	if err != nil {
		t.Fatalf("DraftProject returned error: %v", err)
	}
	if result.Report.Status != sharedreport.StatusComplete {
		t.Fatalf("report = %#v", result.Report)
	}
	if result.ProjectPath != filepath.Join(root, project.DefaultFile) {
		t.Fatalf("project path = %q", result.ProjectPath)
	}
	if result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("validation = %#v", result.Validation)
	}
	doc, err := project.Load(result.ProjectPath)
	if err != nil {
		t.Fatalf("load generated project: %v", err)
	}
	if len(doc.Profile.Resources) != 1 || doc.Profile.Resources[0].Operations["create"].OperationID != "createWidget" {
		t.Fatalf("profile = %#v", doc.Profile)
	}
}

func TestDraftProjectNeedsInputWithoutOperationMetadata(t *testing.T) {
	result, err := DraftProject(context.Background(), Options{
		Goal:   "Manage a widget.",
		OutDir: t.TempDir(),
		Context: promptcontext.Context{Sources: []promptcontext.SourceDocument{{
			ID:   "widgets",
			Kind: "openapi",
			URI:  "api.yaml",
		}}},
	})
	if err != nil {
		t.Fatalf("DraftProject returned error: %v", err)
	}
	if result.Report.Status != sharedreport.StatusNeedsInput || result.Report.TopIssue == nil || result.Report.TopIssue.Code != missingMappingCode {
		t.Fatalf("report = %#v", result.Report)
	}
	if result.ProjectPath != "" {
		t.Fatalf("project path = %q, want none", result.ProjectPath)
	}
}

func writeTestOpenAPI(t *testing.T, path, operationID string) {
	t.Helper()
	data := []byte(`openapi: 3.0.0
info:
  title: Authoring Test
  version: v1
paths:
  /widgets:
    post:
      operationId: ` + operationID + `
      responses:
        "200":
          description: ok
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
