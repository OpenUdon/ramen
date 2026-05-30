package stateprojection

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/OpenUdon/ramen/executor"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
)

func ClassifyReadResult(execResult executor.Result, execErr error) (executor.Result, error) {
	if execResult.Missing && execErr == nil {
		return missingResult(execResult, execErr), nil
	}
	if readNotFoundSignal(execErr) || (!execResult.Success && resultNotFoundSignal(execResult)) {
		return missingResult(execResult, execErr), nil
	}
	return execResult, execErr
}

func Project(mapping *tfplan.MappingPlan, execResult executor.Result) (map[string]any, map[string]any) {
	identity := cloneAnyMap(execResult.Identity)
	computed := cloneAnyMap(execResult.Computed)
	if mapping == nil {
		return identity, computed
	}
	for _, binding := range mapping.ResponseBindings {
		value, ok := responseBindingValue(binding, execResult)
		if !ok {
			continue
		}
		if binding.ResponsePath != binding.StatePath {
			deleteDottedAny(identity, binding.ResponsePath)
			deleteDottedAny(computed, binding.ResponsePath)
		}
		if binding.Identity || binding.ResponseDerivedIdentity {
			setDottedAny(identity, binding.StatePath, value)
		}
		if binding.Computed || binding.Observed {
			setDottedAny(computed, binding.StatePath, value)
		}
		if binding.Sensitive {
			if binding.Identity || binding.ResponseDerivedIdentity {
				setDottedAny(identity, binding.StatePath, "${redacted}")
			}
			if binding.Computed || binding.Observed {
				setDottedAny(computed, binding.StatePath, "${redacted}")
			}
		}
	}
	applyNormalizers(identity, mapping.Normalizers)
	applyNormalizers(computed, mapping.Normalizers)
	return identity, computed
}

func missingResult(execResult executor.Result, execErr error) executor.Result {
	execResult.Success = true
	execResult.Missing = true
	if execErr != nil {
		message := execErr.Error()
		if !slices.Contains(execResult.Messages, message) {
			execResult.Messages = append(execResult.Messages, message)
		}
	}
	return execResult
}

func resultNotFoundSignal(execResult executor.Result) bool {
	for _, message := range execResult.Messages {
		if readNotFoundText(message) {
			return true
		}
	}
	for _, event := range execResult.Events {
		if readNotFoundText(event.Message) {
			return true
		}
		for key, value := range event.Metadata {
			if readNotFoundText(key) || readNotFoundText(fmt.Sprint(value)) {
				return true
			}
		}
	}
	return false
}

func readNotFoundSignal(err error) bool {
	return err != nil && readNotFoundText(err.Error())
}

func readNotFoundText(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	normalized := strings.NewReplacer("_", " ", "-", " ", ".", " ", ":", " ").Replace(text)
	switch {
	case strings.Contains(text, "notfound"):
		return true
	case strings.Contains(normalized, "not found"):
		return true
	case strings.Contains(normalized, "no such entity"):
		return true
	case strings.Contains(normalized, "does not exist"):
		return true
	case strings.Contains(normalized, "404"):
		return true
	default:
		return false
	}
}

func responseBindingValue(binding project.ResponseBinding, execResult executor.Result) (any, bool) {
	for _, values := range []map[string]any{execResult.Identity, execResult.Computed} {
		if value, ok := dottedAny(values, binding.ResponsePath); ok {
			return value, true
		}
		if value, ok := dottedAny(values, binding.StatePath); ok {
			return value, true
		}
	}
	return nil, false
}

func cloneAnyMap(values map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func dottedAny(values map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	var cur any = values
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func setDottedAny(values map[string]any, path string, value any) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || parts[0] == "" {
		return
	}
	cur := values
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[part] = next
		}
		cur = next
	}
	cur[parts[len(parts)-1]] = value
}

func deleteDottedAny(values map[string]any, path string) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || parts[0] == "" {
		return
	}
	deleteDottedAnyParts(values, parts)
}

func deleteDottedAnyParts(values map[string]any, parts []string) bool {
	if len(parts) == 1 {
		delete(values, parts[0])
		return len(values) == 0
	}
	next, ok := values[parts[0]].(map[string]any)
	if !ok {
		return len(values) == 0
	}
	if deleteDottedAnyParts(next, parts[1:]) {
		delete(values, parts[0])
	}
	return len(values) == 0
}

func applyNormalizers(values map[string]any, normalizers []project.Normalizer) {
	for _, normalizer := range normalizers {
		value, ok := dottedAny(values, normalizer.Path)
		if !ok {
			continue
		}
		normalized, keep := normalizeValue(value, normalizer.Kind)
		if keep {
			setDottedAny(values, normalizer.Path, normalized)
		} else {
			deleteDottedAny(values, normalizer.Path)
		}
	}
}

func normalizeValue(value any, kind string) (any, bool) {
	switch strings.TrimSpace(kind) {
	case "json_semantic":
		text, ok := value.(string)
		if !ok {
			return value, true
		}
		var decoded any
		if err := json.Unmarshal([]byte(text), &decoded); err != nil {
			return value, true
		}
		data, err := json.Marshal(decoded)
		if err != nil {
			return value, true
		}
		return string(data), true
	case "case_fold":
		text, ok := value.(string)
		if !ok {
			return value, true
		}
		return strings.ToLower(text), true
	case "unordered_collection":
		values, ok := value.([]any)
		if !ok {
			return value, true
		}
		out := slices.Clone(values)
		slices.SortFunc(out, func(a, b any) int {
			left, _ := json.Marshal(a)
			right, _ := json.Marshal(b)
			return strings.Compare(string(left), string(right))
		})
		return out, true
	case "empty_null_absent_equivalent":
		if value == nil {
			return nil, false
		}
		switch v := value.(type) {
		case string:
			return v, strings.TrimSpace(v) != ""
		case []any:
			return v, len(v) > 0
		case map[string]any:
			return v, len(v) > 0
		default:
			return value, true
		}
	case "sensitive_placeholder":
		return "${redacted}", true
	default:
		return value, true
	}
}
