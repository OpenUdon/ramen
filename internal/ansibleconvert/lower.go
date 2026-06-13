package ansibleconvert

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/OpenUdon/uws/uws1"
)

// loweredTask pairs a parsed task with its assigned identifiers.
type loweredTask struct {
	task       *Task
	stepID     string
	opID       string
	isHandler  bool
	hostFanOut bool
	skipped    bool
}

type lowerer struct {
	idx        *ArgspecIndex
	doc        *uws1.Document
	diags      []Diagnostic
	usedIDs    map[string]bool
	registered map[string]string // register var -> stepID
	vars       map[string]bool
	// neededOutputs[stepID][outputName] = response path (dot form)
	neededOutputs map[string]map[string]string
	sourcesUsed   map[string]bool
	varCounter    int
}

// LowerPlaybook converts the parsed playbook into a UWS 1.6 document plus
// review diagnostics using default lowering options. The document may be
// incomplete when strict diagnostics are present; it remains schema-valid for
// review.
func LowerPlaybook(pb *Playbook, idx *ArgspecIndex) (*uws1.Document, []Diagnostic) {
	return LowerPlaybookWithOptions(pb, idx, LowerOptions{})
}

// LowerPlaybookWithOptions converts the parsed playbook into a UWS 1.6
// document plus review diagnostics.
func LowerPlaybookWithOptions(pb *Playbook, idx *ArgspecIndex, opts LowerOptions) (*uws1.Document, []Diagnostic) {
	lw := &lowerer{
		idx:           idx,
		usedIDs:       map[string]bool{},
		registered:    map[string]string{},
		vars:          map[string]bool{},
		neededOutputs: map[string]map[string]string{},
		sourcesUsed:   map[string]bool{},
	}
	title := pb.Plays[0].Name
	if len(pb.Plays) > 1 {
		title = fmt.Sprintf("%s (+%d more plays)", title, len(pb.Plays)-1)
		lw.addDiag(Diagnostic{Code: CodePlaybookShape, Severity: "info",
			Message: fmt.Sprintf("playbook has %d plays; all tasks were lowered into one sequence workflow", len(pb.Plays))})
	}
	lw.doc = &uws1.Document{
		UWS:  "1.6.0",
		Info: &uws1.Info{Title: title, Version: "1.0.0"},
	}

	// Collect play vars and host posture.
	variables := map[string]any{}
	for _, play := range pb.Plays {
		for name, value := range play.Vars {
			if _, dup := variables[name]; dup {
				lw.addDiag(Diagnostic{Code: CodePlaybookShape, Severity: "warning",
					Message: fmt.Sprintf("variable %q is defined by more than one play; the last definition wins", name)})
			}
			variables[name] = value
			lw.vars[name] = true
		}
		if playNeedsHostFanOut(play.Hosts) {
			if opts.HostFanOut {
				lw.addDiag(Diagnostic{Code: CodeHostsRuntimeOwned, Severity: "info",
					Message: fmt.Sprintf("play %q targets hosts %q; task steps fan out over $inputs.hosts and connection details remain runtime-owned", play.Name, strings.TrimSpace(play.Hosts))})
			} else {
				lw.addDiag(Diagnostic{Code: CodeHostsRuntimeOwned, Severity: "info",
					Message: fmt.Sprintf("play %q targets hosts %q; host fan-out and connection are runtime-owned (stage-1 inventory posture)", play.Name, strings.TrimSpace(play.Hosts))})
			}
		}
	}

	// Pre-pass: flatten tasks and assign IDs. Register bindings are added only
	// after a producer has successfully lowered, preserving playbook order.
	var flat []*loweredTask
	notifiersByHandler := map[string][]*loweredTask{}
	needsHostsInput := false
	for _, play := range pb.Plays {
		hostFanOut := opts.HostFanOut && playNeedsHostFanOut(play.Hosts)
		if hostFanOut {
			needsHostsInput = true
		}
		for _, task := range play.PreTasks {
			flat = append(flat, lw.flatten(task, nil, hostFanOut)...)
		}
		for _, task := range play.Tasks {
			flat = append(flat, lw.flatten(task, nil, hostFanOut)...)
		}
		for _, task := range play.PostTasks {
			flat = append(flat, lw.flatten(task, nil, hostFanOut)...)
		}
	}

	// Main pass: lower task operations and steps.
	var steps []*uws1.Step
	for _, lt := range flat {
		step := lw.lowerTask(lt)
		if step == nil {
			// A skipped producer must not leave its register binding behind:
			// later consumers would otherwise emit $steps references to a step
			// that does not exist. Consumers always follow their producer in
			// playbook order, so deleting here makes them fail closed.
			if lt.task.Register != "" {
				delete(lw.registered, lt.task.Register)
			}
			continue
		}
		steps = append(steps, step)
		if lt.task.Register != "" {
			lw.registered[lt.task.Register] = lt.stepID
		}
		for _, handlerName := range lt.task.Notify {
			notifiersByHandler[handlerName] = append(notifiersByHandler[handlerName], lt)
		}
	}

	// Handlers: declared order, only when notified.
	for _, play := range pb.Plays {
		for _, handler := range play.Handlers {
			notifiers := notifiersByHandler[handler.Name]
			if len(notifiers) == 0 {
				lw.addDiag(Diagnostic{Code: CodeHandlerUnnotified, Severity: "info", Task: handler.Name,
					Message: fmt.Sprintf("handler %q is never notified and was not lowered", handler.Name)})
				continue
			}
			if step := lw.lowerHandler(handler, notifiers); step != nil {
				steps = append(steps, step)
			}
			delete(notifiersByHandler, handler.Name)
		}
	}
	for handlerName := range notifiersByHandler {
		lw.addDiag(Diagnostic{Code: CodePlaybookShape, Severity: "error", StrictFailure: true,
			Message: fmt.Sprintf("notify references unknown handler %q", handlerName)})
	}

	// Apply registered-output needs to producing operations.
	lw.applyNeededOutputs()

	if len(variables) > 0 {
		lw.ensureComponents()
		for name, value := range variables {
			lw.doc.Components.Variables[name] = value
		}
	}
	workflow := &uws1.Workflow{
		WorkflowID: "main",
		Type:       uws1.WorkflowTypeSequence,
		Steps:      steps,
	}
	if needsHostsInput {
		workflow.Inputs = &uws1.ParamSchema{
			Type:     "object",
			Required: []string{"hosts"},
			Properties: map[string]*uws1.ParamSchema{
				"hosts": {Type: "array", Items: &uws1.ParamSchema{Type: "string"}},
			},
		}
	}
	lw.doc.Workflows = []*uws1.Workflow{workflow}
	return lw.doc, lw.diags
}

