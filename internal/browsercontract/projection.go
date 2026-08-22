package browsercontract

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/OpenUdon/uws/browserauthentication"
)

// Projection is the stable, secret-free browser contract carried by a Ramen
// plan. It binds lowering and approval to the exact reviewed source metadata.
type Projection struct {
	UWSOperationID  string                    `json:"uws_operation_id"`
	ActionID        string                    `json:"action_id"`
	Request         map[string]any            `json:"request,omitempty"`
	ProfileVersion  string                    `json:"profile_version"`
	ProfileRef      string                    `json:"profile_ref"`
	ProfilePath     string                    `json:"profile_path"`
	ProfileDigest   string                    `json:"profile_digest"`
	Session         string                    `json:"session,omitempty"`
	ExternalSession bool                      `json:"external_session,omitempty"`
	Outputs         []OutputProjection        `json:"outputs,omitempty"`
	Contexts        []ContextProjection       `json:"contexts,omitempty"`
	SideEffects     []string                  `json:"side_effects"`
	Confirmation    ConfirmationProjection    `json:"confirmation"`
	Authentication  *AuthenticationProjection `json:"authentication,omitempty"`
}

type OutputProjection struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Source   string `json:"source"`
	Context  string `json:"context,omitempty"`
	Presence bool   `json:"presence,omitempty"`
}

type ContextProjection struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Parent string `json:"parent"`
	Origin string `json:"origin"`
	Path   string `json:"path,omitempty"`
	Name   string `json:"name,omitempty"`
}

type ConfirmationProjection struct {
	Required bool   `json:"required"`
	Prompt   string `json:"prompt,omitempty"`
}

type CredentialBindingProjection struct {
	Slot    string `json:"slot"`
	Binding string `json:"binding"`
}

type AuthenticationProjection struct {
	UWSOperationID     string                        `json:"uws_operation_id"`
	CallProfile        string                        `json:"call_profile"`
	ProfileVersion     string                        `json:"profile_version"`
	ProfileRef         string                        `json:"profile_ref"`
	ProfilePath        string                        `json:"profile_path"`
	ProfileDigest      string                        `json:"profile_digest"`
	Flow               string                        `json:"flow"`
	TimeoutSeconds     float64                       `json:"timeout_seconds"`
	Session            string                        `json:"session"`
	CredentialBindings []CredentialBindingProjection `json:"credential_bindings"`
	Contexts           []ContextProjection           `json:"contexts,omitempty"`
}

// NewProjection copies a validated role into a deterministic plan projection.
func NewProjection(role *Role) *Projection {
	if role == nil || role.Profile == nil || role.Operation == nil {
		return nil
	}
	projection := &Projection{
		UWSOperationID:  strings.TrimSpace(role.Operation.OperationID),
		ActionID:        strings.TrimSpace(role.ActionID),
		Request:         cloneMap(role.Operation.Request),
		ProfileVersion:  strings.TrimSpace(role.Profile.Version),
		ProfileRef:      strings.TrimSpace(role.ProfileRef),
		ProfilePath:     role.Profile.Path,
		ProfileDigest:   role.Profile.Digest,
		Session:         strings.TrimSpace(role.Session),
		ExternalSession: role.ExternalSession,
		Contexts:        projectContexts(role.Profile.Contexts),
		SideEffects:     slices.Clone(role.Action.SideEffects),
		Confirmation: ConfirmationProjection{
			Required: role.Action.ConfirmationPolicy.Required,
			Prompt:   role.Action.ConfirmationPolicy.Prompt,
		},
	}
	slices.Sort(projection.SideEffects)
	for name, output := range role.Action.Outputs {
		projection.Outputs = append(projection.Outputs, OutputProjection{
			Name: name, Type: output.Type, Source: output.Source,
			Context: output.Context, Presence: output.Presence,
		})
	}
	slices.SortFunc(projection.Outputs, func(a, b OutputProjection) int { return strings.Compare(a.Name, b.Name) })
	if role.Authentication != nil && role.Authentication.Profile != nil && role.Authentication.Operation != nil {
		auth := role.Authentication
		projected := &AuthenticationProjection{
			UWSOperationID: strings.TrimSpace(auth.Operation.OperationID),
			CallProfile:    strings.TrimSpace(auth.CallProfile),
			ProfileVersion: strings.TrimSpace(auth.Profile.Profile.Profile),
			ProfileRef:     strings.TrimSpace(auth.ProfileRef),
			ProfilePath:    auth.Profile.Path,
			ProfileDigest:  auth.Profile.Digest,
			Flow:           strings.TrimSpace(auth.Flow),
			Session:        strings.TrimSpace(auth.Session),
			Contexts:       authenticationContexts(auth.Profile.Profile.Contexts),
		}
		if auth.Operation.Timeout != nil {
			projected.TimeoutSeconds = *auth.Operation.Timeout
		}
		for slot, binding := range auth.CredentialBindings {
			projected.CredentialBindings = append(projected.CredentialBindings, CredentialBindingProjection{Slot: slot, Binding: binding})
		}
		slices.SortFunc(projected.CredentialBindings, func(a, b CredentialBindingProjection) int { return strings.Compare(a.Slot, b.Slot) })
		projection.Authentication = projected
	}
	return projection
}

func projectContexts(contexts map[string]Context) []ContextProjection {
	out := make([]ContextProjection, 0, len(contexts))
	for name, context := range contexts {
		out = append(out, ContextProjection{ID: name, Kind: context.Kind, Parent: context.Parent, Origin: context.Origin, Path: context.Path, Name: context.Name})
	}
	slices.SortFunc(out, func(a, b ContextProjection) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func authenticationContexts(contexts map[string]browserauthentication.Context) []ContextProjection {
	out := make([]ContextProjection, 0, len(contexts))
	for name, context := range contexts {
		out = append(out, ContextProjection{ID: name, Kind: context.Kind, Parent: context.Parent, Origin: context.Origin, Path: context.Path, Name: context.Name})
	}
	slices.SortFunc(out, func(a, b ContextProjection) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
