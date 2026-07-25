package ansibleconvert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestLoadArgspecsRejectsInvalidSchemaDocuments(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "unknown field",
			content: `{"argspec":"uws.ansible.1.0","collection":"acme.tools","modules":{"acme.tools.file":{"parameters":{},"unexpected":true}}}`,
		},
		{
			name:    "missing required field",
			content: `{"argspec":"uws.ansible.1.0","collection":"acme.tools"}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeArgspecFixture(t, tc.content)
			_, err := LoadArgspecs([]ArgspecInput{{ID: "tools", Path: path}})
			if err == nil || !strings.Contains(err.Error(), "schema validation failed") {
				t.Fatalf("LoadArgspecs error = %v, want schema validation failure", err)
			}
		})
	}
}

func TestLoadArgspecsRejectsCollectionMismatch(t *testing.T) {
	path := writeArgspecFixture(t, `{
  "argspec": "uws.ansible.1.0",
  "collection": "acme.tools",
  "modules": {
    "other.tools.file": {"parameters": {}}
  }
}`)
	_, err := LoadArgspecs([]ArgspecInput{{ID: "tools", Path: path}})
	if err == nil || !strings.Contains(err.Error(), "does not belong to declared collection acme.tools") {
		t.Fatalf("LoadArgspecs error = %v, want collection mismatch", err)
	}
}

func TestLoadArgspecsRejectsAmbiguousAliasOwnership(t *testing.T) {
	path := writeArgspecFixture(t, `{
  "argspec": "uws.ansible.1.0",
  "collection": "acme.tools",
  "modules": {
    "acme.tools.file": {
      "parameters": {
        "path": {"type": "path", "aliases": ["dest"]},
        "dest": {"type": "path"}
      }
    }
  }
}`)
	_, err := LoadArgspecs([]ArgspecInput{{ID: "tools", Path: path}})
	if err == nil || !strings.Contains(err.Error(), `parameter spelling "dest" is owned by both`) {
		t.Fatalf("LoadArgspecs error = %v, want ambiguous alias ownership", err)
	}
}

func TestConvertNormalizesArgspecAliasesAndOmitsConflicts(t *testing.T) {
	argspecPath := writeArgspecFixture(t, `{
  "argspec": "uws.ansible.1.0",
  "collection": "acme.tools",
  "modules": {
    "acme.tools.file": {
      "parameters": {
        "path": {"type": "path", "required": true, "aliases": ["dest"]},
        "state": {"type": "str"}
      }
    }
  }
}`)
	playbookPath := writePlaybookFixture(t, `- name: aliases
  hosts: localhost
  tasks:
    - name: Alias only
      acme.tools.file:
        dest: /tmp/alias
        state: file
    - name: Equal duplicate
      acme.tools.file:
        path: /tmp/equal
        dest: /tmp/equal
    - name: Conflicting duplicate
      acme.tools.file:
        path: /tmp/one
        dest: /tmp/two
    - name: Safe sibling
      acme.tools.file:
        path: /tmp/safe
`)
	result, doc := convertFixture(t, playbookPath, argspecPath, true)

	for opID, wantPath := range map[string]string{
		"alias_only":      "/tmp/alias",
		"equal_duplicate": "/tmp/equal",
		"safe_sibling":    "/tmp/safe",
	} {
		op := findOperation(doc, opID)
		if op == nil {
			t.Fatalf("operation %q was not lowered: %#v", opID, doc.Operations)
		}
		body, _ := op.Request["body"].(map[string]any)
		if got := body["path"]; got != wantPath {
			t.Fatalf("%s canonical path = %#v, want %q", opID, got, wantPath)
		}
		if _, exists := body["dest"]; exists {
			t.Fatalf("%s retained alias key in request body: %#v", opID, body)
		}
	}
	if op := findOperation(doc, "conflicting_duplicate"); op != nil {
		t.Fatalf("conflicting canonical/alias task leaked into partial output: %#v", op)
	}
	if step := findStep(doc.Workflows[0].Steps, "conflicting_duplicate"); step != nil {
		t.Fatalf("conflicting canonical/alias step leaked into partial output: %#v", step)
	}
	if !hasDiagnostic(result.Diagnostics, CodeArgspecViolation, "Conflicting duplicate", "conflicting values") {
		t.Fatalf("missing conflicting alias diagnostic: %#v", result.Diagnostics)
	}
}

func TestConvertArgspecViolationsOmitAffectedTasks(t *testing.T) {
	argspecPath := writeArgspecFixture(t, `{
  "argspec": "uws.ansible.1.0",
  "collection": "acme.tools",
  "modules": {
    "acme.tools.account": {
      "parameters": {
        "name": {"type": "str", "required": true},
        "state": {"type": "str", "choices": ["present", "absent"]},
        "token": {"type": "str", "noLog": true}
      }
    }
  }
}`)
	playbookPath := writePlaybookFixture(t, `- name: validation
  hosts: localhost
  tasks:
    - name: Missing required
      acme.tools.account:
        state: present
    - name: Invalid choice
      acme.tools.account:
        name: choice
        state: unknown
    - name: Unknown parameter
      acme.tools.account:
        name: unknown
        extra: value
    - name: Literal secret
      acme.tools.account:
        name: secret
        token: plaintext
    - name: Safe task
      acme.tools.account:
        name: safe
        state: present
`)
	result, doc := convertFixture(t, playbookPath, argspecPath, true)

	if findOperation(doc, "safe_task") == nil {
		t.Fatalf("safe task did not lower: %#v", doc.Operations)
	}
	for _, skipped := range []string{"missing_required", "invalid_choice", "unknown_parameter", "literal_secret"} {
		if findOperation(doc, skipped) != nil || findStep(doc.Workflows[0].Steps, skipped) != nil {
			t.Fatalf("argspec-invalid task %q leaked into partial workflow", skipped)
		}
	}
	if !hasDiagnostic(result.Diagnostics, CodeNoLogLiteral, "Literal secret", "symbolic credential binding") {
		t.Fatalf("missing noLog diagnostic: %#v", result.Diagnostics)
	}
}

func TestConvertLoopAndWithItemsFailsClosed(t *testing.T) {
	argspecPath := writeArgspecFixture(t, `{
  "argspec": "uws.ansible.1.0",
  "collection": "acme.tools",
  "modules": {
    "acme.tools.file": {
      "parameters": {"path": {"type": "path", "required": true}}
    }
  }
}`)
	playbookPath := writePlaybookFixture(t, `- name: loops
  hosts: localhost
  tasks:
    - name: Ambiguous loop
      acme.tools.file:
        path: "{{ item }}"
      loop: [/tmp/one]
      with_items: [/tmp/two]
    - name: Safe sibling
      acme.tools.file:
        path: /tmp/safe
`)
	result, doc := convertFixture(t, playbookPath, argspecPath, true)

	if findOperation(doc, "ambiguous_loop") != nil || findStep(doc.Workflows[0].Steps, "ambiguous_loop") != nil {
		t.Fatalf("loop/with_items task leaked into partial workflow")
	}
	if findOperation(doc, "safe_sibling") == nil {
		t.Fatalf("safe sibling did not lower: %#v", doc.Operations)
	}
	if !hasDiagnostic(result.Diagnostics, CodeDirectiveTodo, "Ambiguous loop", "cannot be specified together") {
		t.Fatalf("missing loop/with_items diagnostic: %#v", result.Diagnostics)
	}
}

func writeArgspecFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "argspec.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write argspec fixture: %v", err)
	}
	return path
}

func writePlaybookFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "playbook.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write playbook fixture: %v", err)
	}
	return path
}

func convertFixture(t *testing.T, playbookPath, argspecPath string, ignoreUnsupported bool) (*Result, *uws1.Document) {
	t.Helper()
	result, err := Convert(context.Background(), Options{
		PlaybookPath:      playbookPath,
		Argspecs:          []ArgspecInput{{ID: "tools", Path: argspecPath}},
		OutDir:            t.TempDir(),
		IgnoreUnsupported: ignoreUnsupported,
	})
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	if result.UWSPath == "" {
		t.Fatalf("partial workflow was not written: %#v", result)
	}
	data, err := os.ReadFile(result.UWSPath)
	if err != nil {
		t.Fatalf("read converted workflow: %v", err)
	}
	var doc uws1.Document
	if err := convert.UnmarshalYAML(data, &doc); err != nil {
		t.Fatalf("parse converted workflow: %v", err)
	}
	return result, &doc
}

func hasDiagnostic(diags []Diagnostic, code, task, messagePart string) bool {
	for _, diag := range diags {
		if diag.Code == code && diag.Task == task && diag.StrictFailure && strings.Contains(diag.Message, messagePart) {
			return true
		}
	}
	return false
}
