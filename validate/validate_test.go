package validate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenUdon/ramen/project"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestRunValidatesNativeProjectAndAPISourceOperations(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeValidateOpenAPI(t, root, "api.yaml", "createExample")
	projectPath := writeValidateProject(t, root, project.Profile{
		Version: project.Version,
		APISources: []project.APISource{{
			Kind: "openapi",
			ID:   "api",
			Path: sourcePath,
		}},
		Resources: []project.Resource{{
			Address: "example_resource.test",
			Kind:    "resource",
			Type:    "example_resource",
			Operations: map[string]project.OperationRole{
				"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"},
			},
			IdentityAttributes: []project.IdentityAttribute{{Name: "id", Path: "id"}},
		}},
	})

	result, err := Run(context.Background(), Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Valid || result.Summary.Diagnostics != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunRejectsUnknownRequestBindingEncoding(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeValidateOpenAPI(t, root, "api.yaml", "createExample")
	projectPath := writeValidateProject(t, root, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: sourcePath}},
		Resources: []project.Resource{{
			Address:    "example_resource.test",
			Kind:       "resource",
			Type:       "example_resource",
			Attributes: map[string]any{"name": "example"},
			Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"}},
			RequestBindings: []project.RequestBinding{{
				OperationRole: "create",
				OperationID:   "createExample",
				Path:          "name",
				RequestPath:   "name",
				Encoding:      "rot13",
			}},
		}},
	})

	result, err := Run(context.Background(), Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Valid || !hasValidateCode(result, "validate.binding_encoding_unknown") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunReportsUnknownOperation(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeValidateOpenAPI(t, root, "api.yaml", "createExample")
	projectPath := writeValidateProject(t, root, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: sourcePath}},
		Resources: []project.Resource{{
			Address:    "example_resource.test",
			Kind:       "resource",
			Type:       "example_resource",
			Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "missingOperation"}},
		}},
	})

	result, err := Run(context.Background(), Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Valid || !hasValidateCode(result, "validate.operation_unknown") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunReportsProjectStructuralDiagnostics(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeValidateOpenAPI(t, root, "api.yaml", "createExample")
	projectPath := writeValidateProject(t, root, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: sourcePath}},
		Redaction:  project.Redaction{Paths: []string{""}},
		Resources: []project.Resource{
			{
				Address:            "example_resource.a",
				Kind:               "resource",
				Type:               "example_resource",
				Dependencies:       []string{"example_resource.b", "example_resource.missing"},
				IdentityAttributes: []project.IdentityAttribute{{Name: "", Path: ""}},
				Operations:         map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"}},
			},
			{
				Address: "example_resource.b",
				Kind:    "resource",
				Type:    "example_resource",
				Dependencies: []string{
					"example_resource.a",
				},
				Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"}},
			},
		},
	})

	result, err := Run(context.Background(), Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	for _, code := range []string{"validate.dependency_missing", "validate.dependency_cycle", "validate.identity_invalid", "validate.redaction_invalid"} {
		if !hasValidateCode(result, code) {
			t.Fatalf("result missing %s: %#v", code, result.Diagnostics)
		}
	}
}

