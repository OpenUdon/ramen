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

type staticResolver struct {
	opts             Options
	diags            []Diagnostic
	roleHandlersSeen map[string]bool
}

type roleResolution struct {
	name string
	path string
}

const (
	varPriorityRoleDefault = 10
	varPriorityVarsFile    = 30
	varPriorityPlay        = 50
	varPriorityRoleVars    = 60
	varPriorityExtra       = 100
)

func (r *staticResolver) addDiag(d Diagnostic) {
	r.diags = append(r.diags, d)
}

func resolveStatic(pb *Playbook, opts Options) []Diagnostic {
	r := &staticResolver{opts: opts}
	for _, play := range pb.Plays {
		r.initializePlayVars(play)
		r.roleHandlersSeen = map[string]bool{}
		r.resolveVarsFiles(play)
		var roleTasks []*Task
		for _, role := range play.Roles {
			roleTasks = append(roleTasks, r.expandRole(play, role.Name, role.SourceFile, role.Line, role.Column, nil, "tasks")...)
		}
		play.Tasks = append(roleTasks, r.resolveTaskList(play, play.Tasks, nil, "tasks")...)
		play.PreTasks = r.resolveTaskList(play, play.PreTasks, nil, "pre_tasks")
		play.PostTasks = r.resolveTaskList(play, play.PostTasks, nil, "post_tasks")
		play.Handlers = r.resolveTaskList(play, play.Handlers, nil, "handlers")
	}
	return r.diags
}

func (r *staticResolver) resolveVarsFiles(play *Play) {
	for _, value := range play.VarsFiles {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if containsTemplate(value) {
			r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: play.Name,
				Message: fmt.Sprintf("vars_files entry %q is templated; static variable ingestion requires literal file paths", value)})
			continue
		}
		path := resolveRelativeTo(play.SourceFile, value)
		vars, ok := r.readStaticVarsFile(path, fmt.Sprintf("vars_files %q", value), play.Name)
		if !ok {
			continue
		}
		r.mergeVars(play, vars, varPriorityVarsFile, fmt.Sprintf("vars_files %s", path), play.Name)
	}
}

func (r *staticResolver) resolveTaskList(play *Play, tasks []*Task, importStack []string, section string) []*Task {
	var out []*Task
	for _, task := range tasks {
		switch {
		case task.StaticImport != "":
			out = append(out, r.expandTaskImport(play, task, importStack, section)...)
		case task.ImportRole != "":
			tasks := r.expandRole(play, task.ImportRole, task.SourceFile, task.Line, task.Column, importStack, section)
			out = append(out, r.applyImportWrapper(task, tasks)...)
		case task.Block != nil:
			task.Block = r.resolveTaskList(play, task.Block, importStack, section)
			out = append(out, task)
		default:
			out = append(out, task)
		}
	}
	return out
}

func (r *staticResolver) expandTaskImport(play *Play, task *Task, importStack []string, section string) []*Task {
	pathText := strings.TrimSpace(task.StaticImport)
	if pathText == "" || containsTemplate(pathText) {
		r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: task.Name,
			Message: fmt.Sprintf("import_tasks path %q is not a static literal; the imported tasks were not lowered", pathText)})
		return nil
	}
	path := filepath.Clean(resolveRelativeTo(task.SourceFile, pathText))
	if stackContains(importStack, path) {
		r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: task.Name,
			Message: fmt.Sprintf("import_tasks cycle detected at %s; the imported tasks were not lowered", path)})
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: task.Name,
			Message: fmt.Sprintf("import_tasks file %s could not be read: %v", path, err)})
		return nil
	}
	tasks, err := parseStandaloneTaskFile(data, path, play.Name, section, task.Role, append(importStack, path))
	if err != nil {
		r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: task.Name,
			Message: fmt.Sprintf("import_tasks file %s could not be parsed: %v", path, err)})
		return nil
	}
	return r.applyImportWrapper(task, r.resolveTaskList(play, tasks, append(importStack, path), section))
}

