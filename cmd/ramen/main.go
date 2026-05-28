package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tfapply "github.com/OpenUdon/ramen/apply"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/internal/tfconvert"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/reconcile"
	"github.com/OpenUdon/ramen/state"
)

const version = "0.1.0"

type repeatedStringFlag []string

func (f *repeatedStringFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *repeatedStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: ramen <command>\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Commands:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  apply     execute approved desired-state mutations through a trusted executor\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  convert   generate Ramen review scaffolding from supported source formats\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  destroy   delete tracked resources through a trusted executor\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  import    attach an existing resource identity to state\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  init      create or migrate local Ramen state\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  plan      emit a static desired-state plan without mutation\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  refresh   read tracked resources and update state through a trusted executor\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  version   print version\n")
	}
	flag.Parse()

	command := "help"
	if flag.NArg() > 0 {
		command = flag.Arg(0)
	}
	switch command {
	case "apply":
		runApplyCommand(flag.Args()[1:])
	case "convert":
		runConvertCommand(flag.Args()[1:])
	case "destroy":
		runDestroyCommand(flag.Args()[1:])
	case "import":
		runImportCommand(flag.Args()[1:])
	case "init":
		runInitCommand(flag.Args()[1:])
	case "plan":
		runPlanCommand(flag.Args()[1:])
	case "refresh":
		runRefreshCommand(flag.Args()[1:])
	case "version":
		fmt.Println(version)
	case "-h", "--help", "help":
		flag.Usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", command)
		flag.Usage()
		os.Exit(2)
	}
}