// flatten expands block constructs into a linear task list, inheriting the
// block-level when onto children that have none.
func (lw *lowerer) flatten(task *Task, inheritedWhen []string, hostFanOut bool) []*loweredTask {
	if task.DynamicInclude != "" {
		lw.addDiag(Diagnostic{Code: CodeDynamicInclude, Severity: "error", StrictFailure: true, Task: task.Name,
			Message: fmt.Sprintf("%s cannot be statically lowered; inline the tasks or convert them separately", task.DynamicInclude)})
		return nil
	}
	// A guard at this level and an inherited guard would AND in Ansible; UWS
	// core has no logical operators, so the combination fails closed.
	if len(task.When) > 0 && len(inheritedWhen) > 0 {
		lw.addDiag(Diagnostic{Code: CodeJinjaUnsupported, Severity: "error", StrictFailure: true, Task: task.Name,
			Message: "a block-level when and a task-level when combine with AND in Ansible; UWS core supports a single comparison — the guarded task was not lowered"})
		return nil
	}
	if task.Block != nil {
		// delegate_to / run_once on a block apply to every task inside it.
		if len(task.HardDirectives) > 0 {
			for _, directive := range task.HardDirectives {
				lw.addDiag(Diagnostic{Code: CodeDelegateUnsupported, Severity: "error", StrictFailure: true, Task: task.Name,
					Message: fmt.Sprintf("directive %q on a block changes the execution target for all of its tasks; the block was not lowered", directive)})
			}
			return nil
		}
		when := task.When
		if len(when) == 0 {
			when = inheritedWhen
		}
		unsupportedBlockFlow := false
		if len(task.Rescue) > 0 {
			lw.addDiag(Diagnostic{Code: CodeRescueTodo, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: "block rescue tasks are not lowered yet; the block was not lowered"})
			unsupportedBlockFlow = true
		}
		if len(task.Always) > 0 {
			lw.addDiag(Diagnostic{Code: CodeAlwaysUnsupported, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: "block always (finally) has no UWS core equivalent; the block was not lowered"})
			unsupportedBlockFlow = true
		}
		if unsupportedBlockFlow {
			return nil
		}
		var out []*loweredTask
		for _, child := range task.Block {
			out = append(out, lw.flatten(child, when, hostFanOut)...)
		}
		return out
	}
	if len(task.When) == 0 {
		task.When = inheritedWhen
	}
	base := sanitizeID(task.Name)
	if base == "" {
		base = sanitizeID(shortModuleName(task.Module))
	}
	stepID := lw.uniqueID(base)
	return []*loweredTask{{task: task, stepID: stepID, opID: stepID, hostFanOut: hostFanOut}}
}

