package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/OpenUdon/evidence/digest"
	"github.com/OpenUdon/ramen/internal/redact"
	"gopkg.in/yaml.v3"
)

const InputsVersion = "ramen.inputs.v1"

type ValuesOptions struct {
	VarFiles []string
	Vars     []string
}

type ResolvedInputs struct {
	Version string          `json:"version"`
	Digest  string          `json:"digest,omitempty"`
	Values  []ResolvedValue `json:"values,omitempty"`
}

type ResolvedValue struct {
	Name      string `json:"name"`
	Type      string `json:"type,omitempty"`
	Source    string `json:"source"`
	Sensitive bool   `json:"sensitive,omitempty"`
	Value     any    `json:"value,omitempty"`
	Digest    string `json:"digest,omitempty"`
}

type ValueDiagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Name     string `json:"name,omitempty"`
	Path     string `json:"path,omitempty"`
}

type resolvedValueInternal struct {
	ResolvedValue
	raw any
}

var (
	variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	variableRefPattern  = regexp.MustCompile(`\$\{var\.([A-Za-z_][A-Za-z0-9_]*)\}`)
)

func ResolveProfile(profile Profile, projectDir string, opts ValuesOptions) (Profile, ResolvedInputs, []ValueDiagnostic) {
	values, inputs, diagnostics := ResolveValues(profile, projectDir, opts)
	resolved, applyDiagnostics := ApplyValues(profile, values)
	diagnostics = append(diagnostics, applyDiagnostics...)
	return resolved, inputs, diagnostics
}

func ResolveValues(profile Profile, projectDir string, opts ValuesOptions) (map[string]resolvedValueInternal, ResolvedInputs, []ValueDiagnostic) {
	decls := map[string]Variable{}
	order := make([]string, 0, len(profile.Variables))
	var diagnostics []ValueDiagnostic
	for _, variable := range profile.Variables {
		name := strings.TrimSpace(variable.Name)
		if name == "" {
			continue
		}
		if !variableNamePattern.MatchString(name) {
			diagnostics = append(diagnostics, ValueDiagnostic{Code: "values.name_invalid", Severity: "error", Message: fmt.Sprintf("variable %s has an invalid name", name), Name: name})
			continue
		}
		if _, exists := decls[name]; exists {
			diagnostics = append(diagnostics, ValueDiagnostic{Code: "values.duplicate_variable", Severity: "error", Message: fmt.Sprintf("duplicate variable %s", name), Name: name})
			continue
		}
		variable.Name = name
		if variable.Type == "" {
			variable.Type = "any"
		}
		decls[name] = variable
		order = append(order, name)
	}
	values := map[string]resolvedValueInternal{}
	for _, name := range order {
		decl := decls[name]
		if decl.Default != nil {
			if !valueMatchesType(decl.Default, decl.Type) {
				diagnostics = append(diagnostics, ValueDiagnostic{Code: "values.type_mismatch", Severity: "error", Message: fmt.Sprintf("variable %s default expects %s", name, decl.Type), Name: name})
				continue
			}
			values[name] = resolvedValue(name, decl, decl.Default, "default")
		}
	}
	for _, rawPath := range opts.VarFiles {
		path := resolveValuePath(projectDir, rawPath)
		fileValues, err := loadValuesFile(path)
		if err != nil {
			diagnostics = append(diagnostics, ValueDiagnostic{Code: "values.file_load_error", Severity: "error", Message: err.Error(), Path: path})
			continue
		}
		keys := sortedKeys(fileValues)
		for _, name := range keys {
			decl, ok := decls[name]
			if !ok {
				diagnostics = append(diagnostics, ValueDiagnostic{Code: "values.unknown_variable", Severity: "error", Message: fmt.Sprintf("values file sets unknown variable %s", name), Name: name, Path: path})
				continue
			}
			value := fileValues[name]
			if !valueMatchesType(value, decl.Type) {
				diagnostics = append(diagnostics, ValueDiagnostic{Code: "values.type_mismatch", Severity: "error", Message: fmt.Sprintf("variable %s expects %s", name, decl.Type), Name: name, Path: path})
				continue
			}
			values[name] = resolvedValue(name, decl, value, path)
		}
	}
	for _, assignment := range opts.Vars {
		name, value, err := parseCLIValue(assignment)
		if err != nil {
			diagnostics = append(diagnostics, ValueDiagnostic{Code: "values.var_invalid", Severity: "error", Message: err.Error()})
			continue
		}
		decl, ok := decls[name]
		if !ok {
			diagnostics = append(diagnostics, ValueDiagnostic{Code: "values.unknown_variable", Severity: "error", Message: fmt.Sprintf("--var sets unknown variable %s", name), Name: name})
			continue
		}
		if !valueMatchesType(value, decl.Type) {
			diagnostics = append(diagnostics, ValueDiagnostic{Code: "values.type_mismatch", Severity: "error", Message: fmt.Sprintf("variable %s expects %s", name, decl.Type), Name: name})
			continue
		}
		values[name] = resolvedValue(name, decl, value, "cli")
	}
	for _, name := range order {
		if _, ok := values[name]; !ok {
			diagnostics = append(diagnostics, ValueDiagnostic{Code: "values.required", Severity: "error", Message: fmt.Sprintf("variable %s has no value", name), Name: name})
		}
	}
	inputs := ResolvedInputs{Version: InputsVersion}
	for _, name := range order {
		if value, ok := values[name]; ok {
			inputs.Values = append(inputs.Values, value.ResolvedValue)
		}
	}
	inputs.Digest = inputsDigest(inputs)
	return values, inputs, diagnostics
}

