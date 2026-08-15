package ansibleconvert

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	maxInventoryBytes  = 8 << 20
	maxInventoryFiles  = 32
	maxInventoryHosts  = 4096
	maxInventoryGroups = 1024
)

type staticInventory struct {
	hosts   map[string]map[string]any
	groups  map[string]*staticInventoryGroup
	invalid bool
}

type staticInventoryGroup struct {
	vars     map[string]any
	hosts    map[string]bool
	children map[string]bool
}

func newStaticInventory() *staticInventory {
	return &staticInventory{hosts: map[string]map[string]any{}, groups: map[string]*staticInventoryGroup{}}
}

func (inv *staticInventory) group(name string) *staticInventoryGroup {
	group := inv.groups[name]
	if group == nil {
		group = &staticInventoryGroup{vars: map[string]any{}, hosts: map[string]bool{}, children: map[string]bool{}}
		inv.groups[name] = group
	}
	return group
}

func loadStaticInventory(paths []string) (*staticInventory, []Diagnostic) {
	if len(paths) == 0 {
		return nil, nil
	}
	inv := newStaticInventory()
	if len(paths) > maxInventoryFiles {
		inv.invalid = true
		return inv, []Diagnostic{inventoryDiagnostic(fmt.Sprintf("%d inventory inputs exceed the static limit of %d", len(paths), maxInventoryFiles))}
	}
	var diags []Diagnostic
	for _, path := range paths {
		label := filepath.Base(filepath.Clean(path))
		info, err := os.Lstat(path)
		if err != nil {
			diags = append(diags, inventoryDiagnostic(fmt.Sprintf("inventory %q could not be inspected: %s", label, inventoryIOReason(err))))
			inv.invalid = true
			continue
		}
		if !info.Mode().IsRegular() {
			diags = append(diags, inventoryDiagnostic(fmt.Sprintf("inventory %q must be a regular file; directories and symlinks are not read", label)))
			inv.invalid = true
			continue
		}
		if info.Size() > maxInventoryBytes {
			diags = append(diags, inventoryDiagnostic(fmt.Sprintf("inventory %q is %d bytes, exceeding the %d-byte static limit", label, info.Size(), maxInventoryBytes)))
			inv.invalid = true
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			diags = append(diags, inventoryDiagnostic(fmt.Sprintf("inventory %q could not be read: %s", label, inventoryIOReason(err))))
			inv.invalid = true
			continue
		}
		var parseErr error
		switch strings.ToLower(filepath.Ext(path)) {
		case ".ini":
			parseErr = parseINIInventory(inv, data)
		case ".yaml", ".yml", ".json":
			parseErr = parseYAMLInventory(inv, data)
		default:
			parseErr = fmt.Errorf("unsupported extension %q (want .ini, .yaml, .yml, or .json)", filepath.Ext(path))
		}
		if parseErr != nil {
			diags = append(diags, inventoryDiagnostic(fmt.Sprintf("inventory %q is not supported static inventory: %v", label, parseErr)))
			inv.invalid = true
		}
	}
	if len(inv.hosts) > maxInventoryHosts {
		diags = append(diags, inventoryDiagnostic(fmt.Sprintf("inventory contains %d hosts, exceeding the static limit of %d", len(inv.hosts), maxInventoryHosts)))
		inv.invalid = true
	}
	if len(inv.groups) > maxInventoryGroups {
		diags = append(diags, inventoryDiagnostic(fmt.Sprintf("inventory contains %d groups, exceeding the static limit of %d", len(inv.groups), maxInventoryGroups)))
		inv.invalid = true
	}
	if !inv.invalid {
		for _, name := range sortedGroupNames(inv.groups) {
			if err := inv.collectGroupHosts(name, map[string]bool{}, map[string]bool{}); err != nil {
				diags = append(diags, inventoryDiagnostic(fmt.Sprintf("inventory group graph is invalid: %v", err)))
				inv.invalid = true
				break
			}
		}
	}
	return inv, diags
}

func inventoryDiagnostic(message string) Diagnostic {
	return Diagnostic{Code: CodeInventoryInvalid, Severity: "error", StrictFailure: true, Message: message}
}

func inventoryIOReason(err error) string {
	switch {
	case os.IsNotExist(err):
		return "file does not exist"
	case os.IsPermission(err):
		return "permission denied"
	default:
		return "I/O error"
	}
}

func applyInventoryTargets(pb *Playbook, inv *staticInventory, supplied bool) []Diagnostic {
	if !supplied {
		return nil
	}
	var diags []Diagnostic
	for _, play := range pb.Plays {
		if !playNeedsHostFanOut(play.Hosts) {
			play.InventoryResolved = true
			continue
		}
		if inv == nil || inv.invalid {
			play.InventoryFailed = true
			continue
		}
		hosts, err := inv.selectHosts(strings.TrimSpace(play.Hosts))
		if err != nil {
			play.InventoryFailed = true
			diags = append(diags, Diagnostic{Code: CodeInventoryPattern, Severity: "error", StrictFailure: true, Task: play.Name,
				Message: fmt.Sprintf("play target %q cannot be resolved from static inventory: %v", strings.TrimSpace(play.Hosts), err)})
			continue
		}
		play.InventoryHosts = hosts
		play.InventoryResolved = true
	}
	return diags
}

