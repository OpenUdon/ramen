package ansibleconvert

import (
	"fmt"
	"strings"

	"github.com/OpenUdon/uws/uws1"
)

func (lw *lowerer) lowerConditionParts(taskName string, conditions []string, ctx *exprContext, label string) ([]string, bool) {
	dnf, ok := lw.lowerConditionDNF(taskName, conditions, ctx, label)
	if !ok {
		return nil, false
	}
	if len(dnf) != 1 {
		lw.addDiag(Diagnostic{Code: CodeJinjaUnsupported, Severity: "error", StrictFailure: true, Task: taskName,
			Message: fmt.Sprintf("%s: OR-style condition cannot be lowered into success criteria; the task was not lowered", label)})
		return nil, false
	}
	return dnf[0], true
}

func (lw *lowerer) lowerConditionDNF(taskName string, conditions []string, ctx *exprContext, label string) (conditionDNF, bool) {
	var dnf conditionDNF = conditionDNF{{}}
	for _, condition := range conditions {
		lowered, ok, reason := lowerWhenDNF(condition, ctx)
		if !ok {
			lw.addDiag(Diagnostic{Code: CodeJinjaUnsupported, Severity: "error", StrictFailure: true, Task: taskName,
				Message: fmt.Sprintf("%s: %s; the task was not lowered", label, reason)})
			return nil, false
		}
		dnf = andDNF(dnf, lowered)
	}
	return dnf, true
}

func (lw *lowerer) lowerSingleCondition(taskName, condition string, ctx *exprContext, label string) (string, bool) {
	parts, ok := lw.lowerConditionParts(taskName, []string{condition}, ctx, label)
	if !ok || len(parts) == 0 {
		return "", ok
	}
	return parts[0], true
}

func (lw *lowerer) wrapGuardedStep(step *uws1.Step, conditions []string, task *Task) *uws1.Step {
	switch len(conditions) {
	case 0:
		return step
	case 1:
		step.When = conditions[0]
		return step
	}
	inner := step
	for i := len(conditions) - 1; i >= 0; i-- {
		wrapper := &uws1.Step{
			StepID: lw.uniqueID(fmt.Sprintf("%s_guard_%d", step.StepID, i+1)),
			Type:   uws1.WorkflowTypeSwitch,
			Cases: []*uws1.Case{{
				CaseFields: uws1.CaseFields{
					Name: fmt.Sprintf("condition_%d", i+1),
					When: conditions[i],
				},
				Steps: []*uws1.Step{inner},
			}},
			Extensions: map[string]any{
				ExtensionAnsibleProvenance: ansibleProvenance(task),
			},
		}
		inner = wrapper
	}
	return inner
}

func (lw *lowerer) wrapGuardedStepDNF(step *uws1.Step, dnf conditionDNF, task *Task) *uws1.Step {
	if len(dnf) == 0 {
		return step
	}
	if len(dnf) == 1 {
		return lw.wrapGuardedStep(step, dnf[0], task)
	}
	wrapper := &uws1.Step{
		StepID: lw.uniqueID(step.StepID + "_guard_or"),
		Type:   uws1.WorkflowTypeSwitch,
		Extensions: map[string]any{
			ExtensionAnsibleProvenance: ansibleProvenance(task),
		},
	}
	for i, group := range dnf {
		inner := cloneStepForGuardCase(step, lw.uniqueID(fmt.Sprintf("%s_or_%d", step.StepID, i+1)))
		c := &uws1.Case{
			CaseFields: uws1.CaseFields{
				Name: fmt.Sprintf("condition_%d", i+1),
			},
			Steps: []*uws1.Step{inner},
		}
		switch len(group) {
		case 0:
		case 1:
			c.When = group[0]
		default:
			c.When = group[0]
			c.Steps = []*uws1.Step{lw.wrapGuardedStep(inner, group[1:], task)}
		}
		wrapper.Cases = append(wrapper.Cases, c)
	}
	return wrapper
}

func conditionDNFWrapsStep(dnf conditionDNF) bool {
	if len(dnf) > 1 {
		return true
	}
	return len(dnf) == 1 && len(dnf[0]) > 1
}

func cloneStepForGuardCase(step *uws1.Step, stepID string) *uws1.Step {
	if step == nil {
		return nil
	}
	clone := *step
	clone.StepID = stepID
	clone.Inputs = cloneMapAny(step.Inputs)
	clone.DependsOn = append([]string(nil), step.DependsOn...)
	clone.Extensions = cloneMapAny(step.Extensions)
	clone.Steps = nil
	clone.Cases = nil
	clone.Default = nil
	return &clone
}

func (lw *lowerer) appendUntilRetryPolicy(op *uws1.Operation, task *Task, loweredUntil []string) {
	for _, condition := range loweredUntil {
		op.SuccessCriteria = append(op.SuccessCriteria, &uws1.Criterion{Condition: condition})
	}
	retryLimit := 0
	if task.Retries != nil {
		retryLimit = *task.Retries - 1
	}
	if retryLimit <= 0 {
		return
	}
	action := &uws1.FailureAction{Name: "retry_until", Type: "retry", RetryLimit: retryLimit}
	if task.Delay != nil {
		action.RetryAfter = *task.Delay
	}
	op.OnFailure = append(op.OnFailure, action)
}

func invertSimpleComparison(condition string) (string, bool) {
	m := whenCompareRE.FindStringSubmatch(strings.TrimSpace(condition))
	if m == nil {
		return "", false
	}
	var op string
	switch m[2] {
	case "==":
		op = "!="
	case "!=":
		op = "=="
	case "<":
		op = ">="
	case "<=":
		op = ">"
	case ">":
		op = "<="
	case ">=":
		op = "<"
	default:
		return "", false
	}
	return fmt.Sprintf("%s %s %s", strings.TrimSpace(m[1]), op, strings.TrimSpace(m[3])), true
}
