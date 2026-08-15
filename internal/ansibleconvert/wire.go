package ansibleconvert

import (
	"encoding/json"
	"fmt"

	"github.com/OpenUdon/uws/uws1"
)

const (
	// ArgspecVersion is the discriminator accepted by Ramen-owned argspecs.
	ArgspecVersion = "ramen.ansible.1.0"

	// ProfileName selects Ramen's Ansible module-call operation profile.
	ProfileName = "ramen.ansible-module-call.1.0"

	// ExtensionAnsibleModule carries the module leaf and argspec reference.
	ExtensionAnsibleModule = "x-ramen-ansible-module"

	// ExtensionAnsibleProvenance carries static playbook source provenance.
	ExtensionAnsibleProvenance = "x-ramen-ansible-provenance"

	retiredArgspecVersion      = "uws.ansible.1.0"
	retiredProfileName         = "uws.ansible-module-call.1.0"
	retiredExtensionModule     = "x-uws-ansible-module"
	retiredExtensionProvenance = "x-ansible"
)

// OperationAnsibleModule is the typed payload for x-ramen-ansible-module.
type OperationAnsibleModule struct {
	Module  string            `json:"module,omitempty" hcl:"module,optional"`
	Argspec *ArgspecReference `json:"argspec,omitempty" hcl:"argspec,block"`
}

// ArgspecReference identifies the Ramen argspec used for static review and
// validation. All fields are required when a reference is present.
type ArgspecReference struct {
	SourceID   string `json:"sourceId,omitempty" hcl:"sourceId,optional"`
	URL        string `json:"url,omitempty" hcl:"url,optional"`
	Collection string `json:"collection,omitempty" hcl:"collection,optional"`
}

// ReadOperationExtension strictly decodes x-ramen-ansible-module from an
// extension map. It never interprets the retired UWS extension or profile.
func ReadOperationExtension(extensions map[string]any) (*OperationAnsibleModule, bool, error) {
	if len(extensions) == 0 {
		return nil, false, nil
	}
	if err := rejectRetiredAnsibleExtensions(extensions); err != nil {
		return nil, false, err
	}
	value, ok := extensions[ExtensionAnsibleModule]
	if !ok {
		return nil, false, nil
	}
	envelope, err := json.Marshal(map[string]any{ExtensionAnsibleModule: value})
	if err != nil {
		return nil, false, fmt.Errorf("marshal %s extension: %w", ExtensionAnsibleModule, err)
	}
	if err := ValidateModuleCallDocument(envelope); err != nil {
		return nil, false, err
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, false, fmt.Errorf("marshal %s payload: %w", ExtensionAnsibleModule, err)
	}
	var out OperationAnsibleModule
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, false, fmt.Errorf("unmarshal %s extension: %w", ExtensionAnsibleModule, err)
	}
	return &out, true, nil
}

// SetOperationExtension validates and encodes x-ramen-ansible-module into an
// extension map.
func SetOperationExtension(dst *map[string]any, value *OperationAnsibleModule) error {
	if value == nil {
		return nil
	}
	envelope, err := json.Marshal(map[string]any{ExtensionAnsibleModule: value})
	if err != nil {
		return err
	}
	if err := ValidateModuleCallDocument(envelope); err != nil {
		return err
	}
	var generic map[string]any
	if err := json.Unmarshal(envelope, &generic); err != nil {
		return err
	}
	if *dst == nil {
		*dst = make(map[string]any)
	}
	(*dst)[ExtensionAnsibleModule] = generic[ExtensionAnsibleModule]
	return nil
}

// ValidateOperationExtensions checks the complete Ramen Ansible operation
// selector and module payload while allowing unrelated generic extensions.
func ValidateOperationExtensions(extensions map[string]any) error {
	if err := rejectRetiredAnsibleExtensions(extensions); err != nil {
		return err
	}
	profile, ok := extensions[uws1.ExtensionOperationProfile]
	if !ok {
		return fmt.Errorf("missing %s selector", uws1.ExtensionOperationProfile)
	}
	profileName, ok := profile.(string)
	if !ok || profileName != ProfileName {
		return fmt.Errorf("%s must be %q", uws1.ExtensionOperationProfile, ProfileName)
	}
	_, ok, err := ReadOperationExtension(extensions)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("missing %s extension", ExtensionAnsibleModule)
	}
	return nil
}

func rejectRetiredAnsibleExtensions(extensions map[string]any) error {
	if _, ok := extensions[retiredExtensionModule]; ok {
		return fmt.Errorf("retired Ansible extension %q is not accepted; use %q", retiredExtensionModule, ExtensionAnsibleModule)
	}
	if _, ok := extensions[retiredExtensionProvenance]; ok {
		return fmt.Errorf("retired Ansible provenance extension %q is not accepted; use %q", retiredExtensionProvenance, ExtensionAnsibleProvenance)
	}
	if profile, ok := extensions[uws1.ExtensionOperationProfile]; ok && profile == retiredProfileName {
		return fmt.Errorf("retired Ansible operation profile %q is not accepted; use %q", retiredProfileName, ProfileName)
	}
	return nil
}