func (inv *staticInventory) selectHosts(pattern string) ([]string, error) {
	if pattern == "" {
		return nil, fmt.Errorf("empty host pattern")
	}
	if containsTemplate(pattern) || strings.ContainsAny(pattern, ",:*?!&[]") {
		return nil, fmt.Errorf("only all, one exact host, or one exact group is supported")
	}
	if pattern == "all" {
		return sortedBoolKeys(hostNames(inv.hosts)), nil
	}
	_, hostMatch := inv.hosts[pattern]
	_, groupMatch := inv.groups[pattern]
	if hostMatch && groupMatch {
		return nil, fmt.Errorf("name is ambiguous because it identifies both a host and a group")
	}
	if hostMatch {
		return []string{pattern}, nil
	}
	if !groupMatch {
		return nil, fmt.Errorf("no exact host or group has that name")
	}
	selected := map[string]bool{}
	if err := inv.collectGroupHosts(pattern, selected, map[string]bool{}); err != nil {
		return nil, err
	}
	hosts := sortedBoolKeys(selected)
	if len(hosts) == 0 {
		return nil, fmt.Errorf("group %q selects no hosts", pattern)
	}
	return hosts, nil
}

func (inv *staticInventory) collectGroupHosts(name string, selected, visiting map[string]bool) error {
	if visiting[name] {
		return fmt.Errorf("group children contain a cycle at %q", name)
	}
	group := inv.groups[name]
	if group == nil {
		return fmt.Errorf("child group %q is not defined", name)
	}
	visiting[name] = true
	for host := range group.hosts {
		selected[host] = true
	}
	for child := range group.children {
		if err := inv.collectGroupHosts(child, selected, visiting); err != nil {
			return err
		}
	}
	delete(visiting, name)
	return nil
}

func parseINIInventory(inv *staticInventory, data []byte) error {
	section := "all"
	mode := "hosts"
	inv.group("all")
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(stripINIComment(scanner.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			header := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			section, mode = header, "hosts"
			if base, suffix, ok := strings.Cut(header, ":"); ok {
				section = strings.TrimSpace(base)
				mode = strings.TrimSpace(suffix)
				if mode != "vars" && mode != "children" {
					return fmt.Errorf("line %d section suffix %q is unsupported", lineNo, mode)
				}
			}
			if err := validInventoryName(section, "group"); err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			inv.group(section)
			continue
		}
		switch mode {
		case "vars":
			name, raw, ok := strings.Cut(line, "=")
			if !ok || strings.TrimSpace(name) == "" {
				return fmt.Errorf("line %d group variable must be NAME=VALUE", lineNo)
			}
			value, err := parseInventoryScalar(strings.TrimSpace(raw))
			if err != nil {
				return fmt.Errorf("line %d variable %q: %w", lineNo, strings.TrimSpace(name), err)
			}
			if err := mergeInventoryVar(inv.group(section).vars, strings.TrimSpace(name), value); err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
		case "children":
			child := strings.TrimSpace(line)
			if err := validInventoryName(child, "group"); err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			inv.group(section).children[child] = true
			inv.group(child)
		default:
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			host := fields[0]
			if err := validInventoryName(host, "host"); err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			hostVars := inv.hosts[host]
			if hostVars == nil {
				hostVars = map[string]any{}
				inv.hosts[host] = hostVars
			}
			inv.group(section).hosts[host] = true
			for _, field := range fields[1:] {
				name, raw, ok := strings.Cut(field, "=")
				if !ok || name == "" {
					return fmt.Errorf("line %d host variables must be NAME=VALUE tokens", lineNo)
				}
				value, err := parseInventoryScalar(raw)
				if err != nil {
					return fmt.Errorf("line %d host variable %q: %w", lineNo, name, err)
				}
				if err := mergeInventoryVar(hostVars, name, value); err != nil {
					return fmt.Errorf("line %d: %w", lineNo, err)
				}
			}
		}
	}
	return scanner.Err()
}

func parseYAMLInventory(inv *staticInventory, data []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	node := documentNode(&root)
	if node == nil || node.Kind != yaml.MappingNode {
		return fmt.Errorf("root must be a mapping of inventory groups")
	}
	seen := map[string]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		name := strings.TrimSpace(node.Content[i].Value)
		if seen[name] {
			return fmt.Errorf("group %q is duplicated", name)
		}
		seen[name] = true
		if err := parseYAMLInventoryGroup(inv, name, node.Content[i+1], ""); err != nil {
			return err
		}
	}
	return nil
}

