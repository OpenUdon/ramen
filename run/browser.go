package run

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/OpenUdon/evidence/digest"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/internal/browsercontract"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/uws/browserauthentication"
	"github.com/OpenUdon/uws/uws1"
)

// BrowserArtifact is one validated external browser contract referenced by an
// imperative UWS document. References and credential names remain symbolic.
type BrowserArtifact struct {
	Kind         string   `json:"kind"`
	Profile      string   `json:"profile"`
	Reference    string   `json:"reference"`
	Digest       string   `json:"digest"`
	SourceID     string   `json:"source_id,omitempty"`
	OperationIDs []string `json:"operation_ids,omitempty"`
}

type browserRunContract struct {
	Artifacts    []BrowserArtifact
	Requirements executor.BrowserRequirements
	Digest       string
}

func loadBrowserRunContract(documentPath string, doc *uws1.Document) (*browserRunContract, error) {
	contract := &browserRunContract{}
	if doc == nil {
		return nil, fmt.Errorf("UWS document is required")
	}
	anchorDir := filepath.Dir(documentPath)
	projectDoc := &project.Document{Dir: anchorDir, UWS: doc}
	profiles := map[string]*browsercontract.Profile{}
	sources := map[string]project.APISource{}
	artifactsBySource := map[string]int{}
	for _, sourceDescription := range doc.SourceDescriptions {
		if sourceDescription == nil || sourceDescription.EffectiveType() != uws1.SourceDescriptionTypeBrowserProfile {
			continue
		}
		profile, err := browsercontract.LoadProfile(anchorDir, sourceDescription.URL)
		if err != nil {
			return nil, fmt.Errorf("browser source %s: %w", sourceDescription.Name, err)
		}
		if err := browsercontract.ValidateProfileVersion(doc.UWS, profile.Version); err != nil {
			return nil, fmt.Errorf("browser source %s: %w", sourceDescription.Name, err)
		}
		source := project.APISource{Kind: string(uws1.SourceDescriptionTypeBrowserProfile), ID: sourceDescription.Name, Path: profile.Path}
		profiles[sourceDescription.Name] = profile
		sources[sourceDescription.Name] = source
		projectDoc.Profile.APISources = append(projectDoc.Profile.APISources, source)
		artifactsBySource[sourceDescription.Name] = len(contract.Artifacts)
		contract.Artifacts = append(contract.Artifacts, BrowserArtifact{
			Kind: "browser-profile", Profile: profile.Version, Reference: sourceDescription.URL,
			Digest: profile.Digest, SourceID: sourceDescription.Name,
		})
	}
	usedAuthentication := map[string]bool{}
	for _, operation := range doc.Operations {
		if operation == nil {
			continue
		}
		profile := profiles[operation.SourceDescription]
		if profile == nil {
			continue
		}
		actionID, err := browsercontract.SelectedActionID(operation)
		if err != nil {
			return nil, err
		}
		action, ok := profile.Actions[actionID]
		if !ok {
			return nil, fmt.Errorf("browser action %s was not found", actionID)
		}
		purpose := "update"
		if len(action.SideEffects) == 1 && action.SideEffects[0] == "read_only" {
			purpose = "read"
		}
		role := project.OperationRole{
			SourceKind: string(uws1.SourceDescriptionTypeBrowserProfile), SourceID: operation.SourceDescription,
			OperationID: actionID, UWSOperationRef: operation.OperationID,
		}
		resource := project.Resource{Address: "run." + operation.OperationID}
		for _, dependency := range operation.DependsOn {
			candidate := operationWithID(doc, dependency)
			if candidate == nil {
				continue
			}
			call, ok, err := browserauthentication.ReadAuthenticationExtension(candidate.Extensions)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			for _, binding := range call.CredentialBindings {
				resource.CredentialBindings = append(resource.CredentialBindings, binding)
			}
		}
		role.CredentialBindings = slices.Clone(resource.CredentialBindings)
		validated, err := browsercontract.ValidateRole(projectDoc, resource, purpose, role, sources[operation.SourceDescription], profile)
		if err != nil {
			return nil, fmt.Errorf("browser operation %s: %w", operation.OperationID, err)
		}
		artifactIndex := artifactsBySource[operation.SourceDescription]
		contract.Artifacts[artifactIndex].OperationIDs = append(contract.Artifacts[artifactIndex].OperationIDs, operation.OperationID)
		mergeBrowserRequirements(&contract.Requirements, validated)
		if validated.Authentication != nil {
			auth := validated.Authentication
			if !usedAuthentication[auth.Operation.OperationID] {
				contract.Artifacts = append(contract.Artifacts, BrowserArtifact{
					Kind: "browser-authentication", Profile: auth.Profile.Profile.Profile,
					Reference: auth.ProfileRef, Digest: auth.Profile.Digest,
					OperationIDs: []string{auth.Operation.OperationID},
				})
			}
			usedAuthentication[auth.Operation.OperationID] = true
		}
	}
	for _, operation := range doc.Operations {
		if operation == nil {
			continue
		}
		_, hasCall := operation.Extensions[browserauthentication.ExtensionAuthentication]
		isCallProfile := operation.ExtensionProfile() == browserauthentication.CallProfileName || operation.ExtensionProfile() == browserauthentication.ContextCallProfileName
		if (hasCall || isCallProfile) && !usedAuthentication[operation.OperationID] {
			return nil, fmt.Errorf("authentication operation %s must be a direct dependency of a browser operation", operation.OperationID)
		}
	}
	normalizeBrowserArtifacts(contract.Artifacts)
	encoded, err := json.Marshal(contract.Artifacts)
	if err != nil {
		return nil, err
	}
	contract.Digest = digest.SHA256String(encoded)
	return contract, nil
}

