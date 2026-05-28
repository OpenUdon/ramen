package redact

import (
	"regexp"
	"strings"
)

const Value = "${redacted}"

var secretValuePattern = regexp.MustCompile(`(?i)(AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

func Map(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		if SensitiveKey(key) {
			out[key] = Value
			continue
		}
		out[key] = Any(value)
	}
	return out
}

func Any(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return Map(v)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = Any(v[i])
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(v))
		for i := range v {
			out[i] = Map(v[i])
		}
		return out
	case []string:
		out := make([]string, len(v))
		for i := range v {
			out[i] = String(v[i])
		}
		return out
	case string:
		return String(v)
	default:
		return value
	}
}

func SensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"secret", "token", "password", "passwd", "authorization", "credential", "access_key", "private_key"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func String(value string) string {
	lower := strings.ToLower(value)
	for _, marker := range []string{"token", "secret", "password", "authorization", "credential"} {
		if strings.Contains(lower, marker) {
			return Value
		}
	}
	if secretValuePattern.MatchString(value) {
		return Value
	}
	return value
}
