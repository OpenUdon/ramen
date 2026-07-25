package ansibleconvert

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Playbook is the parsed intermediate representation of an Ansible playbook
// file: an ordered list of plays.
type Playbook struct {
	Plays []*Play
}

// Play holds one play's target, variables, tasks, and handlers.
type Play struct {
	Name       string
	Hosts      string
	Vars       map[string]any
	VarsFiles  []string
	Roles      []*RoleRef
	PreTasks   []*Task
	Tasks      []*Task
	PostTasks  []*Task
	Handlers   []*Task
	SourceFile string
	Line       int
	Column     int
}

// Task is one task (or handler) entry. Exactly one of Module or Block is set
// for lowerable tasks; StaticImport and ImportRole mark static constructs that
// are resolved before lowering, while DynamicInclude marks include_* constructs
// that cannot be statically resolved.
type Task struct {
	Name                       string
	Module                     string // FQCN, normalized (short names get ansible.builtin.)
	Args                       map[string]any
	FreeForm                   string // free-form module argument (shell/command style)
	When                       []string
	Loop                       any // string template or YAML list literal
	Register                   string
	Notify                     []string
	Listen                     []string
	Tags                       []string
	Vars                       map[string]any
	ChangedWhen                []string
	FailedWhen                 []string
	Until                      []string
	Retries                    *int
	Delay                      *float64
	IgnoreErrors               bool
	AnyErrorsFatal             bool
	Throttle                   *int
	Block                      []*Task
	Rescue                     []*Task
	Always                     []*Task
	StaticImport               string
	ImportRole                 string
	DynamicInclude             string       // the include_* key that made this task dynamic
	TodoDirectives             []string     // directives recorded for review (until/retries/...)
	HardDirectives             []string     // directives that block conversion (delegate_to/...)
	InfoDirectives             []string     // runtime-owned directives noted for review (become/...)
	StrictDirectiveDiagnostics []Diagnostic // parsed control directives that must fail closed
	PlayName                   string
	Section                    string
	Role                       string
	ImportStack                []string
	SourceFile                 string
	Line                       int
	Column                     int
}

type RoleRef struct {
	Name       string
	SourceFile string
	Line       int
	Column     int
}

// taskDirectives are playbook keys that are task directives rather than module
// invocations.
var taskDirectives = map[string]bool{
	"name": true, "when": true, "loop": true, "with_items": true,
	"register": true, "notify": true, "block": true, "rescue": true,
	"always": true, "vars": true, "tags": true, "args": true,
	"become": true, "become_user": true, "become_method": true,
	"delegate_to": true, "run_once": true, "changed_when": true,
	"failed_when": true, "until": true, "retries": true, "delay": true,
	"environment": true, "no_log": true, "ignore_errors": true,
	"listen": true, "any_errors_fatal": true, "throttle": true,
	"import_tasks": true, "import_role": true, "include_tasks": true,
	"include_role": true, "include_vars": true,
}

var dynamicIncludeKeys = []string{"include_tasks", "include_role", "include_vars"}

// ParsePlaybook parses playbook YAML into the IR. Parsing is shape-only; no
// Jinja2 evaluation, no file inclusion, no module execution.
func ParsePlaybook(data []byte) (*Playbook, []Diagnostic, error) {
	return parsePlaybookFile(data, "")
}

func parsePlaybookFile(data []byte, sourceFile string) (*Playbook, []Diagnostic, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("playbook is not a YAML list of plays: %w", err)
	}
	node := documentNode(&root)
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil, nil, fmt.Errorf("playbook is not a YAML list of plays")
	}
	if len(node.Content) == 0 {
		return nil, nil, fmt.Errorf("playbook contains no plays")
	}
	pb := &Playbook{}
	var diags []Diagnostic
	for i, playNode := range node.Content {
		rawPlay, err := mapFromNode(playNode)
		if err != nil {
			return nil, diags, fmt.Errorf("play %d: %w", i+1, err)
		}
		play := &Play{
			Name:       stringValue(rawPlay.value("name")),
			Hosts:      stringValue(rawPlay.value("hosts")),
			Vars:       mapValue(rawPlay.value("vars")),
			VarsFiles:  stringListValue(rawPlay.value("vars_files")),
			SourceFile: sourceFile,
			Line:       playNode.Line,
			Column:     playNode.Column,
		}
		if play.Name == "" {
			play.Name = fmt.Sprintf("play_%d", i+1)
		}
		play.Roles = parseRoleRefs(rawPlay.node("roles"), sourceFile, play.Name, &diags)
		var perr error
		play.PreTasks, perr = parseTaskList(rawPlay.node("pre_tasks"), sourceFile, play.Name, "pre_tasks", "", nil, &diags)
		if perr != nil {
			return nil, diags, fmt.Errorf("play %q pre_tasks: %w", play.Name, perr)
		}
		play.Tasks, perr = parseTaskList(rawPlay.node("tasks"), sourceFile, play.Name, "tasks", "", nil, &diags)
		if perr != nil {
			return nil, diags, fmt.Errorf("play %q tasks: %w", play.Name, perr)
		}
		play.PostTasks, perr = parseTaskList(rawPlay.node("post_tasks"), sourceFile, play.Name, "post_tasks", "", nil, &diags)
		if perr != nil {
			return nil, diags, fmt.Errorf("play %q post_tasks: %w", play.Name, perr)
		}
		play.Handlers, perr = parseTaskList(rawPlay.node("handlers"), sourceFile, play.Name, "handlers", "", nil, &diags)
		if perr != nil {
			return nil, diags, fmt.Errorf("play %q handlers: %w", play.Name, perr)
		}
		pb.Plays = append(pb.Plays, play)
	}
	return pb, diags, nil
}

func hasNonEmptyYAMLValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	case string:
		return strings.TrimSpace(x) != ""
	default:
		return true
	}
}

func parseTaskList(node *yaml.Node, sourceFile, playName, section, role string, importStack []string, diags *[]Diagnostic) ([]*Task, error) {
	if node == nil {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("expected a YAML list, got %s", yamlKind(node.Kind))
	}
	var tasks []*Task
	for i, entry := range node.Content {
		task, err := parseTask(entry, sourceFile, playName, section, role, importStack, diags)
		if err != nil {
			return nil, fmt.Errorf("task %d: %w", i+1, err)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func parseTask(node *yaml.Node, sourceFile, playName, section, role string, importStack []string, diags *[]Diagnostic) (*Task, error) {
	m, err := mapFromNode(node)
	if err != nil {
		return nil, err
	}
	task := &Task{
		Name:        stringValue(m.value("name")),
		Register:    stringValue(m.value("register")),
		When:        stringListValue(m.value("when")),
		Notify:      stringListValue(m.value("notify")),
		Listen:      stringListValue(m.value("listen")),
		Tags:        stringListValue(m.value("tags")),
		Vars:        mapValue(m.value("vars")),
		ChangedWhen: stringListValue(m.value("changed_when")),
		FailedWhen:  stringListValue(m.value("failed_when")),
		Until:       stringListValue(m.value("until")),
		PlayName:    playName,
		Section:     section,
		Role:        role,
		ImportStack: append([]string(nil), importStack...),
		SourceFile:  sourceFile,
		Line:        node.Line,
		Column:      node.Column,
	}
	if m.has("loop") && m.has("with_items") {
		task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, strictDirectiveDiagnostic(
			task.Name,
			"loop/with_items",
			"cannot be specified together because their precedence is ambiguous",
			"",
		))
	} else if loop := m.value("loop"); loop != nil {
		task.Loop = loop
	} else if loop := m.value("with_items"); loop != nil {
		task.Loop = loop
	}
	for _, directive := range unsupportedWithDirectives(m) {
		task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, strictDirectiveDiagnostic(task.Name, directive, "uses a legacy loop form that is not lowered; use loop or with_items for static item lists", ""))
	}
	if m.has("retries") {
		value, ok, reason := staticIntDirective(m.value("retries"), true)
		if ok {
			task.Retries = &value
		} else {
			task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, strictDirectiveDiagnostic(task.Name, "retries", "must be a static positive integer", reason))
		}
	}
	if m.has("delay") {
		value, ok, reason := staticFloatDirective(m.value("delay"), false)
		if ok {
			task.Delay = &value
		} else {
			task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, strictDirectiveDiagnostic(task.Name, "delay", "must be a static non-negative number", reason))
		}
	}
	if m.has("ignore_errors") {
		value, ok, reason := staticBoolDirective(m.value("ignore_errors"))
		if ok {
			task.IgnoreErrors = value
		} else {
			task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, strictDirectiveDiagnostic(task.Name, "ignore_errors", "must be a static boolean", reason))
		}
	}
	if m.has("any_errors_fatal") {
		value, ok, reason := staticBoolDirective(m.value("any_errors_fatal"))
		if ok {
			task.AnyErrorsFatal = value
		} else {
			task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, strictDirectiveDiagnostic(task.Name, "any_errors_fatal", "must be a static boolean", reason))
		}
	}
	if m.has("throttle") {
		value, ok, reason := staticIntDirective(m.value("throttle"), true)
		if ok {
			task.Throttle = &value
		} else {
			task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, strictDirectiveDiagnostic(task.Name, "throttle", "must be a static positive integer", reason))
		}
	}

	for _, key := range dynamicIncludeKeys {
		if m.has(key) {
			task.DynamicInclude = key
			return task, nil
		}
	}
	if m.has("import_tasks") {
		task.StaticImport = stringValue(m.value("import_tasks"))
		return task, nil
	}
	if m.has("import_role") {
		task.ImportRole = roleNameFromValue(m.value("import_role"))
		return task, nil
	}

	if rawBlock := m.node("block"); rawBlock != nil {
		var err error
		task.Block, err = parseTaskList(rawBlock, sourceFile, playName, section, role, importStack, diags)
		if err != nil {
			return nil, fmt.Errorf("block: %w", err)
		}
		task.Rescue, err = parseTaskList(m.node("rescue"), sourceFile, playName, section, role, importStack, diags)
		if err != nil {
			return nil, fmt.Errorf("rescue: %w", err)
		}
		task.Always, err = parseTaskList(m.node("always"), sourceFile, playName, section, role, importStack, diags)
		if err != nil {
			return nil, fmt.Errorf("always: %w", err)
		}
	}

	// Record directive posture for review.
	for key := range m.values {
		switch key {
		case "become", "become_user", "become_method", "environment", "no_log":
			task.InfoDirectives = append(task.InfoDirectives, key)
		case "delegate_to", "run_once":
			task.HardDirectives = append(task.HardDirectives, key)
		}
	}
	sort.Strings(task.InfoDirectives)
	sort.Strings(task.HardDirectives)
	sort.Strings(task.TodoDirectives)

	// The remaining non-directive key is the module invocation.
	var moduleKeys []string
	for key := range m.values {
		if !isTaskDirectiveKey(key) && task.DynamicInclude == "" {
			moduleKeys = append(moduleKeys, key)
		}
	}
	sort.Strings(moduleKeys)
	if task.Block != nil {
		return task, nil
	}
	if len(moduleKeys) == 0 {
		task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, taskShapeDiagnostic(task.Name, "has no module invocation"))
		return task, nil
	}
	if len(moduleKeys) > 1 {
		task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, taskShapeDiagnostic(task.Name, fmt.Sprintf("has multiple module keys: %v", moduleKeys)))
		return task, nil
	}
	task.Module = normalizeFQCN(moduleKeys[0])
	switch args := m.value(moduleKeys[0]).(type) {
	case map[string]any:
		task.Args = args
	case string:
		task.FreeForm = args
	case nil:
		task.Args = map[string]any{}
	default:
		task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, taskShapeDiagnostic(task.Name, fmt.Sprintf("module %s has unsupported argument shape %T", task.Module, args)))
		return task, nil
	}
	if extra := mapValue(m.value("args")); extra != nil {
		if task.Args == nil {
			task.Args = map[string]any{}
		}
		for k, v := range extra {
			task.Args[k] = v
		}
	}
	return task, nil
}

