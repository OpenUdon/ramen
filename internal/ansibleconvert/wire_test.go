package ansibleconvert

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/OpenUdon/uws/uws1"
)

func TestRamenAnsibleWireConstants(t *testing.T) {
	if ArgspecVersion != "ramen.ansible.1.0" || ProfileName != "ramen.ansible-module-call.1.0" {
		t.Fatalf("unexpected Ramen Ansible identifiers: argspec=%q profile=%q", ArgspecVersion, ProfileName)
	}
	if ExtensionAnsibleModule != "x-ramen-ansible-module" || ExtensionAnsibleProvenance != "x-ramen-ansible-provenance" {
		t.Fatalf("unexpected Ramen Ansible extensions: module=%q provenance=%q", ExtensionAnsibleModule, ExtensionAnsibleProvenance)
	}
}

func TestAnsibleWireStructsHaveHCLTags(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(OperationAnsibleModule{}),
		reflect.TypeOf(ArgspecReference{}),
	} {
		t.Run(typ.Name(), func(t *testing.T) {
			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				if field.IsExported() && field.Tag.Get("hcl") == "" {
					t.Fatalf("%s.%s must have an hcl tag", typ.Name(), field.Name)
				}
			}
		})
	}
}

func TestAnsibleModuleCallSchemaMatchesWireTypes(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(embeddedModuleCallSchema, &schema); err != nil {
		t.Fatal(err)
	}
	definitions, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("Ansible module-call schema has no $defs object")
	}
	tests := []struct {
		definition string
		goType     reflect.Type
	}{
		{definition: "operation-ansible-module", goType: reflect.TypeFor[OperationAnsibleModule]()},
		{definition: "argspec-reference", goType: reflect.TypeFor[ArgspecReference]()},
	}
	for _, test := range tests {
		t.Run(test.definition, func(t *testing.T) {
			node, ok := definitions[test.definition].(map[string]any)
			if !ok {
				t.Fatalf("schema definition %q is missing or not an object", test.definition)
			}
			properties, ok := node["properties"].(map[string]any)
			if !ok {
				t.Fatalf("schema definition %q has no properties", test.definition)
			}
			schemaNames := make([]string, 0, len(properties))
			for name := range properties {
				schemaNames = append(schemaNames, name)
			}
			slices.Sort(schemaNames)
			wireNames := make([]string, 0, test.goType.NumField())
			for i := range test.goType.NumField() {
				name := strings.Split(test.goType.Field(i).Tag.Get("json"), ",")[0]
				wireNames = append(wireNames, name)
			}
			slices.Sort(wireNames)
			if !slices.Equal(schemaNames, wireNames) {
				t.Fatalf("schema/type properties differ for %s: schema=%v type=%v", test.goType.Name(), schemaNames, wireNames)
			}
		})
	}
}

func TestReadSetAndValidateOperationExtension(t *testing.T) {
	extensions := map[string]any{uws1.ExtensionOperationProfile: ProfileName}
	err := SetOperationExtension(&extensions, &OperationAnsibleModule{
		Module: "ansible.builtin.apt",
		Argspec: &ArgspecReference{
			SourceID:   "builtin",
			URL:        "./ansible-builtin.argspec.json",
			Collection: "ansible.builtin",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateOperationExtensions(extensions); err != nil {
		t.Fatalf("validate operation extensions: %v", err)
	}
	got, ok, err := ReadOperationExtension(extensions)
	if err != nil || !ok {
		t.Fatalf("read operation extension: ok=%v err=%v", ok, err)
	}
	if got.Module != "ansible.builtin.apt" || got.Argspec == nil || got.Argspec.SourceID != "builtin" {
		t.Fatalf("decoded extension = %#v", got)
	}
}

func TestOperationExtensionsRejectRetiredIdentifiers(t *testing.T) {
	validModule := map[string]any{"module": "ansible.builtin.apt"}
	tests := map[string]map[string]any{
		"retired profile": {
			uws1.ExtensionOperationProfile: retiredProfileName,
			ExtensionAnsibleModule:         validModule,
		},
		"retired module extension": {
			uws1.ExtensionOperationProfile: ProfileName,
			retiredExtensionModule:         validModule,
		},
		"retired provenance extension": {
			uws1.ExtensionOperationProfile: ProfileName,
			ExtensionAnsibleModule:         validModule,
			retiredExtensionProvenance:     map[string]any{},
		},
	}
	for name, extensions := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateOperationExtensions(extensions); err == nil || !strings.Contains(err.Error(), "retired") {
				t.Fatalf("retired extensions error = %v", err)
			}
		})
	}
}

func TestOperationExtensionRejectsMalformedReference(t *testing.T) {
	tests := map[string]*ArgspecReference{
		"missing fields":        {SourceID: "builtin"},
		"whitespace URL":        {SourceID: "builtin", URL: " \t", Collection: "ansible.builtin"},
		"mismatched collection": {SourceID: "builtin", URL: "argspec.json", Collection: "community.general"},
	}
	for name, reference := range tests {
		t.Run(name, func(t *testing.T) {
			extensions := map[string]any{uws1.ExtensionOperationProfile: ProfileName}
			err := SetOperationExtension(&extensions, &OperationAnsibleModule{
				Module:  "ansible.builtin.apt",
				Argspec: reference,
			})
			if err == nil {
				t.Fatal("malformed argspec reference was accepted")
			}
		})
	}
}

func TestReadOperationExtensionRejectsMismatchedReferenceCollection(t *testing.T) {
	extensions := map[string]any{
		uws1.ExtensionOperationProfile: ProfileName,
		ExtensionAnsibleModule: map[string]any{
			"module": "ansible.builtin.apt",
			"argspec": map[string]any{
				"sourceId":   "builtin",
				"url":        "argspec.json",
				"collection": "community.general",
			},
		},
	}
	if _, ok, err := ReadOperationExtension(extensions); ok || err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched argspec reference: ok=%t err=%v", ok, err)
	}
}
