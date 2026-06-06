//go:build udon

package udon

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenUdon/uws/uws1"
	"github.com/genelet/udon/pkg/credentials"
	"github.com/genelet/udon/pkg/uwsprofile"
)

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
