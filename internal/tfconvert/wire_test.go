package tfconvert

import (
	"strings"
	"testing"

	"github.com/OpenUdon/uws/uws1"
)

func TestTerraformWireConstants(t *testing.T) {
	if TerraformProvenanceVersion != "ramen.terraform.provenance.v1" || TerraformReviewTODOProfile != "ramen.terraform-review-todo.1.0" {
		t.Fatalf("unexpected Terraform identifiers: provenance=%q profile=%q", TerraformProvenanceVersion, TerraformReviewTODOProfile)
	}
	if RequestTerraformProvenance != "x-ramen-terraform" || RequestCredentialBindings != "x-ramen-credential-bindings" || RequestReviewTODO != "x-ramen-todo" {
		t.Fatalf("unexpected Terraform request keys: provenance=%q credentials=%q todo=%q", RequestTerraformProvenance, RequestCredentialBindings, RequestReviewTODO)
	}
}

func TestSetAndReadTerraformRequestMetadataPreservesStandardFields(t *testing.T) {
	request := map[string]any{"body": map[string]any{"name": "example"}}
	if err := SetTerraformRequestMetadata(&request, validTerraformRequestMetadata()); err != nil {
		t.Fatal(err)
	}
	if request["body"] == nil {
		t.Fatalf("standard request body was removed: %#v", request)
	}
	metadata, ok, err := ReadTerraformRequestMetadata(request)
	if err != nil || !ok {
		t.Fatalf("read metadata: ok=%t err=%v", ok, err)
	}
	if metadata.Provenance == nil || metadata.Provenance.Version != TerraformProvenanceVersion || metadata.Provenance.Object.Address != "example.test" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestTerraformOperationValidation(t *testing.T) {
	resolved := &uws1.Operation{
		OperationID:       "createExample",
		SourceDescription: "example",
		SourceOperationID: "createExample",
		Request:           map[string]any{"body": map[string]any{"name": "example"}},
	}
	if err := SetTerraformRequestMetadata(&resolved.Request, validTerraformRequestMetadata()); err != nil {
		t.Fatal(err)
	}
	if applicable, err := ValidateTerraformOperation(resolved); !applicable || err != nil {
		t.Fatalf("resolved operation: applicable=%t err=%v", applicable, err)
	}

	unresolved := &uws1.Operation{
		OperationID: "reviewExample",
		Request:     map[string]any{},
		Extensions:  map[string]any{uws1.ExtensionOperationProfile: TerraformReviewTODOProfile},
	}
	metadata := validTerraformRequestMetadata()
	metadata.TODO = "operation.unresolved"
	if err := SetTerraformRequestMetadata(&unresolved.Request, metadata); err != nil {
		t.Fatal(err)
	}
	if applicable, err := ValidateTerraformOperation(unresolved); !applicable || err != nil {
		t.Fatalf("unresolved operation: applicable=%t err=%v", applicable, err)
	}

	unrelated := &uws1.Operation{OperationID: "unrelated", Request: map[string]any{"body": map[string]any{}}}
	if applicable, err := ValidateTerraformOperation(unrelated); applicable || err != nil {
		t.Fatalf("unrelated operation: applicable=%t err=%v", applicable, err)
	}

	nativeWithCredentials := &uws1.Operation{
		OperationID: "nativeWithCredentials",
		Request: map[string]any{
			"body":                    map[string]any{"name": "example"},
			RequestCredentialBindings: []any{"example.default"},
		},
	}
	if applicable, err := ValidateTerraformOperation(nativeWithCredentials); applicable || err != nil {
		t.Fatalf("native credential operation: applicable=%t err=%v", applicable, err)
	}
}

func TestTerraformOperationRejectsLegacyAndInconsistentMetadata(t *testing.T) {
	validEnvelope := func(t *testing.T) map[string]any {
		t.Helper()
		request := map[string]any{}
		if err := SetTerraformRequestMetadata(&request, validTerraformRequestMetadata()); err != nil {
			t.Fatal(err)
		}
		return request
	}
	tests := map[string]*uws1.Operation{
		"retired profile": {
			OperationID: "review", Request: validEnvelope(t), Extensions: map[string]any{uws1.ExtensionOperationProfile: retiredTerraformReviewTODOProfile},
		},
		"unversioned provenance": {
			OperationID: "review", Request: map[string]any{RequestTerraformProvenance: map[string]any{"object": map[string]any{"address": "example.test", "kind": "resource", "type": "example", "name": "test"}, "attributes": map[string]any{}}},
		},
		"unknown Terraform key": {
			OperationID: "review", Request: map[string]any{RequestTerraformProvenance + "-future": true},
		},
		"resolved with todo profile": {
			OperationID: "create", SourceDescription: "example", SourceOperationID: "create", Request: validEnvelope(t), Extensions: map[string]any{uws1.ExtensionOperationProfile: TerraformReviewTODOProfile},
		},
		"unresolved without todo": {
			OperationID: "review", Request: validEnvelope(t), Extensions: map[string]any{uws1.ExtensionOperationProfile: TerraformReviewTODOProfile},
		},
		"unresolved without profile": {
			OperationID: "review", Request: validEnvelope(t),
		},
	}
	for name, operation := range tests {
		t.Run(name, func(t *testing.T) {
			applicable, err := ValidateTerraformOperation(operation)
			if !applicable || err == nil {
				t.Fatalf("invalid operation: applicable=%t err=%v operation=%#v", applicable, err, operation)
			}
		})
	}
}

func TestTerraformMetadataRejectsUnknownNestedField(t *testing.T) {
	request := map[string]any{
		RequestTerraformProvenance: map[string]any{
			"version":    TerraformProvenanceVersion,
			"object":     map[string]any{"address": "example.test", "kind": "resource", "type": "example", "name": "test"},
			"attributes": map[string]any{},
			"future":     true,
		},
	}
	_, ok, err := ReadTerraformRequestMetadata(request)
	if ok || err == nil || !strings.Contains(err.Error(), "validate Terraform conversion metadata") {
		t.Fatalf("unknown nested field: ok=%t err=%v", ok, err)
	}
}

func validTerraformRequestMetadata() TerraformRequestMetadata {
	return TerraformRequestMetadata{
		Provenance: &TerraformProvenance{
			Version: TerraformProvenanceVersion,
			Object:  TerraformObject{Address: "example.test", Kind: "resource", Type: "example", Name: "test"},
			Attributes: map[string]any{
				"name": "example",
			},
			IdentityAttributes: []TerraformIdentityAttribute{{Name: "id", TerraformPath: "name", RequestKeys: []string{"name"}, ResponsePaths: []string{"result.name"}, Required: true}},
		},
		CredentialBindings: []string{"example.default"},
	}
}
