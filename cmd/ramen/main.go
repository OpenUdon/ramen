package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/OpenUdon/ramen/internal/tfconvert"
	tfplan "github.com/OpenUdon/ramen/plan"
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
		fmt.Fprintf(flag.CommandLine.Output(), "  convert   generate Ramen review scaffolding from supported source formats\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  init      create or migrate local Ramen state\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  plan      emit a static desired-state plan without mutation\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  version   print version\n")
	}
	flag.Parse()

	command := "help"
	if flag.NArg() > 0 {
		command = flag.Arg(0)
	}
	switch command {
	case "convert":
		runConvertCommand(flag.Args()[1:])
	case "init":
		runInitCommand(flag.Args()[1:])
	case "plan":
		runPlanCommand(flag.Args()[1:])
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
