package ansibleconvert

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// argspecDocument is the uws.ansible.1.0 wire shape (versions/ansible.1.0.json
// in the uws repo).
type argspecDocument struct {
	Argspec    string                   `json:"argspec"`
	Collection string                   `json:"collection"`
	Modules    map[string]argspecModule `json:"modules"`
}

type argspecModule struct {
	ShortDescription string                      `json:"shortDescription"`
	Parameters       map[string]argspecParameter `json:"parameters"`
	Returns          map[string]json.RawMessage  `json:"returns"`
}

type argspecParameter struct {
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Choices  []any    `json:"choices"`
	Aliases  []string `json:"aliases"`
	NoLog    bool     `json:"noLog"`
}

// ArgspecIndex resolves module FQCNs across all supplied argspec documents.
type ArgspecIndex struct {
	bySource map[string]ArgspecInput
	byFQCN   map[string]moduleRef
}

type moduleRef struct {
	SourceID string
	Module   argspecModule
}

// LoadArgspecs reads and indexes the supplied uws.ansible.1.0 documents.
func LoadArgspecs(inputs []ArgspecInput) (*ArgspecIndex, error) {
	idx := &ArgspecIndex{bySource: map[string]ArgspecInput{}, byFQCN: map[string]moduleRef{}}
	for _, input := range inputs {
		data, err := os.ReadFile(input.Path)
		if err != nil {
			return nil, fmt.Errorf("argspec %s: %w", input.ID, err)
		}
		var doc argspecDocument
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("argspec %s: %w", input.ID, err)
		}
		if doc.Argspec != "uws.ansible.1.0" {
			return nil, fmt.Errorf("argspec %s: unsupported argspec %q (want uws.ansible.1.0)", input.ID, doc.Argspec)
		}
		if _, dup := idx.bySource[input.ID]; dup {
			return nil, fmt.Errorf("argspec %s: duplicate source ID", input.ID)
		}
		idx.bySource[input.ID] = input
		for fqcn, module := range doc.Modules {
			if existing, dup := idx.byFQCN[fqcn]; dup {
				return nil, fmt.Errorf("module %s declared by both %s and %s", fqcn, existing.SourceID, input.ID)
			}
			idx.byFQCN[fqcn] = moduleRef{SourceID: input.ID, Module: module}
		}
	}
	return idx, nil
}

// Lookup resolves an FQCN to the argspec source that declares it.
func (idx *ArgspecIndex) Lookup(fqcn string) (string, argspecModule, bool) {
	ref, ok := idx.byFQCN[fqcn]
	return ref.SourceID, ref.Module, ok
}

// Source returns the original input for a source ID.
func (idx *ArgspecIndex) Source(id string) (ArgspecInput, bool) {
	input, ok := idx.bySource[id]
	return input, ok
}

// ValidateArgs checks lowered module arguments against the module's parameter
// specification. UWS runtime expressions are opaque to choices validation.
func (idx *ArgspecIndex) ValidateArgs(taskName, fqcn string, args map[string]any) []Diagnostic {
	_, module, ok := idx.Lookup(fqcn)
	if !ok {
		return nil
	}
	canonical := map[string]string{}
	for name, param := range module.Parameters {
		canonical[name] = name
		for _, alias := range param.Aliases {
			canonical[alias] = name
		}
	}
	var diags []Diagnostic
	seen := map[string]bool{}
	for name, value := range args {
		paramName, known := canonical[name]
		if !known {
			diags = append(diags, Diagnostic{
				Code: CodeArgspecViolation, Severity: "error", StrictFailure: true, Task: taskName,
				Message: fmt.Sprintf("module %s has no parameter %q", fqcn, name),
			})
			continue
		}
		seen[paramName] = true
		param := module.Parameters[paramName]
		if param.NoLog {
			if s, isString := value.(string); !isString || !isRuntimeExpression(s) {
				diags = append(diags, Diagnostic{
					Code: CodeNoLogLiteral, Severity: "error", StrictFailure: true, Task: taskName,
					Message: fmt.Sprintf("parameter %q of %s is sensitive (noLog); use a symbolic credential binding, not a literal", paramName, fqcn),
				})
			}
			continue
		}
		if len(param.Choices) > 0 {
			if s, isString := value.(string); isString && !isRuntimeExpression(s) && !choiceAllowed(param.Choices, s) {
				diags = append(diags, Diagnostic{
					Code: CodeArgspecViolation, Severity: "error", StrictFailure: true, Task: taskName,
					Message: fmt.Sprintf("parameter %q of %s: %q is not one of the documented choices", paramName, fqcn, s),
				})
			}
		}
	}
	for name, param := range module.Parameters {
		if param.Required && !seen[name] {
			diags = append(diags, Diagnostic{
				Code: CodeArgspecViolation, Severity: "error", StrictFailure: true, Task: taskName,
				Message: fmt.Sprintf("module %s requires parameter %q", fqcn, name),
			})
		}
	}
	return diags
}

func choiceAllowed(choices []any, value string) bool {
	for _, choice := range choices {
		if fmt.Sprintf("%v", choice) == value {
			return true
		}
	}
	return false
}

func isRuntimeExpression(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "$")
}
