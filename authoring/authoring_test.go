package authoring

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/apitools"
	"github.com/OpenUdon/authoring/promptcontext"
	sharedreport "github.com/OpenUdon/authoring/report"
	"github.com/OpenUdon/ramen/project"
	uwsconvert "github.com/OpenUdon/uws/convert"
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
	if result.ProjectHCLPath != filepath.Join(root, "project.uws.hcl") {
		t.Fatalf("project HCL path = %q", result.ProjectHCLPath)
	}
	hclData, err := os.ReadFile(result.ProjectHCLPath)
	if err != nil {
		t.Fatalf("read generated HCL project: %v", err)
	}
	if _, err := uwsconvert.HCLToJSON(hclData); err != nil {
		t.Fatalf("generated HCL project does not parse: %v", err)
	}
	if result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("validation = %#v", result.Validation)
	}
	doc, err := project.Load(result.ProjectPath)
	if err != nil {
		t.Fatalf("load generated project: %v", err)
	}
	if len(doc.Profile.Resources) != 1 || doc.Profile.Resources[0].Operations["post"].OperationID != "createWidget" || doc.Profile.Resources[0].Operations["post"].Method != "POST" {
		t.Fatalf("profile = %#v", doc.Profile)
	}
	if len(doc.Profile.Resources[0].Schema) != 2 || doc.Profile.Resources[0].Redaction.Paths[0] != "token" {
		t.Fatalf("schema/redaction = %#v", doc.Profile.Resources[0])
	}
}

func TestAPILifecycleResourceMarksMappedPathParameterRequired(t *testing.T) {
	ctx := promptcontext.Context{
		Sources: []promptcontext.SourceDocument{{
			ID:   "widgets",
			Kind: "openapi",
			URI:  "api.yaml",
		}},
		Operations: []promptcontext.OperationCandidate{{
			ID:              "widgets#createWidget",
			SourceID:        "widgets",
			OperationID:     "createWidget",
			Verb:            "POST",
			Path:            "/widgets",
			RequestSchemaID: "createWidgetRequest",
		}, {
			ID:          "widgets#getWidget",
			SourceID:    "widgets",
			OperationID: "getWidget",
			Verb:        "GET",
			Path:        "/widgets/{name}",
			Metadata: map[string]string{
				"parameters": `[{"name":"name","in":"path","type":"string","required":true}]`,
			},
		}},
		Schemas: []promptcontext.SchemaHint{{
			ID: "createWidgetRequest",
			Fields: []promptcontext.FieldHint{
				{Name: "metadata", Type: "object"},
				{Name: "metadata.name", Type: "string"},
			},
		}},
	}
	resource := APILifecycleResource(ctx, ctx.Operations[0], "Create and read a widget using name `${var.widget_name}`.", "widget")
	for _, path := range resource.Schema {
		if path.Path == "metadata.name" {
			if !path.Required || !path.Identity {
				t.Fatalf("metadata.name schema path = %#v", path)
			}
			return
		}
	}
	t.Fatalf("metadata.name missing from schema: %#v", resource.Schema)
}

func TestDraftProjectDownloadsRemoteAPISourceBesideProject(t *testing.T) {
	allowUnsafeAPIToolsHosts(t)
	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte(`openapi: 3.0.0
info:
  title: Remote Authoring Test
  version: v1
paths:
  /widgets:
    post:
      operationId: createWidget
      responses:
        "200":
          description: ok
`))
	}))
	defer server.Close()
	sourceURL := server.URL + "/openapi.yaml"
	result, err := DraftProject(context.Background(), Options{
		Goal:        "Manage a widget through a remote API source.",
		ProjectName: "Remote Widget Manager",
		OutDir:      root,
		Context: promptcontext.Context{
			Sources: []promptcontext.SourceDocument{{
				ID:   "widgets",
				Kind: "openapi",
				URI:  sourceURL,
			}},
			Operations: []promptcontext.OperationCandidate{{
				ID:          "widgets#createWidget",
				SourceID:    "widgets",
				OperationID: "createWidget",
				Verb:        "POST",
				Path:        "/widgets",
				Metadata:    map[string]string{"source_path": sourceURL},
			}},
		},
	})
	if err != nil {
		t.Fatalf("DraftProject returned error: %v", err)
	}
	staged := filepath.Join(root, "widgets.yaml")
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("staged remote API source missing: %v", err)
	}
	text, err := os.ReadFile(result.ProjectPath)
	if err != nil {
		t.Fatalf("read generated project: %v", err)
	}
	if strings.Contains(string(text), sourceURL) || !strings.Contains(string(text), "widgets.yaml") {
		t.Fatalf("project did not reference staged remote API source:\n%s", text)
	}
	doc, err := project.Load(result.ProjectPath)
	if err != nil {
		t.Fatalf("load generated project: %v", err)
	}
	if got := doc.Profile.APISources[0].Path; got != staged {
		t.Fatalf("api source path = %q, want %q", got, staged)
	}
}