func isTaskDirectiveKey(key string) bool {
	return taskDirectives[key] || strings.HasPrefix(key, "with_")
}

func unsupportedWithDirectives(m nodeMap) []string {
	var directives []string
	for key := range m.values {
		if strings.HasPrefix(key, "with_") && key != "with_items" {
			directives = append(directives, key)
		}
	}
	sort.Strings(directives)
	return directives
}

type nodeMap struct {
	values map[string]any
	nodes  map[string]*yaml.Node
}

func (m nodeMap) value(key string) any {
	return m.values[key]
}

func (m nodeMap) node(key string) *yaml.Node {
	return m.nodes[key]
}

func (m nodeMap) has(key string) bool {
	_, ok := m.nodes[key]
	return ok
}

func documentNode(root *yaml.Node) *yaml.Node {
	if root == nil {
		return nil
	}
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		return root.Content[0]
	}
	return root
}

func mapFromNode(node *yaml.Node) (nodeMap, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nodeMap{}, fmt.Errorf("expected a mapping, got %s", yamlKind(0))
	}
	out := nodeMap{values: map[string]any{}, nodes: map[string]*yaml.Node{}}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := strings.TrimSpace(node.Content[i].Value)
		var value any
		if err := node.Content[i+1].Decode(&value); err != nil {
			return nodeMap{}, fmt.Errorf("field %q: %w", key, err)
		}
		out.values[key] = value
		out.nodes[key] = node.Content[i+1]
	}
	return out, nil
}

