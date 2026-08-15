package tfconvert

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

const validTerraformMetadataDocument = `{
  "x-ramen-terraform": {
    "version": "ramen.terraform.provenance.v1",
    "object": {
      "address": "aws_iam_role.example",
      "kind": "resource",
      "type": "aws_iam_role",
      "name": "example"
    },
    "attributes": {
      "name": "example",
      "nested": {"enabled": true}
    },
    "identity_attributes": [{
      "name": "role_name",
      "terraform_path": "name",
      "request_keys": ["RoleName"],
      "response_paths": ["Role.RoleName"],
      "required": true
    }]
  },
  "x-ramen-credential-bindings": ["aws.default"]
}`

func TestTerraformSchemaFixedObjectsMatchWireTypes(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(embeddedTerraformSchema, &schema); err != nil {
		t.Fatal(err)
	}
	definitions, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("Terraform schema has no $defs object")
	}
	tests := []struct {
		name   string
		node   map[string]any
		goType reflect.Type
	}{
		{name: "metadata", node: schema, goType: reflect.TypeFor[TerraformRequestMetadata]()},
		{name: "terraform-provenance", node: terraformSchemaDefinition(t, definitions, "terraform-provenance"), goType: reflect.TypeFor[TerraformProvenance]()},
		{name: "terraform-object", node: terraformSchemaDefinition(t, definitions, "terraform-object"), goType: reflect.TypeFor[TerraformObject]()},
		{name: "identity-attribute", node: terraformSchemaDefinition(t, definitions, "identity-attribute"), goType: reflect.TypeFor[TerraformIdentityAttribute]()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertTerraformSchemaPropertiesMatchWireType(t, test.node, test.goType)
		})
	}
}

func terraformSchemaDefinition(t *testing.T, definitions map[string]any, name string) map[string]any {
	t.Helper()
	node, ok := definitions[name].(map[string]any)
	if !ok {
		t.Fatalf("Terraform schema definition %q is missing or not an object", name)
	}
	return node
}

func assertTerraformSchemaPropertiesMatchWireType(t *testing.T, node map[string]any, goType reflect.Type) {
	t.Helper()
	properties, ok := node["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema object has no properties")
	}
	if len(properties) != goType.NumField() {
		t.Fatalf("schema/type property count differs for %s: schema=%d type=%d", goType.Name(), len(properties), goType.NumField())
	}
	for i := range goType.NumField() {
		field := goType.Field(i)
		name := field.Tag.Get("json")
		if comma := strings.IndexByte(name, ','); comma >= 0 {
			name = name[:comma]
		}
		if _, ok := properties[name]; !ok {
			t.Errorf("schema object is missing %s.%s wire property %q", goType.Name(), field.Name, name)
		}
	}
}

func TestEmbeddedTerraformSchemaValidatesOutsideRepository(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if err := validateTerraformMetadataDocument([]byte(validTerraformMetadataDocument)); err != nil {
		t.Fatalf("embedded Terraform metadata validation failed outside repository: %v", err)
	}
}

func TestTerraformSchemaRejectsInvalidMetadata(t *testing.T) {
	tests := map[string]string{
		"missing provenance": `{}`,
		"unversioned provenance": `{
  "x-ramen-terraform":{"object":{"address":"example.test","kind":"resource","type":"example","name":"test"},"attributes":{}}
}`,
		"wrong version": `{
  "x-ramen-terraform":{"version":"ramen.terraform.v0","object":{"address":"example.test","kind":"resource","type":"example","name":"test"},"attributes":{}}
}`,
		"unknown provenance field": `{
  "x-ramen-terraform":{"version":"ramen.terraform.provenance.v1","object":{"address":"example.test","kind":"resource","type":"example","name":"test"},"attributes":{},"future":true}
}`,
		"unknown object field": `{
  "x-ramen-terraform":{"version":"ramen.terraform.provenance.v1","object":{"address":"example.test","kind":"resource","type":"example","name":"test","provider":"example"},"attributes":{}}
}`,
		"invalid kind": `{
  "x-ramen-terraform":{"version":"ramen.terraform.provenance.v1","object":{"address":"example.test","kind":"module","type":"example","name":"test"},"attributes":{}}
}`,
		"missing object name": `{
  "x-ramen-terraform":{"version":"ramen.terraform.provenance.v1","object":{"address":"example.test","kind":"resource","type":"example"},"attributes":{}}
}`,
		"malformed identity": `{
  "x-ramen-terraform":{"version":"ramen.terraform.provenance.v1","object":{"address":"example.test","kind":"resource","type":"example","name":"test"},"attributes":{},"identity_attributes":[{"name":"id"}]}
}`,
		"duplicate credentials": `{
  "x-ramen-terraform":{"version":"ramen.terraform.provenance.v1","object":{"address":"example.test","kind":"resource","type":"example","name":"test"},"attributes":{}},
  "x-ramen-credential-bindings":["example","example"]
}`,
		"empty todo": `{
  "x-ramen-terraform":{"version":"ramen.terraform.provenance.v1","object":{"address":"example.test","kind":"resource","type":"example","name":"test"},"attributes":{}},
  "x-ramen-todo":" "
}`,
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			if err := validateTerraformMetadataDocument([]byte(document)); err == nil {
				t.Fatalf("invalid Terraform metadata was accepted: %s", document)
			}
		})
	}
}