func TestReadOnlyResourceBindsRequiredOperationParameters(t *testing.T) {
	ctx := promptcontext.Context{
		Sources: []promptcontext.SourceDocument{{
			ID:   "azure-resources",
			Kind: "openapi",
			URI:  "../azure-rest-api-specs/specification/resources/resource-manager/Microsoft.Resources/resources/stable/2025-04-01/resources.json",
		}},
		Operations: []promptcontext.OperationCandidate{{
			ID:          "azure-resources#Resources_List",
			SourceID:    "azure-resources",
			OperationID: "Resources_List",
			Verb:        "GET",
			Path:        "/subscriptions/{subscriptionId}/resources",
			Metadata: map[string]string{
				"parameters": `[{"name":"subscriptionId","in":"path","type":"string","required":true},{"name":"api-version","in":"query","type":"string","required":true}]`,
			},
		}},
	}
	resource := ReadOnlyResource(ctx, "List Azure resources", "Azure Resources")
	if got := resource.Attributes["subscriptionId"]; got != "${var.azure_subscription_id}" {
		t.Fatalf("subscriptionId attribute = %#v", got)
	}
	if got := resource.Attributes["api-version"]; got != "2025-04-01" {
		t.Fatalf("api-version attribute = %#v", got)
	}
	if len(resource.RequestBindings) != 2 {
		t.Fatalf("request bindings = %#v", resource.RequestBindings)
	}
	locations := map[string]string{}
	for _, binding := range resource.RequestBindings {
		locations[binding.Path] = binding.Location
	}
	if locations["subscriptionId"] != "path" || locations["api-version"] != "query" {
		t.Fatalf("request binding locations = %#v", resource.RequestBindings)
	}
	result, err := DraftProject(context.Background(), Options{
		Goal:      "List Azure resources",
		OutDir:    t.TempDir(),
		Context:   ctx,
		Resources: []project.Resource{resource},
	})
	if err != nil {
		t.Fatalf("DraftProject returned error: %v", err)
	}
	doc, err := project.Load(result.ProjectPath)
	if err != nil {
		t.Fatalf("load generated project: %v", err)
	}
	found := false
	for _, variable := range doc.Profile.Variables {
		if variable.Name == "azure_subscription_id" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("generated variables = %#v", doc.Profile.Variables)
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

func TestDraftProjectKeepsSourcesReferencedByExplicitResources(t *testing.T) {
	root := t.TempDir()
	writeTestOpenAPIOperations(t, filepath.Join(root, "primary.yaml"), map[string]string{"/buckets": "createBucket"})
	writeTestOpenAPIOperations(t, filepath.Join(root, "secondary.yaml"), map[string]string{"/policies": "createPolicy"})
	resource := project.Resource{
		Address:    "example_policy.main",
		Kind:       "resource",
		Type:       "example_policy",
		Name:       "main",
		Provider:   "openapi",
		Attributes: map[string]any{"name": "policy"},
		Operations: map[string]project.OperationRole{
			"create": {
				Purpose:     "create",
				SourceKind:  "openapi",
				SourceID:    "secondary",
				SourcePath:  "secondary.yaml",
				OperationID: "createPolicy",
			},
		},
		IdentityAttributes: []project.IdentityAttribute{{Name: "name", Path: "name", Required: true}},
		Schema:             []project.SchemaPath{{Path: "name", Type: "string", Required: true, Identity: true}},
		RequestBindings:    []project.RequestBinding{{OperationRole: "create", OperationID: "createPolicy", Path: "name", RequestPath: "name", Required: true, Identity: true}},
		RequiredOperations: []string{"create"},
	}

	result, err := DraftProject(context.Background(), Options{
		Goal:     "Manage a policy using an explicit secondary API source.",
		OutDir:   root,
		Validate: true,
		Context: promptcontext.Context{
			Sources: []promptcontext.SourceDocument{
				{ID: "primary", Kind: "openapi", URI: "primary.yaml"},
				{ID: "secondary", Kind: "openapi", URI: "secondary.yaml"},
			},
			Operations: []promptcontext.OperationCandidate{{
				ID:          "primary#createBucket",
				SourceID:    "primary",
				OperationID: "createBucket",
			}},
		},
		Resources: []project.Resource{resource},
	})
	if err != nil {
		t.Fatalf("DraftProject returned error: %v", err)
	}
	if result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("validation = %#v", result.Validation)
	}
	doc, err := project.Load(result.ProjectPath)
	if err != nil {
		t.Fatalf("load generated project: %v", err)
	}
	seen := map[string]bool{}
	for _, source := range doc.Profile.APISources {
		seen[source.ID] = true
	}
	if !seen["primary"] || !seen["secondary"] {
		t.Fatalf("api sources = %#v", doc.Profile.APISources)
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

func TestPromptContextFromAPISourcesAbsolutizesRelativeLocalPaths(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	writeTestOpenAPI(t, sourcePath, "createWidget")
	t.Chdir(root)
	ctx, err := PromptContextFromAPISources(context.Background(), "Create a widget", []APISourceInput{{
		Kind: "openapi",
		ID:   "widgets",
		Path: "api.yaml",
	}})
	if err != nil {
		t.Fatalf("PromptContextFromAPISources returned error: %v", err)
	}
	if len(ctx.Sources) != 1 || ctx.Sources[0].URI != sourcePath {
		t.Fatalf("source URI = %#v, want %q", ctx.Sources, sourcePath)
	}
	if len(ctx.Operations) == 0 || ctx.Operations[0].Metadata["source_path"] != sourcePath {
		t.Fatalf("operation source path = %#v, want %q", ctx.Operations, sourcePath)
	}
}

func TestPromptContextFromAPISourcesDownloadsRemoteURL(t *testing.T) {
	allowUnsafeAPIToolsHosts(t)
	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte(`openapi: 3.0.0
info:
  title: Remote Context Test
  version: v1
paths:
  /widgets:
    post:
      operationId: createWidget
      responses:
        "200":
          description: ok
`))
	}))
	defer server.Close()
	ctx, err := PromptContextFromAPISources(context.Background(), "Create a widget", []APISourceInput{{
		Kind:        "openapi",
		ID:          "widgets",
		Path:        server.URL + "/openapi.yaml",
		DownloadDir: root,
	}})
	if err != nil {
		t.Fatalf("PromptContextFromAPISources returned error: %v", err)
	}
	staged := filepath.Join(root, "widgets.yaml")
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("downloaded API source missing: %v", err)
	}
	if len(ctx.Sources) != 1 || ctx.Sources[0].URI != staged {
		t.Fatalf("source URI = %#v, want %q", ctx.Sources, staged)
	}
	if len(ctx.Operations) == 0 || ctx.Operations[0].Metadata["source_path"] != staged {
		t.Fatalf("operation source path = %#v, want %q", ctx.Operations, staged)
	}
}

func allowUnsafeAPIToolsHosts(t *testing.T) {
	t.Helper()
	previous := newAPIToolsClient
	newAPIToolsClient = func() *apitools.Client {
		return &apitools.Client{AllowUnsafeHosts: true}
	}
	t.Cleanup(func() {
		newAPIToolsClient = previous
	})
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
