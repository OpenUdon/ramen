package ansibleconvert

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// exprContext carries the name environments needed to lower Jinja2 references
// into UWS core runtime expressions.
type exprContext struct {
	// vars are play-level variables, addressed as $variables.<name>.
	vars map[string]bool
	// registered maps a register variable name to the producing step ID.
	registered map[string]string
	// currentRegister is the register name of the task currently being lowered.
	currentRegister string
	// taskVars are task-local static variables, addressed as $inputs.<name>.
	taskVars map[string]bool
	// inLoop is true while lowering values inside a forEach scope.
	inLoop bool
	// needOutput records that the producing step must expose a response path
	// as a named output: needOutput(stepID, outputName, responsePath).
	needOutput func(stepID, outputName, responsePath string)
}

var (
	wholeTemplateRE = regexp.MustCompile(`^\{\{\s*(.+?)\s*\}\}$`)
	identRE         = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	dottedRE        = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)+$`)
	whenCompareRE   = regexp.MustCompile(`^(.+?)\s*(==|!=|<=|>=|<|>)\s*(.+)$`)
)

// lowerValue lowers one Ansible scalar string. Literals pass through; a
// whole-string {{ ... }} template lowers to a UWS expression when the inner
// reference is a known variable, loop item, or registered result path.
// Anything else (filters, lookups, math, mixed interpolation) fails with a
// reason for the diagnostic.
func lowerValue(s string, ctx *exprContext) (string, bool, string) {
	if !strings.Contains(s, "{{") {
		return s, true, ""
	}
	m := wholeTemplateRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", false, "mixed text and template interpolation is not part of UWS core expressions"
	}
	return lowerReference(m[1], ctx)
}

// lowerReference lowers a bare Jinja2 reference (no surrounding braces).
func lowerReference(inner string, ctx *exprContext) (string, bool, string) {
	inner = strings.TrimSpace(inner)
	if strings.ContainsAny(inner, "|(+-*/%['\"") {
		return "", false, fmt.Sprintf("expression %q uses Jinja2 features (filters, lookups, math, or indexing) outside UWS core", inner)
	}
	switch {
	case inner == "item":
		if !ctx.inLoop {
			return "", false, "item referenced outside a loop"
		}
		return "$item", true, ""
	case inner == "index" || inner == "ansible_loop.index0":
		if !ctx.inLoop {
			return "", false, "index referenced outside a loop"
		}
		return "$index", true, ""
	case identRE.MatchString(inner):
		if ctx.taskVars[inner] {
			return "$inputs." + inner, true, ""
		}
		if ctx.vars[inner] {
			return "$variables." + inner, true, ""
		}
		if _, isRegistered := ctx.registered[inner]; isRegistered {
			return "", false, fmt.Sprintf("registered variable %q referenced as a whole object; reference a field path instead", inner)
		}
		// Unknown bare identifier: assume a runtime-supplied workflow input.
		return "$inputs." + inner, true, ""
	case dottedRE.MatchString(inner):
		head, rest, _ := strings.Cut(inner, ".")
		if strings.HasPrefix(inner, "ansible_facts.") || strings.HasPrefix(head, "ansible_") {
			return "", false, fmt.Sprintf("fact reference %q requires a facts-gathering step; not lowered automatically", inner)
		}
		if stepID, isRegistered := ctx.registered[head]; isRegistered {
			outputName := strings.ReplaceAll(rest, ".", "_")
			if ctx.needOutput != nil {
				ctx.needOutput(stepID, outputName, rest)
			}
			return fmt.Sprintf("$steps.%s.outputs.%s", stepID, outputName), true, ""
		}
		if ctx.currentRegister != "" && head == ctx.currentRegister {
			return "$response.body." + rest, true, ""
		}
		if ctx.taskVars[head] {
			return "$inputs." + inner, true, ""
		}
		if ctx.vars[head] {
			return "$variables." + inner, true, ""
		}
		return "", false, fmt.Sprintf("unknown dotted reference %q", inner)
	default:
		return "", false, fmt.Sprintf("expression %q is not a lowerable reference", inner)
	}
}

// lowerWhen lowers one Ansible `when:` condition (Jinja2 without braces) into
// a UWS core boolean expression: a single binary comparison, or a bare truthy
// reference lowered to `<expr> == true`.
func lowerWhen(cond string, ctx *exprContext) (string, bool, string) {
	cond = strings.TrimSpace(cond)
	lower := strings.ToLower(cond)
	for _, kw := range []string{" and ", " or ", " not ", " is ", " in "} {
		if strings.Contains(lower, kw) {
			return "", false, fmt.Sprintf("when condition %q uses %q; UWS core supports a single binary comparison", cond, strings.TrimSpace(kw))
		}
	}
	if m := whenCompareRE.FindStringSubmatch(cond); m != nil {
		left, ok, reason := lowerReference(stripTemplateBraces(m[1]), ctx)
		if !ok {
			return "", false, reason
		}
		operand, ok, reason := lowerWhenOperand(m[3], ctx)
		if !ok {
			return "", false, reason
		}
		return fmt.Sprintf("%s %s %s", left, m[2], operand), true, ""
	}
	ref, ok, reason := lowerReference(stripTemplateBraces(cond), ctx)
	if !ok {
		return "", false, reason
	}
	return ref + " == true", true, ""
}

// lowerWhenOperand lowers a comparison right-hand side to a JSON literal.
func lowerWhenOperand(s string, ctx *exprContext) (string, bool, string) {
	s = strings.TrimSpace(s)
	switch {
	case s == "true" || s == "True":
		return "true", true, ""
	case s == "false" || s == "False":
		return "false", true, ""
	case s == "none" || s == "None" || s == "null":
		return "null", true, ""
	}
	if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) && len(s) >= 2 {
		return s, true, ""
	}
	if (strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`)) && len(s) >= 2 {
		return strconv.Quote(strings.Trim(s, `'`)), true, ""
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return s, true, ""
	}
	if ref, ok, reason := lowerReference(stripTemplateBraces(s), ctx); ok && strings.HasPrefix(ref, "$") {
		return ref, true, ""
	} else if reason != "" && (identRE.MatchString(s) || dottedRE.MatchString(s)) {
		return "", false, reason
	}
	return "", false, fmt.Sprintf("comparison operand %q is not a JSON literal", s)
}

// stripTemplateBraces tolerates `{{ var }}` inside when conditions, which is
// redundant but common in real playbooks.
func stripTemplateBraces(s string) string {
	s = strings.TrimSpace(s)
	if m := wholeTemplateRE.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return s
}