func runRefreshCommand(args []string) {
	fs := flag.NewFlagSet("refresh", flag.ExitOnError)
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	mock := fs.Bool("mock", false, "Use the public mock executor instead of a live trusted executor")
	outDir := fs.String("out", "", "Optional directory for generated read UWS action documents")
	var apiSources repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen refresh [--config-dir DIR] [--state PATH] --api-source KIND:ID=PATH --mock [--out DIR]\n")
		fmt.Fprintf(fs.Output(), "\nReads tracked resources through a trusted executor and records redacted refresh revisions. Public builds only include the mock executor.\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	sources, err := parseReconcileAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	exec := reconcileExecutor(*mock)
	result, err := reconcile.Refresh(commandContext(), reconcile.Options{ConfigDir: *configDir, StatePath: statePathOrDefault(*statePath, *configDir), APISources: sources, OutDir: *outDir, Executor: exec})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: refresh read=%d failed=%d\n", result.Summary.Read, result.Summary.Failed)
}

func runDestroyCommand(args []string) {
	fs := flag.NewFlagSet("destroy", flag.ExitOnError)
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	autoApprove := fs.Bool("auto-approve", false, "Approve planned delete mutations without an interactive prompt")
	mock := fs.Bool("mock", false, "Use the public mock executor instead of a live trusted executor")
	outDir := fs.String("out", "", "Optional directory for generated delete UWS action documents")
	var apiSources repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen destroy [--config-dir DIR] [--state PATH] --api-source KIND:ID=PATH --auto-approve --mock [--out DIR]\n")
		fmt.Fprintf(fs.Output(), "\nDeletes tracked resources through a trusted executor in deterministic reverse order. Public builds only include the mock executor.\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	sources, err := parseReconcileAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := reconcile.Destroy(commandContext(), reconcile.Options{ConfigDir: *configDir, StatePath: statePathOrDefault(*statePath, *configDir), APISources: sources, AutoApprove: *autoApprove, OutDir: *outDir, Executor: reconcileExecutor(*mock)})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: destroy delete=%d failed=%d\n", result.Summary.Delete, result.Summary.Failed)
}

func runImportCommand(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	typeName := fs.String("type", "", "Terraform/OpenTofu resource type")
	provider := fs.String("provider", "", "Provider address")
	identity := fs.String("identity", "{}", "Identity JSON object")
	sourceKind := fs.String("source-kind", "", "Optional API source kind")
	sourceID := fs.String("source-id", "", "Optional API source ID")
	operationID := fs.String("operation-id", "", "Optional read/import operation ID")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen import ADDRESS --type TYPE --identity JSON [--state PATH]\n")
		fmt.Fprintf(fs.Output(), "\nAttaches an existing resource identity to local Ramen state without executing Terraform, providers, or API source operations.\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	var identityMap map[string]any
	if err := json.Unmarshal([]byte(*identity), &identityMap); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := reconcile.Import(commandContext(), reconcile.ImportOptions{StatePath: *statePath, Address: fs.Arg(0), Type: *typeName, Provider: *provider, Identity: identityMap, SourceKind: *sourceKind, SourceID: *sourceID, OperationID: *operationID})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: import imported=%d run=%d\n", result.Summary.Imported, result.RunID)
}

func commandContext() context.Context {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()
	return ctx
}

func reconcileExecutor(mock bool) executor.Executor {
	if mock {
		return &executor.MockExecutor{}
	}
	return nil
}

func statePathOrDefault(path, configDir string) string {
	if strings.TrimSpace(path) != "" {
		return path
	}
	return state.DefaultPath(configDir)
}

func runApplyCommand(args []string) {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	autoApprove := fs.Bool("auto-approve", false, "Approve planned create/update mutations without an interactive prompt")
	mock := fs.Bool("mock", false, "Use the public mock executor instead of a live trusted executor")
	outDir := fs.String("out", "", "Optional directory for generated executor-ready UWS action documents")
	var apiSources repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen apply [--config-dir DIR] [--state PATH] --api-source KIND:ID=PATH --auto-approve --mock [--out DIR]\n")
		fmt.Fprintf(fs.Output(), "\nBuilds a static desired-state plan, requires explicit mutation approval, generates executor-ready UWS action documents, and hands approved mutations to a trusted executor. Public builds only include the mock executor; live execution requires an opt-in adapter build.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	sources, err := parseApplyAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	path := *statePath
	if strings.TrimSpace(path) == "" {
		path = state.DefaultPath(*configDir)
	}
	var exec executor.Executor
	if *mock {
		exec = &executor.MockExecutor{}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := tfapply.Apply(ctx, tfapply.Options{
		ConfigDir:   *configDir,
		StatePath:   path,
		APISources:  sources,
		AutoApprove: *autoApprove,
		OutDir:      *outDir,
		Executor:    exec,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: apply create=%d update=%d delete=%d no-op=%d skipped=%d failed=%d blocked=%d executed=%d\n", result.Summary.Create, result.Summary.Update, result.Summary.Delete, result.Summary.NoOp, result.Summary.Skipped, result.Summary.Failed, result.Summary.Blocked, len(result.Executed))
	if result.RunID != 0 {
		fmt.Printf("  run:   %d\n", result.RunID)
	}
	if len(result.GeneratedDocuments) > 0 {
		fmt.Printf("  uws:   %s\n", *outDir)
	}
}

func runInitCommand(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen init [--config-dir DIR] [--state PATH]\n")
		fmt.Fprintf(fs.Output(), "\nCreates or migrates local Ramen SQLite state. It does not execute Terraform, providers, API source operations, or UWS workflows.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	path := *statePath
	if strings.TrimSpace(path) == "" {
		path = state.DefaultPath(*configDir)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := state.Init(ctx, path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: initialized state %s\n", path)
}

func runPlanCommand(args []string) {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	action := fs.String("action", "create", "Desired managed-resource action for absent resources")
	outPath := fs.String("out", "", "Optional JSON plan output path")
	var apiSources repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen plan [--config-dir DIR] [--state PATH] --api-source KIND:ID=PATH [--action create] [--out PATH]\n")
		fmt.Fprintf(fs.Output(), "\nBuilds a deterministic static desired-state plan from Terraform/OpenTofu facts, API source metadata, and recorded SQLite state. It does not execute Terraform, providers, API source operations, refresh, apply, destroy, or UWS workflows.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	sources, err := parsePlanAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	path := *statePath
	if strings.TrimSpace(path) == "" {
		path = state.DefaultPath(*configDir)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := tfplan.Build(ctx, tfplan.Options{
		ConfigDir:  *configDir,
		StatePath:  path,
		APISources: sources,
		Action:     *action,
		OutPath:    *outPath,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	summary := result.Plan.Summary
	fmt.Printf("ramen: plan create=%d update=%d delete=%d no-op=%d diagnostics=%d\n", summary.Create, summary.Update, summary.Delete, summary.NoOp, summary.Diagnostics)
	if result.OutPath != "" {
		fmt.Printf("  plan: %s\n", result.OutPath)
	}
	for _, diag := range result.Diagnostics {
		if diag.Severity == "error" {
			os.Exit(1)
		}
	}
}

func runConvertCommand(args []string) {
	if len(args) == 0 || args[0] != "tf" {
		fmt.Fprintln(os.Stderr, "usage: ramen convert tf [--config-dir DIR] --api-source KIND:ID=PATH [--openapi ID=PATH] [--action create|update|delete|replace] [--target ADDRESS] [--out DIR] [--strict]")
		os.Exit(2)
	}
	runConvertTFCommand(args[1:])
}

func runConvertTFCommand(args []string) {
	fs := flag.NewFlagSet("convert tf", flag.ExitOnError)
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	action := fs.String("action", "", "Managed resource action: create, update, delete, or replace")
	outDir := fs.String("out", "./.ramen/convert", "Output directory for draft review artifacts")
	strict := fs.Bool("strict", false, "Fail when strict-failure diagnostics remain")
	var openAPIs repeatedStringFlag
	var apiSources repeatedStringFlag
	var targets repeatedStringFlag
	fs.Var(&openAPIs, "openapi", "Repeatable OpenAPI input as ID=PATH")
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Var(&targets, "target", "Repeatable Terraform address target")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen convert tf [--config-dir DIR] --api-source KIND:ID=PATH [--openapi ID=PATH] [--action create|update|delete|replace] [--target ADDRESS] [--out DIR] [--strict]\n")
		fmt.Fprintf(fs.Output(), "\nGenerates draft Ramen review scaffolding from static Terraform/OpenTofu configuration and local API source documents. It does not execute Terraform, providers, API source operations, or UWS workflows.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	inputs, err := parseOpenAPIFlags(openAPIs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	sources, err := parseAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := tfconvert.Convert(ctx, tfconvert.Options{
		ConfigDir:  *configDir,
		OpenAPIs:   inputs,
		APISources: sources,
		Action:     *action,
		Targets:    []string(targets),
		OutDir:     *outDir,
		Strict:     *strict,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if result != nil {
			fmt.Fprintf(os.Stderr, "diagnostics: %s\n", result.DiagnosticsJSON)
		}
		os.Exit(1)
	}
	fmt.Printf("ramen: convert tf wrote %s\n", result.OutDir)
	fmt.Printf("  project:     %s\n", result.ProjectPath)
	fmt.Printf("  uws:         %s\n", result.UWSPath)
	fmt.Printf("  conversion:  %s\n", result.ConversionPath)
	fmt.Printf("  mappings:    %s\n", result.MappingsPath)
	fmt.Printf("  plan:        %s\n", result.PlanJSONPath)
	fmt.Printf("  diagnostics: %s\n", result.DiagnosticsJSON)
	fmt.Printf("  review:      %s\n", result.ReviewPath)
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