func parseRoleRefs(node *yaml.Node, sourceFile, playName string, diags *[]Diagnostic) []*RoleRef {
	if node == nil {
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		*diags = append(*diags, Diagnostic{Code: CodePlaybookShape, Severity: "error", StrictFailure: true, Task: playName,
			Message: fmt.Sprintf("play roles must be a YAML list, got %s", yamlKind(node.Kind))})
		return nil
	}
	var out []*RoleRef
	for _, item := range node.Content {
		name := roleNameFromNode(item)
		if name == "" {
			*diags = append(*diags, Diagnostic{Code: CodePlaybookShape, Severity: "error", StrictFailure: true, Task: playName,
				Message: "play role entry must be a static string or mapping with role/name"})
			continue
		}
		out = append(out, &RoleRef{Name: name, SourceFile: sourceFile, Line: item.Line, Column: item.Column})
	}
	return out
}

func roleNameFromNode(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	var value any
	if err := node.Decode(&value); err != nil {
		return ""
	}
	return roleNameFromValue(value)
}

func roleNameFromValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		for _, key := range []string{"name", "role"} {
			if s := stringValue(v[key]); s != "" {
				return s
			}
		}
	}
	return ""
}

func yamlKind(kind yaml.Kind) string {
	switch kind {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return "unknown"
	}
}

// normalizeFQCN expands short module names to the ansible.builtin collection.
func normalizeFQCN(name string) string {
	if strings.Count(name, ".") >= 2 {
		return name
	}
	return "ansible.builtin." + name
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func stringListValue(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return []string{strings.TrimSpace(x)}
	case bool:
		return []string{fmt.Sprintf("%v", x)}
	case []any:
		var out []string
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, strings.TrimSpace(s))
			} else {
				out = append(out, fmt.Sprintf("%v", item))
			}
		}
		return out
	default:
		return []string{fmt.Sprintf("%v", x)}
	}
}

func mapValue(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func staticIntDirective(v any, positive bool) (int, bool, string) {
	var value int
	switch x := v.(type) {
	case int:
		value = x
	case int64:
		value = int(x)
		if int64(value) != x {
			return 0, false, "value is outside the supported integer range"
		}
	case float64:
		value = int(x)
		if float64(value) != x {
			return 0, false, "value must be an integer"
		}
	case string:
		text := strings.TrimSpace(x)
		if isTemplateString(text) {
			return 0, false, "templated values are not static"
		}
		parsed, err := strconv.ParseInt(text, 10, 0)
		if err != nil {
			return 0, false, "value is not an integer"
		}
		value = int(parsed)
		if int64(value) != parsed {
			return 0, false, "value is outside the supported integer range"
		}
	default:
		return 0, false, "value is not an integer"
	}
	if positive && value <= 0 {
		return 0, false, "value must be greater than zero"
	}
	return value, true, ""
}

func staticFloatDirective(v any, positive bool) (float64, bool, string) {
	var value float64
	switch x := v.(type) {
	case int:
		value = float64(x)
	case int64:
		value = float64(x)
	case float64:
		value = x
	case string:
		text := strings.TrimSpace(x)
		if isTemplateString(text) {
			return 0, false, "templated values are not static"
		}
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return 0, false, "value is not a number"
		}
		value = parsed
	default:
		return 0, false, "value is not a number"
	}
	if positive && value <= 0 {
		return 0, false, "value must be greater than zero"
	}
	if !positive && value < 0 {
		return 0, false, "value must be non-negative"
	}
	return value, true, ""
}

func staticBoolDirective(v any) (bool, bool, string) {
	switch x := v.(type) {
	case bool:
		return x, true, ""
	case string:
		text := strings.TrimSpace(x)
		if isTemplateString(text) {
			return false, false, "templated values are not static"
		}
		switch strings.ToLower(text) {
		case "true", "yes", "on", "1":
			return true, true, ""
		case "false", "no", "off", "0":
			return false, true, ""
		default:
			return false, false, "value is not a recognized boolean"
		}
	default:
		return false, false, "value is not a boolean"
	}
}

func isTemplateString(text string) bool {
	return strings.Contains(text, "{{") || strings.Contains(text, "}}")
}

func strictDirectiveDiagnostic(taskName, directive, requirement, reason string) Diagnostic {
	message := fmt.Sprintf("directive %q %s; the task was not lowered", directive, requirement)
	if strings.TrimSpace(reason) != "" {
		message = fmt.Sprintf("%s (%s)", message, reason)
	}
	return Diagnostic{Code: CodeDirectiveTodo, Severity: "error", StrictFailure: true, Task: taskName, Message: message}
}

func taskShapeDiagnostic(taskName, message string) Diagnostic {
	return Diagnostic{Code: CodePlaybookShape, Severity: "error", StrictFailure: true, Task: taskName,
		Message: fmt.Sprintf("task %q %s; the task was not lowered", taskName, message)}
}
