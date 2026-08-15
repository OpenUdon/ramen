package ansibleconvert

import (
	"fmt"
	"reflect"
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
	hostSource string
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
	varCounter    int
	targetUWS     string
}

// LowerPlaybook converts the parsed playbook into a UWS document plus review
// diagnostics using default lowering options. The document may be incomplete
// when strict diagnostics are present; it remains schema-valid for review.
func LowerPlaybook(pb *Playbook, idx *ArgspecIndex) (*uws1.Document, []Diagnostic) {
	return LowerPlaybookWithOptions(pb, idx, LowerOptions{})
}

// LowerPlaybookWithOptions converts the parsed playbook into a UWS document
// plus review diagnostics.
func LowerPlaybookWithOptions(pb *Playbook, idx *ArgspecIndex, opts LowerOptions) (*uws1.Document, []Diagnostic) {
	targetUWS := normalizeTargetUWS(opts.TargetUWS)
	lw := &lowerer{
		idx:           idx,
		usedIDs:       map[string]bool{},
		registered:    map[string]string{},
		vars:          map[string]bool{},
		neededOutputs: map[string]map[string]string{},
		targetUWS:     targetUWS,
	}
	title := pb.Plays[0].Name
	if len(pb.Plays) > 1 {
		title = fmt.Sprintf("%s (+%d more plays)", title, len(pb.Plays)-1)
		lw.addDiag(Diagnostic{Code: CodePlaybookShape, Severity: "info",
			Message: fmt.Sprintf("playbook has %d plays; all tasks were lowered into one sequence workflow", len(pb.Plays))})
	}
	lw.doc = &uws1.Document{
		UWS:  targetUWSDocumentVersion(targetUWS),
		Info: &uws1.Info{Title: title, Version: "1.0.0"},
	}

	// Collect play vars and host posture.
	variables := map[string]any{}
	for _, play := range pb.Plays {
		for name, value := range play.Vars {
			if existing, dup := variables[name]; dup && !reflect.DeepEqual(existing, value) {
				lw.addDiag(Diagnostic{Code: CodeVariableConflict, Severity: "error", StrictFailure: true,
					Message: fmt.Sprintf("variable %q has conflicting static values across plays; Ansible precedence was not approximated", name)})
				continue
			}
			variables[name] = value
			lw.vars[name] = true
		}
		if playNeedsHostFanOut(play.Hosts) {
			if play.InventoryFailed {
				// The inventory diagnostic is emitted by the bounded loader; do not
				// advertise the legacy runtime-input posture for a failed selection.
			} else if play.InventoryResolved {
				lw.addDiag(Diagnostic{Code: CodeHostsRuntimeOwned, Severity: "info",
					Message: fmt.Sprintf("play %q targets hosts %q; static inventory selected %d host(s), while connection details remain runtime-owned", play.Name, strings.TrimSpace(play.Hosts), len(play.InventoryHosts))})
			} else if opts.HostFanOut {
				lw.addDiag(Diagnostic{Code: CodeHostsRuntimeOwned, Severity: "info",
					Message: fmt.Sprintf("play %q targets hosts %q; task steps fan out over $inputs.hosts and connection details remain runtime-owned", play.Name, strings.TrimSpace(play.Hosts))})
			} else {
				lw.addDiag(Diagnostic{Code: CodeHostsRuntimeOwned, Severity: "info",
					Message: fmt.Sprintf("play %q targets hosts %q; host fan-out and connection are runtime-owned (stage-1 inventory posture)", play.Name, strings.TrimSpace(play.Hosts))})
			}
		}
		if play.InventoryResolved && playNeedsHostFanOut(play.Hosts) {
			base := "inventory_" + sanitizeID(play.Name) + "_hosts"
			if base == "inventory__hosts" {
				base = "inventory_hosts"
			}
			name := base
			for suffix := 2; ; suffix++ {
				if _, exists := variables[name]; !exists {
					break
				}
				name = fmt.Sprintf("%s_%d", base, suffix)
			}
			play.InventoryVariable = name
			hosts := make([]any, len(play.InventoryHosts))
			for i, host := range play.InventoryHosts {
				hostVars := cloneMapAny(play.InventoryHostVars[host])
				for varName := range play.Vars {
					delete(hostVars, varName)
				}
				if hostVars == nil {
					hostVars = map[string]any{}
				}
				hosts[i] = map[string]any{"host": host, "vars": hostVars}
			}
			variables[name] = hosts
			lw.vars[name] = true
		}
	}

	// Pre-pass: flatten tasks and assign IDs. Register bindings are added only
	// after a producer has successfully lowered, preserving playbook order.
	var flat []*loweredTask
	notifiersByHandler := map[string][]*loweredTask{}
	needsHostsInput := false
	for _, play := range pb.Plays {
		if play.InventoryFailed || play.StaticScopeFailed {
			continue
		}
		hostFanOut := opts.HostFanOut && playNeedsHostFanOut(play.Hosts)
		playVars := boolKeys(play.Vars)
		hostVars := cloneBoolMap(play.InventoryVarNames)
		for name := range playVars {
			delete(hostVars, name)
		}
		annotateTaskScopes(play.PreTasks, playVars, hostVars)
		annotateTaskScopes(play.Tasks, playVars, hostVars)
		annotateTaskScopes(play.PostTasks, playVars, hostVars)
		annotateTaskScopes(play.Handlers, playVars, hostVars)
		if hostFanOut && play.InventoryVariable == "" {
			needsHostsInput = true
		}
		for _, task := range play.PreTasks {
			flat = append(flat, lw.flatten(task, nil, hostFanOut, play.InventoryVariable)...)
		}
		for _, task := range play.Tasks {
			flat = append(flat, lw.flatten(task, nil, hostFanOut, play.InventoryVariable)...)
		}
		for _, task := range play.PostTasks {
			flat = append(flat, lw.flatten(task, nil, hostFanOut, play.InventoryVariable)...)
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
		handlerRefs := lw.handlerRefs(play)
		for _, handler := range play.Handlers {
			refs := handlerRefs[handler]
			var notifiers []*loweredTask
			for _, ref := range refs {
				notifiers = append(notifiers, notifiersByHandler[ref]...)
			}
			if len(notifiers) == 0 {
				lw.addDiag(Diagnostic{Code: CodeHandlerUnnotified, Severity: "info", Task: handler.Name,
					Message: fmt.Sprintf("handler %q is never notified and was not lowered", handler.Name)})
				continue
			}
			if step := lw.lowerHandler(handler, notifiers); step != nil {
				steps = append(steps, step)
			}
			for _, ref := range refs {
				delete(notifiersByHandler, ref)
			}
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
func (lw *lowerer) flatten(task *Task, inheritedWhen []string, hostFanOut bool, hostSource string) []*loweredTask {
	if task.DynamicInclude != "" {
		lw.addDiag(Diagnostic{Code: CodeDynamicInclude, Severity: "error", StrictFailure: true, Task: task.Name,
			Message: fmt.Sprintf("%s cannot be statically lowered; inline the tasks or convert them separately", task.DynamicInclude)})
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
		when := append([]string(nil), inheritedWhen...)
		when = append(when, task.When...)
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
			out = append(out, lw.flatten(child, when, hostFanOut, hostSource)...)
		}
		return out
	}
	if len(task.When) == 0 {
		task.When = inheritedWhen
	} else if len(inheritedWhen) > 0 {
		task.When = append(append([]string(nil), inheritedWhen...), task.When...)
	}
	base := sanitizeID(task.Name)
	if base == "" {
		base = sanitizeID(shortModuleName(task.Module))
	}
	stepID := lw.uniqueID(base)
	return []*loweredTask{{task: task, stepID: stepID, opID: stepID, hostFanOut: hostFanOut, hostSource: hostSource}}
}

// lowerTask converts one flattened task into an operation plus its workflow
// step. Returns nil when the task is skipped with a strict diagnostic.
func (lw *lowerer) lowerTask(lt *loweredTask) *uws1.Step {
	task := lt.task
	if len(task.StrictDirectiveDiagnostics) > 0 {
		for _, diag := range task.StrictDirectiveDiagnostics {
			lw.addDiag(diag)
		}
		lt.skipped = true
		return nil
	}
	if task.Module == "" {
		lt.skipped = true
		return nil
	}
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
		lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "error", StrictFailure: true, Task: task.Name,
			Message: fmt.Sprintf("directive %q changes Ansible task semantics and is not lowered; the task was not lowered", directive)})
		lt.skipped = true
		return nil
	}
	for _, directive := range task.InfoDirectives {
		lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "info", Task: task.Name,
			Message: fmt.Sprintf("directive %q is runtime-owned and stays outside the UWS document", directive)})
	}

	step := &uws1.Step{StepID: lt.stepID, OperationRef: lt.opID}
	taskVars, ok := lw.lowerTaskVars(task)
	if !ok {
		lt.skipped = true
		return nil
	}
	if len(taskVars) > 0 {
		step.Inputs = taskVars
	}
	ctx := &exprContext{vars: task.StaticPlayVars, registered: lw.registered, taskVars: boolKeys(taskVars), hostVars: task.StaticInventoryVars, inHostLoop: lt.hostSource != "", currentRegister: task.Register, needOutput: lw.noteNeededOutput}
	if lt.hostFanOut {
		if task.Loop != nil {
			lw.addDiag(Diagnostic{Code: CodePlaybookShape, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: "host fan-out and task loop combine into a nested Ansible execution matrix; this converter does not lower both loops on one task"})
			lt.skipped = true
			return nil
		}
		if lt.hostSource != "" {
			step.ForEach = "$variables." + lt.hostSource
		} else {
			step.ForEach = "$inputs.hosts"
		}
		if step.Inputs == nil {
			step.Inputs = map[string]any{}
		}
		if lt.hostSource != "" {
			step.Inputs["host"] = "$item.host"
		} else {
			step.Inputs["host"] = "$item"
		}
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

	var guardDNF conditionDNF
	if !lt.isHandler && len(task.When) > 0 {
		lowered, ok := lw.lowerConditionDNF(task.Name, task.When, ctx, "guarded task")
		if !ok {
			lt.skipped = true
			return nil
		}
		if conditionDNFWrapsStep(lowered) && task.Register != "" {
			lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: "when guard would wrap a registered task and cannot expose a stable UWS step output; the task was not lowered"})
			lt.skipped = true
			return nil
		}
		if conditionDNFWrapsStep(lowered) && len(task.Notify) > 0 {
			lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: "when guard would wrap a notifying task and cannot expose a stable changed output for handler gates; the task was not lowered"})
			lt.skipped = true
			return nil
		}
		guardDNF = lowered
	}

	op := &uws1.Operation{
		OperationID:     lt.opID,
		Outputs:         map[string]string{"changed": "$response.body.changed"},
		SuccessCriteria: []*uws1.Criterion{{Condition: "$response.body.failed != true"}},
	}
	if err := lw.bindAnsibleOperation(op, sourceID, task.Module); err != nil {
		lw.addDiag(Diagnostic{Code: CodeArgspecViolation, Severity: "error", StrictFailure: true, Task: task.Name,
			Message: fmt.Sprintf("module %s metadata is invalid: %v", task.Module, err)})
		lt.skipped = true
		return nil
	}
	if len(task.ChangedWhen) > 0 {
		if len(task.ChangedWhen) != 1 {
			lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: "changed_when with multiple conditions needs boolean conjunction semantics; the task was not lowered"})
			lt.skipped = true
			return nil
		}
		lowered, ok := lw.lowerSingleCondition(task.Name, task.ChangedWhen[0], ctx, "changed_when")
		if !ok {
			lt.skipped = true
			return nil
		}
		op.Outputs["changed"] = lowered
	}
	if len(task.FailedWhen) > 0 {
		if len(task.FailedWhen) != 1 {
			lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: "failed_when with multiple conditions needs boolean conjunction semantics; the task was not lowered"})
			lt.skipped = true
			return nil
		}
		lowered, ok := lw.lowerSingleCondition(task.Name, task.FailedWhen[0], ctx, "failed_when")
		if !ok {
			lt.skipped = true
			return nil
		}
		inverted, ok := invertSimpleComparison(lowered)
		if !ok {
			lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: "failed_when could not be inverted into a supported UWS comparison; the task was not lowered"})
			lt.skipped = true
			return nil
		}
		op.SuccessCriteria = []*uws1.Criterion{{Condition: inverted}}
	}
	if len(task.Until) > 0 {
		lowered, ok := lw.lowerConditionParts(task.Name, task.Until, ctx, "until")
		if !ok {
			lt.skipped = true
			return nil
		}
		lw.appendUntilRetryPolicy(op, task, lowered)
	} else if task.Retries != nil || task.Delay != nil {
		lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "warning", Task: task.Name,
			Message: "retries/delay without until do not define a UWS retry success condition; no retry action was emitted"})
	}
	if task.IgnoreErrors {
		lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "error", StrictFailure: true, Task: task.Name,
			Message: "ignore_errors: true needs continue-on-failure semantics not defined by UWS core; the task was not lowered"})
		lt.skipped = true
		return nil
	}
	if task.AnyErrorsFatal {
		// UWS sequence execution already fails fast, so no field is emitted.
	}
	if task.Throttle != nil {
		if lt.hostFanOut && *task.Throttle != 1 {
			lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: "throttle greater than 1 needs host fan-out concurrency semantics outside UWS core; the task was not lowered"})
			lt.skipped = true
			return nil
		} else if !lt.hostFanOut {
			lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "warning", Task: task.Name,
				Message: "throttle only affects lowered host fan-out; no concurrency limit was emitted"})
		}
	}
	if task.Name != "" {
		op.Description = task.Name
	}
	attachAnsibleProvenance(op, step, task)

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
	body, argspecDiags := lw.idx.NormalizeAndValidateArgs(task.Name, task.Module, body)
	lw.diags = append(lw.diags, argspecDiags...)
	if hasStrictFailure(argspecDiags) {
		lt.skipped = true
		return nil
	}
	if len(body) > 0 {
		op.Request = map[string]any{"body": body}
	}

	lw.doc.Operations = append(lw.doc.Operations, op)
	return lw.wrapGuardedStepDNF(step, guardDNF, task)
}