func TestRunTreatsWarningsAsErrorsInStrictMode(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeValidateOpenAPI(t, root, "api.yaml", "createExample")
	unusedPath := writeValidateOpenAPI(t, root, "unused.yaml", "unusedOperation")
	projectPath := writeValidateProject(t, root, project.Profile{
		Version: project.Version,
		APISources: []project.APISource{
			{Kind: "openapi", ID: "api", Path: sourcePath},
			{Kind: "openapi", ID: "unused", Path: unusedPath},
		},
		Resources: []project.Resource{{
			Address:    "example_resource.test",
			Kind:       "resource",
			Type:       "example_resource",
			Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"}},
		}},
	})

	result, err := Run(context.Background(), Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Valid || result.Summary.Warnings != 1 || !hasValidateCode(result, "validate.api_source_unused") {
		t.Fatalf("non-strict result = %#v", result)
	}
	result, err = Run(context.Background(), Options{ProjectPath: projectPath, Strict: true})
	if err != nil {
		t.Fatalf("Run strict returned error: %v", err)
	}
	if result.Valid || result.Summary.Errors != 1 || result.Diagnostics[0].Severity != "error" {
		t.Fatalf("strict result = %#v", result)
	}
}

func TestRunAllowsExternalTerraformDataSourceDependencies(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeValidateOpenAPI(t, root, "api.yaml", "createExample")
	projectPath := writeValidateProject(t, root, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: sourcePath}},
		Resources: []project.Resource{{
			Address:      "example_resource.test",
			Kind:         "resource",
			Type:         "example_resource",
			Dependencies: []string{"data.example_source.current"},
			Operations:   map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"}},
		}},
	})

	result, err := Run(context.Background(), Options{ProjectPath: projectPath, Strict: true})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Valid || hasValidateCode(result, "validate.dependency_missing") {
		t.Fatalf("data source dependency should validate: %#v", result)
	}
}

func TestRunValidatesMappingSchemaMetadata(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeValidateOpenAPI(t, root, "api.yaml", "createExample")
	projectPath := writeValidateProject(t, root, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: sourcePath}},
		Resources: []project.Resource{{
			Address:    "example_resource.test",
			Kind:       "resource",
			Type:       "example_resource",
			Attributes: map[string]any{"name": "example", "mode": "bad", "count": "wrong", "extra": true},
			Schema: []project.SchemaPath{
				{Path: "name", Type: "string", Required: true},
				{Path: "mode", Type: "string", EnumValues: []string{"good"}},
				{Path: "count", Type: "number"},
				{Path: "token", Type: "string", Required: true, Sensitive: true},
			},
			RequestBindings: []project.RequestBinding{
				{OperationRole: "create", Path: "name", RequestPath: "Name", Required: true},
				{OperationRole: "create", Path: "", RequestPath: "Broken"},
			},
			ResponseBindings: []project.ResponseBinding{
				{OperationRole: "read", ResponsePath: "secret", StatePath: "token", Sensitive: true},
			},
			RequiredOperations: []string{"create", "read"},
			Operations:         map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"}},
		}},
	})

	result, err := Run(context.Background(), Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	for _, code := range []string{
		"validate.attribute_required",
		"validate.attribute_unknown",
		"validate.binding_invalid",
		"validate.enum_invalid",
		"validate.operation_role_missing",
		"validate.type_invalid",
	} {
		if !hasValidateCode(result, code) {
			t.Fatalf("result missing %s: %#v", code, result.Diagnostics)
		}
	}
}