// lowerTask converts one flattened task into an operation plus its workflow
// step. Returns nil when the task is skipped with a strict diagnostic.
func (lw *lowerer) lowerTask(lt *loweredTask) *uws1.Step {
	task := lt.task
	sourceID, _, known := lw.idx.Lookup(task.Module)
	if !known {
		lw.addDiag(Diagnostic{Code: CodeModuleUnknown, Severity: "error", StrictFailure: true, Task: task.Name,
			Message: fmt.Sprintf("module %s is not declared by any supplied --argspec document", task.Module)})
		lt.skipped = true
		return nil
	}
	if len(task.HardDirectives) > 0 {
		for _, directive := range task.HardDirectives {
			lw.addDiag(Diagnostic{Code: CodeDelegateUnsupported, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: fmt.Sprintf("directive %q changes the execution target; the task was not lowered", directive)})
		}
		lt.skipped = true
		return nil
	}
	for _, directive := range task.TodoDirectives {
		lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "warning", Task: task.Name,
			Message: fmt.Sprintf("directive %q is recorded for review and not lowered", directive)})
	}
	for _, directive := range task.InfoDirectives {
		lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "info", Task: task.Name,
			Message: fmt.Sprintf("directive %q is runtime-owned and stays outside the UWS document", directive)})
	}

	step := &uws1.Step{StepID: lt.stepID, OperationRef: lt.opID}
	ctx := &exprContext{vars: lw.vars, registered: lw.registered, needOutput: lw.noteNeededOutput}
	if lt.hostFanOut {
		if task.Loop != nil {
			lw.addDiag(Diagnostic{Code: CodePlaybookShape, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: "host fan-out and task loop combine into a nested Ansible execution matrix; this converter does not lower both loops on one task"})
			lt.skipped = true
			return nil
		}
		step.ForEach = "$inputs.hosts"
		step.Inputs = map[string]any{"host": "$item"}
	}

	// Loop lowers to forEach on the step; $item becomes available inside.
	if task.Loop != nil {
		items, ok := lw.lowerLoopItems(task, ctx)
		if !ok {
			lt.skipped = true
			return nil
		}
		step.ForEach = items
		ctx.inLoop = true
	}

	// when: a single condition lowers; multiple conditions are AND in Ansible
	// and UWS core has no logical operators. A guard that cannot be lowered
	// fails closed: emitting the step without its guard would turn a
	// conditional side effect into an unconditional one.
	if len(task.When) == 1 {
		lowered, ok, reason := lowerWhen(task.When[0], ctx)
		if !ok {
			lw.addDiag(Diagnostic{Code: CodeJinjaUnsupported, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: fmt.Sprintf("%s; the guarded task was not lowered", reason)})
			lt.skipped = true
			return nil
		}
		step.When = lowered
	} else if len(task.When) > 1 {
		lw.addDiag(Diagnostic{Code: CodeJinjaUnsupported, Severity: "error", StrictFailure: true, Task: task.Name,
			Message: "multiple when conditions are AND-combined in Ansible; UWS core supports a single comparison — the guarded task was not lowered"})
		lt.skipped = true
		return nil
	}

	op := &uws1.Operation{
		OperationID:       lt.opID,
		SourceDescription: sourceUWSName(sourceID),
		SourceOperationID: task.Module,
		Outputs:           map[string]string{"changed": "$response.body.changed"},
		SuccessCriteria:   []*uws1.Criterion{{Condition: "$response.body.failed != true"}},
	}
	if task.Name != "" {
		op.Description = task.Name
	}

	args := task.Args
	if task.FreeForm != "" {
		if task.Module == "ansible.builtin.shell" || task.Module == "ansible.builtin.command" {
			args = map[string]any{"cmd": task.FreeForm}
		} else {
			lw.addDiag(Diagnostic{Code: CodeJinjaUnsupported, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: fmt.Sprintf("free-form arguments for %s are not lowered; use the explicit argument map", task.Module)})
			lt.skipped = true
			return nil
		}
	}
	body := lw.lowerArgs(task.Name, args, ctx)
	if len(body) > 0 {
		op.Request = map[string]any{"body": body}
	}
	lw.diags = append(lw.diags, lw.idx.ValidateArgs(task.Name, task.Module, body)...)

	lw.sourcesUsed[sourceID] = true
	lw.ensureSourceDescription(sourceID)
	lw.doc.Operations = append(lw.doc.Operations, op)
	return step
}

