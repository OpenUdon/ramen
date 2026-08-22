package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveProfileVariablesPrecedenceAndRedaction(t *testing.T) {
	root := t.TempDir()
	valuesPath := filepath.Join(root, "values.json")
	if err := os.WriteFile(valuesPath, []byte(`{"role_name":"from-file","token":"from-file-secret"}`), 0o644); err != nil {
		t.Fatalf("write values: %v", err)
	}
	profile := Profile{
		Version: Version,
		Variables: []Variable{
			{Name: "role_name", Type: "string", Default: "from-default"},
			{Name: "token", Type: "string", Sensitive: true},
		},
		Resources: []Resource{{
			Address: "example.one",
			Kind:    "resource",
			Type:    "example",
			Attributes: map[string]any{
				"name":  "${var.role_name}",
				"token": "${var.token}",
				"label": "role-${var.role_name}",
			},
			Operations: map[string]OperationRole{"create": {OperationID: "create-${var.role_name}", UWSOperationRef: "uws-${var.role_name}"}},
		}},
	}
	resolved, inputs, diagnostics := ResolveProfile(profile, root, ValuesOptions{VarFiles: []string{"values.json"}, Vars: []string{"role_name=from-cli"}})
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	attrs := resolved.Resources[0].Attributes
	if attrs["name"] != "from-cli" || attrs["token"] != "${redacted}" || attrs["label"] != "role-from-cli" {
		t.Fatalf("resolved attrs = %#v", attrs)
	}
	if got := resolved.Resources[0].Operations["create"].OperationID; got != "create-from-cli" {
		t.Fatalf("operation id = %q", got)
	}
	if got := resolved.Resources[0].Operations["create"].UWSOperationRef; got != "uws-from-cli" {
		t.Fatalf("UWS operation ref = %q", got)
	}
	if inputs.Version != InputsVersion || inputs.Digest == "" || len(inputs.Values) != 2 {
		t.Fatalf("inputs = %#v", inputs)
	}
	for _, value := range inputs.Values {
		if value.Name == "token" {
			if !value.Sensitive || value.Value != "${redacted}" || value.Digest == "" {
				t.Fatalf("sensitive input not redacted/digested: %#v", value)
			}
			if strings.Contains(value.Digest, "from-file-secret") {
				t.Fatalf("digest leaked secret: %#v", value)
			}
		}
	}
}

func TestResolveProfileVariableDiagnostics(t *testing.T) {
	_, _, diagnostics := ResolveProfile(Profile{
		Version: Version,
		Variables: []Variable{
			{Name: "count", Type: "number"},
			{Name: "count", Type: "number"},
			{Name: "bad-name", Type: "string"},
			{Name: "bad_default", Type: "number", Default: "wrong"},
		},
		Resources: []Resource{{
			Address:    "example.one",
			Kind:       "resource",
			Type:       "example",
			Attributes: map[string]any{"name": "${var.missing}"},
			Operations: map[string]OperationRole{"create": {OperationID: "${var.missing_op}"}},
		}},
	}, "", ValuesOptions{Vars: []string{"count=not-number", "extra=value"}})
	if len(diagnostics) < 8 {
		t.Fatalf("diagnostics = %#v, want required/type/unknown/reference diagnostics", diagnostics)
	}
	codes := map[string]bool{}
	paths := map[string]bool{}
	for _, diag := range diagnostics {
		codes[diag.Code] = true
		paths[diag.Path] = true
	}
	for _, code := range []string{"values.type_mismatch", "values.unknown_variable", "values.required", "values.reference_unknown", "values.name_invalid", "values.duplicate_variable"} {
		if !codes[code] {
			t.Fatalf("missing diagnostic %s in %#v", code, diagnostics)
		}
	}
	if !paths["example.one"] {
		t.Fatalf("missing resource path in diagnostics: %#v", diagnostics)
	}
}
