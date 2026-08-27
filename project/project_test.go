package project

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestLoadPreservesContentTrustAcrossNativeFormats(t *testing.T) {
	doc := contentTrustProjectDocument()
	formats := []struct {
		name    string
		file    string
		marshal func(*uws1.Document) ([]byte, error)
	}{
		{name: "json", file: DefaultJSON, marshal: uwsconvert.MarshalJSON},
		{name: "yaml", file: DefaultFile, marshal: uwsconvert.MarshalYAML},
		{name: "hcl", file: DefaultHCL, marshal: uwsconvert.MarshalHCL},
	}
	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			root := t.TempDir()
			data, err := format.marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			writeProjectTestFile(t, filepath.Join(root, format.file), string(data))
			loaded, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(loaded.UWS.ContentTrust, doc.ContentTrust) {
				t.Fatalf("%s contentTrust = %#v, want %#v", format.name, loaded.UWS.ContentTrust, doc.ContentTrust)
			}
		})
	}
}

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

func TestBrowserOperationRoleRequiresAndNormalizesUWSReference(t *testing.T) {
	profile := Profile{
		Version: Version,
		Resources: []Resource{{
			Address: "example.browser",
			Kind:    "resource",
			Type:    "example_browser",
			Operations: map[string]OperationRole{
				"read": {SourceKind: "browser-profile", OperationID: "read_status"},
			},
		}},
	}
	if err := ValidateProfile(profile); err == nil || !strings.Contains(err.Error(), "uws_operation_ref") {
		t.Fatalf("validation error = %v", err)
	}

	root := t.TempDir()
	profile.Resources[0].Operations["read"] = OperationRole{
		SourceKind:      "browser-profile",
		OperationID:     "read_status",
		UWSOperationRef: "  read_status_uws  ",
	}
	writeProjectDocumentForTest(t, filepath.Join(root, DefaultJSON), profile)
	doc, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Profile.Resources[0].Operations["read"].UWSOperationRef; got != "read_status_uws" {
		t.Fatalf("uws operation ref = %q", got)
	}
}

func TestCandidateWorkflowsRoundTripAsNonExecutableMetadata(t *testing.T) {
	root := t.TempDir()
	writeProjectDocumentForTest(t, filepath.Join(root, DefaultJSON), Profile{
		Version: Version,
		CandidateWorkflows: []CandidateWorkflow{
			{Title: "Send notifications", Outcome: "Notify owners", DeferralReason: "Inventory is active", PromotionTrigger: "Inventory workflow is complete"},
			{Title: "Audit widgets", Outcome: "Record widget changes", DeferralReason: "Inventory is active", PromotionTrigger: "Audit is prioritized"},
		},
	})
	doc, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Profile.CandidateWorkflows) != 2 || doc.Profile.CandidateWorkflows[0].Title != "Audit widgets" {
		t.Fatalf("candidate workflows = %#v", doc.Profile.CandidateWorkflows)
	}
	if len(doc.Profile.Resources) != 0 {
		t.Fatalf("candidate workflows became executable resources: %#v", doc.Profile.Resources)
	}
}

func TestCandidateWorkflowValidationRejectsIncompleteShape(t *testing.T) {
	err := ValidateProfile(Profile{Version: Version, CandidateWorkflows: []CandidateWorkflow{{Title: "Later"}}})
	if err == nil || !strings.Contains(err.Error(), "candidate_workflows") {
		t.Fatalf("validation error = %v", err)
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

func contentTrustProjectDocument() *uws1.Document {
	return &uws1.Document{
		UWS:  "1.9.1",
		Info: &uws1.Info{Title: "content_trust_project", Version: "1.0.0"},
		SourceDescriptions: []*uws1.SourceDescription{{
			Name: "browser", URL: "browser-profiles/status.json", Type: uws1.SourceDescriptionTypeBrowserProfile,
		}},
		Operations: []*uws1.Operation{{
			OperationID: "review", SourceDescription: "browser", SourceOperationID: "read_status",
			Outputs: map[string]string{"status": "$response.body.status"},
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID: "main", Type: uws1.WorkflowTypeSequence,
			Inputs: &uws1.ParamSchema{Type: "object", Properties: map[string]*uws1.ParamSchema{"locale": {Type: "string"}}},
			Steps:  []*uws1.Step{{StepID: "review", OperationRef: "review"}},
		}},
		Triggers: []*uws1.Trigger{{TriggerID: "incoming"}},
		ContentTrust: &uws1.ContentTrust{
			SourceDescriptions: map[string]uws1.ContentTrustLevel{"browser": uws1.ContentTrustUntrusted},
			Operations: map[string]*uws1.OperationContentTrust{
				"review": {Default: uws1.ContentTrustUntrusted, Outputs: map[string]uws1.ContentTrustLevel{"status": uws1.ContentTrustTrusted}},
			},
			Triggers: map[string]uws1.ContentTrustLevel{"incoming": uws1.ContentTrustUntrusted},
			Workflows: map[string]*uws1.WorkflowContentTrust{
				"main": {Default: uws1.ContentTrustUnknown, Inputs: map[string]uws1.ContentTrustLevel{"locale": uws1.ContentTrustTrusted}},
			},
		},
		Extensions: map[string]any{ExtensionKey: Profile{Version: Version}},
	}
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
