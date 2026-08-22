//go:build udon

package udon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/uws/uws1"
	"github.com/genelet/udon/generator"
	"github.com/genelet/udon/pkg/credentials"
	"github.com/genelet/udon/pkg/uwsprofile"
)

func TestExecutorRejectsBrowserBeforeRuntimeInvocation(t *testing.T) {
	projected := false
	exec := Executor{OutputProjector: func(context.Context, executor.Request, string) (executor.Result, error) {
		projected = true
		return executor.Result{}, nil
	}}
	action := executor.Action{
		Address: "example.browser", Type: "example_browser", Action: "read",
		Mapping: executor.ActionMapping{SourceKind: "browser-profile", OperationID: "read_status"},
	}
	requirement := executor.RequirementsForBrowser(executor.RequirementsForAction(action), executor.BrowserRequirements{NamedSession: true})
	_, err := exec.Execute(context.Background(), executor.Request{Action: action, Capabilities: requirement})
	if err == nil || !strings.Contains(err.Error(), "unsupported protocol") || projected {
		t.Fatalf("browser request reached Udon runtime: err=%v projected=%t", err, projected)
	}
}

func TestEnsureUdonRequestSectionSchemasAddsQueryAPIParameter(t *testing.T) {
	doc := &uws1.Document{
		Operations: []*uws1.Operation{{
			OperationID:       "delete_database",
			SourceDescription: "azure",
			SourceOperationID: "Databases_Delete",
			Request: map[string]any{
				"path": map[string]any{
					"subscriptionId":    "sub",
					"resourceGroupName": "SQL",
					"serverName":        "greetingland-sql-server",
					"databaseName":      "ramen",
				},
				"query": map[string]any{
					"api-version": "2023-08-01",
				},
			},
		}},
	}

	ensureUdonExecutionHints(doc, "")

	cfg, ok, err := uwsprofile.ReadOperationConfigExtension(doc.Operations[0].Extensions)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || cfg == nil {
		t.Fatalf("x-udon-config was not written")
	}
	if cfg.QueryPars == nil || cfg.QueryPars.Properties["api-version"] == nil {
		t.Fatalf("query api-version schema missing: %#v", cfg.QueryPars)
	}
	for _, key := range []string{"subscriptionId", "resourceGroupName", "serverName", "databaseName"} {
		if cfg.PathPars == nil || cfg.PathPars.Properties[key] == nil {
			t.Fatalf("path %s schema missing: %#v", key, cfg.PathPars)
		}
	}
}

func TestExecutorCredentialResolverUsesBindingOverride(t *testing.T) {
	calls := 0
	exec := Executor{
		CredentialResolvers: map[string]func(context.Context) (string, error){
			"azure_auth": func(context.Context) (string, error) {
				calls++
				return "fresh-token", nil
			},
		},
	}

	got, err := exec.credentialResolver().ResolveCredential(context.Background(), credentials.Request{Binding: "azure_auth"})
	if err != nil {
		t.Fatalf("ResolveCredential error: %v", err)
	}
	if got != "fresh-token" || calls != 1 {
		t.Fatalf("credential override got token=%q calls=%d", got, calls)
	}
}

func TestExecutorCredentialResolverFallsBackToEnv(t *testing.T) {
	t.Setenv("UDON_CREDENTIAL_OTHER_AUTH", "env-token")
	exec := Executor{
		CredentialResolvers: map[string]func(context.Context) (string, error){
			"azure_auth": func(context.Context) (string, error) {
				return "fresh-token", nil
			},
		},
	}

	got, err := exec.credentialResolver().ResolveCredential(context.Background(), credentials.Request{Binding: "other_auth"})
	if err != nil {
		t.Fatalf("ResolveCredential error: %v", err)
	}
	if got != "env-token" {
		t.Fatalf("fallback credential = %q, want env-token", got)
	}
}

