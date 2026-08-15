package tfconvert

import (
	"os"
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
