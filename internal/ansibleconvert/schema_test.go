package ansibleconvert

import (
	"os"
	"testing"
)

const validArgspecDocument = `{
  "argspec": "ramen.ansible.1.0",
  "collection": "ansible.builtin",
  "modules": {
    "ansible.builtin.apt": {
      "parameters": {
        "name": {"type": "str", "required": true}
      }
    }
  }
}`

func TestEmbeddedAnsibleSchemasValidateOutsideRepository(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if err := ValidateArgspecDocument([]byte(validArgspecDocument)); err != nil {
		t.Fatalf("embedded argspec validation failed outside repository: %v", err)
	}
	if err := ValidateModuleCallDocument([]byte(`{
  "x-ramen-ansible-module": {
    "module": "ansible.builtin.apt",
    "argspec": {
      "sourceId": "builtin",
      "url": "./ansible-builtin.argspec.json",
      "collection": "ansible.builtin"
    }
  }
}`)); err != nil {
		t.Fatalf("embedded module-call validation failed outside repository: %v", err)
	}
}

func TestArgspecSchemaRejectsInvalidDocuments(t *testing.T) {
	tests := map[string]string{
		"retired discriminator": `{"argspec":"uws.ansible.1.0","collection":"ansible.builtin","modules":{"ansible.builtin.apt":{"parameters":{}}}}`,
		"unknown field":         `{"argspec":"ramen.ansible.1.0","collection":"ansible.builtin","modules":{"ansible.builtin.apt":{"parameters":{},"unknown":true}}}`,
		"missing modules":       `{"argspec":"ramen.ansible.1.0","collection":"ansible.builtin"}`,
		"invalid FQCN":          `{"argspec":"ramen.ansible.1.0","collection":"ansible.builtin","modules":{"apt":{"parameters":{}}}}`,
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateArgspecDocument([]byte(document)); err == nil {
				t.Fatalf("invalid argspec document was accepted: %s", document)
			}
		})
	}
}

func TestModuleCallSchemaRejectsInvalidDocuments(t *testing.T) {
	tests := map[string]string{
		"retired extension": `{"x-uws-ansible-module":{"module":"ansible.builtin.apt"}}`,
		"unknown field":     `{"x-ramen-ansible-module":{"module":"ansible.builtin.apt","connection":"ssh"}}`,
		"missing module":    `{"x-ramen-ansible-module":{}}`,
		"invalid FQCN":      `{"x-ramen-ansible-module":{"module":"apt"}}`,
		"empty reference":   `{"x-ramen-ansible-module":{"module":"ansible.builtin.apt","argspec":{}}}`,
		"bad source ID":     `{"x-ramen-ansible-module":{"module":"ansible.builtin.apt","argspec":{"sourceId":" ","url":"argspec.json","collection":"ansible.builtin"}}}`,
		"empty URL":         `{"x-ramen-ansible-module":{"module":"ansible.builtin.apt","argspec":{"sourceId":"builtin","url":"","collection":"ansible.builtin"}}}`,
		"bad collection":    `{"x-ramen-ansible-module":{"module":"ansible.builtin.apt","argspec":{"sourceId":"builtin","url":"argspec.json","collection":"builtin"}}}`,
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateModuleCallDocument([]byte(document)); err == nil {
				t.Fatalf("invalid module-call document was accepted: %s", document)
			}
		})
	}
}
