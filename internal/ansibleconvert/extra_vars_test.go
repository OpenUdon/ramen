package ansibleconvert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExtraVarsLiteralFilesAndEqualDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extra.yml")
	if err := os.WriteFile(path, []byte("env: prod\nreplicas: 2\nfeatures: [a, b]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values, diags, valid := loadExtraVars([]string{"env=prod", "@" + path})
	if !valid || len(diags) != 0 {
		t.Fatalf("values=%#v diags=%#v valid=%v", values, diags, valid)
	}
	if values["env"] != "prod" || values["replicas"] != 2 || len(values["features"].([]any)) != 2 {
		t.Fatalf("values=%#v", values)
	}
}

func TestLoadExtraVarsRejectsConflictsSecretsAndDynamicValuesWithoutDisclosure(t *testing.T) {
	for name, inputs := range map[string][]string{
		"conflict": {"env=prod", "env=dev"},
		"secret":   {"api_token=do-not-disclose"},
		"dynamic":  {"env={{ lookup_value }}"},
	} {
		t.Run(name, func(t *testing.T) {
			_, diags, valid := loadExtraVars(inputs)
			if valid || len(diags) == 0 || !diags[0].StrictFailure {
				t.Fatalf("diags=%#v valid=%v", diags, valid)
			}
			message := diags[0].Message
			for _, secret := range []string{"do-not-disclose", "{{ lookup_value }}", "=prod", "=dev"} {
				if strings.Contains(message, secret) {
					t.Fatalf("diagnostic disclosed input value %q: %s", secret, message)
				}
			}
		})
	}
}

func TestLoadExtraVarsRejectsNestedSensitiveKeysWithoutDisclosure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "extra.yml")
	if err := os.WriteFile(path, []byte("config:\n  accounts:\n    - name: deploy\n      password: file-do-not-disclose\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, inputs := range map[string][]string{
		"inline_map":  {"config={password: inline-do-not-disclose}"},
		"inline_list": {"config=[{api_token: list-do-not-disclose}]"},
		"file":        {"@" + path},
	} {
		t.Run(name, func(t *testing.T) {
			_, diags, valid := loadExtraVars(inputs)
			if valid || len(diags) != 1 || diags[0].Code != CodeNoLogLiteral || !diags[0].StrictFailure {
				t.Fatalf("diagnostics=%#v valid=%v", diags, valid)
			}
			if !strings.Contains(diags[0].Message, "credential-shaped key") {
				t.Fatalf("diagnostic = %q", diags[0].Message)
			}
			for _, literal := range []string{"inline-do-not-disclose", "list-do-not-disclose", "file-do-not-disclose"} {
				if strings.Contains(diags[0].Message, literal) {
					t.Fatalf("diagnostic disclosed rejected literal %q: %s", literal, diags[0].Message)
				}
			}
		})
	}
}

func TestConvertExtraVarsOverrideOtherStaticSources(t *testing.T) {
	result, doc := runConvertWithOptions(t, "inventoryvars", Options{
		InventoryPaths: []string{filepath.Join("testdata", "inventoryvars", "inventory.yml")},
		ExtraVars:      []string{"env=extra"},
	})
	if result.StrictFailures != 0 {
		t.Fatalf("conversion diagnostics: %#v", result.Diagnostics)
	}
	if doc.Components == nil || doc.Components.Variables["env"] != "extra" {
		t.Fatalf("extra vars did not take highest precedence: %#v", doc.Components)
	}
	op := findOperation(doc, "global_precedence")
	body, _ := op.Request["body"].(map[string]any)
	if body["cmd"] != "$variables.env" {
		t.Fatalf("global precedence body = %#v", body)
	}
}

func TestInvalidExtraVarsSuppressAffectedPlayInPartialMode(t *testing.T) {
	outDir := t.TempDir()
	result, err := Convert(context.Background(), Options{
		PlaybookPath: filepath.Join("testdata", "nginx", "playbook.yml"),
		Argspecs: []ArgspecInput{
			{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
		},
		ExtraVars:         []string{"config={password: do-not-disclose}"},
		Mode:              "partial",
		IgnoreUnsupported: true,
		OutDir:            outDir,
	})
	if err != nil {
		t.Fatalf("Convert failed before producing diagnostics: %v", err)
	}
	if result.UWSPath != "" || result.HCLPath != "" || result.StrictFailures == 0 {
		t.Fatalf("invalid variable scope widened partial output: %#v", result)
	}
	for _, path := range []string{result.ReviewMD, result.ManifestPath, result.DiagnosticsJSON, result.DiagnosticsMD} {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(data), "do-not-disclose") {
			t.Fatalf("artifact %s disclosed rejected value:\n%s", path, data)
		}
	}
}