func hasStrictFailure(diags []Diagnostic) bool {
	for _, diag := range diags {
		if diag.StrictFailure {
			return true
		}
	}
	return false
}

// normalizeTargetUWS resolves the declared uws version of the emitted document.
// The emitted shape no longer varies by target: Ansible module leaves are
// always extension-owned operations, which are valid at every listed version.
func normalizeTargetUWS(target string) string {
	switch strings.TrimSpace(target) {
	case "", "1.5", "1.5.0":
		return TargetUWS15
	case "1.6", "1.6.0":
		return TargetUWS16
	case "1.7", "1.7.0":
		return TargetUWS17
	default:
		return strings.TrimSpace(target)
	}
}

func targetUWSDocumentVersion(target string) string {
	switch target {
	case TargetUWS16:
		return "1.6.0"
	case TargetUWS17:
		return "1.7.0"
	default:
		return "1.5.0"
	}
}

// bindAnsibleOperation emits the module leaf as an extension-owned operation.
// The managed host does not expose the collection module as a pre-existing
// named operation; the control node supplies its implementation. UWS 1.6
// briefly offered a first-class ansible-module source type; UWS 1.7 removed it.
func (lw *lowerer) bindAnsibleOperation(op *uws1.Operation, sourceID, module string) error {
	op.Extensions = map[string]any{
		uws1.ExtensionOperationProfile: ProfileName,
	}
	input, _ := lw.idx.Source(sourceID)
	return SetOperationExtension(&op.Extensions, &OperationAnsibleModule{
		Module: module,
		Argspec: &ArgspecReference{
			SourceID:   sourceID,
			URL:        input.Path,
			Collection: lw.idx.Collection(sourceID),
		},
	})
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
				Message: "handler notification after host fan-out needs per-host changed evaluation; UWS forEach aggregates changed outputs, so the handler was not lowered"})
			return nil
		}
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
		conditions, ok := lw.handlerGateConditions(handler, fmt.Sprintf("$steps.%s.outputs.changed == true", active[0].stepID))
		if !ok {
			return nil
		}
		return lw.wrapGuardedStepDNF(step, conditions, handler)
	}
	wrapper := &uws1.Step{
		StepID: lw.uniqueID(stepID + "_notify"),
		Type:   uws1.WorkflowTypeSwitch,
		Extensions: map[string]any{
			ExtensionAnsibleProvenance: ansibleProvenance(handler),
		},
	}
	for i, notifier := range active {
		inner := &uws1.Step{
			StepID:       lw.uniqueID(fmt.Sprintf("%s_run_%d", stepID, i+1)),
			OperationRef: lt.opID,
			Inputs:       cloneMapAny(step.Inputs),
			Extensions: map[string]any{
				ExtensionAnsibleProvenance: ansibleProvenance(handler),
			},
		}
		c := &uws1.Case{Steps: []*uws1.Step{inner}}
		c.Name = fmt.Sprintf("notified_by_%s", notifier.stepID)
		conditions, ok := lw.handlerGateConditions(handler, fmt.Sprintf("$steps.%s.outputs.changed == true", notifier.stepID))
		if !ok {
			return nil
		}
		if len(conditions) == 0 || len(conditions) > 1 {
			if len(conditions) > 1 {
				lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "error", StrictFailure: true, Task: handler.Name,
					Message: "handler when with OR semantics cannot be combined with multi-notifier handler lowering; the handler was not lowered"})
				return nil
			}
			wrapper.Cases = append(wrapper.Cases, c)
			continue
		}
		c.When = conditions[0][0]
		if len(conditions[0]) > 1 {
			c.Steps = []*uws1.Step{lw.wrapGuardedStep(inner, conditions[0][1:], handler)}
		}
		wrapper.Cases = append(wrapper.Cases, c)
	}
	return wrapper
}

