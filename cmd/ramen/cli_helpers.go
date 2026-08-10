package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tfapply "github.com/OpenUdon/ramen/apply"
	"github.com/OpenUdon/ramen/governance"
	"github.com/OpenUdon/ramen/internal/tfconvert"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/reconcile"
	"github.com/OpenUdon/ramen/state"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
	"github.com/OpenUdon/tfconfig"
)

type repeatedStringFlag []string

func (f *repeatedStringFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *repeatedStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func printFlagDefaultsExcluding(fs *flag.FlagSet, excluded map[string]bool) {
	fs.VisitAll(func(f *flag.Flag) {
		if excluded[f.Name] {
			return
		}
		name, usage := flag.UnquoteUsage(f)
		if name != "" {
			fmt.Fprintf(fs.Output(), "  -%s %s\n    \t%s", f.Name, name, usage)
		} else {
			fmt.Fprintf(fs.Output(), "  -%s\n    \t%s", f.Name, usage)
		}
		if f.DefValue != "" && f.DefValue != "false" {
			fmt.Fprintf(fs.Output(), " (default %q)", f.DefValue)
		}
		fmt.Fprint(fs.Output(), "\n")
	})
}

func positionalFirstLast(args []string) []string {
	if len(args) > 1 && !strings.HasPrefix(args[0], "-") {
		return append(slices.Clone(args[1:]), args[0])
	}
	return args
}

func maintenanceLockHolder(command string) string {
	return fmt.Sprintf("state-%s-%d-%d", command, os.Getpid(), time.Now().UTC().UnixNano())
}

func writeJSONOutput(value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(append(data, '\n'))
}

func statePathOrDefault(path, projectPath, configDir, workspace string) string {
	if strings.TrimSpace(path) != "" {
		return path
	}
	resolved, err := state.WorkspacePath(stateBaseDir(projectPath, configDir), workspace)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	return resolved
}

func approvalInputsOrExit(identity, role, context, approvedAt string) []governance.Approver {
	if strings.TrimSpace(identity) == "" && strings.TrimSpace(role) == "" && strings.TrimSpace(context) == "" && strings.TrimSpace(approvedAt) == "" {
		return nil
	}
	if strings.TrimSpace(identity) == "" {
		fmt.Fprintln(os.Stderr, "--approved-by is required when approval metadata is supplied")
		os.Exit(2)
	}
	if strings.TrimSpace(approvedAt) == "" {
		fmt.Fprintln(os.Stderr, "--approved-at is required when --approved-by is supplied")
		os.Exit(2)
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(approvedAt))
	if err != nil {
		fmt.Fprintf(os.Stderr, "--approved-at must be RFC3339: %v\n", err)
		os.Exit(2)
	}
	return []governance.Approver{{Identity: identity, Role: role, Context: context, ApprovedAt: parsed}}
}

func stateBaseDir(projectPath, configDir string) string {
	projectPath = strings.TrimSpace(projectPath)
	if projectPath == "" {
		return configDir
	}
	info, err := os.Stat(projectPath)
	if err == nil && info.IsDir() {
		return projectPath
	}
	return filepath.Dir(projectPath)
}

func validateStaticConfig(configDir string) error {
	doc, err := tfconfig.LoadDir(configDir)
	if err != nil {
		return err
	}
	for _, diag := range doc.Diagnostics {
		if strings.EqualFold(string(diag.Severity), "error") {
			return fmt.Errorf("%s", diagnosticText(diag))
		}
	}
	for _, mod := range doc.Modules {
		for _, diag := range mod.Diagnostics {
			if strings.EqualFold(string(diag.Severity), "error") {
				return fmt.Errorf("%s", diagnosticText(diag))
			}
		}
	}
	return nil
}

func validateNativeProject(projectPath string) error {
	_, err := project.Load(projectPath)
	return err
}

func diagnosticText(diag tfconfig.Diagnostic) string {
	if strings.TrimSpace(diag.Detail) == "" {
		return diag.Summary
	}
	return diag.Summary + ": " + diag.Detail
}

func planHasChanges(doc tfplan.Document) bool {
	return doc.Summary.Create != 0 || doc.Summary.Update != 0 || doc.Summary.Delete != 0 || doc.Summary.Post != 0 || doc.Summary.Put != 0 || doc.Summary.Patch != 0 || doc.Summary.Replace != 0 || doc.Summary.Read != 0
}

func parseOpenAPIFlags(values []string) ([]tfconvert.OpenAPIInput, error) {
	inputs := make([]tfconvert.OpenAPIInput, 0, len(values))
	for _, value := range values {
		id, path, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("--openapi must be ID=PATH, got %q", value)
		}
		inputs = append(inputs, tfconvert.OpenAPIInput{ID: strings.TrimSpace(id), Path: strings.TrimSpace(path)})
	}
	return inputs, nil
}

func parseAPISourceFlags(values []string) ([]tfconvert.APISourceInput, error) {
	inputs := make([]tfconvert.APISourceInput, 0, len(values))
	for _, value := range values {
		left, path, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(left) == "" || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("--api-source must be KIND:ID=PATH, got %q", value)
		}
		kind, id, ok := strings.Cut(left, ":")
		if !ok || strings.TrimSpace(kind) == "" || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("--api-source must be KIND:ID=PATH, got %q", value)
		}
		inputs = append(inputs, tfconvert.APISourceInput{Kind: strings.TrimSpace(kind), ID: strings.TrimSpace(id), Path: strings.TrimSpace(path)})
	}
	return inputs, nil
}

func parsePlanAPISourceFlags(values []string) ([]tfplan.APISourceInput, error) {
	inputs := make([]tfplan.APISourceInput, 0, len(values))
	for _, value := range values {
		left, path, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(left) == "" || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("--api-source must be KIND:ID=PATH, got %q", value)
		}
		kind, id, ok := strings.Cut(left, ":")
		if !ok || strings.TrimSpace(kind) == "" || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("--api-source must be KIND:ID=PATH, got %q", value)
		}
		inputs = append(inputs, tfplan.APISourceInput{Kind: strings.TrimSpace(kind), ID: strings.TrimSpace(id), Path: strings.TrimSpace(path)})
	}
	return inputs, nil
}

func parseApplyAPISourceFlags(values []string) ([]tfapply.APISourceInput, error) {
	planInputs, err := parsePlanAPISourceFlags(values)
	if err != nil {
		return nil, err
	}
	inputs := make([]tfapply.APISourceInput, len(planInputs))
	for i, input := range planInputs {
		inputs[i] = tfapply.APISourceInput(input)
	}
	return inputs, nil
}

func parseValidateAPISourceFlags(values []string) ([]ramenvalidate.APISourceInput, error) {
	planInputs, err := parsePlanAPISourceFlags(values)
	if err != nil {
		return nil, err
	}
	inputs := make([]ramenvalidate.APISourceInput, len(planInputs))
	for i, input := range planInputs {
		inputs[i] = ramenvalidate.APISourceInput(input)
	}
	return inputs, nil
}

func parseReconcileAPISourceFlags(values []string) ([]reconcile.APISourceInput, error) {
	planInputs, err := parsePlanAPISourceFlags(values)
	if err != nil {
		return nil, err
	}
	inputs := make([]reconcile.APISourceInput, len(planInputs))
	for i, input := range planInputs {
		inputs[i] = reconcile.APISourceInput(input)
	}
	return inputs, nil
}
