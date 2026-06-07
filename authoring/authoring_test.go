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

func TestAPILifecycleResourceAliasesGooglePathParametersToNameIdentity(t *testing.T) {
	ctx := promptcontext.Context{
		Sources: []promptcontext.SourceDocument{{ID: "storage", Kind: "google-discovery", URI: "storage.json"}},
		Operations: []promptcontext.OperationCandidate{{
			ID:              "storage.buckets.insert",
			SourceID:        "storage",
			OperationID:     "storage.buckets.insert",
			Verb:            "POST",
			Path:            "/b",
			RequestSchemaID: "request:storage.buckets.insert",
		}, {
			ID:               "storage.buckets.get",
			SourceID:         "storage",
			OperationID:      "storage.buckets.get",
			Verb:             "GET",
			Path:             "/b/{bucket}",
			ResponseSchemaID: "response:storage.buckets.get",
			Metadata: map[string]string{
				"parameters": `[{"name":"bucket","in":"path","type":"string","required":true}]`,
			},
		}},
		Schemas: []promptcontext.SchemaHint{{
			ID: "request:storage.buckets.insert",
			Fields: []promptcontext.FieldHint{
				{Name: "name", Type: "string", Required: true},
				{Name: "location", Type: "string", Required: true},
			},
		}, {
			ID: "response:storage.buckets.get",
			Fields: []promptcontext.FieldHint{
				{Name: "name", Type: "string"},
				{Name: "location", Type: "string"},
			},
		}},
	}
	resource := APILifecycleResource(ctx, ctx.Operations[0], "Create and read a bucket using bucket_name `${var.bucket_name}`.", "bucket")
	if got := identityPathSet(resource.IdentityAttributes); !got["name"] || got["bucket"] {
		t.Fatalf("identity attributes = %#v", resource.IdentityAttributes)
	}
	if binding := requestBindingFor(resource.RequestBindings, "read", "bucket"); binding == nil || binding.Path != "name" || !binding.Identity {
		t.Fatalf("bucket read binding = %#v in %#v", binding, resource.RequestBindings)
	}
}

func TestAPILifecycleResourcePreservesCloudflareAccountScopeAndAliasesDatabaseID(t *testing.T) {
	ctx := promptcontext.Context{
		Sources: []promptcontext.SourceDocument{{ID: "cloudflare", Kind: "openapi", URI: "cloudflare.json"}},
		Operations: []promptcontext.OperationCandidate{{
			ID:              "createD1Database",
			SourceID:        "cloudflare",
			OperationID:     "createD1Database",
			Verb:            "POST",
			Path:            "/accounts/{account_id}/d1/database",
			RequestSchemaID: "request:createD1Database",
			Metadata: map[string]string{
				"parameters": `[{"name":"account_id","in":"path","type":"string","required":true}]`,
			},
		}, {
			ID:               "getD1Database",
			SourceID:         "cloudflare",
			OperationID:      "getD1Database",
			Verb:             "GET",
			Path:             "/accounts/{account_id}/d1/database/{database_id}",
			ResponseSchemaID: "response:getD1Database",
			Metadata: map[string]string{
				"parameters": `[{"name":"account_id","in":"path","type":"string","required":true},{"name":"database_id","in":"path","type":"string","required":true}]`,
			},
		}},
		Schemas: []promptcontext.SchemaHint{{
			ID:     "request:createD1Database",
			Fields: []promptcontext.FieldHint{{Name: "name", Type: "string", Required: true}},
		}, {
			ID: "response:getD1Database",
			Fields: []promptcontext.FieldHint{
				{Name: "result.name", Type: "string"},
				{Name: "result.uuid", Type: "string"},
			},
		}},
	}
	resource := APILifecycleResource(ctx, ctx.Operations[0], "Create and read a D1 database named `${var.database_name}` for account `${var.account_id}`.", "d1")
	identities := identityPathSet(resource.IdentityAttributes)
	if !identities["name"] || identities["database_id"] {
		t.Fatalf("identity attributes = %#v", resource.IdentityAttributes)
	}
	if binding := requestBindingFor(resource.RequestBindings, "read", "database_id"); binding == nil || binding.Path != "name" {
		t.Fatalf("database_id read binding = %#v in %#v", binding, resource.RequestBindings)
	}
	if binding := requestBindingFor(resource.RequestBindings, "read", "account_id"); binding == nil || binding.Path != "account_id" {
		t.Fatalf("account_id read binding = %#v in %#v", binding, resource.RequestBindings)
	}
}

