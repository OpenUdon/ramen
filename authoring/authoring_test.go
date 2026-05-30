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
				ID:              "widgets#createWidget",
				SourceID:        "widgets",
				OperationID:     "createWidget",
				Verb:            "POST",
				Path:            "/widgets",
				Summary:         "Create a widget.",
				RequestSchemaID: "createWidgetRequest",
			}},
			Schemas: []promptcontext.SchemaHint{{
				ID:       "createWidgetRequest",
				Required: []string{"name", "token"},
				Fields: []promptcontext.FieldHint{
					{Name: "name", Type: "string", Required: true},
					{Name: "token", Type: "string", Required: true, Sensitive: true},
				},
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
	if len(doc.Profile.Resources[0].Schema) != 2 || doc.Profile.Resources[0].Redaction.Paths[0] != "token" {
		t.Fatalf("schema/redaction = %#v", doc.Profile.Resources[0])
	}
}

func TestDraftProjectWritesDesiredStateAndRunsGates(t *testing.T) {
	root := t.TempDir()
	writeTestOpenAPIOperations(t, filepath.Join(root, "api.yaml"), map[string]string{
		"/buckets":  "createBucket",
		"/policies": "createPolicy",
	})
	resources := []project.Resource{
		{
			Address:    "example_bucket.main",
			Kind:       "resource",
			Type:       "example_bucket",
			Name:       "main",
			Provider:   "openapi",
			Attributes: map[string]any{"name": "${var.bucket_name}"},
			Operations: map[string]project.OperationRole{
				"create": {
					Purpose:     "create",
					SourceKind:  "openapi",
					SourceID:    "api",
					SourcePath:  "api.yaml",
					OperationID: "createBucket",
				},
			},
			IdentityAttributes: []project.IdentityAttribute{{Name: "name", Path: "name", Required: true}},
			Schema:             []project.SchemaPath{{Path: "name", Type: "string", Required: true, Identity: true}},
			RequestBindings:    []project.RequestBinding{{OperationRole: "create", OperationID: "createBucket", Path: "name", RequestPath: "name", Required: true, Identity: true}},
			RequiredOperations: []string{"create"},
		},
		{
			Address:      "example_policy.main",
			Kind:         "resource",
			Type:         "example_policy",
			Name:         "main",
			Provider:     "openapi",
			Attributes:   map[string]any{"bucket_name": "${var.bucket_name}", "token": "${var.api_token}"},
			Dependencies: []string{"example_bucket.main"},
			Operations: map[string]project.OperationRole{
				"create": {
					Purpose:            "create",
					SourceKind:         "openapi",
					SourceID:           "api",
					SourcePath:         "api.yaml",
					OperationID:        "createPolicy",
					CredentialBindings: []string{"operator-api"},
				},
			},
			IdentityAttributes: []project.IdentityAttribute{{Name: "bucket_name", Path: "bucket_name", Required: true}},
			Schema: []project.SchemaPath{
				{Path: "bucket_name", Type: "string", Required: true, Identity: true},
				{Path: "token", Type: "string", Required: true, Sensitive: true},
			},
			RequestBindings: []project.RequestBinding{
				{OperationRole: "create", OperationID: "createPolicy", Path: "bucket_name", RequestPath: "bucket_name", Required: true, Identity: true},
				{OperationRole: "create", OperationID: "createPolicy", Path: "token", RequestPath: "token", Required: true},
			},
			RequiredOperations: []string{"create"},
			CredentialBindings: []string{"operator-api"},
			Redaction:          project.Redaction{Paths: []string{"token"}},
		},
	}
	result, err := DraftProject(context.Background(), Options{
		Goal:        "Manage an example bucket and policy.",
		ProjectName: "Example Desired State",
		OutDir:      root,
		Validate:    true,
		Graph:       true,
		Plan:        true,
		Variables: []project.Variable{
			{Name: "bucket_name", Type: "string", Default: "test-bucket"},
			{Name: "api_token", Type: "string", Default: "redacted", Sensitive: true},
		},
		Resources: resources,
		Redaction: project.Redaction{Paths: []string{"resources.*.token"}},
		Context: promptcontext.Context{
			Sources: []promptcontext.SourceDocument{{
				ID:   "api",
				Kind: "openapi",
				URI:  "api.yaml",
			}},
			Operations: []promptcontext.OperationCandidate{{
				ID:          "api#createBucket",
				SourceID:    "api",
				OperationID: "createBucket",
			}},
		},
	})
	if err != nil {
		t.Fatalf("DraftProject returned error: %v", err)
	}
	if result.Report.Status != sharedreport.StatusComplete {
		t.Fatalf("report = %#v", result.Report)
	}
	if result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("validation = %#v", result.Validation)
	}
	if result.Graph == nil || len(result.Graph.Nodes) != 2 || len(result.Graph.Edges) != 1 {
		t.Fatalf("graph = %#v", result.Graph)
	}
	if result.Plan == nil || result.Plan.Plan.Errored || result.Plan.Plan.Summary.Create != 2 {
		t.Fatalf("plan = %#v", result.Plan)
	}
	doc, err := project.Load(result.ProjectPath)
	if err != nil {
		t.Fatalf("load generated project: %v", err)
	}
	if len(doc.Profile.Variables) != 2 || len(doc.Profile.Resources) != 2 {
		t.Fatalf("profile = %#v", doc.Profile)
	}
	if doc.Profile.Resources[1].Dependencies[0] != "example_bucket.main" || doc.Profile.Resources[1].Redaction.Paths[0] != "token" {
		t.Fatalf("resource metadata = %#v", doc.Profile.Resources[1])
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

func writeTestOpenAPIOperations(t *testing.T, path string, operations map[string]string) {
	t.Helper()
	data := []byte("openapi: 3.0.0\ninfo:\n  title: Authoring Test\n  version: v1\npaths:\n")
	for path, operationID := range operations {
		data = append(data, []byte("  "+path+":\n    post:\n      operationId: "+operationID+"\n      responses:\n        \"200\":\n          description: ok\n")...)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