func (r *staticResolver) applyImportWrapper(wrapper *Task, tasks []*Task) []*Task {
	if wrapper == nil || len(tasks) == 0 {
		return tasks
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		task.When = append(append([]string(nil), wrapper.When...), task.When...)
		task.Tags = compactSorted(append(append([]string(nil), wrapper.Tags...), task.Tags...))
		task.HardDirectives = append(append([]string(nil), wrapper.HardDirectives...), task.HardDirectives...)
		task.TodoDirectives = append(append([]string(nil), wrapper.TodoDirectives...), task.TodoDirectives...)
		task.InfoDirectives = append(append([]string(nil), wrapper.InfoDirectives...), task.InfoDirectives...)
		task.StrictDirectiveDiagnostics = append(cloneDiagnostics(wrapper.StrictDirectiveDiagnostics), task.StrictDirectiveDiagnostics...)
		task.Vars = mergeTaskVarsForImport(wrapper, task)
		inheritStringListDirective(wrapper.ChangedWhen, &task.ChangedWhen, "changed_when", task)
		inheritStringListDirective(wrapper.FailedWhen, &task.FailedWhen, "failed_when", task)
		inheritStringListDirective(wrapper.Until, &task.Until, "until", task)
		inheritIntDirective(wrapper.Retries, &task.Retries, "retries", task)
		inheritFloatDirective(wrapper.Delay, &task.Delay, "delay", task)
		inheritIntDirective(wrapper.Throttle, &task.Throttle, "throttle", task)
		if wrapper.IgnoreErrors {
			task.IgnoreErrors = true
		}
		if wrapper.AnyErrorsFatal {
			task.AnyErrorsFatal = true
		}
	}
	return tasks
}

func cloneDiagnostics(in []Diagnostic) []Diagnostic {
	if len(in) == 0 {
		return nil
	}
	return append([]Diagnostic(nil), in...)
}

func mergeTaskVarsForImport(wrapper, task *Task) map[string]any {
	if len(wrapper.Vars) == 0 {
		return task.Vars
	}
	out := make(map[string]any, len(wrapper.Vars)+len(task.Vars))
	for name, value := range wrapper.Vars {
		out[name] = value
	}
	for name, value := range task.Vars {
		if existing, ok := out[name]; ok && !reflect.DeepEqual(existing, value) {
			task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, Diagnostic{Code: CodeVariableConflict, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: fmt.Sprintf("task-local var %q from static import wrapper conflicts with imported task value; Ansible precedence was not approximated", name)})
			continue
		}
		out[name] = value
	}
	return out
}

func inheritStringListDirective(wrapperValues []string, taskValues *[]string, directive string, task *Task) {
	if len(wrapperValues) == 0 {
		return
	}
	if len(*taskValues) > 0 {
		task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, importDirectiveConflict(directive, task.Name))
		return
	}
	*taskValues = append([]string(nil), wrapperValues...)
}

func inheritIntDirective(wrapperValue *int, taskValue **int, directive string, task *Task) {
	if wrapperValue == nil {
		return
	}
	if *taskValue != nil && **taskValue != *wrapperValue {
		task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, importDirectiveConflict(directive, task.Name))
		return
	}
	value := *wrapperValue
	*taskValue = &value
}

func inheritFloatDirective(wrapperValue *float64, taskValue **float64, directive string, task *Task) {
	if wrapperValue == nil {
		return
	}
	if *taskValue != nil && **taskValue != *wrapperValue {
		task.StrictDirectiveDiagnostics = append(task.StrictDirectiveDiagnostics, importDirectiveConflict(directive, task.Name))
		return
	}
	value := *wrapperValue
	*taskValue = &value
}

func importDirectiveConflict(directive, taskName string) Diagnostic {
	return Diagnostic{Code: CodeDirectiveTodo, Severity: "error", StrictFailure: true, Task: taskName,
		Message: fmt.Sprintf("directive %q appears on both a static import wrapper and an imported task; Ansible precedence was not approximated", directive)}
}

