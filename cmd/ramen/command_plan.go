package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	tfplan "github.com/OpenUdon/ramen/plan"
)

func runPlanCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	workspace := fs.String("workspace", "", "Workspace name; defaults to the base local state path")
	action := fs.String("action", "", "Desired action; native API-first projects default to their declared API operation, Terraform/OpenTofu config defaults to create")
	destroy := fs.Bool("destroy", false, "Deprecated compatibility flag for Terraform/OpenTofu-shaped delete plans; prefer native API DELETE operation roles")
	outPath := fs.String("out", "", "Optional JSON plan output path; also writes a sibling .hcl plan view")
	hclOutPath := fs.String("hcl-out", "", "Optional HCL plan output path; defaults to sibling .hcl when --out is set")
	detailedExitCode := fs.Bool("detailed-exitcode", false, "Return 2 when the plan has changes, 1 on errors, and 0 when empty")
	var apiSources repeatedStringFlag
	var targets repeatedStringFlag
	var excludes repeatedStringFlag
	var replaces repeatedStringFlag
	var varFiles repeatedStringFlag
	var cliVars repeatedStringFlag
	var policyFiles repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, google-discovery, or browser-profile")
	fs.Var(&targets, "target", "Repeatable native resource address to include with dependency closure")
	fs.Var(&excludes, "exclude", "Repeatable native resource address to exclude with dependent closure")
	fs.Var(&replaces, "replace", "Repeatable native resource address to force replacement in the plan")
	fs.Var(&varFiles, "var-file", "Repeatable native Ramen values file; later files override earlier files")
	fs.Var(&cliVars, "var", "Repeatable native Ramen variable assignment as name=value; overrides defaults and files")
	fs.Var(&policyFiles, "policy-file", "Repeatable Ramen policy file for plan-time governance")
	approvedBy := fs.String("approved-by", "", "Approver identity to bind into the plan approval artifact")
	approvalRole := fs.String("approval-role", "", "Approver role for policy approval routing")
	approvalContext := fs.String("approval-context", "", "Free-form approval context")
	approvedAt := fs.String("approved-at", "", "Approval timestamp in RFC3339 format")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen plan [--project DIR|FILE | --config-dir DIR] [--state PATH] [--workspace NAME] [--api-source KIND:ID=PATH] [--var-file PATH] [--var name=value] [--policy-file PATH] [--approved-by ID --approved-at RFC3339] [--target ADDRESS] [--exclude ADDRESS] [--replace ADDRESS] [--out PATH] [--hcl-out PATH]\n")
		fmt.Fprintf(fs.Output(), "\nBuilds a deterministic desired-state plan from native UWS/Ramen project artifacts or transitional Terraform/OpenTofu facts, API source metadata, and recorded SQLite state. It does not execute Terraform, providers, API source operations, refresh, apply, or UWS workflows.\n\n")
		printFlagDefaultsExcluding(fs, map[string]bool{"destroy": true})
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	destroySet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "destroy" {
			destroySet = true
		}
	})
	if destroySet {
		fmt.Fprintln(os.Stderr, "ramen plan --destroy is deprecated; model DELETE operations in the native project and use ramen apply --plan")
	}
	sources, err := parsePlanAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	path := *statePath
	if strings.TrimSpace(path) == "" {
		path = statePathOrDefault(*statePath, *projectPath, *configDir, *workspace)
	}
	approvers := approvalInputsOrExit(*approvedBy, *approvalRole, *approvalContext, *approvedAt)
	result, err := tfplan.Build(ctx, tfplan.Options{
		ConfigDir:   *configDir,
		ProjectPath: *projectPath,
		StatePath:   path,
		APISources:  sources,
		VarFiles:    []string(varFiles),
		Vars:        []string(cliVars),
		PolicyFiles: []string(policyFiles),
		Approvers:   approvers,
		Workspace:   *workspace,
		Action:      *action,
		OutPath:     *outPath,
		HCLPath:     *hclOutPath,
		Targets:     []string(targets),
		Excludes:    []string(excludes),
		Replaces:    []string(replaces),
		Destroy:     *destroy,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	summary := result.Plan.Summary
	fmt.Printf("ramen: plan create=%d update=%d delete=%d post=%d put=%d patch=%d replace=%d no-op=%d read=%d diagnostics=%d\n", summary.Create, summary.Update, summary.Delete, summary.Post, summary.Put, summary.Patch, summary.Replace, summary.NoOp, summary.Read, summary.Diagnostics)
	if result.OutPath != "" {
		fmt.Printf("  plan: %s\n", result.OutPath)
	}
	if result.HCLPath != "" {
		fmt.Printf("  plan-hcl: %s\n", result.HCLPath)
	}
	for _, diag := range result.Diagnostics {
		if diag.Severity == "error" {
			fmt.Fprintf(os.Stderr, "%s: %s\n", diag.Code, diag.Message)
		}
	}
	for _, diag := range result.Diagnostics {
		if diag.Severity == "error" {
			os.Exit(1)
		}
	}
	if *detailedExitCode && planHasChanges(result.Plan) {
		os.Exit(2)
	}
}
