//go:build udon

package udon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenUdon/uws/uws1"
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
	doc := &uws1.Document{
		SourceDescriptions: []*uws1.SourceDescription{{
			Name: "azure",
			Type: uws1.SourceDescriptionTypeOpenAPI,
			URL:  "azure.json",
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
