package validate

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/OpenUdon/apitools"
	"github.com/OpenUdon/ramen/graph"
	"github.com/OpenUdon/ramen/project"
)

const Version = "ramen.validate.v1"

type Options struct {
	ProjectPath string
	APISources  []APISourceInput
	Strict      bool
}

type APISourceInput struct {
	Kind string
	ID   string
	Path string
}

type Result struct {
	Version     string       `json:"version"`
	ProjectPath string       `json:"project_path,omitempty"`
	Strict      bool         `json:"strict,omitempty"`
	Valid       bool         `json:"valid"`
	Summary     Summary      `json:"summary"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type Summary struct {
	Errors      int `json:"errors"`
	Warnings    int `json:"warnings"`
	Diagnostics int `json:"diagnostics"`
}

type Diagnostic struct {
	Code          string `json:"code"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	Address       string `json:"address,omitempty"`
	APISourceKind string `json:"api_source_kind,omitempty"`
	APISourceID   string `json:"api_source_id,omitempty"`
	OperationID   string `json:"operation_id,omitempty"`
}

func Run(ctx context.Context, opts Options) (*Result, error) {
	result := &Result{
		Version: Version,
		Strict:  opts.Strict,
	}
	if strings.TrimSpace(opts.ProjectPath) == "" {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "validate.project_required", Severity: "error", Message: "--project is required"})
		finalize(result)
		return result, nil
	}
	doc, err := project.Load(opts.ProjectPath)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "validate.project_load_error", Severity: "error", Message: err.Error()})
		finalize(result)
		return result, nil
	}
	result.ProjectPath = doc.Path
	result.Diagnostics = append(result.Diagnostics, validateProfile(doc.Profile)...)
	sources, sourceDiagnostics := loadAPISources(ctx, mergeAPISources(doc.Profile, opts.APISources))
	result.Diagnostics = append(result.Diagnostics, sourceDiagnostics...)
	result.Diagnostics = append(result.Diagnostics, validateOperations(doc.Profile, sources)...)
	result.Diagnostics = append(result.Diagnostics, unusedSourceDiagnostics(doc.Profile)...)
	if opts.Strict {
		for i := range result.Diagnostics {
			if result.Diagnostics[i].Severity == "warning" {
				result.Diagnostics[i].Severity = "error"
			}
		}
	}
	finalize(result)
	return result, nil
}

func validateProfile(profile project.Profile) []Diagnostic {
	var diagnostics []Diagnostic
	addresses := map[string]bool{}
	for _, resource := range profile.Resources {
		addresses[resource.Address] = true
		for _, dep := range resource.Dependencies {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			if !addresses[dep] {
				found := false
				for _, candidate := range profile.Resources {
					if candidate.Address == dep {
						found = true
						break
					}
				}
				if !found {
					diagnostics = append(diagnostics, Diagnostic{Code: "validate.dependency_missing", Severity: "error", Message: fmt.Sprintf("resource %s depends on missing resource %s", resource.Address, dep), Address: resource.Address})
				}
			}
		}
		for _, path := range resource.Lifecycle.IgnorePaths {
			if strings.TrimSpace(path) == "" {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.lifecycle_invalid", Severity: "error", Message: fmt.Sprintf("resource %s has an empty lifecycle ignore path", resource.Address), Address: resource.Address})
			}
		}
		for _, trigger := range resource.Lifecycle.ReplaceTriggeredBy {
			if strings.TrimSpace(trigger) == "" {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.lifecycle_invalid", Severity: "error", Message: fmt.Sprintf("resource %s has an empty replace trigger", resource.Address), Address: resource.Address})
			}
		}
		for _, identity := range resource.IdentityAttributes {
			if strings.TrimSpace(identity.Name) == "" || strings.TrimSpace(identity.Path) == "" {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.identity_invalid", Severity: "error", Message: fmt.Sprintf("resource %s identity attributes require name and path", resource.Address), Address: resource.Address})
			}
		}
		diagnostics = append(diagnostics, redactionDiagnostics(resource.Redaction, resource.Address)...)
		for purpose, role := range resource.Operations {
			rolePurpose := strings.TrimSpace(firstNonEmpty(role.Purpose, purpose))
			if rolePurpose == "" {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.operation_role_invalid", Severity: "error", Message: fmt.Sprintf("resource %s has an empty operation role", resource.Address), Address: resource.Address})
			} else if !knownOperationPurpose(rolePurpose) {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.operation_role_unknown", Severity: "warning", Message: fmt.Sprintf("resource %s uses non-standard operation role %s", resource.Address, rolePurpose), Address: resource.Address, OperationID: role.OperationID})
			}
			if strings.TrimSpace(role.SourceKind) == "" || strings.TrimSpace(role.SourceID) == "" {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.source_reference_missing", Severity: "error", Message: fmt.Sprintf("resource %s %s operation requires source_kind and source_id", resource.Address, rolePurpose), Address: resource.Address, OperationID: role.OperationID})
			}
		}
	}
	diagnostics = append(diagnostics, redactionDiagnostics(profile.Redaction, "")...)
	var nodes []graph.Node
	for _, resource := range profile.Resources {
		nodes = append(nodes, graph.Node{Address: resource.Address, DependsOn: resource.Dependencies})
	}
	if _, err := graph.Sort(nodes); err != nil {
		diagnostics = append(diagnostics, Diagnostic{Code: "validate.dependency_cycle", Severity: "error", Message: err.Error()})
	}
	return diagnostics
}

