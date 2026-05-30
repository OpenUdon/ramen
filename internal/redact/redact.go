// Package redact applies OpenUdon Ramen's conservative redaction policy on top
// of the shared evidence/redact primitives. Ramen deliberately redacts an
// entire string that mentions a secret-like keyword, because untrusted executor
// output may embed secrets in free text. Structural traversal and
// provider/credential pattern redaction are delegated to evidence/redact.
package redact

import (
	"regexp"

	ev "github.com/OpenUdon/evidence/redact"
)

// Value is the marker substituted for redacted content.
const Value = ev.Value

// policy layers Ramen's blunt whole-string keyword rule onto evidence's
// surgical redaction. The pattern matches any string (multiline via the s
// flag) that contains a secret-like keyword as a substring, so the whole value
// collapses to the marker.
var policy = ev.Options{
	ExtraPatterns: []*regexp.Regexp{
		regexp.MustCompile(`(?is).*(?:token|secret|password|authorization|credential).*`),
	},
}

// Map redacts a map[string]any document.
func Map(in map[string]any) map[string]any { return ev.Map(in, policy) }

// Any redacts common JSON/YAML-like values.
func Any(value any) any { return ev.Any(value, policy) }

// SensitiveKey reports whether a key name usually carries a secret value.
func SensitiveKey(key string) bool { return ev.SensitiveKey(key, policy) }

// String redacts secret-like content from value.
func String(value string) string { return ev.String(value, policy) }
