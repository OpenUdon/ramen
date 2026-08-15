package ansibleconvert

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxExtraVarsBytes = 8 << 20

func applyExtraVars(pb *Playbook, inputs []string) []Diagnostic {
	values, diags, valid := loadExtraVars(inputs)
	if !valid {
		for _, play := range pb.Plays {
			play.StaticScopeFailed = true
		}
		return diags
	}
	for _, play := range pb.Plays {
		if play.Vars == nil {
			play.Vars = map[string]any{}
		}
		if play.VarPriorities == nil {
			play.VarPriorities = map[string]int{}
		}
		if play.VarSources == nil {
			play.VarSources = map[string]string{}
		}
		for name, value := range values {
			play.Vars[name] = value
			play.VarPriorities[name] = varPriorityExtra
			play.VarSources[name] = "extra vars"
		}
	}
	return diags
}

func loadExtraVars(inputs []string) (map[string]any, []Diagnostic, bool) {
	values := map[string]any{}
	sources := map[string]string{}
	valid := true
	var diags []Diagnostic
	for _, input := range inputs {
		input = strings.TrimSpace(input)
		var current map[string]any
		var source string
		var err error
		if strings.HasPrefix(input, "@") {
			path := strings.TrimSpace(strings.TrimPrefix(input, "@"))
			source = "@" + filepath.Base(filepath.Clean(path))
			current, err = readExtraVarsFile(path)
		} else {
			name, raw, ok := strings.Cut(input, "=")
			name = strings.TrimSpace(name)
			source = "inline extra var " + name
			if !ok || !identRE.MatchString(name) {
				err = fmt.Errorf("inline extra vars must use an identifier NAME=VALUE")
			} else {
				var value any
				if decodeErr := yaml.Unmarshal([]byte(raw), &value); decodeErr != nil || !isStaticAnsibleValue(value) {
					err = fmt.Errorf("inline extra var %q must contain a literal static YAML value", name)
				} else {
					current = map[string]any{name: value}
				}
			}
		}
		if err != nil {
			diags = append(diags, Diagnostic{Code: CodeExtraVarsInvalid, Severity: "error", StrictFailure: true,
				Message: fmt.Sprintf("extra-vars input %q is invalid: %v", safeExtraVarsLabel(input), err)})
			valid = false
			continue
		}
		for name, value := range current {
			if sensitivePath, sensitive := sensitiveVariablePath(name, value); sensitive {
				diags = append(diags, Diagnostic{Code: CodeNoLogLiteral, Severity: "error", StrictFailure: true,
					Message: fmt.Sprintf("extra variable %q contains credential-shaped key %q; use a symbolic runtime credential binding instead of embedding a literal", name, sensitivePath)})
				valid = false
				continue
			}
			if existing, ok := values[name]; ok && !reflect.DeepEqual(existing, value) {
				diags = append(diags, Diagnostic{Code: CodeVariableConflict, Severity: "error", StrictFailure: true,
					Message: fmt.Sprintf("extra variable %q conflicts between %s and %s at equal static precedence", name, sources[name], source)})
				valid = false
				continue
			}
			values[name] = value
			sources[name] = source
		}
	}
	return values, diags, valid
}

func readExtraVarsFile(path string) (map[string]any, error) {
	label := filepath.Base(filepath.Clean(path))
	if path == "" || path == "." {
		return nil, fmt.Errorf("extra-vars file path is empty")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("file %q could not be inspected: %s", label, inventoryIOReason(err))
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("file %q must be a regular file; directories and symlinks are not read", label)
	}
	if info.Size() > maxExtraVarsBytes {
		return nil, fmt.Errorf("file %q exceeds the %d-byte static limit", label, maxExtraVarsBytes)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".json":
	default:
		return nil, fmt.Errorf("file %q has unsupported extension (want .yaml, .yml, or .json)", label)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("file %q could not be read: %s", label, inventoryIOReason(err))
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("file %q is not valid YAML/JSON", label)
	}
	node := documentNode(&root)
	if node == nil {
		return nil, fmt.Errorf("file %q must contain a static variable map", label)
	}
	values, err := decodeInventoryMap(node, fmt.Sprintf("extra-vars file %q", label))
	if err != nil {
		return nil, err
	}
	return values, nil
}

func safeExtraVarsLabel(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "@") {
		return "@" + filepath.Base(filepath.Clean(strings.TrimSpace(strings.TrimPrefix(input, "@"))))
	}
	name, _, _ := strings.Cut(input, "=")
	name = strings.TrimSpace(name)
	if name == "" {
		return "inline"
	}
	return name
}

func isSensitiveVariableName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, marker := range []string{"password", "passwd", "secret", "token", "credential", "private_key", "api_key"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func sensitiveVariablePath(name string, value any) (string, bool) {
	if isSensitiveVariableName(name) {
		return name, true
	}
	return sensitiveValuePath(value, name)
}

func sensitiveValuePath(value any, path string) (string, bool) {
	switch value := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childPath := path + "." + key
			if isSensitiveVariableName(key) {
				return childPath, true
			}
			if nestedPath, sensitive := sensitiveValuePath(value[key], childPath); sensitive {
				return nestedPath, true
			}
		}
	case []any:
		for index, item := range value {
			if nestedPath, sensitive := sensitiveValuePath(item, fmt.Sprintf("%s[%d]", path, index)); sensitive {
				return nestedPath, true
			}
		}
	}
	return "", false
}