func (lw *lowerer) handlerGateConditions(handler *Task, notifierGate string) (conditionDNF, bool) {
	if len(handler.When) == 0 {
		return conditionDNF{{notifierGate}}, true
	}
	ctx := &exprContext{vars: handler.StaticPlayVars, registered: lw.registered, taskVars: boolKeys(handler.Vars), hostVars: handler.StaticInventoryVars, currentRegister: handler.Register, needOutput: lw.noteNeededOutput}
	guard, ok := lw.lowerConditionDNF(handler.Name, handler.When, ctx, "handler guard")
	if !ok {
		return nil, false
	}
	out := make(conditionDNF, 0, len(guard))
	for _, group := range guard {
		combined := []string{notifierGate}
		combined = append(combined, group...)
		out = append(out, combined)
	}
	return out, true
}

func (lw *lowerer) lowerTaskVars(task *Task) (map[string]any, bool) {
	if len(task.Vars) == 0 {
		return nil, true
	}
	out := make(map[string]any, len(task.Vars))
	for name, value := range task.Vars {
		if !isStaticAnsibleValue(value) {
			lw.addDiag(Diagnostic{Code: CodeDirectiveTodo, Severity: "error", StrictFailure: true, Task: task.Name,
				Message: fmt.Sprintf("task-local var %q is not static; the task was not lowered", name)})
			return nil, false
		}
		out[name] = value
	}
	return out, true
}

