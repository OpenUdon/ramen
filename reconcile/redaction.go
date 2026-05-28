package reconcile

import (
	"regexp"
	"strings"
)

const redactedValue = "${redacted}"

var secretValuePattern = regexp.MustCompile(`(?i)(AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

func redactMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		if sensitiveKey(key) {
			out[key] = redactedValue
			continue
		}
		if nested, ok := value.(map[string]any); ok {
			out[key] = redactMap(nested)
			continue
		}
		if text, ok := value.(string); ok {
			out[key] = redactString(text)
			continue
		}
		out[key] = value
	}
	return out
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"secret", "token", "password", "passwd", "authorization", "credential", "access_key", "private_key"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func redactString(value string) string {
	lower := strings.ToLower(value)
	for _, marker := range []string{"token", "secret", "password", "authorization", "credential"} {
		if strings.Contains(lower, marker) {
			return redactedValue
		}
	}
	if secretValuePattern.MatchString(value) {
		return redactedValue
	}
	return value
}