// lowerHandler lowers a notified handler. One notifier gates the handler step
// directly on the notifier's changed output. Multiple notifiers lower to a
// switch step whose cases each gate on one notifier and reference the same
// handler operation; switch executes at most one matching case, preserving
// Ansible's run-once handler semantics without logical OR.
func (lw *lowerer) lowerHandler(handler *Task, notifiers []*loweredTask) *uws1.Step {
	for _, notifier := range notifiers {
		if notifier.hostFanOut {
			lw.addDiag(Diagnostic{Code: CodePlaybookShape, Severity: "error", StrictFailure: true, Task: handler.Name,
				Message: "handler notification after host fan-out needs per-host changed evaluation; UWS 1.6 forEach aggregates changed outputs, so the handler was not lowered"})
			return nil
		}
	}
	// A handler's own when would AND with the notifier gate in Ansible; UWS
	// core has no logical operators, so the combination fails closed rather
	// than dropping the handler's guard.
	if len(handler.When) > 0 {
		lw.addDiag(Diagnostic{Code: CodeJinjaUnsupported, Severity: "error", StrictFailure: true, Task: handler.Name,
			Message: "a handler-level when combines with the notifier changed gate using AND; UWS core supports a single comparison — the handler was not lowered"})
		return nil
	}
	base := sanitizeID(handler.Name)
	if base == "" {
		base = sanitizeID(shortModuleName(handler.Module))
	}
	stepID := lw.uniqueID(base)
	lt := &loweredTask{task: handler, stepID: stepID, opID: stepID, isHandler: true}
	step := lw.lowerTask(lt)
	if step == nil {
		return nil
	}
	var active []*loweredTask
	for _, notifier := range notifiers {
		if !notifier.skipped {
			active = append(active, notifier)
		}
	}
	if len(active) == 0 {
		return nil
	}
	if len(active) == 1 {
		step.When = fmt.Sprintf("$steps.%s.outputs.changed == true", active[0].stepID)
		return step
	}
	wrapper := &uws1.Step{
		StepID: lw.uniqueID(stepID + "_notify"),
		Type:   uws1.WorkflowTypeSwitch,
	}
	for i, notifier := range active {
		inner := &uws1.Step{
			StepID:       lw.uniqueID(fmt.Sprintf("%s_run_%d", stepID, i+1)),
			OperationRef: lt.opID,
		}
		c := &uws1.Case{Steps: []*uws1.Step{inner}}
		c.Name = fmt.Sprintf("notified_by_%s", notifier.stepID)
		c.When = fmt.Sprintf("$steps.%s.outputs.changed == true", notifier.stepID)
		wrapper.Cases = append(wrapper.Cases, c)
	}
	return wrapper
}

// lowerLoopItems lowers a task loop into a forEach expression. List literals
// are hoisted into components.variables so the expression stays in core.
func (lw *lowerer) lowerLoopItems(task *Task, ctx *exprContext) (string, bool) {
	switch loop := task.Loop.(type) {
	case string:
		lowered, ok, reason := lowerValue(loop, ctx)
		if !ok || !strings.HasPrefix(lowered, "$") {
			if reason == "" {
				reason = fmt.Sprintf("loop value %q is not a lowerable expression", loop)
			}
			lw.addDiag(Diagnostic{Code: CodeJinjaUnsupported, Severity: "error", StrictFailure: true, Task: task.Name, Message: reason})
			return "", false
		}
		return lowered, true
	case []any:
		lw.varCounter++
		name := fmt.Sprintf("%s_items", sanitizeID(task.Name))
		if name == "_items" || lw.vars[name] {
			name = fmt.Sprintf("loop_items_%d", lw.varCounter)
		}
		lw.ensureComponents()
		lw.doc.Components.Variables[name] = loop
		lw.vars[name] = true
		return "$variables." + name, true
	default:
		lw.addDiag(Diagnostic{Code: CodeJinjaUnsupported, Severity: "error", StrictFailure: true, Task: task.Name,
			Message: fmt.Sprintf("loop value of type %T is not lowered", task.Loop)})
		return "", false
	}
}

