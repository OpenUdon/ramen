package tfconvert

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OpenUdon/uws/uws1"
)

const (
	TerraformProvenanceVersion = "ramen.terraform.provenance.v1"
	TerraformReviewTODOProfile = "ramen.terraform-review-todo.1.0"

	RequestTerraformProvenance = "x-ramen-terraform"
	RequestCredentialBindings  = "x-ramen-credential-bindings"
	RequestReviewTODO          = "x-ramen-todo"

	retiredTerraformReviewTODOProfile = "ramen-review-todo"
)

// TerraformRequestMetadata is the Ramen-owned review metadata carried beside
// standard UWS request locations on operations emitted by static conversion.
type TerraformRequestMetadata struct {
	Provenance         *TerraformProvenance `json:"x-ramen-terraform"`
	CredentialBindings []string             `json:"x-ramen-credential-bindings,omitempty"`
	TODO               string               `json:"x-ramen-todo,omitempty"`
}

type TerraformProvenance struct {
	Version            string                       `json:"version"`
	Object             TerraformObject              `json:"object"`
	Attributes         map[string]any               `json:"attributes"`
	IdentityAttributes []TerraformIdentityAttribute `json:"identity_attributes,omitempty"`
}

type TerraformObject struct {
	Address string `json:"address"`
	Kind    string `json:"kind"`
	Type    string `json:"type"`
	Name    string `json:"name"`
}

type TerraformIdentityAttribute struct {
	Name          string   `json:"name"`
	TerraformPath string   `json:"terraform_path"`
	RequestKeys   []string `json:"request_keys,omitempty"`
	ResponsePaths []string `json:"response_paths,omitempty"`
	Required      bool     `json:"required,omitempty"`
}

// SetTerraformRequestMetadata validates and adds conversion-owned keys while
// retaining standard UWS request fields already present in dst.
func SetTerraformRequestMetadata(dst *map[string]any, metadata TerraformRequestMetadata) error {
	envelope, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal Terraform conversion metadata: %w", err)
	}
	if err := validateTerraformMetadataDocument(envelope); err != nil {
		return err
	}
	var generic map[string]any
	if err := json.Unmarshal(envelope, &generic); err != nil {
		return fmt.Errorf("decode Terraform conversion metadata: %w", err)
	}
	if *dst == nil {
		*dst = make(map[string]any)
	}
	for key, value := range generic {
		(*dst)[key] = value
	}
	return nil
}

// ReadTerraformRequestMetadata strictly reads conversion metadata without
// interpreting unrelated standard UWS request fields.
func ReadTerraformRequestMetadata(request map[string]any) (*TerraformRequestMetadata, bool, error) {
	envelope, ok := terraformMetadataEnvelope(request)
	if !ok {
		return nil, false, nil
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, false, fmt.Errorf("marshal Terraform conversion metadata: %w", err)
	}
	if err := validateTerraformMetadataDocument(data); err != nil {
		return nil, false, err
	}
	var metadata TerraformRequestMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, false, fmt.Errorf("decode Terraform conversion metadata: %w", err)
	}
	return &metadata, true, nil
}

// ValidateTerraformOperation validates an operation only when it carries
// Terraform conversion metadata or either Terraform review profile. The bool
// reports whether the operation belongs to that contract.
func ValidateTerraformOperation(operation *uws1.Operation) (bool, error) {
	if operation == nil {
		return false, nil
	}
	profile := operation.ExtensionProfile()
	applicable := hasTerraformContractMarker(operation.Request) || profile == TerraformReviewTODOProfile || profile == retiredTerraformReviewTODOProfile
	if !applicable {
		return false, nil
	}
	if profile == retiredTerraformReviewTODOProfile {
		return true, fmt.Errorf("retired Terraform review profile %q is not accepted; use %q", retiredTerraformReviewTODOProfile, TerraformReviewTODOProfile)
	}
	metadata, present, err := ReadTerraformRequestMetadata(operation.Request)
	if err != nil {
		return true, err
	}
	if !present || metadata == nil {
		return true, fmt.Errorf("missing %s metadata", RequestTerraformProvenance)
	}
	if operation.HasSourceBinding() {
		if !operation.HasCompleteSourceBinding() {
			return true, fmt.Errorf("Terraform conversion operation %q has an incomplete source binding", operation.OperationID)
		}
		if profile != "" {
			return true, fmt.Errorf("source-bound Terraform conversion operation %q must not carry %s", operation.OperationID, uws1.ExtensionOperationProfile)
		}
		if strings.TrimSpace(metadata.TODO) != "" {
			return true, fmt.Errorf("source-bound Terraform conversion operation %q must not carry %s", operation.OperationID, RequestReviewTODO)
		}
		return true, nil
	}
	if profile != TerraformReviewTODOProfile {
		return true, fmt.Errorf("unresolved Terraform conversion operation %q must carry %s: %s", operation.OperationID, uws1.ExtensionOperationProfile, TerraformReviewTODOProfile)
	}
	if strings.TrimSpace(metadata.TODO) == "" {
		return true, fmt.Errorf("unresolved Terraform conversion operation %q must carry %s", operation.OperationID, RequestReviewTODO)
	}
	return true, nil
}

func hasTerraformContractMarker(request map[string]any) bool {
	for key := range request {
		if key == RequestTerraformProvenance || strings.HasPrefix(key, RequestTerraformProvenance+"-") {
			return true
		}
	}
	return false
}

func terraformMetadataEnvelope(request map[string]any) (map[string]any, bool) {
	envelope := map[string]any{}
	for key, value := range request {
		switch key {
		case RequestTerraformProvenance, RequestCredentialBindings, RequestReviewTODO:
			envelope[key] = value
		default:
			if strings.HasPrefix(key, RequestTerraformProvenance+"-") {
				envelope[key] = value
			}
		}
	}
	return envelope, len(envelope) > 0
}
