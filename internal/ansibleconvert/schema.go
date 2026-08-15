package ansibleconvert

import (
	"bytes"
	_ "embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	argspecSchemaResource    = "https://github.com/OpenUdon/ramen/internal/ansibleconvert/schemas/ansible.1.0.json"
	moduleCallSchemaResource = "https://github.com/OpenUdon/ramen/internal/ansibleconvert/schemas/ansible-module-call.1.0.json"
)

//go:embed schemas/ansible.1.0.json
var embeddedArgspecSchema []byte

//go:embed schemas/ansible-module-call.1.0.json
var embeddedModuleCallSchema []byte

var (
	argspecSchemaOnce sync.Once
	argspecSchema     *jsonschema.Schema
	argspecSchemaErr  error

	moduleCallSchemaOnce sync.Once
	moduleCallSchema     *jsonschema.Schema
	moduleCallSchemaErr  error
)

// ValidateArgspecDocument validates one JSON argspec document against Ramen's
// embedded ramen.ansible.1.0 schema.
func ValidateArgspecDocument(data []byte) error {
	schema, err := compiledArgspecSchema()
	if err != nil {
		return err
	}
	return validateJSONWithSchema(schema, data, "Ansible argspec")
}

// ValidateModuleCallDocument validates one JSON extension envelope against
// Ramen's embedded module-call schema.
func ValidateModuleCallDocument(data []byte) error {
	schema, err := compiledModuleCallSchema()
	if err != nil {
		return err
	}
	return validateJSONWithSchema(schema, data, "Ansible module call")
}

func compiledArgspecSchema() (*jsonschema.Schema, error) {
	argspecSchemaOnce.Do(func() {
		argspecSchema, argspecSchemaErr = compileEmbeddedSchema(argspecSchemaResource, embeddedArgspecSchema)
	})
	if argspecSchemaErr != nil {
		return nil, fmt.Errorf("compile embedded Ansible argspec schema: %w", argspecSchemaErr)
	}
	return argspecSchema, nil
}

func compiledModuleCallSchema() (*jsonschema.Schema, error) {
	moduleCallSchemaOnce.Do(func() {
		moduleCallSchema, moduleCallSchemaErr = compileEmbeddedSchema(moduleCallSchemaResource, embeddedModuleCallSchema)
	})
	if moduleCallSchemaErr != nil {
		return nil, fmt.Errorf("compile embedded Ansible module-call schema: %w", moduleCallSchemaErr)
	}
	return moduleCallSchema, nil
}

func compileEmbeddedSchema(resource string, data []byte) (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(resource, document); err != nil {
		return nil, err
	}
	return compiler.Compile(resource)
}

func validateJSONWithSchema(schema *jsonschema.Schema, data []byte, label string) error {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode %s JSON: %w", label, err)
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate %s: %w", label, err)
	}
	return nil
}
