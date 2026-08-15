package ansibleconvert

import (
	"reflect"
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
	extensions := map[string]any{uws1.ExtensionOperationProfile: ProfileName}
	err := SetOperationExtension(&extensions, &OperationAnsibleModule{
		Module:  "ansible.builtin.apt",
		Argspec: &ArgspecReference{SourceID: "builtin"},
	})
	if err == nil {
		t.Fatal("malformed argspec reference was accepted")
	}
}
