package apply

import (
	"regexp"
	"strings"

	"github.com/OpenUdon/ramen/executor"
)

const redactedValue = "${redacted}"

var secretValuePattern = regexp.MustCompile(`(?i)(AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

func redactExecutorResult(result executor.Result) executor.Result {
	result.Identity = redactMap(result.Identity)
	result.Computed = redactMap(result.Computed)
	for i, msg := range result.Messages {
		result.Messages[i] = redactString(msg)
	}
	return result
}

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
		out[key] = redactValue(value)
	}
	return out
}

func redactValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return redactMap(v)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = redactValue(v[i])
		}
		return out
	case string:
		return redactString(v)
	default:
		return value
	}
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
	if secretValuePattern.MatchString(value) {
		return redactedValue
	}
	return value
}
