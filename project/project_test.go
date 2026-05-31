package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestLoadResolvesDefaultProjectAndRelativeSourcePaths(t *testing.T) {
	root := t.TempDir()
	writeProjectTestFile(t, filepath.Join(root, "sources", "api.json"), "{}")
	writeProjectDocumentForTest(t, filepath.Join(root, DefaultJSON), Profile{
		Version: Version,
		APISources: []APISource{{
			Kind: "openapi",
			ID:   "api",
			Path: "sources/api.json",
		}},
		Resources: []Resource{{
			Address: "example_resource.test",
			Kind:    "resource",
			Type:    "example_resource",
			Operations: map[string]OperationRole{
				"create": {SourceKind: "openapi", SourceID: "api", SourcePath: "sources/api.json", OperationID: "createExample"},
			},
		}},
	})

	doc, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	want := filepath.Join(root, "sources", "api.json")
	if got := doc.Profile.APISources[0].Path; got != want {
		t.Fatalf("api source path = %q, want %q", got, want)
	}
	if got := doc.Profile.Resources[0].Operations["create"].SourcePath; got != want {
		t.Fatalf("operation source path = %q, want %q", got, want)
	}
}

func TestLoadPreservesRemoteSourcePaths(t *testing.T) {
	root := t.TempDir()
	sourceURL := "https://example.com/openapi.yaml"
	writeProjectDocumentForTest(t, filepath.Join(root, DefaultJSON), Profile{
		Version: Version,
		APISources: []APISource{{
			Kind: "openapi",
			ID:   "api",
			Path: sourceURL,
		}},
		Resources: []Resource{{
			Address: "example_resource.test",
			Kind:    "resource",
			Type:    "example_resource",
			Operations: map[string]OperationRole{
				"create": {SourceKind: "openapi", SourceID: "api", SourcePath: sourceURL, OperationID: "createExample"},
			},
		}},
	})

	doc, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := doc.Profile.APISources[0].Path; got != sourceURL {
		t.Fatalf("api source path = %q, want %q", got, sourceURL)
	}
	if got := doc.Profile.Resources[0].Operations["create"].SourcePath; got != sourceURL {
		t.Fatalf("operation source path = %q, want %q", got, sourceURL)
	}
}

func TestLoadRequiresRamenProfileExtension(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, DefaultJSON)
	writeProjectDocumentForTest(t, path, Profile{})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc uws1.Document
	if err := uwsconvert.UnmarshalJSON(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc.Extensions = nil
	data, err = uwsconvert.MarshalJSONIndent(&doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeProjectTestFile(t, path, string(data))

	if _, err := Load(path); err == nil {
		t.Fatalf("Load succeeded without %s extension", ExtensionKey)
	}
}

func TestLoadRunsUWSSchemaValidationBeforeProfileValidation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, DefaultJSON)
	writeProjectTestFile(t, path, `{
  "uws": "1.4.0",
  "info": {"title": "schema_invalid", "version": "1.0.0"},
  "operations": [
    {"operationId": "review", "x-uws-operation-profile": "ramen-project-test"}
  ],
  "workflows": [
    {"workflowId": "main", "steps": [{"stepId": "review", "operationRef": "review"}]}
  ],
  "x-ramen-desired-state": {
    "version": "ramen.project.v1"
  }
}`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "jsonschema validation failed") {
		t.Fatalf("expected UWS schema validation failure, got %v", err)
	}
}

func writeProjectDocumentForTest(t *testing.T, path string, profile Profile) {
	t.Helper()
	doc := &uws1.Document{
		UWS: "1.4.0",
		Info: &uws1.Info{
			Title:   "project_fixture",
			Version: "1.0.0",
		},
		Operations: []*uws1.Operation{{
			OperationID: "review",
			Request:     map[string]any{"x-test": true},
			Extensions:  map[string]any{uws1.ExtensionOperationProfile: "ramen-project-test"},
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
			ExtensionKey: profile,
		},
	}
	data, err := uwsconvert.MarshalJSONIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeProjectTestFile(t, path, string(data))
}

func writeProjectTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