func TestAPILifecycleResourceKeepsAzureGlobalScopeParamsOutOfIdentity(t *testing.T) {
	ctx := promptcontext.Context{
		Sources: []promptcontext.SourceDocument{{ID: "cosmos", Kind: "openapi", URI: "cosmos.json"}},
		Operations: []promptcontext.OperationCandidate{{
			ID:              "DatabaseAccounts_CreateOrUpdate",
			SourceID:        "cosmos",
			OperationID:     "DatabaseAccounts_CreateOrUpdate",
			Verb:            "PUT",
			Path:            "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.DocumentDB/databaseAccounts/{accountName}",
			RequestSchemaID: "request:DatabaseAccounts_CreateOrUpdate",
			Metadata: map[string]string{
				"parameters": `[{"name":"subscriptionId","in":"path","type":"string","required":true},{"name":"resourceGroupName","in":"path","type":"string","required":true},{"name":"accountName","in":"path","type":"string","required":true},{"name":"api-version","in":"query","type":"string","required":true}]`,
			},
		}, {
			ID:               "DatabaseAccounts_Get",
			SourceID:         "cosmos",
			OperationID:      "DatabaseAccounts_Get",
			Verb:             "GET",
			Path:             "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.DocumentDB/databaseAccounts/{accountName}",
			ResponseSchemaID: "response:DatabaseAccounts_Get",
			Metadata: map[string]string{
				"parameters": `[{"name":"subscriptionId","in":"path","type":"string","required":true},{"name":"resourceGroupName","in":"path","type":"string","required":true},{"name":"accountName","in":"path","type":"string","required":true},{"name":"api-version","in":"query","type":"string","required":true},{"name":"location","in":"query","type":"string","required":true}]`,
			},
		}},
		Schemas: []promptcontext.SchemaHint{{
			ID: "request:DatabaseAccounts_CreateOrUpdate",
			Fields: []promptcontext.FieldHint{
				{Name: "location", Type: "string", Required: true},
				{Name: "properties.databaseAccountOfferType", Type: "string", Required: true},
			},
		}, {
			ID:     "response:DatabaseAccounts_Get",
			Fields: []promptcontext.FieldHint{{Name: "name", Type: "string"}, {Name: "location", Type: "string"}},
		}},
	}
	resource := APILifecycleResource(ctx, ctx.Operations[0], "Create and read a Cosmos account named `${var.account_name}`.", "cosmos")
	identities := identityPathSet(resource.IdentityAttributes)
	for _, noisy := range []string{"subscriptionId", "api-version", "location"} {
		if identities[noisy] {
			t.Fatalf("%s should not be identity: %#v", noisy, resource.IdentityAttributes)
		}
	}
	if !identities["name"] {
		t.Fatalf("name identity missing: %#v", resource.IdentityAttributes)
	}
	location := schemaPathFor(resource.Schema, "location")
	if location == nil || !location.Required || location.Computed || location.ReadOnly || location.ResponseDerivedIdentity {
		t.Fatalf("location schema path = %#v in %#v", location, resource.Schema)
	}
}

