package ansibleconvert

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OpenUdon/ramen/internal/convertcore"
)

// Convert parses the playbook, lowers it against the supplied argspec
// documents, and writes review artifacts: a validated UWS 1.6 document
// (YAML + HCL) plus diagnostics (JSON + Markdown). Conversion is static; no
// Ansible, module, or workflow execution happens here.
func Convert(_ context.Context, opts Options) (*Result, error) {
	data, err := os.ReadFile(opts.PlaybookPath)
	if err != nil {
		return nil, fmt.Errorf("read playbook: %w", err)
	}
	playbook, parseDiags, err := ParsePlaybook(data)
	if err != nil {
		return nil, fmt.Errorf("parse playbook: %w", err)
	}
	idx, err := LoadArgspecs(opts.Argspecs)
	if err != nil {
		return nil, err
	}

	doc, lowerDiags := LowerPlaybook(playbook, idx)
	diags := append(parseDiags, lowerDiags...)
	if len(doc.Operations) == 0 {
		diags = append(diags, Diagnostic{Code: CodePlaybookShape, Severity: "error", StrictFailure: true,
			Message: "no tasks could be lowered; no UWS document was written — see the diagnostics above for each skipped task"})
	}
	if diags == nil {
		diags = []Diagnostic{}
	}
	sortDiagnostics(diags)

	result := &Result{
		DiagnosticsJSON: filepath.Join(opts.OutDir, "expected", "diagnostics.json"),
		DiagnosticsMD:   filepath.Join(opts.OutDir, "expected", "diagnostics.md"),
		Diagnostics:     diags,
	}
	for _, d := range diags {
		if d.StrictFailure {
			result.StrictFailures++
		}
	}

	// Diagnostics are the primary review artifact: write them first so they
	// survive even when no document can be emitted.
	if err := writeDiagnostics(result, diags); err != nil {
		return nil, err
	}
	if len(doc.Operations) > 0 {
		result.UWSPath = filepath.Join(opts.OutDir, "workflows", "workflow.uws.yaml")
		result.HCLPath = filepath.Join(opts.OutDir, "workflows", "workflow.hcl")
		if err := convertcore.WriteDocumentFormats(doc, result.UWSPath, result.HCLPath); err != nil {
			return nil, fmt.Errorf("write UWS document: %w", err)
		}
	}
	return result, nil
}

func sortDiagnostics(diags []Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].Code != diags[j].Code {
			return diags[i].Code < diags[j].Code
		}
		if diags[i].Task != diags[j].Task {
			return diags[i].Task < diags[j].Task
		}
		return diags[i].Message < diags[j].Message
	})
}

func writeDiagnostics(result *Result, diags []Diagnostic) error {
	if err := os.MkdirAll(filepath.Dir(result.DiagnosticsJSON), 0o755); err != nil {
		return err
	}
	payload := struct {
		Version     string       `json:"version"`
		Diagnostics []Diagnostic `json:"diagnostics"`
	}{Version: "ramen.ansible-convert.v1", Diagnostics: diags}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(result.DiagnosticsJSON, append(data, '\n'), 0o644); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# Ansible Conversion Diagnostics\n\n")
	if len(diags) == 0 {
		b.WriteString("No diagnostics. The playbook lowered cleanly.\n")
	} else {
		b.WriteString("| Code | Severity | Task | Message |\n|---|---|---|---|\n")
		for _, d := range diags {
			task := d.Task
			if task == "" {
				task = "-"
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", d.Code, d.Severity, task, strings.ReplaceAll(d.Message, "|", "\\|"))
		}
	}
	return os.WriteFile(result.DiagnosticsMD, []byte(b.String()), 0o644)
}