func recheckBrowserRunContract(documentPath string, doc *uws1.Document, expected *browserRunContract) error {
	if expected == nil || len(expected.Artifacts) == 0 {
		return nil
	}
	current, err := loadBrowserRunContract(documentPath, doc)
	if err != nil {
		return fmt.Errorf("run.browser_artifact_changed: %w", err)
	}
	if current.Digest != expected.Digest {
		return fmt.Errorf("run.browser_artifact_changed: referenced browser artifacts no longer match approval")
	}
	return nil
}

func mergeBrowserRequirements(requirements *executor.BrowserRequirements, role *browsercontract.Role) {
	if requirements == nil || role == nil || role.Profile == nil {
		return
	}
	requirements.Contexts = requirements.Contexts || len(role.Profile.Contexts) > 0
	requirements.NamedSession = requirements.NamedSession || role.Session != ""
	requirements.ExternalSession = requirements.ExternalSession || role.ExternalSession
	requirements.Authentication = requirements.Authentication || role.Authentication != nil
	requirements.AuthenticationApproval = requirements.AuthenticationApproval || role.Authentication != nil
	if role.Authentication != nil && role.Authentication.Profile != nil {
		requirements.Contexts = requirements.Contexts || len(role.Authentication.Profile.Profile.Contexts) > 0
	}
	for _, output := range role.Action.Outputs {
		if output.Presence || (output.Type != "" && output.Type != "string") {
			requirements.ScalarOutputs = true
		}
	}
	for _, effect := range role.Action.SideEffects {
		if effect != "read_only" {
			requirements.MutationApproval = true
		}
	}
}

func normalizeBrowserArtifacts(artifacts []BrowserArtifact) {
	for i := range artifacts {
		artifacts[i].Reference = filepath.ToSlash(strings.TrimSpace(artifacts[i].Reference))
		slices.Sort(artifacts[i].OperationIDs)
		artifacts[i].OperationIDs = slices.Compact(artifacts[i].OperationIDs)
	}
	slices.SortFunc(artifacts, func(a, b BrowserArtifact) int {
		left := a.Kind + "\x00" + a.Reference + "\x00" + a.SourceID + "\x00" + strings.Join(a.OperationIDs, "\x00")
		right := b.Kind + "\x00" + b.Reference + "\x00" + b.SourceID + "\x00" + strings.Join(b.OperationIDs, "\x00")
		return strings.Compare(left, right)
	})
}

func operationWithID(doc *uws1.Document, id string) *uws1.Operation {
	for _, operation := range doc.Operations {
		if operation != nil && operation.OperationID == id {
			return operation
		}
	}
	return nil
}