func (r *staticResolver) expandRole(play *Play, roleName, sourceFile string, line, column int, importStack []string, section string) []*Task {
	roleName = strings.TrimSpace(roleName)
	taskName := roleName
	if roleName == "" || containsTemplate(roleName) {
		r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: taskName,
			Message: fmt.Sprintf("role name %q is not a static literal; role tasks were not lowered", roleName)})
		return nil
	}
	role, ok := r.resolveRole(roleName, taskName)
	if !ok {
		return nil
	}
	if stackContains(importStack, role.path) {
		r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: taskName,
			Message: fmt.Sprintf("role cycle detected at %s; role tasks were not lowered", role.path)})
		return nil
	}
	for _, source := range []struct {
		rel      string
		priority int
	}{
		{rel: filepath.Join("defaults", "main.yml"), priority: varPriorityRoleDefault},
		{rel: filepath.Join("vars", "main.yml"), priority: varPriorityRoleVars},
	} {
		path := filepath.Join(role.path, source.rel)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		vars, varsOK := r.readStaticVarsFile(path, fmt.Sprintf("role %s %s", role.name, source.rel), taskName)
		if varsOK {
			r.mergeVars(play, vars, source.priority, fmt.Sprintf("role %s %s", role.name, source.rel), taskName)
		}
	}
	handlersPath := filepath.Join(role.path, "handlers", "main.yml")
	if _, err := os.Stat(handlersPath); err == nil {
		handlers, err := parseStandaloneTaskFile(nilRead(handlersPath), handlersPath, play.Name, "handlers", role.name, append(importStack, role.path))
		if err != nil {
			r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: taskName,
				Message: fmt.Sprintf("role handlers file %s could not be parsed: %v", handlersPath, err)})
		} else {
			play.Handlers = append(play.Handlers, r.resolveTaskList(play, handlers, append(importStack, role.path), "handlers")...)
		}
	}
	tasksPath := filepath.Join(role.path, "tasks", "main.yml")
	if _, err := os.Stat(tasksPath); err != nil {
		r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: taskName,
			Message: fmt.Sprintf("role %s has no tasks/main.yml at %s; role tasks were not lowered", role.name, tasksPath)})
		return nil
	}
	tasks, err := parseStandaloneTaskFile(nilRead(tasksPath), tasksPath, play.Name, section, role.name, append(importStack, role.path))
	if err != nil {
		r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: taskName,
			Message: fmt.Sprintf("role tasks file %s could not be parsed: %v", tasksPath, err)})
		return nil
	}
	for _, task := range tasks {
		if task.Line == 0 {
			task.SourceFile = sourceFile
			task.Line = line
			task.Column = column
		}
	}
	return r.resolveTaskList(play, tasks, append(importStack, role.path), section)
}

func nilRead(path string) []byte {
	data, _ := os.ReadFile(path)
	return data
}

func parseStandaloneTaskFile(data []byte, sourceFile, playName, section, role string, importStack []string) ([]*Task, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	var diags []Diagnostic
	tasks, err := parseTaskList(documentNode(&root), sourceFile, playName, section, role, importStack, &diags)
	if err != nil {
		return nil, err
	}
	if len(diags) > 0 {
		return nil, fmt.Errorf("unexpected parse diagnostics: %#v", diags)
	}
	return tasks, nil
}