func ApplyValues(profile Profile, values map[string]resolvedValueInternal) (Profile, []ValueDiagnostic) {
	var diagnostics []ValueDiagnostic
	for i := range profile.Resources {
		resource := &profile.Resources[i]
		attrs, diags := interpolateMap(resource.Attributes, values, resource.Address)
		resource.Attributes = attrs
		diagnostics = append(diagnostics, diags...)
		for _, roleName := range sortedKeys(resource.Operations) {
			role := resource.Operations[roleName]
			role.SourceKind, diags = interpolateString(role.SourceKind, values, resource.Address, roleName+".source_kind")
			diagnostics = append(diagnostics, diags...)
			role.SourceID, diags = interpolateString(role.SourceID, values, resource.Address, roleName+".source_id")
			diagnostics = append(diagnostics, diags...)
			role.SourcePath, diags = interpolateString(role.SourcePath, values, resource.Address, roleName+".source_path")
			diagnostics = append(diagnostics, diags...)
			role.OperationID, diags = interpolateString(role.OperationID, values, resource.Address, roleName+".operation_id")
			diagnostics = append(diagnostics, diags...)
			resource.Operations[roleName] = role
		}
	}
	return profile, diagnostics
}

func resolvedValue(name string, decl Variable, raw any, source string) resolvedValueInternal {
	value := raw
	if decl.Sensitive {
		value = redact.Value
	}
	return resolvedValueInternal{
		ResolvedValue: ResolvedValue{
			Name:      name,
			Type:      firstNonEmpty(decl.Type, "any"),
			Source:    source,
			Sensitive: decl.Sensitive,
			Value:     value,
			Digest:    valueDigest(raw),
		},
		raw: raw,
	}
}

func loadValuesFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, &out)
	default:
		err = json.Unmarshal(data, &out)
	}
	if err != nil {
		return nil, err
	}
	if nested, ok := out["values"].(map[string]any); ok {
		out = nested
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func parseCLIValue(assignment string) (string, any, error) {
	name, raw, ok := strings.Cut(assignment, "=")
	name = strings.TrimSpace(name)
	if !ok || name == "" {
		return "", nil, fmt.Errorf("--var must use name=value")
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err == nil {
		return name, value, nil
	}
	if parsed, err := strconv.ParseBool(raw); err == nil {
		return name, parsed, nil
	}
	if parsed, err := strconv.ParseFloat(raw, 64); err == nil {
		return name, parsed, nil
	}
	return name, raw, nil
}

func interpolateMap(in map[string]any, values map[string]resolvedValueInternal, address string) (map[string]any, []ValueDiagnostic) {
	if len(in) == 0 {
		return in, nil
	}
	out := map[string]any{}
	var diagnostics []ValueDiagnostic
	keys := sortedKeys(in)
	for _, key := range keys {
		value, diags := interpolateValue(in[key], values, address, key)
		out[key] = value
		diagnostics = append(diagnostics, diags...)
	}
	return out, diagnostics
}

func interpolateValue(value any, values map[string]resolvedValueInternal, address, attrPath string) (any, []ValueDiagnostic) {
	switch typed := value.(type) {
	case string:
		matches := variableRefPattern.FindAllStringSubmatch(typed, -1)
		if len(matches) == 0 {
			return typed, nil
		}
		if len(matches) == 1 && matches[0][0] == typed {
			resolved, ok := values[matches[0][1]]
			if !ok {
				return typed, []ValueDiagnostic{{Code: "values.reference_unknown", Severity: "error", Message: fmt.Sprintf("%s references unknown variable %s", attrPath, matches[0][1]), Name: matches[0][1], Path: address}}
			}
			if resolved.Sensitive {
				return redact.Value, nil
			}
			return resolved.raw, nil
		}
		out := typed
		for _, match := range matches {
			resolved, ok := values[match[1]]
			if !ok {
				return typed, []ValueDiagnostic{{Code: "values.reference_unknown", Severity: "error", Message: fmt.Sprintf("%s references unknown variable %s", attrPath, match[1]), Name: match[1], Path: address}}
			}
			replacement := fmt.Sprint(resolved.raw)
			if resolved.Sensitive {
				replacement = redact.Value
			}
			out = strings.ReplaceAll(out, match[0], replacement)
		}
		return out, nil
	case map[string]any:
		out := map[string]any{}
		var diagnostics []ValueDiagnostic
		for _, key := range sortedKeys(typed) {
			resolved, diags := interpolateValue(typed[key], values, address, attrPath+"."+key)
			out[key] = resolved
			diagnostics = append(diagnostics, diags...)
		}
		return out, diagnostics
	case []any:
		out := make([]any, len(typed))
		var diagnostics []ValueDiagnostic
		for i, item := range typed {
			resolved, diags := interpolateValue(item, values, address, fmt.Sprintf("%s[%d]", attrPath, i))
			out[i] = resolved
			diagnostics = append(diagnostics, diags...)
		}
		return out, diagnostics
	default:
		return value, nil
	}
}

func interpolateString(value string, values map[string]resolvedValueInternal, address, attrPath string) (string, []ValueDiagnostic) {
	resolved, diagnostics := interpolateValue(value, values, address, attrPath)
	if text, ok := resolved.(string); ok {
		return text, diagnostics
	}
	return fmt.Sprint(resolved), diagnostics
}

func valueMatchesType(value any, typ string) bool {
	switch strings.TrimSpace(typ) {
	case "", "any":
		return true
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		switch value.(type) {
		case int, int64, float64, float32, json.Number:
			return true
		default:
			return false
		}
	case "bool":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "list":
		_, ok := value.([]any)
		return ok
	default:
		return true
	}
}

func resolveValuePath(projectDir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) || projectDir == "" {
		return path
	}
	return filepath.Join(projectDir, path)
}

func inputsDigest(inputs ResolvedInputs) string {
	payload := struct {
		Version string          `json:"version"`
		Values  []ResolvedValue `json:"values,omitempty"`
	}{Version: inputs.Version, Values: inputs.Values}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return digest.SHA256String(data)
}

func valueDigest(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return digest.SHA256String(data)
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
