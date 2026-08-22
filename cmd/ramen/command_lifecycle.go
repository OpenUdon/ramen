package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tfapply "github.com/OpenUdon/ramen/apply"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/reconcile"
	ramenrun "github.com/OpenUdon/ramen/run"
	"github.com/OpenUdon/ramen/state"
)

func runRunCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	statePath := fs.String("state", "", "SQLite state path; defaults to the UWS document directory .ramen/state.db")
	workspace := fs.String("workspace", "", "Workspace name; defaults to the base local state path")
	checkMode := fs.Bool("check", false, "Validate and preview without executor calls or state writes")
	autoApprove := fs.Bool("auto-approve", false, "Approve imperative execution after reviewing the approval digest")
	approvalDigest := fs.String("approval-digest", "", "Digest-bound approval for this UWS document and target set")
	mock := fs.Bool("mock", false, "Use the public mock executor instead of a live trusted executor")
	outDir := fs.String("out", "", "Optional executor output directory")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	var targets repeatedStringFlag
	var policyFiles repeatedStringFlag
	fs.Var(&targets, "target", "Repeatable run target; defaults to one target named default")
	fs.Var(&policyFiles, "policy-file", "Repeatable Ramen policy file for run governance")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen run UWS_FILE [--target NAME] [--policy-file PATH] [--state PATH] [--workspace NAME] [--check | --auto-approve | --approval-digest DIGEST] --mock [--out DIR] [--json]\n")
		fmt.Fprintf(fs.Output(), "\nExecutes an approved imperative UWS runbook through the trusted executor boundary without treating outputs as desired-state resources. --check validates and previews without executor calls or state writes.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(positionalFirstLast(args)); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	docPath := strings.TrimSpace(fs.Arg(0))
	path := *statePath
	if strings.TrimSpace(path) == "" {
		path = statePathOrDefault("", docPath, filepath.Dir(docPath), *workspace)
	}
	var exec executor.Executor
	if *mock {
		exec = &executor.MockExecutor{}
	}
	result, err := ramenrun.Execute(ctx, ramenrun.Options{
		DocumentPath:   docPath,
		StatePath:      path,
		Workspace:      *workspace,
		Targets:        []string(targets),
		PolicyFiles:    []string(policyFiles),
		Check:          *checkMode,
		AutoApprove:    *autoApprove,
		ApprovalDigest: *approvalDigest,
		OutDir:         *outDir,
		Executor:       exec,
	})
	if err != nil {
		if result != nil && *jsonOut {
			writeJSONOutput(result)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(result)
		return
	}
	fmt.Printf("ramen: run targets=%d executed=%d skipped=%d failed=%d\n", result.Summary.Targets, result.Summary.Executed, result.Summary.Skipped, result.Summary.Failed)
	fmt.Printf("  approval_digest: %s\n", result.ApprovalDigest)
	if len(result.BrowserArtifacts) > 0 {
		fmt.Printf("  browser_artifacts: %d\n", len(result.BrowserArtifacts))
	}
	if result.RunID != 0 {
		fmt.Printf("  run: %d\n", result.RunID)
	}
}

func runRefreshCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("refresh", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	workspace := fs.String("workspace", "", "Workspace name; defaults to the base local state path")
	mock := fs.Bool("mock", false, "Use the public mock executor instead of a live trusted executor")
	executorMode := fs.String("executor", "", "Trusted executor to use: mock or udon")
	udonOutputDir := fs.String("udon-output", "", "Optional root directory for udon runtime artifacts when --executor udon is selected")
	outDir := fs.String("out", "", "Optional directory for generated read UWS action documents")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	var apiSources repeatedStringFlag
	var varFiles repeatedStringFlag
	var cliVars repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, google-discovery, or browser-profile")
	fs.Var(&varFiles, "var-file", "Repeatable native Ramen values file; later files override earlier files")
	fs.Var(&cliVars, "var", "Repeatable native Ramen variable assignment as name=value; overrides defaults and files")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen refresh [--project DIR|FILE | --config-dir DIR] [--state PATH] [--workspace NAME] [--api-source KIND:ID=PATH] [--var-file PATH] [--var name=value] [--mock | --executor udon] [--udon-output DIR] [--out DIR] [--json]\n")
		fmt.Fprintf(fs.Output(), "\nReads tracked resources through a trusted executor and records redacted refresh revisions. Public builds only include the mock executor; live udon execution requires an opt-in adapter build.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	sources, err := parseReconcileAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	exec, err := selectTrustedExecutor(*executorMode, *mock, *udonOutputDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := reconcile.Refresh(ctx, reconcile.Options{ConfigDir: *configDir, ProjectPath: *projectPath, StatePath: statePathOrDefault(*statePath, *projectPath, *configDir, *workspace), APISources: sources, VarFiles: []string(varFiles), Vars: []string(cliVars), Workspace: *workspace, OutDir: *outDir, Executor: exec})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(result)
		return
	}
	fmt.Printf("ramen: refresh read=%d changed=%d unchanged=%d missing=%d skipped=%d failed=%d\n", result.Summary.Read, result.Summary.Changed, result.Summary.Unchanged, result.Summary.Missing, result.Summary.Skipped, result.Summary.Failed)
}

func runImportCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory used to compute plan-compatible desired hashes")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory used to compute plan-compatible desired hashes")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	workspace := fs.String("workspace", "", "Workspace name; defaults to the base local state path")
	typeName := fs.String("type", "", "Terraform/OpenTofu resource type")
	provider := fs.String("provider", "", "Provider address")
	identity := fs.String("identity", "{}", "Identity JSON object")
	sourceKind := fs.String("source-kind", "", "Optional API source kind")
	sourceID := fs.String("source-id", "", "Optional API source ID")
	operationID := fs.String("operation-id", "", "Optional read/import operation ID")
	var apiSources repeatedStringFlag
	var varFiles repeatedStringFlag
	var cliVars repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; when ADDRESS exists in config, import records the plan-compatible desired hash")
	fs.Var(&varFiles, "var-file", "Repeatable native Ramen values file; later files override earlier files")
	fs.Var(&cliVars, "var", "Repeatable native Ramen variable assignment as name=value; overrides defaults and files")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen import ADDRESS --type TYPE --identity JSON [--project DIR|FILE | --config-dir DIR] [--state PATH] [--workspace NAME] [--api-source KIND:ID=PATH] [--var-file PATH] [--var name=value]\n")
		fmt.Fprintf(fs.Output(), "\nAttaches an existing resource identity to local Ramen state without executing Terraform, providers, or API source operations.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(positionalFirstLast(args)); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	var identityMap map[string]any
	if err := json.Unmarshal([]byte(*identity), &identityMap); err != nil {
		fmt.Fprintf(os.Stderr, "import.identity_invalid: identity must be a JSON object: %v\n", err)
		os.Exit(2)
	}
	if identityMap == nil {
		fmt.Fprintln(os.Stderr, "import.identity_invalid: identity must be a JSON object")
		os.Exit(2)
	}
	sources, err := parseReconcileAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := reconcile.Import(ctx, reconcile.ImportOptions{
		ConfigDir:   *configDir,
		ProjectPath: *projectPath,
		StatePath:   statePathOrDefault(*statePath, *projectPath, *configDir, *workspace),
		APISources:  sources,
		VarFiles:    []string(varFiles),
		Vars:        []string(cliVars),
		Workspace:   *workspace,
		Address:     fs.Arg(0),
		Type:        *typeName,
		Provider:    *provider,
		Identity:    identityMap,
		SourceKind:  *sourceKind,
		SourceID:    *sourceID,
		OperationID: *operationID,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: import imported=%d run=%d\n", result.Summary.Imported, result.RunID)
}

func runApplyCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	workspace := fs.String("workspace", "", "Workspace name; defaults to the base local state path")
	planPath := fs.String("plan", "", "Digest-bound Ramen plan artifact to verify and execute")
	autoApprove := fs.Bool("auto-approve", false, "Approve planned actions without an interactive prompt")
	mock := fs.Bool("mock", false, "Use the public mock executor instead of a live trusted executor")
	executorMode := fs.String("executor", "", "Trusted executor to use: mock or udon")
	udonOutputDir := fs.String("udon-output", "", "Optional root directory for udon runtime artifacts when --executor udon is selected")
	outDir := fs.String("out", "", "Optional directory for generated executor-ready UWS action documents")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	var apiSources repeatedStringFlag
	var varFiles repeatedStringFlag
	var cliVars repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, google-discovery, or browser-profile")
	fs.Var(&varFiles, "var-file", "Repeatable native Ramen values file; later files override earlier files")
	fs.Var(&cliVars, "var", "Repeatable native Ramen variable assignment as name=value; overrides defaults and files")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen apply [--plan PLAN.json | --project DIR|FILE | --config-dir DIR] [--state PATH] [--workspace NAME] [--api-source KIND:ID=PATH] [--var-file PATH] [--var name=value] --auto-approve [--mock | --executor udon] [--udon-output DIR] [--out DIR] [--json]\n")
		fmt.Fprintf(fs.Output(), "\nVerifies a digest-bound plan artifact or builds the same approval contract from project inputs, requires explicit approval, generates executor-ready UWS action documents, and hands approved plan actions to a trusted executor. Public builds only include the mock executor; live udon execution requires an opt-in adapter build.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	configDirSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "config-dir" {
			configDirSet = true
		}
	})
	sources, err := parseApplyAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	path := *statePath
	if strings.TrimSpace(*planPath) != "" {
		// Keep plan artifacts self-contained: when replaying an approved plan, let
		// it supply the state path so this command remains valid from alternate
		// working directories.
		path = ""
	} else if strings.TrimSpace(path) == "" {
		path = statePathOrDefault(*statePath, *projectPath, *configDir, *workspace)
	}
	configDirValue := *configDir
	if strings.TrimSpace(*planPath) != "" && !configDirSet {
		configDirValue = ""
	}
	exec, err := selectTrustedExecutor(*executorMode, *mock, *udonOutputDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := tfapply.Apply(ctx, tfapply.Options{
		ConfigDir:   configDirValue,
		ProjectPath: *projectPath,
		StatePath:   path,
		APISources:  sources,
		VarFiles:    []string(varFiles),
		Vars:        []string(cliVars),
		Workspace:   *workspace,
		PlanPath:    *planPath,
		AutoApprove: *autoApprove,
		OutDir:      *outDir,
		Executor:    exec,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if result != nil {
			for _, msg := range result.Errors {
				fmt.Fprintln(os.Stderr, msg)
			}
		}
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(result)
		return
	}
	fmt.Printf("ramen: apply create=%d update=%d delete=%d post=%d put=%d patch=%d read=%d no-op=%d skipped=%d failed=%d blocked=%d executed=%d\n", result.Summary.Create, result.Summary.Update, result.Summary.Delete, result.Summary.Post, result.Summary.Put, result.Summary.Patch, result.Summary.Read, result.Summary.NoOp, result.Summary.Skipped, result.Summary.Failed, result.Summary.Blocked, len(result.Executed))
	if result.RunID != 0 {
		fmt.Printf("  run:   %d\n", result.RunID)
	}
	if len(result.GeneratedDocuments) > 0 {
		fmt.Printf("  uws:   %s\n", *outDir)
	}
}

func runInitCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	workspace := fs.String("workspace", "", "Workspace name; defaults to the base local state path")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen init [--project DIR|FILE | --config-dir DIR] [--state PATH] [--workspace NAME]\n")
		fmt.Fprintf(fs.Output(), "\nValidates a native UWS/Ramen project when --project is supplied, otherwise validates Terraform/OpenTofu configuration with tfconfig, then creates or migrates local Ramen SQLite state. It does not execute Terraform, providers, API source operations, module downloads, backend initialization, or UWS workflows.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	path := *statePath
	if strings.TrimSpace(path) == "" {
		path = statePathOrDefault(*statePath, *projectPath, *configDir, *workspace)
	}
	var validationErr error
	if strings.TrimSpace(*projectPath) != "" {
		validationErr = validateNativeProject(*projectPath)
	} else {
		validationErr = validateStaticConfig(*configDir)
	}
	if validationErr != nil {
		fmt.Fprintln(os.Stderr, validationErr)
		os.Exit(1)
	}
	if err := state.Init(ctx, path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: initialized state %s\n", path)
}
