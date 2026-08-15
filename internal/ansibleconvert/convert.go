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
	"github.com/OpenUdon/uws/uws1"
)

// Convert parses the playbook, lowers it against the supplied argspec
// documents, and writes review artifacts: a validated UWS document (YAML +
// HCL) plus diagnostics (JSON + Markdown). Conversion is static; no Ansible,
// module, or workflow execution happens here.
func Convert(_ context.Context, opts Options) (*Result, error) {
	opts = normalizeOptions(opts)
	switch opts.TargetUWS {
	case TargetUWS15, TargetUWS16, TargetUWS17:
	default:
		return nil, fmt.Errorf("unsupported Ansible conversion target UWS %q (want 1.5, 1.6, or 1.7)", opts.TargetUWS)
	}
	data, err := os.ReadFile(opts.PlaybookPath)
	if err != nil {
		return nil, fmt.Errorf("read playbook: %w", err)
	}
	playbook, parseDiags, err := parsePlaybookFile(data, opts.PlaybookPath)
	if err != nil {
		return nil, fmt.Errorf("parse playbook: %w", err)
	}
	resolveDiags := resolveStatic(playbook, opts)
	idx, err := LoadArgspecs(opts.Argspecs)
	if err != nil {
		return nil, err
	}

	doc, lowerDiags := LowerPlaybookWithOptions(playbook, idx, LowerOptions{
		HostFanOut: len(opts.InventoryPaths) > 0,
		TargetUWS:  opts.TargetUWS,
	})
	diags := append(parseDiags, resolveDiags...)
	diags = append(diags, lowerDiags...)
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
		ReviewMD:        filepath.Join(opts.OutDir, "expected", "review.md"),
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
	if len(doc.Operations) > 0 && (result.StrictFailures == 0 || opts.IgnoreUnsupported) {
		result.UWSPath = filepath.Join(opts.OutDir, "workflows", "workflow.uws.yaml")
		result.HCLPath = filepath.Join(opts.OutDir, "workflows", "workflow.hcl")
		if err := convertcore.WriteDocumentFormats(doc, result.UWSPath, result.HCLPath); err != nil {
			return nil, fmt.Errorf("write UWS document: %w", err)
		}
	}
	if err := writeReview(result, doc, opts); err != nil {
		return nil, err
	}
	return result, nil
}