func TestAPILifecycleResourceResponseBindingsUseReadResponseSchemaOnly(t *testing.T) {
	ctx := promptcontext.Context{
		Sources: []promptcontext.SourceDocument{{ID: "widgets", Kind: "openapi", URI: "api.yaml"}},
		Operations: []promptcontext.OperationCandidate{{
			ID:              "createWidget",
			SourceID:        "widgets",
			OperationID:     "createWidget",
			Verb:            "POST",
			Path:            "/widgets",
			RequestSchemaID: "request:createWidget",
		}, {
			ID:               "getWidget",
			SourceID:         "widgets",
			OperationID:      "getWidget",
			Verb:             "GET",
			Path:             "/widgets/{name}",
			ResponseSchemaID: "response:getWidget",
			Metadata: map[string]string{
				"parameters": `[{"name":"name","in":"path","type":"string","required":true}]`,
			},
		}},
		Schemas: []promptcontext.SchemaHint{{
			ID:     "request:createWidget",
			Fields: []promptcontext.FieldHint{{Name: "name", Type: "string", Required: true}},
		}, {
			ID: "response:getWidget",
			Fields: []promptcontext.FieldHint{
				{Name: "id", Type: "string"},
				{Name: "status", Type: "string"},
			},
		}},
	}
	resource := APILifecycleResource(ctx, ctx.Operations[0], "Create and read a widget.", "widget")
	paths := responseStatePathSet(resource.ResponseBindings)
	if !paths["id"] || !paths["status"] || paths["name"] {
		t.Fatalf("response bindings = %#v", resource.ResponseBindings)
	}
}

func TestAPILifecycleResourceDoesNotBindResponseOnlySchemaToMutationRequest(t *testing.T) {
	ctx := promptcontext.Context{
		Sources: []promptcontext.SourceDocument{{ID: "widgets", Kind: "openapi", URI: "api.yaml"}},
		Operations: []promptcontext.OperationCandidate{{
			ID:               "createWidget",
			SourceID:         "widgets",
			OperationID:      "createWidget",
			Verb:             "POST",
			Path:             "/widgets",
			ResponseSchemaID: "response:createWidget",
		}, {
			ID:               "getWidget",
			SourceID:         "widgets",
			OperationID:      "getWidget",
			Verb:             "GET",
			Path:             "/widgets/{name}",
			ResponseSchemaID: "response:getWidget",
			Metadata: map[string]string{
				"parameters": `[{"name":"name","in":"path","type":"string","required":true}]`,
			},
		}},
		Schemas: []promptcontext.SchemaHint{{
			ID: "response:createWidget",
			Fields: []promptcontext.FieldHint{
				{Name: "id", Type: "string"},
				{Name: "status", Type: "string"},
			},
		}, {
			ID: "response:getWidget",
			Fields: []promptcontext.FieldHint{
				{Name: "id", Type: "string"},
				{Name: "status", Type: "string"},
			},
		}},
	}
	resource := APILifecycleResource(ctx, ctx.Operations[0], "Create and read a widget.", "widget")
	for _, binding := range resource.RequestBindings {
		if binding.OperationRole == "read" {
			continue
		}
		if binding.Path == "id" || binding.Path == "status" || binding.RequestPath == "id" || binding.RequestPath == "status" {
			t.Fatalf("mutation request binding includes response-only field: %#v in %#v", binding, resource.RequestBindings)
		}
	}
	paths := responseStatePathSet(resource.ResponseBindings)
	if !paths["id"] || !paths["status"] {
		t.Fatalf("response bindings = %#v", resource.ResponseBindings)
	}
}