func isStaticAnsibleValue(value any) bool {
	switch v := value.(type) {
	case string:
		return !strings.Contains(v, "{{") && !strings.Contains(v, "{%")
	case []any:
		for _, item := range v {
			if !isStaticAnsibleValue(item) {
				return false
			}
		}
	case map[string]any:
		for _, item := range v {
			if !isStaticAnsibleValue(item) {
				return false
			}
		}
	}
	return true
}

func boolKeys(values map[string]any) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]bool, len(values))
	for key := range values {
		out[key] = true
	}
	return out
}

func cloneMapAny(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneBoolMap(values map[string]bool) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]bool, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func annotateTaskScopes(tasks []*Task, playVars, hostVars map[string]bool) {
	for _, task := range tasks {
		if task == nil {
			continue
		}
		task.StaticPlayVars = cloneBoolMap(playVars)
		task.StaticInventoryVars = cloneBoolMap(hostVars)
		annotateTaskScopes(task.Block, playVars, hostVars)
		annotateTaskScopes(task.Rescue, playVars, hostVars)
		annotateTaskScopes(task.Always, playVars, hostVars)
	}
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

func (lw *lowerer) ensureComponents() {
	if lw.doc.Components == nil {
		lw.doc.Components = &uws1.Components{Variables: map[string]any{}}
	}
	if lw.doc.Components.Variables == nil {
		lw.doc.Components.Variables = map[string]any{}
	}
}

func (lw *lowerer) handlerRefs(play *Play) map[*Task][]string {
	refsByHandler := map[*Task][]string{}
	seen := map[string]*Task{}
	for _, handler := range play.Handlers {
		var refs []string
		if strings.TrimSpace(handler.Name) != "" {
			refs = append(refs, strings.TrimSpace(handler.Name))
		}
		for _, listen := range handler.Listen {
			if strings.TrimSpace(listen) != "" {
				refs = append(refs, strings.TrimSpace(listen))
			}
		}
		refs = compactSorted(refs)
		refsByHandler[handler] = refs
		for _, ref := range refs {
			if existing := seen[ref]; existing != nil {
				lw.addDiag(Diagnostic{Code: CodePlaybookShape, Severity: "error", StrictFailure: true, Task: handler.Name,
					Message: fmt.Sprintf("handler name/listen alias %q is duplicated in play %q", ref, play.Name)})
				continue
			}
			seen[ref] = handler
		}
	}
	return refsByHandler
}

func attachAnsibleProvenance(op *uws1.Operation, step *uws1.Step, task *Task) {
	prov := ansibleProvenance(task)
	if op.Extensions == nil {
		op.Extensions = map[string]any{}
	}
	op.Extensions[ExtensionAnsibleProvenance] = prov
	step.Extensions = map[string]any{ExtensionAnsibleProvenance: prov}
}

func ansibleProvenance(task *Task) map[string]any {
	prov := map[string]any{
		"version":    "ramen.ansible.provenance.v1",
		"sourceFile": task.SourceFile,
		"line":       task.Line,
		"column":     task.Column,
		"play":       task.PlayName,
		"section":    task.Section,
		"task":       task.Name,
	}
	if task.Role != "" {
		prov["role"] = task.Role
	}
	if len(task.ImportStack) > 0 {
		prov["importStack"] = append([]string(nil), task.ImportStack...)
	}
	if len(task.Tags) > 0 {
		prov["tags"] = append([]string(nil), task.Tags...)
	}
	return prov
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
