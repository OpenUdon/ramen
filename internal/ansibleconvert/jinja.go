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
	// hostVars are inventory variables available only while a statically
	// selected inventory host object is the current forEach item.
	hostVars   map[string]bool
	inHostLoop bool
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
	if strings.ContainsAny(inner, "|+-*/%['\"") {
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
		if ctx.inHostLoop && ctx.hostVars[inner] {
			return "$item.vars." + inner, true, ""
		}
		if _, isRegistered := ctx.registered[inner]; isRegistered {
			return "", false, fmt.Sprintf("registered variable %q referenced as a whole object; reference a field path instead", inner)
		}
		// Unknown bare identifier: assume a runtime-supplied workflow input.
		return "$inputs." + inner, true, ""
	case dottedRE.MatchString(inner):
		head, rest, _ := strings.Cut(inner, ".")
		if head == "item" {
			if !ctx.inLoop {
				return "", false, "item field referenced outside a task loop"
			}
			return "$item." + rest, true, ""
		}
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
		if ctx.inHostLoop && ctx.hostVars[head] {
			return "$item.vars." + inner, true, ""
		}
		return "", false, fmt.Sprintf("unknown dotted reference %q", inner)
	default:
		return "", false, fmt.Sprintf("expression %q is not a lowerable reference", inner)
	}
}

// conditionDNF is a disjunction of conjunction groups. Each inner slice is a
// set of UWS core comparisons that must all pass.
type conditionDNF [][]string

// lowerWhenDNF lowers one Ansible `when:` condition into portable UWS guard
// groups. It accepts simple boolean composition only when every leaf lowers to
// a UWS core comparison.
func lowerWhenDNF(cond string, ctx *exprContext) (conditionDNF, bool, string) {
	cond = strings.TrimSpace(cond)
	if cond == "" {
		return nil, false, "empty condition"
	}
	cond = trimOuterParens(cond)
	if left, right, ok := splitTopLevelKeyword(cond, "or"); ok {
		leftDNF, ok, reason := lowerWhenDNF(left, ctx)
		if !ok {
			return nil, false, reason
		}
		rightDNF, ok, reason := lowerWhenDNF(right, ctx)
		if !ok {
			return nil, false, reason
		}
		return append(leftDNF, rightDNF...), true, ""
	}
	if left, right, ok := splitTopLevelKeyword(cond, "and"); ok {
		leftDNF, ok, reason := lowerWhenDNF(left, ctx)
		if !ok {
			return nil, false, reason
		}
		rightDNF, ok, reason := lowerWhenDNF(right, ctx)
		if !ok {
			return nil, false, reason
		}
		return andDNF(leftDNF, rightDNF), true, ""
	}
	if rest, ok := trimKeywordPrefix(cond, "not"); ok {
		lowered, ok, reason := lowerNegatedWhenLeaf(rest, ctx)
		if !ok {
			return nil, false, reason
		}
		return conditionDNF{{lowered}}, true, ""
	}
	lowered, ok, reason := lowerWhenLeaf(cond, ctx)
	if !ok {
		return nil, false, reason
	}
	return conditionDNF{{lowered}}, true, ""
}

func andDNF(left, right conditionDNF) conditionDNF {
	var out conditionDNF
	for _, l := range left {
		for _, r := range right {
			group := append([]string(nil), l...)
			group = append(group, r...)
			out = append(out, group)
		}
	}
	return out
}

// lowerWhenLeaf lowers one non-composite Ansible condition into a UWS core
// boolean expression: a single binary comparison, an `is defined` check, or a
// bare truthy reference lowered to `<expr> == true`.
func lowerWhenLeaf(cond string, ctx *exprContext) (string, bool, string) {
	cond = strings.TrimSpace(cond)
	if refText, negated, ok := parseDefinedTest(cond); ok {
		ref, ok, reason := lowerReference(stripTemplateBraces(refText), ctx)
		if !ok {
			return "", false, reason
		}
		if negated {
			return ref + " == null", true, ""
		}
		return ref + " != null", true, ""
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

func lowerNegatedWhenLeaf(cond string, ctx *exprContext) (string, bool, string) {
	cond = trimOuterParens(strings.TrimSpace(cond))
	if strings.ContainsAny(strings.ToLower(cond), "()") {
		return "", false, fmt.Sprintf("negated condition %q is not a simple lowerable leaf", cond)
	}
	if refText, negated, ok := parseDefinedTest(cond); ok {
		ref, ok, reason := lowerReference(stripTemplateBraces(refText), ctx)
		if !ok {
			return "", false, reason
		}
		if negated {
			return ref + " != null", true, ""
		}
		return ref + " == null", true, ""
	}
	if m := whenCompareRE.FindStringSubmatch(cond); m != nil {
		lowered, ok, reason := lowerWhenLeaf(cond, ctx)
		if !ok {
			return "", false, reason
		}
		inverted, ok := invertSimpleComparison(lowered)
		if !ok {
			return "", false, fmt.Sprintf("condition %q could not be inverted into UWS comparison syntax", cond)
		}
		return inverted, true, ""
	}
	ref, ok, reason := lowerReference(stripTemplateBraces(cond), ctx)
	if !ok {
		return "", false, reason
	}
	return ref + " != true", true, ""
}

func parseDefinedTest(cond string) (ref string, negated bool, ok bool) {
	parts := strings.Fields(cond)
	if len(parts) == 3 && parts[1] == "is" && parts[2] == "defined" {
		return parts[0], false, true
	}
	if len(parts) == 4 && parts[1] == "is" && parts[2] == "not" && parts[3] == "defined" {
		return parts[0], true, true
	}
	return "", false, false
}

func trimOuterParens(s string) string {
	for {
		s = strings.TrimSpace(s)
		if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") || !outerParensWrapWhole(s) {
			return s
		}
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
}

func outerParensWrapWhole(s string) bool {
	depth := 0
	quote := rune(0)
	for i, r := range s {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i != len(s)-1 {
				return false
			}
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

func splitTopLevelKeyword(s, keyword string) (string, string, bool) {
	lower := strings.ToLower(s)
	depth := 0
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		switch ch {
		case '(':
			depth++
			continue
		case ')':
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth != 0 || !keywordAt(lower, i, keyword) {
			continue
		}
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+len(keyword):]), true
	}
	return "", "", false
}

func keywordAt(s string, i int, keyword string) bool {
	if i < 0 || i+len(keyword) > len(s) || s[i:i+len(keyword)] != keyword {
		return false
	}
	beforeOK := i == 0 || isKeywordBoundary(s[i-1])
	after := i + len(keyword)
	afterOK := after == len(s) || isKeywordBoundary(s[after])
	return beforeOK && afterOK
}

func trimKeywordPrefix(s, keyword string) (string, bool) {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	if !keywordAt(lower, 0, keyword) {
		return "", false
	}
	return strings.TrimSpace(s[len(keyword):]), true
}

func isKeywordBoundary(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '(' || ch == ')'
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
