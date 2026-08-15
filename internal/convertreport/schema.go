package convertreport

import (
	"bytes"
	_ "embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	manifestSchemaResource    = "https://github.com/OpenUdon/ramen/internal/convertreport/schemas/manifest.v1.json"
	diagnosticsSchemaResource = "https://github.com/OpenUdon/ramen/internal/convertreport/schemas/diagnostics.v1.json"
)

//go:embed schemas/manifest.v1.json
var embeddedManifestSchema []byte

//go:embed schemas/diagnostics.v1.json
var embeddedDiagnosticsSchema []byte

var (
	manifestOnce      sync.Once
	manifestSchema    *jsonschema.Schema
	manifestErr       error
	diagnosticsOnce   sync.Once
	diagnosticsSchema *jsonschema.Schema
	diagnosticsErr    error
)

func ValidateManifest(data []byte) error {
	schema, err := compiledSchema(&manifestOnce, &manifestSchema, &manifestErr, manifestSchemaResource, embeddedManifestSchema)
	if err != nil {
		return fmt.Errorf("compile embedded conversion manifest schema: %w", err)
	}
	return validate(schema, data, "conversion manifest")
}

func ValidateDiagnostics(data []byte) error {
	schema, err := compiledSchema(&diagnosticsOnce, &diagnosticsSchema, &diagnosticsErr, diagnosticsSchemaResource, embeddedDiagnosticsSchema)
	if err != nil {
		return fmt.Errorf("compile embedded conversion diagnostics schema: %w", err)
	}
	return validate(schema, data, "conversion diagnostics")
}

func compiledSchema(once *sync.Once, target **jsonschema.Schema, targetErr *error, resource string, data []byte) (*jsonschema.Schema, error) {
	once.Do(func() {
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			*targetErr = err
			return
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(resource, document); err != nil {
			*targetErr = err
			return
		}
		*target, *targetErr = compiler.Compile(resource)
	})
	return *target, *targetErr
}

func validate(schema *jsonschema.Schema, data []byte, label string) error {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode %s JSON: %w", label, err)
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate %s: %w", label, err)
	}
	return nil
}
