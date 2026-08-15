package tfconvert

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
)

func TestLoadProviderSchemasSelectsOfflineSnapshot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "schemas.json")
	writeFileForTest(t, path, testProviderSchemaJSON("1.0", `
    "registry.terraform.io/hashicorp/aws": {
      "provider": {"version": 0, "block": {"attributes": {}}},
      "resource_schemas": {
        "aws_instance": {"version": 1, "block": {"attributes": {"name": {"optional": true}}}}
      },
      "data_source_schemas": {
        "aws_ami": {"version": 0, "block": {"attributes": {"id": {"computed": true}}}}
      }
    }`))
	state := conversionState{opts: Options{ProviderSchemas: []ProviderSchemaInput{{ID: "aws", Path: path}}}}
	state.loadProviderSchemas()
	if len(state.diagnostics) != 0 || len(state.providerSchemas) != 1 {
		t.Fatalf("provider schemas/diagnostics = %#v/%#v", state.providerSchemas, state.diagnostics)
	}
	doc := state.providerSchemas[0]
	if doc.ID != "aws" || doc.Address != "registry.terraform.io/hashicorp/aws" ||
		!slices.Equal(doc.ResourceTypes, []string{"aws_instance"}) || !slices.Equal(doc.DataTypes, []string{"aws_ami"}) {
		t.Fatalf("selected provider schema = %#v", doc)
	}
}

func TestLoadProviderSchemasFailsClosed(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		body  string
		code  string
		twice bool
	}{
		{name: "invalid json", id: "aws", body: `{`, code: "provider_schema.invalid_json"},
		{name: "unsupported format", id: "aws", body: testProviderSchemaJSON("2.0", `"aws": {"resource_schemas": {"aws_instance": {"block": {}}}}`), code: "provider_schema.unsupported_format"},
		{name: "provider not found", id: "google", body: testProviderSchemaJSON("1.0", `"registry.terraform.io/hashicorp/aws": {"resource_schemas": {"aws_instance": {"block": {}}}}`), code: "provider_schema.provider_not_found"},
		{name: "ambiguous provider", id: "aws", body: testProviderSchemaJSON("1.0", `
      "example.com/one/aws": {"resource_schemas": {"aws_instance": {"block": {}}}},
      "example.com/two/aws": {"resource_schemas": {"aws_instance": {"block": {}}}}`), code: "provider_schema.provider_ambiguous"},
		{name: "empty provider", id: "aws", body: testProviderSchemaJSON("1.0", `"aws": {}`), code: "provider_schema.empty"},
		{name: "duplicate id", id: "aws", body: testProviderSchemaJSON("1.0", `"aws": {"resource_schemas": {"aws_instance": {"block": {}}}}`), code: "provider_schema.duplicate_id", twice: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "schemas.json")
			writeFileForTest(t, path, tt.body)
			inputs := []ProviderSchemaInput{{ID: tt.id, Path: path}}
			if tt.twice {
				inputs = append(inputs, inputs[0])
			}
			state := conversionState{opts: Options{ProviderSchemas: inputs}}
			state.loadProviderSchemas()
			if !hasDiagnostic(state.diagnostics, tt.code) {
				t.Fatalf("diagnostics = %#v, want %s", state.diagnostics, tt.code)
			}
			for _, diagnostic := range state.diagnostics {
				if diagnostic.Code == tt.code && (!diagnostic.StrictFailure || diagnostic.ProviderSchemaID != tt.id) {
					t.Fatalf("diagnostic = %#v", diagnostic)
				}
			}
		})
	}
}

func testProviderSchemaJSON(format, providers string) string {
	return `{"format_version":` + quoteJSON(format) + `,"provider_schemas":{` + providers + `}}`
}

func quoteJSON(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