type sourceDoc struct {
	Kind       string
	ID         string
	Path       string
	Operations map[string]bool
}

func loadAPISources(ctx context.Context, inputs []APISourceInput) ([]sourceDoc, []Diagnostic) {
	var docs []sourceDoc
	var diagnostics []Diagnostic
	seen := map[string]bool{}
	for _, input := range inputs {
		input.Kind = normalizeAPISourceKind(input.Kind)
		input.ID = strings.TrimSpace(input.ID)
		input.Path = strings.TrimSpace(input.Path)
		if input.Kind == "" || input.ID == "" || input.Path == "" {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.api_source_invalid", Severity: "error", Message: "API source inputs require kind, id, and path", APISourceKind: input.Kind, APISourceID: input.ID})
			continue
		}
		key := input.Kind + "\x00" + input.ID
		if seen[key] {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.api_source_duplicate", Severity: "error", Message: fmt.Sprintf("API source %s:%s is duplicated", input.Kind, input.ID), APISourceKind: input.Kind, APISourceID: input.ID})
			continue
		}
		seen[key] = true
		inventory, err := apitools.BuildAPISourceOperationInventory(ctx, apitools.APISourceInventoryOptions{
			Documents: []apitools.APISourceDocument{{
				Kind:         input.Kind,
				Name:         input.ID,
				Path:         input.Path,
				RelativePath: packageAPISourcePath(input.Kind, input.ID, input.Path),
			}},
		})
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.api_source_load_error", Severity: "error", Message: err.Error(), APISourceKind: input.Kind, APISourceID: input.ID})
			continue
		}
		for _, diag := range inventory.Diagnostics {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.api_source_" + strings.ReplaceAll(diag.Code, ".", "_"), Severity: normalizeSeverity(diag.Severity), Message: diag.Message, APISourceKind: input.Kind, APISourceID: input.ID})
		}
		ops := map[string]bool{}
		for _, op := range inventory.Operations {
			if strings.TrimSpace(op.OperationID) != "" {
				ops[op.OperationID] = true
			}
		}
		docs = append(docs, sourceDoc{Kind: input.Kind, ID: input.ID, Path: input.Path, Operations: ops})
	}
	slices.SortFunc(docs, func(a, b sourceDoc) int {
		if diff := cmp.Compare(a.Kind, b.Kind); diff != 0 {
			return diff
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return docs, diagnostics
}

func validateOperations(profile project.Profile, sources []sourceDoc) []Diagnostic {
	var diagnostics []Diagnostic
	for _, resource := range profile.Resources {
		for purpose, role := range resource.Operations {
			rolePurpose := firstNonEmpty(role.Purpose, purpose)
			roleKind := normalizeAPISourceKind(role.SourceKind)
			if roleKind == "" {
				roleKind = strings.TrimSpace(role.SourceKind)
			}
			var matchedSource *sourceDoc
			for i := range sources {
				source := &sources[i]
				if source.Kind == roleKind && source.ID == strings.TrimSpace(role.SourceID) {
					matchedSource = source
					break
				}
			}
			if matchedSource == nil {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.source_reference_missing", Severity: "error", Message: fmt.Sprintf("resource %s %s operation references missing API source %s:%s", resource.Address, rolePurpose, role.SourceKind, role.SourceID), Address: resource.Address, APISourceKind: role.SourceKind, APISourceID: role.SourceID, OperationID: role.OperationID})
				continue
			}
			if strings.TrimSpace(role.OperationID) == "" {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.operation_missing", Severity: "error", Message: fmt.Sprintf("resource %s %s operation requires operation_id", resource.Address, rolePurpose), Address: resource.Address, APISourceKind: role.SourceKind, APISourceID: role.SourceID})
				continue
			}
			if !matchedSource.Operations[role.OperationID] {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.operation_unknown", Severity: "error", Message: fmt.Sprintf("operation %s for resource %s was not found in API source %s:%s", role.OperationID, resource.Address, role.SourceKind, role.SourceID), Address: resource.Address, APISourceKind: role.SourceKind, APISourceID: role.SourceID, OperationID: role.OperationID})
			}
		}
	}
	return diagnostics
}

func unusedSourceDiagnostics(profile project.Profile) []Diagnostic {
	used := map[string]bool{}
	for _, resource := range profile.Resources {
		for _, role := range resource.Operations {
			used[sourceKey(role.SourceKind, role.SourceID)] = true
		}
	}
	var diagnostics []Diagnostic
	for _, source := range profile.APISources {
		if !used[sourceKey(source.Kind, source.ID)] {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.api_source_unused", Severity: "warning", Message: fmt.Sprintf("API source %s:%s is not referenced by any resource operation", source.Kind, source.ID), APISourceKind: source.Kind, APISourceID: source.ID})
		}
	}
	return diagnostics
}

func redactionDiagnostics(redaction project.Redaction, address string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, path := range redaction.Paths {
		if strings.TrimSpace(path) == "" {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.redaction_invalid", Severity: "error", Message: "redaction paths must not be empty", Address: address})
		}
	}
	return diagnostics
}

func mergeAPISources(profile project.Profile, overrides []APISourceInput) []APISourceInput {
	byKey := map[string]APISourceInput{}
	for _, source := range profile.APISources {
		input := APISourceInput{Kind: normalizeAPISourceKind(source.Kind), ID: strings.TrimSpace(source.ID), Path: source.Path}
		byKey[sourceKey(input.Kind, input.ID)] = input
	}
	for _, override := range overrides {
		override.Kind = normalizeAPISourceKind(override.Kind)
		override.ID = strings.TrimSpace(override.ID)
		override.Path = strings.TrimSpace(override.Path)
		byKey[sourceKey(override.Kind, override.ID)] = override
	}
	inputs := make([]APISourceInput, 0, len(byKey))
	for _, input := range byKey {
		inputs = append(inputs, input)
	}
	slices.SortFunc(inputs, func(a, b APISourceInput) int {
		if diff := cmp.Compare(a.Kind, b.Kind); diff != 0 {
			return diff
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return inputs
}

func sourceKey(kind, id string) string {
	normalizedKind := normalizeAPISourceKind(kind)
	if normalizedKind == "" {
		normalizedKind = strings.TrimSpace(kind)
	}
	return normalizedKind + "\x00" + strings.TrimSpace(id)
}

func finalize(result *Result) {
	slices.SortFunc(result.Diagnostics, func(a, b Diagnostic) int {
		left := []string{a.Severity, a.Code, a.Address, a.APISourceKind, a.APISourceID, a.OperationID, a.Message}
		right := []string{b.Severity, b.Code, b.Address, b.APISourceKind, b.APISourceID, b.OperationID, b.Message}
		return slices.Compare(left, right)
	})
	for _, diag := range result.Diagnostics {
		switch diag.Severity {
		case "error":
			result.Summary.Errors++
		case "warning":
			result.Summary.Warnings++
		}
	}
	result.Summary.Diagnostics = len(result.Diagnostics)
	result.Valid = result.Summary.Errors == 0
}

func knownOperationPurpose(purpose string) bool {
	switch strings.TrimSpace(purpose) {
	case "read", "create", "update", "delete", "import":
		return true
	default:
		return false
	}
}

func normalizeAPISourceKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "openapi", "openapi3", "swagger":
		return "openapi"
	case "aws-smithy", "smithy", "aws":
		return "aws-smithy"
	case "google-discovery", "google", "discovery":
		return "google-discovery"
	default:
		return ""
	}
}

func normalizeSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "error":
		return "error"
	case "warning", "warn":
		return "warning"
	default:
		return "info"
	}
}

func packageAPISourcePath(kind, id, sourcePath string) string {
	name := strings.TrimSpace(id)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	}
	ext := filepath.Ext(sourcePath)
	if ext == "" {
		ext = ".json"
	}
	return filepath.ToSlash(filepath.Join(kind, name+ext))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
