package apply

import (
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/internal/redact"
)

const redactedValue = redact.Value

func redactExecutorResult(result executor.Result) executor.Result {
	result.Identity = redactMap(result.Identity)
	result.Computed = redactMap(result.Computed)
	for i, msg := range result.Messages {
		result.Messages[i] = redactString(msg)
	}
	for i := range result.Events {
		result.Events[i].Message = redactString(result.Events[i].Message)
		result.Events[i].Metadata = redactMap(result.Events[i].Metadata)
	}
	return result
}

func redactMap(in map[string]any) map[string]any {
	return redact.Map(in)
}

func redactValue(value any) any {
	return redact.Any(value)
}

func sensitiveKey(key string) bool {
	return redact.SensitiveKey(key)
}

func redactString(value string) string {
	return redact.String(value)
}