func (r *staticResolver) resolveRole(roleName, taskName string) (roleResolution, bool) {
	var matches []string
	for _, root := range r.roleSearchPaths() {
		path := filepath.Join(root, roleName)
		if dirExists(path) {
			matches = append(matches, filepath.Clean(path))
		}
	}
	parts := strings.Split(roleName, ".")
	if len(parts) == 3 {
		for _, root := range r.opts.CollectionsPaths {
			path := filepath.Join(root, "ansible_collections", parts[0], parts[1], "roles", parts[2])
			if dirExists(path) {
				matches = append(matches, filepath.Clean(path))
			}
		}
	}
	matches = compactSorted(matches)
	switch len(matches) {
	case 0:
		r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: taskName,
			Message: fmt.Sprintf("role %q was not found in static roles paths or collections paths", roleName)})
		return roleResolution{}, false
	case 1:
		return roleResolution{name: roleName, path: matches[0]}, true
	default:
		r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: taskName,
			Message: fmt.Sprintf("role %q resolved ambiguously: %s", roleName, strings.Join(matches, ", "))})
		return roleResolution{}, false
	}
}

func (r *staticResolver) roleSearchPaths() []string {
	if len(r.opts.RolesPaths) > 0 {
		return r.opts.RolesPaths
	}
	return []string{filepath.Join(r.opts.ProjectDir, "roles")}
}

func (r *staticResolver) readStaticVarsFile(path, label, taskName string) (map[string]any, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: taskName,
			Message: fmt.Sprintf("%s could not be read: %v", label, err)})
		return nil, false
	}
	var vars map[string]any
	if err := yaml.Unmarshal(data, &vars); err != nil {
		r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: taskName,
			Message: fmt.Sprintf("%s is not valid YAML: %v", label, err)})
		return nil, false
	}
	if vars == nil {
		r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: taskName,
			Message: fmt.Sprintf("%s must be a static YAML map", label)})
		return nil, false
	}
	for name, value := range vars {
		if !identRE.MatchString(name) || !isStaticAnsibleValue(value) {
			r.addDiag(Diagnostic{Code: CodeStaticResolution, Severity: "error", StrictFailure: true, Task: taskName,
				Message: fmt.Sprintf("%s variable %q must be a literal static value with an identifier name", label, name)})
			return nil, false
		}
	}
	return vars, true
}

func (r *staticResolver) initializePlayVars(play *Play) {
	if play.VarPriorities == nil {
		play.VarPriorities = map[string]int{}
	}
	if play.VarSources == nil {
		play.VarSources = map[string]string{}
	}
	for name := range play.Vars {
		play.VarPriorities[name] = varPriorityPlay
		play.VarSources[name] = "play vars"
	}
}

func (r *staticResolver) mergeVars(play *Play, vars map[string]any, priority int, source, taskName string) {
	if play.Vars == nil {
		play.Vars = map[string]any{}
	}
	r.initializePlayVars(play)
	for name, value := range vars {
		if existing, ok := play.Vars[name]; ok {
			existingPriority := play.VarPriorities[name]
			switch {
			case existingPriority > priority:
				continue
			case existingPriority == priority && !reflect.DeepEqual(existing, value):
				r.addDiag(Diagnostic{Code: CodeVariableConflict, Severity: "error", StrictFailure: true, Task: taskName,
					Message: fmt.Sprintf("variable %q from %s conflicts with %s at equal static precedence", name, source, play.VarSources[name])})
				continue
			}
		}
		play.Vars[name] = value
		play.VarPriorities[name] = priority
		play.VarSources[name] = source
	}
}

func resolveRelativeTo(sourceFile, value string) string {
	if filepath.IsAbs(value) {
		return value
	}
	base := filepath.Dir(sourceFile)
	if strings.TrimSpace(base) == "" || base == "." {
		base = "."
	}
	return filepath.Join(base, value)
}

func containsTemplate(value string) bool {
	return strings.Contains(value, "{{") || strings.Contains(value, "}}")
}

func stackContains(stack []string, path string) bool {
	clean := filepath.Clean(path)
	for _, item := range stack {
		if filepath.Clean(item) == clean {
			return true
		}
	}
	return false
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func compactSorted(values []string) []string {
	sort.Strings(values)
	out := values[:0]
	var prev string
	for _, value := range values {
		if value == prev {
			continue
		}
		out = append(out, value)
		prev = value
	}
	return out
}
