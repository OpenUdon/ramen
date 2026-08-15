package tfconvert

import (
	"bytes"
	_ "embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const terraformSchemaResource = "https://github.com/OpenUdon/ramen/internal/tfconvert/schemas/terraform-conversion.v1.json"

//go:embed schemas/terraform-conversion.v1.json
var embeddedTerraformSchema []byte

var (
	terraformSchemaOnce sync.Once
	terraformSchema     *jsonschema.Schema
	terraformSchemaErr  error
)

func validateTerraformMetadataDocument(data []byte) error {
	schema, err := compiledTerraformSchema()
	if err != nil {
		return err
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode Terraform conversion metadata JSON: %w", err)
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate Terraform conversion metadata: %w", err)
	}
	return nil
}

func compiledTerraformSchema() (*jsonschema.Schema, error) {
	terraformSchemaOnce.Do(func() {
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(embeddedTerraformSchema))
		if err != nil {
			terraformSchemaErr = err
			return
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(terraformSchemaResource, document); err != nil {
			terraformSchemaErr = err
			return
		}
		terraformSchema, terraformSchemaErr = compiler.Compile(terraformSchemaResource)
	})
	if terraformSchemaErr != nil {
		return nil, fmt.Errorf("compile embedded Terraform conversion schema: %w", terraformSchemaErr)
	}
	return terraformSchema, nil
}
