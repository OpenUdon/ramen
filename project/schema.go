package project

import (
	"bytes"
	_ "embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const projectSchemaResource = "https://github.com/OpenUdon/ramen/project/schemas/project.v1.json"

//go:embed schemas/project.v1.json
var embeddedProjectSchema []byte

var (
	projectSchemaOnce sync.Once
	projectSchema     *jsonschema.Schema
	projectSchemaErr  error
)

func validateProfileDocument(data []byte) error {
	schema, err := compiledProjectSchema()
	if err != nil {
		return err
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode Ramen project profile JSON: %w", err)
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate Ramen project profile: %w", err)
	}
	return nil
}

func compiledProjectSchema() (*jsonschema.Schema, error) {
	projectSchemaOnce.Do(func() {
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(embeddedProjectSchema))
		if err != nil {
			projectSchemaErr = err
			return
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(projectSchemaResource, document); err != nil {
			projectSchemaErr = err
			return
		}
		projectSchema, projectSchemaErr = compiler.Compile(projectSchemaResource)
	})
	if projectSchemaErr != nil {
		return nil, fmt.Errorf("compile embedded Ramen project schema: %w", projectSchemaErr)
	}
	return projectSchema, nil
}