func parseYAMLInventoryGroup(inv *staticInventory, name string, node *yaml.Node, parent string) error {
	if err := validInventoryName(name, "group"); err != nil {
		return err
	}
	group := inv.group(name)
	if parent != "" {
		inv.group(parent).children[name] = true
	}
	if node.Kind == yaml.ScalarNode && (node.Tag == "!!null" || strings.TrimSpace(node.Value) == "") {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("group %q must be a mapping", name)
	}
	seen := map[string]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		field := strings.TrimSpace(node.Content[i].Value)
		value := node.Content[i+1]
		if seen[field] {
			return fmt.Errorf("group %q field %q is duplicated", name, field)
		}
		seen[field] = true
		switch field {
		case "vars":
			vars, err := decodeInventoryMap(value, fmt.Sprintf("group %q vars", name))
			if err != nil {
				return err
			}
			for key, item := range vars {
				if err := mergeInventoryVar(group.vars, key, item); err != nil {
					return fmt.Errorf("group %q: %w", name, err)
				}
			}
		case "hosts":
			if value.Kind != yaml.MappingNode {
				return fmt.Errorf("group %q hosts must be a mapping", name)
			}
			hostSeen := map[string]bool{}
			for j := 0; j+1 < len(value.Content); j += 2 {
				host := strings.TrimSpace(value.Content[j].Value)
				if hostSeen[host] {
					return fmt.Errorf("group %q host %q is duplicated", name, host)
				}
				hostSeen[host] = true
				if err := validInventoryName(host, "host"); err != nil {
					return err
				}
				hostVars, err := decodeNullableInventoryMap(value.Content[j+1], fmt.Sprintf("host %q variables", host))
				if err != nil {
					return err
				}
				current := inv.hosts[host]
				if current == nil {
					current = map[string]any{}
					inv.hosts[host] = current
				}
				for key, item := range hostVars {
					if err := mergeInventoryVar(current, key, item); err != nil {
						return fmt.Errorf("host %q: %w", host, err)
					}
				}
				group.hosts[host] = true
			}
		case "children":
			if value.Kind != yaml.MappingNode {
				return fmt.Errorf("group %q children must be a mapping", name)
			}
			childSeen := map[string]bool{}
			for j := 0; j+1 < len(value.Content); j += 2 {
				child := strings.TrimSpace(value.Content[j].Value)
				if childSeen[child] {
					return fmt.Errorf("group %q child %q is duplicated", name, child)
				}
				childSeen[child] = true
				if err := parseYAMLInventoryGroup(inv, child, value.Content[j+1], name); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("group %q has unsupported field %q", name, field)
		}
	}
	return nil
}

func decodeInventoryMap(node *yaml.Node, label string) (map[string]any, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a mapping", label)
	}
	seen := map[string]bool{}
	out := map[string]any{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		name := strings.TrimSpace(node.Content[i].Value)
		if !identRE.MatchString(name) {
			return nil, fmt.Errorf("%s variable name %q is invalid", label, name)
		}
		if seen[name] {
			return nil, fmt.Errorf("%s variable %q is duplicated", label, name)
		}
		seen[name] = true
		var value any
		if err := node.Content[i+1].Decode(&value); err != nil {
			return nil, fmt.Errorf("%s variable %q: %w", label, name, err)
		}
		if !isStaticAnsibleValue(value) {
			return nil, fmt.Errorf("%s variable %q is not a literal static value", label, name)
		}
		out[name] = value
	}
	return out, nil
}

func decodeNullableInventoryMap(node *yaml.Node, label string) (map[string]any, error) {
	if node.Kind == yaml.ScalarNode && (node.Tag == "!!null" || strings.TrimSpace(node.Value) == "") {
		return map[string]any{}, nil
	}
	return decodeInventoryMap(node, label)
}

func parseInventoryScalar(raw string) (any, error) {
	if raw == "" {
		return "", nil
	}
	var value any
	if err := yaml.Unmarshal([]byte(raw), &value); err != nil {
		return nil, err
	}
	if !isStaticAnsibleValue(value) {
		return nil, fmt.Errorf("value is not a literal static scalar")
	}
	switch value.(type) {
	case []any, map[string]any:
		return nil, fmt.Errorf("INI values must be scalar")
	}
	return value, nil
}

func mergeInventoryVar(dst map[string]any, name string, value any) error {
	if !identRE.MatchString(name) {
		return fmt.Errorf("variable name %q is invalid", name)
	}
	if existing, ok := dst[name]; ok && !reflect.DeepEqual(existing, value) {
		return fmt.Errorf("variable %q has conflicting values at the same inventory precedence", name)
	}
	dst[name] = value
	return nil
}

func validInventoryName(name, kind string) error {
	name = strings.TrimSpace(name)
	if name == "" || containsTemplate(name) || strings.ContainsAny(name, "[],:*?!&") || strings.ContainsAny(name, " \t\r\n") {
		return fmt.Errorf("%s name %q is not an exact static name", kind, name)
	}
	return nil
}

func stripINIComment(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
		return ""
	}
	for _, marker := range []string{" #", " ;"} {
		if index := strings.Index(line, marker); index >= 0 {
			line = line[:index]
		}
	}
	return line
}

func hostNames(hosts map[string]map[string]any) map[string]bool {
	out := make(map[string]bool, len(hosts))
	for host := range hosts {
		out[host] = true
	}
	return out
}

func sortedBoolKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortedGroupNames(values map[string]*staticInventoryGroup) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