func TestAPILifecycleResourceAddsSourceSupportedAzureLROHints(t *testing.T) {
	ctx := promptcontext.Context{
		Sources: []promptcontext.SourceDocument{{ID: "azure", Kind: "openapi", URI: "azure.json"}},
		Operations: []promptcontext.OperationCandidate{{
			ID:              "Databases_CreateOrUpdate",
			SourceID:        "azure",
			OperationID:     "Databases_CreateOrUpdate",
			Verb:            "PUT",
			Path:            "/subscriptions/{subscriptionId}/databases/{databaseName}",
			RequestSchemaID: "request:Databases_CreateOrUpdate",
			Metadata: map[string]string{
				"x-ms-long-running-operation": "true",
				"parameters":                  `[{"name":"subscriptionId","in":"path","type":"string","required":true},{"name":"databaseName","in":"path","type":"string","required":true}]`,
			},
		}, {
			ID:               "Databases_Get",
			SourceID:         "azure",
			OperationID:      "Databases_Get",
			Verb:             "GET",
			Path:             "/subscriptions/{subscriptionId}/databases/{databaseName}",
			ResponseSchemaID: "response:Databases_Get",
			Metadata: map[string]string{
				"parameters": `[{"name":"subscriptionId","in":"path","type":"string","required":true},{"name":"databaseName","in":"path","type":"string","required":true}]`,
			},
		}},
		Schemas: []promptcontext.SchemaHint{{
			ID:     "request:Databases_CreateOrUpdate",
			Fields: []promptcontext.FieldHint{{Name: "name", Type: "string"}},
		}, {
			ID:     "response:Databases_Get",
			Fields: []promptcontext.FieldHint{{Name: "name", Type: "string"}},
		}},
	}
	resource := APILifecycleResource(ctx, ctx.Operations[0], "Create and read an Azure SQL database.", "database")
	if resource.RuntimeHints == nil || resource.RuntimeHints.Retry["max_attempts"] != 3 || resource.RuntimeHints.Waiter["until"] != "exists" {
		t.Fatalf("runtime hints = %#v", resource.RuntimeHints)
	}
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

func TestPromptContextFromAPISourcesPropagatesResponseSchemaAndCredentials(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "cloudflare.yaml")
	mustWriteAuthoringTestFile(t, sourcePath, []byte(`openapi: 3.0.0
info:
  title: Cloudflare Test
  version: v1
components:
  securitySchemes:
    api_token:
      type: http
      scheme: bearer
paths:
  /accounts/{account_id}/d1/database/{database_id}:
    get:
      operationId: getD1Database
      security:
        - api_token: []
      parameters:
        - name: account_id
          in: path
          required: true
          schema: {type: string}
        - name: database_id
          in: path
          required: true
          schema: {type: string}
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  result:
                    type: object
                    properties:
                      uuid:
                        type: string
                      name:
                        type: string
`))
	ctx, err := PromptContextFromAPISources(context.Background(), "Read a D1 database", []APISourceInput{{
		Kind: "openapi",
		ID:   "cloudflare",
		Path: sourcePath,
	}})
	if err != nil {
		t.Fatalf("PromptContextFromAPISources returned error: %v", err)
	}
	if len(ctx.Operations) != 1 || ctx.Operations[0].ResponseSchemaID == "" {
		t.Fatalf("operation response schema id missing: %#v", ctx.Operations)
	}
	if got := ctx.Operations[0].CredentialBindings; len(got) != 1 || got[0] != "cloudflare_api_token" {
		t.Fatalf("credential bindings = %#v", got)
	}
	if len(ctx.Credentials) != 1 || ctx.Credentials[0].Name != "cloudflare_api_token" {
		t.Fatalf("credentials = %#v", ctx.Credentials)
	}
	schema := schemaForID(ctx, ctx.Operations[0].ResponseSchemaID)
	if schema.ID == "" {
		t.Fatalf("response schema not found in %#v", ctx.Schemas)
	}
	fields := map[string]bool{}
	for _, field := range schema.Fields {
		fields[field.Name] = true
	}
	if !fields["result.name"] || !fields["result.uuid"] {
		t.Fatalf("response schema fields = %#v", schema.Fields)
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

func identityPathSet(attrs []project.IdentityAttribute) map[string]bool {
	out := map[string]bool{}
	for _, attr := range attrs {
		out[attr.Path] = true
	}
	return out
}

func requestBindingFor(bindings []project.RequestBinding, role, requestPath string) *project.RequestBinding {
	for i := range bindings {
		if bindings[i].OperationRole == role && bindings[i].RequestPath == requestPath {
			return &bindings[i]
		}
	}
	return nil
}

func responseStatePathSet(bindings []project.ResponseBinding) map[string]bool {
	out := map[string]bool{}
	for _, binding := range bindings {
		out[binding.StatePath] = true
	}
	return out
}

func schemaPathFor(paths []project.SchemaPath, path string) *project.SchemaPath {
	for i := range paths {
		if paths[i].Path == path {
			return &paths[i]
		}
	}
	return nil
}

func mustWriteAuthoringTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
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