func normalizeOptions(opts Options) Options {
	opts.TargetUWS = normalizeTargetUWS(opts.TargetUWS)
	if strings.TrimSpace(opts.ProjectDir) == "" {
		dir := filepath.Dir(opts.PlaybookPath)
		if dir == "" {
			dir = "."
		}
		opts.ProjectDir = dir
	}
	return opts
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

func writeReview(result *Result, doc *uws1.Document, opts Options) error {
	if err := os.MkdirAll(filepath.Dir(result.ReviewMD), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Ansible Conversion Review\n\n")
	b.WriteString("Generated artifacts are static review scaffolding. Ramen did not execute Ansible, modules, inventory connections, API source operations, or UWS workflows.\n\n")

	b.WriteString("## Conversion Summary\n\n")
	fmt.Fprintf(&b, "- Playbook: `%s`\n", opts.PlaybookPath)
	fmt.Fprintf(&b, "- Project directory: `%s`\n", opts.ProjectDir)
	fmt.Fprintf(&b, "- Argspec documents: `%d`\n", len(opts.Argspecs))
	fmt.Fprintf(&b, "- Roles paths: `%s`\n", reviewList(opts.RolesPaths))
	fmt.Fprintf(&b, "- Collections paths: `%s`\n", reviewList(opts.CollectionsPaths))
	fmt.Fprintf(&b, "- Inventory inputs: `%s`\n", reviewList(opts.InventoryPaths))
	fmt.Fprintf(&b, "- Extra vars: `%s`\n", reviewList(opts.ExtraVars))
	fmt.Fprintf(&b, "- Lowered operations: `%d`\n", len(doc.Operations))
	fmt.Fprintf(&b, "- Diagnostics: `%d`\n", len(result.Diagnostics))
	fmt.Fprintf(&b, "- Strict failures: `%d`\n", result.StrictFailures)

	b.WriteString("\n## Artifact Paths\n\n")
	writeArtifactPath(&b, "UWS document", result.UWSPath)
	writeArtifactPath(&b, "HCL document", result.HCLPath)
	writeArtifactPath(&b, "Diagnostics JSON", result.DiagnosticsJSON)
	writeArtifactPath(&b, "Diagnostics Markdown", result.DiagnosticsMD)
	writeArtifactPath(&b, "Review Markdown", result.ReviewMD)

	b.WriteString("\n## Lowered Operations\n\n")
	if len(doc.Operations) == 0 {
		b.WriteString("No operations were lowered.\n")
	} else {
		stepRefs := operationStepRefs(doc)
		b.WriteString("| Operation | Source | Module | Workflow Steps |\n|---|---|---|---|\n")
		for _, op := range doc.Operations {
			refs := stepRefs[op.OperationID]
			if len(refs) == 0 {
				refs = []string{"-"}
			}
			source, module := operationReviewBinding(op)
			fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | %s |\n",
				escapeTable(op.OperationID),
				escapeTable(source),
				escapeTable(module),
				escapeTable(strings.Join(refs, ", ")))
		}
	}

	b.WriteString("\n## Diagnostics Summary\n\n")
	if len(result.Diagnostics) == 0 {
		b.WriteString("No diagnostics. The playbook lowered cleanly.\n")
	} else {
		counts := map[string]int{}
		for _, diag := range result.Diagnostics {
			counts[diag.Severity]++
		}
		for _, severity := range sortedKeys(counts) {
			fmt.Fprintf(&b, "- `%s`: `%d`\n", severity, counts[severity])
		}
	}

	b.WriteString("\n## Unsupported Gate\n\n")
	switch {
	case result.StrictFailures == 0:
		b.WriteString("Status: `pass`. No strict-failure diagnostics were emitted.\n")
	case !opts.IgnoreUnsupported:
		fmt.Fprintf(&b, "Status: `fail`. `%d` strict-failure diagnostics were emitted, so workflow artifacts were not written. Rerun with `--ignore-unsupported` to write a partial workflow that omits unsupported constructs.\n", result.StrictFailures)
	default:
		fmt.Fprintf(&b, "Status: `ignored`. `%d` strict-failure diagnostics were emitted, but `--ignore-unsupported` allowed partial workflow artifacts to be written without unsupported constructs.\n", result.StrictFailures)
	}
	return os.WriteFile(result.ReviewMD, []byte(b.String()), 0o644)
}

func reviewList(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func writeArtifactPath(b *strings.Builder, label, path string) {
	if strings.TrimSpace(path) == "" {
		fmt.Fprintf(b, "- %s: not written\n", label)
		return
	}
	fmt.Fprintf(b, "- %s: `%s`\n", label, path)
}

func operationStepRefs(doc *uws1.Document) map[string][]string {
	refs := map[string][]string{}
	for _, workflow := range doc.Workflows {
		collectStepRefs(refs, workflow.Steps)
	}
	for operationID := range refs {
		sort.Strings(refs[operationID])
	}
	return refs
}

func operationReviewBinding(op *uws1.Operation) (source, module string) {
	source = op.SourceDescription
	module = op.SourceOperationID
	if source != "" || module != "" {
		return source, module
	}
	payload, ok, err := ReadOperationExtension(op.Extensions)
	if err != nil || !ok || payload == nil {
		return source, module
	}
	module = payload.Module
	if payload.Argspec != nil {
		source = payload.Argspec.SourceID
	}
	return source, module
}

func collectStepRefs(refs map[string][]string, steps []*uws1.Step) {
	for _, step := range steps {
		if step == nil {
			continue
		}
		if step.OperationRef != "" {
			refs[step.OperationRef] = append(refs[step.OperationRef], step.StepID)
		}
		collectStepRefs(refs, step.Steps)
		for _, c := range step.Cases {
			if c != nil {
				collectStepRefs(refs, c.Steps)
			}
		}
		collectStepRefs(refs, step.Default)
	}
}

func sortedKeys(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func escapeTable(value string) string {
	if value == "" {
		return "-"
	}
	return strings.ReplaceAll(value, "|", "\\|")
}
