package tfconvert

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/internal/convertreport"
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

func TestConvertUsesProviderSchemaAsValidationEvidence(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	apiPath := filepath.Join(root, "api.yaml")
	schemaPath := filepath.Join(root, "provider-schema.json")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "aws_instance" "web" {
  name = "web"
}
`)
	writeFileForTest(t, apiPath, providerSchemaTestOpenAPI)
	writeFileForTest(t, schemaPath, testProviderSchemaJSON("1.0", `
    "registry.terraform.io/hashicorp/aws": {
      "provider": {"version": 0, "block": {"attributes": {}}},
      "resource_schemas": {
        "aws_instance": {"version": 1, "block": {"attributes": {
          "name": {"required": true},
          "role": {"optional": true},
          "id": {"computed": true}
        }}}
      }
    }`))
	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir, OpenAPIs: []OpenAPIInput{{ID: "aws", Path: apiPath}},
		ProviderSchemas: []ProviderSchemaInput{{ID: "aws", Path: schemaPath}},
		Action:          "create", OutDir: filepath.Join(root, "out"),
	})
	if err != nil {
		t.Fatalf("conversion with provider evidence failed: %v: %#v", err, result.Diagnostics)
	}
	var artifact conversionArtifact
	if err := json.Unmarshal([]byte(readFileForTest(t, result.ConversionPath)), &artifact); err != nil {
		t.Fatalf("decode conversion: %v", err)
	}
	if len(artifact.ProviderSchemas) != 1 || artifact.ProviderSchemas[0].ID != "aws" || artifact.ProviderSchemas[0].Source != filepath.Base(schemaPath) || len(artifact.ProviderSchemas[0].ResourceTypes) != 1 {
		t.Fatalf("provider schema evidence = %#v", artifact.ProviderSchemas)
	}
	review := readFileForTest(t, result.ReviewPath)
	if !strings.Contains(review, "Offline Provider Schema Evidence") || !strings.Contains(review, "validation evidence only, not an API operation contract") {
		t.Fatalf("review lacks provider evidence boundary:\n%s", review)
	}
	var manifest convertreport.Manifest
	if err := json.Unmarshal([]byte(readFileForTest(t, result.ManifestPath)), &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	foundInput, foundCoverage := false, false
	for _, input := range manifest.Inputs {
		if input.Kind == "terraform-provider-schema" && input.ID == "aws" && input.Path == filepath.Base(schemaPath) && len(input.SHA256) == 64 {
			foundInput = true
		}
	}
	for _, item := range manifest.Coverage.Items {
		if item.Kind == "provider-schema" && item.ID == "aws" && item.Disposition == "converted" {
			foundCoverage = true
		}
	}
	if !foundInput || !foundCoverage {
		t.Fatalf("provider schema manifest evidence missing: %#v", manifest)
	}
}

func TestConvertDiagnosesProviderConfigurationMismatch(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	apiPath := filepath.Join(root, "api.yaml")
	schemaPath := filepath.Join(root, "provider-schema.json")
	writeFileForTest(t, filepath.Join(configDir, "main.tf"), `
resource "aws_instance" "web" {
  id      = "must-not-be-configured"
  mystery = true
}
`)
	writeFileForTest(t, apiPath, providerSchemaTestOpenAPI)
	writeFileForTest(t, schemaPath, testProviderSchemaJSON("1.0", `
    "aws": {"resource_schemas": {
      "aws_instance": {"block": {"attributes": {
        "name": {"required": true},
        "id": {"computed": true}
      }}}
    }}`))
	result, err := Convert(context.Background(), Options{
		ConfigDir: configDir, OpenAPIs: []OpenAPIInput{{ID: "aws", Path: apiPath}},
		ProviderSchemas: []ProviderSchemaInput{{ID: "aws", Path: schemaPath}},
		Action:          "create", OutDir: filepath.Join(root, "out"), Mode: convertreport.ModePartial,
	})
	if err != nil {
		t.Fatalf("partial conversion failed: %v", err)
	}
	for _, code := range []string{
		"provider_schema.attribute_unknown",
		"provider_schema.computed_only_configured",
		"provider_schema.required_attribute_missing",
	} {
		if !hasDiagnostic(result.Diagnostics, code) {
			t.Fatalf("diagnostics missing %s: %#v", code, result.Diagnostics)
		}
	}
}

const providerSchemaTestOpenAPI = `openapi: 3.0.0
info: {title: Instances, version: v1}
paths:
  /instances:
    post:
      operationId: createAwsInstance
      responses:
        "200": {description: ok}
`

func testProviderSchemaJSON(format, providers string) string {
	return `{"format_version":` + quoteJSON(format) + `,"provider_schemas":{` + providers + `}}`
}

func quoteJSON(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
