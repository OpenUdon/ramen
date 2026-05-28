package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

const (
	Version      = "ramen.project.v1"
	ExtensionKey = "x-ramen-desired-state"
	DefaultFile  = "project.uws.yaml"
	DefaultJSON  = "project.uws.json"
	DefaultYAML  = "project.uws.yml"
	WorkflowYAML = "workflows/workflow.uws.yaml"
	WorkflowJSON = "workflows/workflow.uws.json"
	WorkflowYML  = "workflows/workflow.uws.yml"
)

type Document struct {
	Path    string
	Dir     string
	UWS     *uws1.Document
	Profile Profile
}

type Profile struct {
	Version    string         `json:"version"`
	APISources []APISource    `json:"api_sources,omitempty"`
	Variables  []Variable     `json:"variables,omitempty"`
	Resources  []Resource     `json:"resources,omitempty"`
	Redaction  Redaction      `json:"redaction,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type Variable struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Sensitive   bool   `json:"sensitive,omitempty"`
}

type APISource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Path string `json:"path"`
}

type Resource struct {
	Address            string                   `json:"address"`
	Kind               string                   `json:"kind"`
	Type               string                   `json:"type"`
	Name               string                   `json:"name,omitempty"`
	Provider           string                   `json:"provider,omitempty"`
	Attributes         map[string]any           `json:"attributes,omitempty"`
	Lifecycle          Lifecycle                `json:"lifecycle,omitempty"`
	Dependencies       []string                 `json:"dependencies,omitempty"`
	Operations         map[string]OperationRole `json:"operations,omitempty"`
	IdentityAttributes []IdentityAttribute      `json:"identity_attributes,omitempty"`
	CredentialBindings []string                 `json:"credential_bindings,omitempty"`
	Redaction          Redaction                `json:"redaction,omitempty"`
	AI                 *AIMetadata              `json:"ai,omitempty"`
	Metadata           map[string]any           `json:"metadata,omitempty"`
}

type Lifecycle struct {
	PreventDestroy     bool     `json:"prevent_destroy,omitempty"`
	IgnoreAll          bool     `json:"ignore_all,omitempty"`
	IgnorePaths        []string `json:"ignore_paths,omitempty"`
	ReplaceTriggeredBy []string `json:"replace_triggered_by,omitempty"`
}

type OperationRole struct {
	Purpose            string      `json:"purpose,omitempty"`
	SourceKind         string      `json:"source_kind,omitempty"`
	SourceID           string      `json:"source_id,omitempty"`
	SourcePath         string      `json:"source_path,omitempty"`
	OperationID        string      `json:"operation_id,omitempty"`
	CredentialBindings []string    `json:"credential_bindings,omitempty"`
	AI                 *AIMetadata `json:"ai,omitempty"`
}

type AIMetadata struct {
	Confidence  *Confidence `json:"confidence,omitempty"`
	Uncertainty string      `json:"uncertainty,omitempty"`
	Rationale   string      `json:"rationale,omitempty"`
	Citations   []string    `json:"citations,omitempty"`
}

type Confidence struct {
	Score  float64 `json:"score"`
	Reason string  `json:"reason,omitempty"`
}

type IdentityAttribute struct {
	Name          string   `json:"name"`
	Path          string   `json:"path"`
	RequestKeys   []string `json:"request_keys,omitempty"`
	ResponsePaths []string `json:"response_paths,omitempty"`
	Required      bool     `json:"required,omitempty"`
}

type Redaction struct {
	Paths []string `json:"paths,omitempty"`
}

func Load(path string) (*Document, error) {
	resolved, err := ResolvePath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, err
	}
	var doc uws1.Document
	switch strings.ToLower(filepath.Ext(resolved)) {
	case ".json":
		err = uwsconvert.UnmarshalJSON(data, &doc)
	case ".yaml", ".yml":
		err = uwsconvert.UnmarshalYAML(data, &doc)
	default:
		err = fmt.Errorf("unsupported native project document extension %q", filepath.Ext(resolved))
	}
	if err != nil {
		return nil, fmt.Errorf("load UWS project document %s: %w", resolved, err)
	}
	if err := doc.Validate(); err != nil {
		return nil, fmt.Errorf("validate UWS project document %s: %w", resolved, err)
	}
	profile, err := ProfileFromDocument(&doc)
	if err != nil {
		return nil, fmt.Errorf("load Ramen project profile %s: %w", resolved, err)
	}
	dir := filepath.Dir(resolved)
	normalizeProfilePaths(&profile, dir)
	if err := ValidateProfile(profile); err != nil {
		return nil, fmt.Errorf("validate Ramen project profile %s: %w", resolved, err)
	}
	return &Document{Path: resolved, Dir: dir, UWS: &doc, Profile: profile}, nil
}

func ResolvePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return path, nil
	}
	for _, name := range []string{DefaultFile, DefaultJSON, DefaultYAML, WorkflowYAML, WorkflowJSON, WorkflowYML} {
		candidate := filepath.Join(path, filepath.FromSlash(name))
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no native Ramen project document found in %s", path)
}

func ProfileFromDocument(doc *uws1.Document) (Profile, error) {
	if doc == nil {
		return Profile{}, fmt.Errorf("UWS document is required")
	}
	value, ok := doc.Extensions[ExtensionKey]
	if !ok {
		return Profile{}, fmt.Errorf("missing %s extension", ExtensionKey)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return Profile{}, err
	}
	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func ValidateProfile(profile Profile) error {
	if profile.Version != Version {
		return fmt.Errorf("unsupported version %q", profile.Version)
	}
	seenSources := map[string]bool{}
	seenVariables := map[string]bool{}
	for _, variable := range profile.Variables {
		name := strings.TrimSpace(variable.Name)
		if name == "" {
			return fmt.Errorf("variables entries require name")
		}
		if !variableNamePattern.MatchString(name) {
			return fmt.Errorf("variable %s has invalid name", name)
		}
		if seenVariables[name] {
			return fmt.Errorf("duplicate variable %s", name)
		}
		seenVariables[name] = true
		typ := strings.TrimSpace(variable.Type)
		switch typ {
		case "", "any", "string", "number", "bool", "object", "list":
		default:
			return fmt.Errorf("variable %s has unsupported type %q", name, variable.Type)
		}
		if variable.Default != nil && !valueMatchesType(variable.Default, typ) {
			return fmt.Errorf("variable %s default does not match type %q", name, firstNonEmpty(typ, "any"))
		}
	}
	for _, source := range profile.APISources {
		if strings.TrimSpace(source.Kind) == "" || strings.TrimSpace(source.ID) == "" || strings.TrimSpace(source.Path) == "" {
			return fmt.Errorf("api_sources entries require kind, id, and path")
		}
		key := strings.TrimSpace(source.Kind) + "\x00" + strings.TrimSpace(source.ID)
		if seenSources[key] {
			return fmt.Errorf("duplicate API source %s:%s", source.Kind, source.ID)
		}
		seenSources[key] = true
	}
	seenResources := map[string]bool{}
	for _, resource := range profile.Resources {
		if strings.TrimSpace(resource.Address) == "" {
			return fmt.Errorf("resources entries require address")
		}
		if strings.TrimSpace(resource.Kind) == "" {
			return fmt.Errorf("resource %s requires kind", resource.Address)
		}
		if strings.TrimSpace(resource.Type) == "" {
			return fmt.Errorf("resource %s requires type", resource.Address)
		}
		if seenResources[resource.Address] {
			return fmt.Errorf("duplicate resource %s", resource.Address)
		}
		if err := validateAIMetadata(resource.AI); err != nil {
			return fmt.Errorf("resource %s ai metadata invalid: %w", resource.Address, err)
		}
		seenResources[resource.Address] = true
		for purpose, op := range resource.Operations {
			if strings.TrimSpace(purpose) == "" {
				return fmt.Errorf("resource %s has an empty operation role", resource.Address)
			}
			if strings.TrimSpace(op.OperationID) == "" {
				return fmt.Errorf("resource %s %s operation requires operation_id", resource.Address, purpose)
			}
			if err := validateAIMetadata(op.AI); err != nil {
				return fmt.Errorf("resource %s %s operation ai metadata invalid: %w", resource.Address, purpose, err)
			}
		}
	}
	return nil
}

func validateAIMetadata(meta *AIMetadata) error {
	if meta == nil || meta.Confidence == nil {
		return nil
	}
	if meta.Confidence.Score < 0 || meta.Confidence.Score > 1 {
		return fmt.Errorf("confidence score must be between 0 and 1")
	}
	return nil
}

func AttributeStrings(attrs map[string]any) map[string]string {
	out := map[string]string{}
	for key, value := range attrs {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		data, err := json.Marshal(value)
		if err != nil {
			out[key] = fmt.Sprint(value)
			continue
		}
		out[key] = string(data)
	}
	return out
}

func SourceForRole(profile Profile, role OperationRole) APISource {
	for _, source := range profile.APISources {
		if strings.TrimSpace(source.Kind) == strings.TrimSpace(role.SourceKind) && strings.TrimSpace(source.ID) == strings.TrimSpace(role.SourceID) {
			return source
		}
	}
	return APISource{}
}

func normalizeProfilePaths(profile *Profile, dir string) {
	for i := range profile.APISources {
		profile.APISources[i].Kind = strings.TrimSpace(profile.APISources[i].Kind)
		profile.APISources[i].ID = strings.TrimSpace(profile.APISources[i].ID)
		profile.APISources[i].Path = resolveRelativePath(dir, profile.APISources[i].Path)
	}
	for i := range profile.Resources {
		resource := &profile.Resources[i]
		resource.Address = strings.TrimSpace(resource.Address)
		resource.Kind = strings.TrimSpace(resource.Kind)
		resource.Type = strings.TrimSpace(resource.Type)
		resource.Name = strings.TrimSpace(resource.Name)
		resource.Provider = strings.TrimSpace(resource.Provider)
		slices.Sort(resource.Dependencies)
		resource.Dependencies = slices.Compact(resource.Dependencies)
		slices.Sort(resource.CredentialBindings)
		resource.CredentialBindings = slices.Compact(resource.CredentialBindings)
		for purpose, role := range resource.Operations {
			role.Purpose = firstNonEmpty(role.Purpose, purpose)
			role.SourceKind = strings.TrimSpace(role.SourceKind)
			role.SourceID = strings.TrimSpace(role.SourceID)
			role.SourcePath = resolveRelativePath(dir, role.SourcePath)
			role.OperationID = strings.TrimSpace(role.OperationID)
			slices.Sort(role.CredentialBindings)
			role.CredentialBindings = slices.Compact(role.CredentialBindings)
			resource.Operations[purpose] = role
		}
		slices.Sort(resource.Lifecycle.IgnorePaths)
		resource.Lifecycle.IgnorePaths = slices.Compact(resource.Lifecycle.IgnorePaths)
	}
}

func resolveRelativePath(dir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(filepath.Join(dir, filepath.FromSlash(path)))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