func TestRunAcceptsValidMappingSchemaMetadata(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeValidateOpenAPI(t, root, "api.yaml", "createExample")
	readPath := writeValidateOpenAPI(t, root, "read.yaml", "readExample")
	projectPath := writeValidateProject(t, root, project.Profile{
		Version: project.Version,
		APISources: []project.APISource{
			{Kind: "openapi", ID: "api", Path: sourcePath},
			{Kind: "openapi", ID: "read", Path: readPath},
		},
		Resources: []project.Resource{{
			Address: "example_resource.test",
			Kind:    "resource",
			Type:    "example_resource",
			Attributes: map[string]any{
				"name":  "example",
				"mode":  "good",
				"token": "redacted",
			},
			Schema: []project.SchemaPath{
				{Path: "name", Type: "string", Required: true},
				{Path: "mode", Type: "string", EnumValues: []string{"good"}},
				{Path: "token", Type: "string", Sensitive: true},
			},
			RequestBindings: []project.RequestBinding{{OperationRole: "create", Path: "name", RequestPath: "Name", Required: true}},
			ResponseBindings: []project.ResponseBinding{{
				OperationRole: "read",
				ResponsePath:  "secret",
				StatePath:     "token",
				Sensitive:     true,
			}},
			RequiredOperations: []string{"create", "read"},
			Operations: map[string]project.OperationRole{
				"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"},
				"read":   {SourceKind: "openapi", SourceID: "read", OperationID: "readExample"},
			},
			Redaction: project.Redaction{Paths: []string{"token"}},
		}},
	})

	result, err := Run(context.Background(), Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Valid || result.Summary.Diagnostics != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunAcceptsRequiredRequestBindingFromResponseDerivedIdentity(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeValidateOpenAPI(t, root, "api.yaml", "createExample")
	readPath := writeValidateOpenAPI(t, root, "read.yaml", "readExample")
	projectPath := writeValidateProject(t, root, project.Profile{
		Version: project.Version,
		APISources: []project.APISource{
			{Kind: "openapi", ID: "api", Path: sourcePath},
			{Kind: "openapi", ID: "read", Path: readPath},
		},
		Resources: []project.Resource{{
			Address:    "example_resource.test",
			Kind:       "resource",
			Type:       "example_resource",
			Attributes: map[string]any{"name": "example"},
			RequestBindings: []project.RequestBinding{
				{OperationRole: "create", Path: "name", RequestPath: "name", Required: true},
				{OperationRole: "read", Path: "id", RequestPath: "id", Required: true, Identity: true},
			},
			ResponseBindings: []project.ResponseBinding{{
				OperationRole:           "read",
				ResponsePath:            "result.id",
				StatePath:               "id",
				Identity:                true,
				ResponseDerivedIdentity: true,
			}},
			RequiredOperations: []string{"create", "read"},
			Operations: map[string]project.OperationRole{
				"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"},
				"read":   {SourceKind: "openapi", SourceID: "read", OperationID: "readExample"},
			},
		}},
	})

	result, err := Run(context.Background(), Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Valid || result.Summary.Diagnostics != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunValidatesCrossFieldMappingMetadata(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeValidateOpenAPI(t, root, "api.yaml", "createExample")
	projectPath := writeValidateProject(t, root, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: sourcePath}},
		Resources: []project.Resource{{
			Address: "example_resource.test",
			Kind:    "resource",
			Type:    "example_resource",
			Attributes: map[string]any{
				"name": "example",
			},
			Schema: []project.SchemaPath{{
				Path:       "name",
				Type:       "string",
				Updateable: true,
				Immutable:  true,
			}, {
				Path:       "renamable_id",
				Type:       "string",
				Identity:   true,
				Updateable: true,
			}},
			Normalizers: []project.Normalizer{{Path: "name", Kind: "custom_diff"}},
			RequestBindings: []project.RequestBinding{{
				OperationRole: "update",
				OperationID:   "updateExample",
				Path:          "name",
				RequestPath:   "Name",
			}},
			ResponseBindings: []project.ResponseBinding{{
				OperationRole: "create",
				OperationID:   "otherCreate",
				ResponsePath:  "id",
				StatePath:     "id",
			}},
			RuntimeHints: &project.RuntimeHints{
				Retry:  map[string]any{"max_attempts": 0},
				Waiter: map[string]any{"until": "missing", "max_attempts": 0},
				Settle: map[string]any{"before": "update", "duration": "0s", "interval": "nope", "read_expect": "missing"},
			},
			Operations: map[string]project.OperationRole{
				"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"},
			},
		}},
	})

	result, err := Run(context.Background(), Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	for _, code := range []string{
		"validate.binding_operation_missing",
		"validate.binding_operation_mismatch",
		"validate.identity_update_unsupported",
		"validate.normalizer_unknown",
		"validate.retry_invalid",
		"validate.schema_lifecycle_conflict",
		"validate.settle_invalid",
		"validate.settle_read_role_missing",
		"validate.waiter_invalid",
		"validate.waiter_read_role_missing",
	} {
		if !hasValidateCode(result, code) {
			t.Fatalf("result missing %s: %#v", code, result.Diagnostics)
		}
	}
}