// lowerArgs recursively lowers module argument values.
func (lw *lowerer) lowerArgs(taskName string, args map[string]any, ctx *exprContext) map[string]any {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]any, len(args))
	for key, value := range args {
		out[key] = lw.lowerArgValue(taskName, key, value, ctx)
	}
	return out
}

func (lw *lowerer) lowerArgValue(taskName, key string, value any, ctx *exprContext) any {
	switch v := value.(type) {
	case string:
		lowered, ok, reason := lowerValue(v, ctx)
		if !ok {
			lw.addDiag(Diagnostic{Code: CodeJinjaUnsupported, Severity: "error", StrictFailure: true, Task: taskName,
				Message: fmt.Sprintf("argument %q: %s", key, reason)})
			return fmt.Sprintf("UWS-TODO(%s)", v)
		}
		return lowered
	case map[string]any:
		return lw.lowerArgs(taskName, v, ctx)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = lw.lowerArgValue(taskName, fmt.Sprintf("%s[%d]", key, i), item, ctx)
		}
		return out
	default:
		return value
	}
}

func (lw *lowerer) noteNeededOutput(stepID, outputName, responsePath string) {
	if lw.neededOutputs[stepID] == nil {
		lw.neededOutputs[stepID] = map[string]string{}
	}
	lw.neededOutputs[stepID][outputName] = responsePath
}

func (lw *lowerer) applyNeededOutputs() {
	for _, op := range lw.doc.Operations {
		needs := lw.neededOutputs[op.OperationID]
		if len(needs) == 0 {
			continue
		}
		names := make([]string, 0, len(needs))
		for name := range needs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			op.Outputs[name] = "$response.body." + needs[name]
		}
	}
}

func (lw *lowerer) ensureSourceDescription(sourceID string) {
	name := sourceUWSName(sourceID)
	for _, sd := range lw.doc.SourceDescriptions {
		if sd.Name == name {
			return
		}
	}
	input, _ := lw.idx.Source(sourceID)
	lw.doc.SourceDescriptions = append(lw.doc.SourceDescriptions, &uws1.SourceDescription{
		Name: name,
		URL:  input.Path,
		Type: uws1.SourceDescriptionTypeAnsibleModule,
	})
}

// sourceUWSName converts an argspec source ID (often an Ansible collection FQCN
// such as "ansible.builtin") into a UWS sourceDescription name, which must match
// ^[A-Za-z0-9_-]+$. The raw ID stays the argspec lookup key; only the emitted
// name is sanitized so a dotted collection ID does not fail UWS validation.
func sourceUWSName(sourceID string) string {
	if name := sanitizeID(sourceID); name != "" {
		return name
	}
	return "source"
}

func (lw *lowerer) ensureComponents() {
	if lw.doc.Components == nil {
		lw.doc.Components = &uws1.Components{Variables: map[string]any{}}
	}
	if lw.doc.Components.Variables == nil {
		lw.doc.Components.Variables = map[string]any{}
	}
}

func playNeedsHostFanOut(hosts string) bool {
	hosts = strings.TrimSpace(hosts)
	switch hosts {
	case "", "localhost", "127.0.0.1":
		return false
	default:
		return true
	}
}

func (lw *lowerer) addDiag(d Diagnostic) {
	lw.diags = append(lw.diags, d)
}

func (lw *lowerer) uniqueID(base string) string {
	if base == "" {
		base = "task"
	}
	id := base
	for i := 2; lw.usedIDs[id]; i++ {
		id = fmt.Sprintf("%s_%d", base, i)
	}
	lw.usedIDs[id] = true
	return id
}

var nonIDChars = regexp.MustCompile(`[^a-z0-9_]+`)

func sanitizeID(name string) string {
	id := strings.ToLower(strings.TrimSpace(name))
	id = nonIDChars.ReplaceAllString(id, "_")
	return strings.Trim(id, "_")
}

func shortModuleName(fqcn string) string {
	parts := strings.Split(fqcn, ".")
	return parts[len(parts)-1]
}