func TestEnsureUdonRequestBodySchemaAddsPayloadParameters(t *testing.T) {
	doc := &uws1.Document{
		Operations: []*uws1.Operation{{
			OperationID:       "create_database",
			SourceDescription: "azure",
			SourceOperationID: "Databases_CreateOrUpdate",
			Request: map[string]any{
				"body": map[string]any{
					"location": "eastus",
					"sku": map[string]any{
						"name": "Basic",
						"tier": "Basic",
					},
				},
				"x-ramen-apply": map[string]any{
					"action":  "put",
					"address": "resource.sql_database_ramen_m27",
				},
			},
		}},
	}

	ensureUdonExecutionHints(doc, "")

	cfg, ok, err := uwsprofile.ReadOperationConfigExtension(doc.Operations[0].Extensions)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || cfg == nil {
		t.Fatalf("x-udon-config was not written")
	}
	if cfg.PayloadPars == nil {
		t.Fatalf("payload schema missing")
	}
	if cfg.PayloadRequired == nil || !*cfg.PayloadRequired {
		t.Fatalf("payload required flag missing: %#v", cfg.PayloadRequired)
	}
	if got := cfg.PayloadPars.Properties["location"]; got == nil || got.Type != "string" {
		t.Fatalf("location schema = %#v", got)
	}
	sku := cfg.PayloadPars.Properties["sku"]
	if sku == nil || sku.Type != "object" {
		t.Fatalf("sku schema = %#v", sku)
	}
	for _, key := range []string{"name", "tier"} {
		if got := sku.Properties[key]; got == nil || got.Type != "string" {
			t.Fatalf("sku.%s schema = %#v", key, got)
		}
	}
	if cfg.ResponseBody == nil || cfg.ResponseBody.Type != "object" {
		t.Fatalf("response body override missing: %#v", cfg.ResponseBody)
	}
}

