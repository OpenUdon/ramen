package ansibleconvert

import (
	"fmt"
	"sort"
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
	Name      string
	Hosts     string
	Vars      map[string]any
	PreTasks  []*Task
	Tasks     []*Task
	PostTasks []*Task
	Handlers  []*Task
}

// Task is one task (or handler) entry. Exactly one of Module or Block is set
// for lowerable tasks; DynamicInclude marks include_* constructs that cannot
// be statically resolved.
type Task struct {
	Name           string
	Module         string // FQCN, normalized (short names get ansible.builtin.)
	Args           map[string]any
	FreeForm       string // free-form module argument (shell/command style)
	When           []string
	Loop           any // string template or YAML list literal
	Register       string
	Notify         []string
	Block          []*Task
	Rescue         []*Task
	Always         []*Task
	DynamicInclude string   // the include_* key that made this task dynamic
	TodoDirectives []string // directives recorded for review (until/retries/...)
	HardDirectives []string // directives that block conversion (delegate_to/...)
	InfoDirectives []string // runtime-owned directives noted for review (become/...)
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
}

var dynamicIncludeKeys = []string{"include_tasks", "include_role", "import_tasks", "import_role", "include_vars"}

// ParsePlaybook parses playbook YAML into the IR. Parsing is shape-only; no
// Jinja2 evaluation, no file inclusion, no module execution.
func ParsePlaybook(data []byte) (*Playbook, []Diagnostic, error) {
	var raw []map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("playbook is not a YAML list of plays: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil, fmt.Errorf("playbook contains no plays")
	}
	pb := &Playbook{}
	var diags []Diagnostic
	for i, rawPlay := range raw {
		play := &Play{
			Name:  stringValue(rawPlay["name"]),
			Hosts: stringValue(rawPlay["hosts"]),
			Vars:  mapValue(rawPlay["vars"]),
		}
		if play.Name == "" {
			play.Name = fmt.Sprintf("play_%d", i+1)
		}
		recordUnsupportedPlaySections(rawPlay, play.Name, &diags)
		var perr error
		play.PreTasks, perr = parseTaskList(rawPlay["pre_tasks"], &diags)
		if perr != nil {
			return nil, diags, fmt.Errorf("play %q pre_tasks: %w", play.Name, perr)
		}
		play.Tasks, perr = parseTaskList(rawPlay["tasks"], &diags)
		if perr != nil {
			return nil, diags, fmt.Errorf("play %q tasks: %w", play.Name, perr)
		}
		play.PostTasks, perr = parseTaskList(rawPlay["post_tasks"], &diags)
		if perr != nil {
			return nil, diags, fmt.Errorf("play %q post_tasks: %w", play.Name, perr)
		}
		play.Handlers, perr = parseTaskList(rawPlay["handlers"], &diags)
		if perr != nil {
			return nil, diags, fmt.Errorf("play %q handlers: %w", play.Name, perr)
		}
		pb.Plays = append(pb.Plays, play)
	}
	return pb, diags, nil
}

func recordUnsupportedPlaySections(rawPlay map[string]any, playName string, diags *[]Diagnostic) {
	for _, section := range []string{"roles"} {
		if !hasNonEmptyYAMLValue(rawPlay[section]) {
			continue
		}
		*diags = append(*diags, Diagnostic{
			Code:          CodePlaybookShape,
			Severity:      "error",
			Task:          playName,
			StrictFailure: true,
			Message:       fmt.Sprintf("play section %q is not lowered by this converter; inline it into tasks or convert it separately before executing the workflow", section),
		})
	}
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

func parseTaskList(v any, diags *[]Diagnostic) ([]*Task, error) {
	if v == nil {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("expected a YAML list, got %T", v)
	}
	var tasks []*Task
	for i, entry := range list {
		m, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("task %d is not a mapping", i+1)
		}
		task, err := parseTask(m, diags)
		if err != nil {
			return nil, fmt.Errorf("task %d: %w", i+1, err)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func parseTask(m map[string]any, diags *[]Diagnostic) (*Task, error) {
	task := &Task{
		Name:     stringValue(m["name"]),
		Register: stringValue(m["register"]),
		When:     stringListValue(m["when"]),
		Notify:   stringListValue(m["notify"]),
	}
	if loop, ok := m["loop"]; ok {
		task.Loop = loop
	} else if loop, ok := m["with_items"]; ok {
		task.Loop = loop
	}

	for _, key := range dynamicIncludeKeys {
		if _, ok := m[key]; ok {
			task.DynamicInclude = key
			return task, nil
		}
	}

	if rawBlock, ok := m["block"]; ok {
		var err error
		task.Block, err = parseTaskList(rawBlock, diags)
		if err != nil {
			return nil, fmt.Errorf("block: %w", err)
		}
		task.Rescue, err = parseTaskList(m["rescue"], diags)
		if err != nil {
			return nil, fmt.Errorf("rescue: %w", err)
		}
		task.Always, err = parseTaskList(m["always"], diags)
		if err != nil {
			return nil, fmt.Errorf("always: %w", err)
		}
	}

	// Record directive posture for review.
	for key := range m {
		switch key {
		case "become", "become_user", "become_method", "environment", "no_log":
			task.InfoDirectives = append(task.InfoDirectives, key)
		case "delegate_to", "run_once":
			task.HardDirectives = append(task.HardDirectives, key)
		case "until", "retries", "delay", "changed_when", "failed_when", "ignore_errors", "throttle", "any_errors_fatal":
			task.TodoDirectives = append(task.TodoDirectives, key)
		}
	}
	sort.Strings(task.InfoDirectives)
	sort.Strings(task.HardDirectives)
	sort.Strings(task.TodoDirectives)

	// The remaining non-directive key is the module invocation.
	var moduleKeys []string
	for key := range m {
		if !taskDirectives[key] && task.DynamicInclude == "" {
			moduleKeys = append(moduleKeys, key)
		}
	}
	sort.Strings(moduleKeys)
	if task.Block != nil {
		return task, nil
	}
	if len(moduleKeys) == 0 {
		return nil, fmt.Errorf("task %q has no module invocation", task.Name)
	}
	if len(moduleKeys) > 1 {
		return nil, fmt.Errorf("task %q has multiple module keys: %v", task.Name, moduleKeys)
	}
	task.Module = normalizeFQCN(moduleKeys[0])
	switch args := m[moduleKeys[0]].(type) {
	case map[string]any:
		task.Args = args
	case string:
		task.FreeForm = args
	case nil:
		task.Args = map[string]any{}
	default:
		return nil, fmt.Errorf("task %q module %s has unsupported argument shape %T", task.Name, task.Module, args)
	}
	if extra := mapValue(m["args"]); extra != nil {
		if task.Args == nil {
			task.Args = map[string]any{}
		}
		for k, v := range extra {
			task.Args[k] = v
		}
	}
	return task, nil
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