func TestRunAcceptsRuntimeHintsWithReadRole(t *testing.T) {
	root := t.TempDir()
	createPath := writeValidateOpenAPI(t, root, "create.yaml", "createExample")
	readPath := writeValidateOpenAPI(t, root, "read.yaml", "readExample")
	projectPath := writeValidateProject(t, root, project.Profile{
		Version: project.Version,
		APISources: []project.APISource{
			{Kind: "openapi", ID: "create-api", Path: createPath},
			{Kind: "openapi", ID: "read-api", Path: readPath},
		},
		Resources: []project.Resource{{
			Address: "example_resource.test",
			Kind:    "resource",
			Type:    "example_resource",
			Attributes: map[string]any{
				"name": "example",
			},
			Schema:          []project.SchemaPath{{Path: "name", Type: "string", Required: true}},
			RequestBindings: []project.RequestBinding{{OperationRole: "create", OperationID: "createExample", Path: "name", RequestPath: "Name", Required: true}},
			ResponseBindings: []project.ResponseBinding{{
				OperationRole: "read",
				OperationID:   "readExample",
				ResponsePath:  "name",
				StatePath:     "name",
			}},
			RuntimeHints: &project.RuntimeHints{
				Retry:  map[string]any{"max_attempts": 2},
				Waiter: map[string]any{"until": "exists", "max_attempts": 3},
				Settle: map[string]any{"before": "delete", "duration": "10ms", "interval": "1ms", "read_expect": "exists"},
			},
			Operations: map[string]project.OperationRole{
				"create": {SourceKind: "openapi", SourceID: "create-api", OperationID: "createExample"},
				"read":   {SourceKind: "openapi", SourceID: "read-api", OperationID: "readExample"},
			},
		}},
	})

	result, err := Run(context.Background(), Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Valid || result.Summary.Diagnostics != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunReportsLoadAndMissingSourceErrors(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing.yaml")
	projectPath := writeValidateProject(t, root, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: missing}},
		Resources: []project.Resource{{
			Address:    "example_resource.test",
			Kind:       "resource",
			Type:       "example_resource",
			Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"}},
		}},
	})

	result, err := Run(context.Background(), Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Valid || !hasValidateCode(result, "validate.api_source_document_read") {
		t.Fatalf("result = %#v", result.Diagnostics)
	}

	badPath := filepath.Join(root, "bad.uws.json")
	if err := os.WriteFile(badPath, []byte(`{"uws":"1.4.0"`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = Run(context.Background(), Options{ProjectPath: badPath})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Valid || !hasValidateCode(result, "validate.project_load_error") {
		t.Fatalf("bad project result = %#v", result)
	}
}

func hasValidateCode(result *Result, code string) bool {
	for _, diag := range result.Diagnostics {
		if diag.Code == code {
			return true
		}
	}
	return false
}

func writeValidateProject(t *testing.T, dir string, profile project.Profile) string {
	t.Helper()
	path := filepath.Join(dir, project.DefaultJSON)
	doc := &uws1.Document{
		UWS: "1.4.0",
		Info: &uws1.Info{
			Title:   "validate_fixture",
			Version: "1.0.0",
		},
		Operations: []*uws1.Operation{{
			OperationID: "review",
			Request:     map[string]any{"x-test": true},
			Extensions:  map[string]any{uws1.ExtensionOperationProfile: "ramen-validate-test"},
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID: "main",
			Type:       uws1.WorkflowTypeSequence,
			Steps: []*uws1.Step{{
				StepID:       "review",
				OperationRef: "review",
			}},
		}},
		Extensions: map[string]any{project.ExtensionKey: profile},
	}
	data, err := uwsconvert.MarshalJSONIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeValidateOpenAPI(t *testing.T, dir, name, operationID string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	data := []byte(`openapi: 3.0.0
info:
  title: Validate Test
  version: v1
paths:
  /examples:
    post:
      operationId: ` + operationID + `
      responses:
        "200":
          description: ok
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