func TestEnsureUdonExecutionHintsAllowsDiscoveryUploadOptIn(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "storage.discovery.json"), []byte(`{
  "name": "storage",
  "version": "v1",
  "baseUrl": "https://storage.googleapis.com/storage/v1/",
  "parameters": {
    "uploadType": {"type": "string", "location": "query"}
  },
  "resources": {
    "objects": {
      "methods": {
        "insert": {
          "id": "storage.objects.insert",
          "path": "b/{bucket}/o",
          "httpMethod": "POST",
          "parameters": {
            "bucket": {"type": "string", "location": "path", "required": true},
            "name": {"type": "string", "location": "query"}
          },
          "request": {"$ref": "Object"},
          "response": {"$ref": "Object"},
          "mediaUpload": {
            "protocols": {
              "simple": {"multipart": true, "path": "/upload/storage/v1/b/{bucket}/o"}
            }
          }
        }
      }
    }
  },
  "schemas": {
    "Object": {
      "type": "object",
      "properties": {
        "name": {"type": "string"},
        "contentType": {"type": "string"},
        "metadata": {"type": "object"}
      }
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := &uws1.Document{
		UWS:  "1.4.0",
		Info: &uws1.Info{Title: "Storage Upload", Version: "1.0.0"},
		SourceDescriptions: []*uws1.SourceDescription{{
			Name: "storage",
			Type: uws1.SourceDescriptionTypeGoogleDiscovery,
			URL:  "storage.discovery.json",
		}},
		Operations: []*uws1.Operation{{
			OperationID:       "create_object",
			SourceDescription: "storage",
			SourceOperationID: "storage.objects.insert",
			Request: map[string]any{
				"path":  map[string]any{"bucket": "example-bucket"},
				"query": map[string]any{"name": "note.txt", "uploadType": "multipart"},
				"body": map[string]any{
					"metadata": map[string]any{"name": "note.txt", "contentType": "text/plain"},
					"content":  "hello",
				},
			},
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID: "main",
			Type:       uws1.WorkflowTypeSequence,
			Steps:      []*uws1.Step{{StepID: "create_object", OperationRef: "create_object"}},
		}},
	}
	ensureUdonExecutionHints(doc, dir)
	plan, err := generator.NewRuntimePlanFromUWSDocument(doc, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ExecCache().Operations) != 1 {
		t.Fatalf("operations = %d, want 1", len(plan.ExecCache().Operations))
	}
	op := plan.ExecCache().Operations[0]
	if op.Path != "/upload/storage/v1/b/{bucket}/o" {
		t.Fatalf("path = %q, want upload path", op.Path)
	}
	if op.RequestMediaType != "multipart/related" {
		t.Fatalf("request media type = %q, want multipart/related", op.RequestMediaType)
	}
	if op.Discovery == nil || !op.Discovery.UseUpload {
		t.Fatalf("discovery config = %#v, want useUpload", op.Discovery)
	}
}

func TestEnsureUdonExecutionHintsAddsAzureAuthRequirement(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "azure.json")
	if err := os.WriteFile(sourcePath, []byte(`{
  "swagger": "2.0",
  "securityDefinitions": {
    "azure_auth": {
      "type": "oauth2",
      "flow": "implicit",
      "authorizationUrl": "https://login.microsoftonline.com/common/oauth2/authorize",
      "scopes": {"user_impersonation": "impersonate your user account"}
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	assertAzureAuthRequirementFromSource(t, dir, "azure.json")
}

func TestEnsureUdonExecutionHintsAddsAzureAuthRequirementFromOpenAPI3(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "azure-oas3.json")
	if err := os.WriteFile(sourcePath, []byte(`{
  "openapi": "3.0.0",
  "components": {
    "securitySchemes": {
      "azure_auth": {
        "type": "http",
        "scheme": "bearer"
      }
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	assertAzureAuthRequirementFromSource(t, dir, "azure-oas3.json")
}

func assertAzureAuthRequirementFromSource(t *testing.T, dir, sourceURL string) {
	t.Helper()
	doc := &uws1.Document{
		SourceDescriptions: []*uws1.SourceDescription{{
			Name: "azure",
			Type: uws1.SourceDescriptionTypeOpenAPI,
			URL:  sourceURL,
		}},
		Operations: []*uws1.Operation{{
			OperationID:       "delete_database",
			SourceDescription: "azure",
			SourceOperationID: "Databases_Delete",
		}},
	}

	ensureUdonExecutionHints(doc, dir)

	cfg, ok, err := uwsprofile.ReadOperationConfigExtension(doc.Operations[0].Extensions)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || cfg == nil || len(cfg.Security) != 1 {
		t.Fatalf("security config missing: ok=%t cfg=%#v", ok, cfg)
	}
	got := cfg.Security[0]
	if got.Name != "azure_auth" || got.Binding != "azure_auth" || len(got.Scopes) != 1 || got.Scopes[0] != "user_impersonation" {
		t.Fatalf("security config = %#v", got)
	}
}

func TestEnsureUdonExecutionHintsAcceptsLongRunningResponseShapeForCreateOrUpdate(t *testing.T) {
	doc := &uws1.Document{
		Operations: []*uws1.Operation{{
			OperationID:       "create_database",
			SourceDescription: "azure",
			SourceOperationID: "Databases_CreateOrUpdate",
			Request: map[string]any{
				"path": map[string]any{
					"subscriptionId":    "sub",
					"resourceGroupName": "SQL",
					"serverName":        "greetingland-sql-server",
					"databaseName":      "ramen",
				},
				"query": map[string]any{
					"api-version": "2023-08-01",
				},
				"body": map[string]any{
					"location": "eastus",
					"sku":      map[string]any{"name": "Basic", "tier": "Basic"},
				},
				"x-ramen-apply": map[string]any{
					"action": "put",
				},
			},
		}},
	}

	ensureUdonExecutionHints(doc, "")

	cfg, ok, err := uwsprofile.ReadOperationConfigExtension(doc.Operations[0].Extensions)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || cfg == nil {
		t.Fatalf("x-udon-config was not written")
	}
	if cfg.PathPars == nil || cfg.PathPars.Properties["subscriptionId"] == nil || cfg.PathPars.Properties["databaseName"] == nil {
		t.Fatalf("path schema missing: %#v", cfg.PathPars)
	}
	if cfg.QueryPars == nil || cfg.QueryPars.Properties["api-version"] == nil {
		t.Fatalf("query schema missing: %#v", cfg.QueryPars)
	}
	if cfg.PayloadPars == nil {
		t.Fatalf("payload schema missing: %#v", cfg.PayloadPars)
	}
	if cfg.ResponseBody == nil || cfg.ResponseBody.Type != "object" {
		t.Fatalf("response body override missing: %#v", cfg.ResponseBody)
	}
}
