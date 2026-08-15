package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/OpenUdon/ramen/internal/ansibleconvert"
	"github.com/OpenUdon/ramen/internal/tfconvert"
)

func runConvertCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		convertUsage(os.Stderr, "ramen convert")
		os.Exit(2)
	}
	switch args[0] {
	case "-h", "--help", "help":
		convertUsage(os.Stdout, "ramen convert")
	case "tf":
		runConvertTFCommand(ctx, args[1:])
	case "ansible":
		runConvertAnsibleCommand(ctx, args[1:])
	default:
		// Backward compatible: bare flags keep converting Terraform/OpenTofu.
		runConvertTFCommand(ctx, args)
	}
}

func convertUsage(out *os.File, command string) {
	fmt.Fprintf(out, "Usage: %s [tf] [--config-dir DIR] --api-source KIND:ID=PATH [--openapi ID=PATH] [--provider-schema ID=PATH] [--action create|update|delete|replace] [--target ADDRESS] [--out DIR] [--mode strict|partial] [--strict]\n", command)
	fmt.Fprintf(out, "       %s ansible --playbook FILE [--argspec ID=PATH] [--argspec-dir DIR] [--project-dir DIR] [--roles-path DIR] [--collections-path DIR] [--inventory FILE] [--extra-var NAME=VALUE] [--target-uws 1.5|1.6|1.7] [--out DIR] [--mode strict|partial] [--strict|--ignore-unsupported]\n\n", command)
	fmt.Fprintf(out, "Converts Terraform/OpenTofu configuration (default or `tf`) or an Ansible playbook (`ansible`) into native Ramen/UWS project artifacts. It does not execute Terraform, providers, Ansible modules, API source operations, or UWS workflows.\n\n")
	fmt.Fprintf(out, "Terraform conversion requires API-source server-operation authority; provider schemas are optional client-shape evidence. Ansible argspec and inventory inputs are client-side facts; SSH and module execution remain trusted-runtime responsibilities.\n\n")
}

func runConvertAnsibleCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("ramen convert ansible", flag.ExitOnError)
	playbook := fs.String("playbook", "", "Ansible playbook YAML file")
	outDir := fs.String("out", ".ramen/convert-ansible", "Output directory for converted artifacts")
	projectDir := fs.String("project-dir", "", "Static Ansible project root (defaults to the playbook directory)")
	modeFlag := fs.String("mode", "", "Conversion mode: strict or partial (default: strict during the transition)")
	strict := fs.Bool("strict", false, "Deprecated alias for --mode strict")
	ignoreUnsupported := fs.Bool("ignore-unsupported", false, "Deprecated alias for --mode partial")
	targetUWS := fs.String("target-uws", "1.5", "UWS version declared by the emitted document: 1.5, 1.6, or 1.7. Module leaves are extension-owned at every version, so the shape does not change")
	var argspecs repeatedStringFlag
	var argspecDirs repeatedStringFlag
	var rolesPaths repeatedStringFlag
	var collectionsPaths repeatedStringFlag
	var inventoryPaths repeatedStringFlag
	var extraVars repeatedStringFlag
	fs.Var(&argspecs, "argspec", "Collection argspec document as ID=PATH (repeatable; ramen.ansible.1.0 shape)")
	fs.Var(&argspecDirs, "argspec-dir", "Directory recursively containing bounded regular *.argspec.json documents (repeatable; source IDs derive from collection names)")
	fs.Var(&rolesPaths, "roles-path", "Static Ansible roles search path for resolving play roles/import_role (repeatable)")
	fs.Var(&collectionsPaths, "collections-path", "Static Ansible collections search path for resolving FQCN collection roles (repeatable)")
	fs.Var(&inventoryPaths, "inventory", "Bounded static INI/YAML/JSON inventory file; resolves all, exact host, or exact group targets without connecting (repeatable)")
	fs.Var(&extraVars, "extra-var", "Literal static extra variable NAME=VALUE or @file at highest precedence (repeatable; values are redacted from reports)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen convert ansible --playbook FILE [--argspec ID=PATH] [--argspec-dir DIR] [--project-dir DIR] [--roles-path DIR] [--collections-path DIR] [--inventory FILE] [--extra-var NAME=VALUE] [--target-uws 1.5|1.6|1.7] [--out DIR] [--mode strict|partial]\n\n")
		fmt.Fprintf(fs.Output(), "Converts an Ansible playbook into a reviewable UWS workflow. Argspec and bounded inventory inputs are client-side facts; SSH, connections, credentials, and module execution stay in a separately approved trusted runtime. Ansible module leaves are emitted as extension-owned operations carrying ramen.ansible-module-call.1.0; --target-uws only selects the uws version the document declares, defaulting to 1.5. Unsupported constructs are reported explicitly. Strict mode (the transitional default) exits 3 and suppresses workflows; partial mode omits unsupported constructs and exits 0. --strict and --ignore-unsupported are deprecated mode aliases.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if strings.TrimSpace(*playbook) == "" {
		fmt.Fprintln(os.Stderr, "ramen convert ansible: --playbook is required")
		os.Exit(2)
	}
	mode, err := resolveConversionMode(*modeFlag, "strict", *strict, *ignoreUnsupported)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ramen convert ansible: %v\n", err)
		os.Exit(2)
	}
	target := strings.TrimSpace(*targetUWS)
	switch target {
	case "",
		ansibleconvert.TargetUWS15, ansibleconvert.TargetUWS15 + ".0",
		ansibleconvert.TargetUWS16, ansibleconvert.TargetUWS16 + ".0",
		ansibleconvert.TargetUWS17, ansibleconvert.TargetUWS17 + ".0":
	default:
		fmt.Fprintf(os.Stderr, "ramen convert ansible: unsupported --target-uws %q (want 1.5, 1.6, or 1.7)\n", *targetUWS)
		os.Exit(2)
	}
	specs := make([]ansibleconvert.ArgspecInput, 0, len(argspecs))
	for _, value := range argspecs {
		id, path, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(path) == "" {
			fmt.Fprintf(os.Stderr, "ramen convert ansible: --argspec must be ID=PATH, got %q\n", value)
			os.Exit(2)
		}
		specs = append(specs, ansibleconvert.ArgspecInput{ID: strings.TrimSpace(id), Path: strings.TrimSpace(path)})
	}
	result, err := ansibleconvert.Convert(ctx, ansibleconvert.Options{
		PlaybookPath:      *playbook,
		Argspecs:          specs,
		ArgspecDirs:       []string(argspecDirs),
		OutDir:            *outDir,
		Mode:              mode,
		Strict:            *strict,
		ProjectDir:        *projectDir,
		RolesPaths:        []string(rolesPaths),
		CollectionsPaths:  []string(collectionsPaths),
		InventoryPaths:    []string(inventoryPaths),
		ExtraVars:         []string(extraVars),
		TargetUWS:         target,
		IgnoreUnsupported: mode == "partial",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Converted playbook: %s\n", *playbook)
	if result.UWSPath != "" {
		fmt.Printf("UWS document: %s\n", result.UWSPath)
	} else {
		fmt.Println("UWS document: not written (unsupported features or no lowerable tasks; see diagnostics)")
	}
	fmt.Printf("Diagnostics: %s (%d total, %d strict)\n", result.DiagnosticsJSON, len(result.Diagnostics), result.StrictFailures)
	fmt.Printf("Review: %s\n", result.ReviewMD)
	fmt.Printf("Manifest: %s\n", result.ManifestPath)
	if result.StrictFailures > 0 && mode == "strict" {
		fmt.Fprintf(os.Stderr, "ramen convert ansible: unsupported Ansible features found (%d strict diagnostics); workflow artifacts were not written\n", result.StrictFailures)
		for _, diag := range result.Diagnostics {
			if !diag.StrictFailure {
				continue
			}
			task := diag.Task
			if task == "" {
				task = "-"
			}
			fmt.Fprintf(os.Stderr, "- %s task=%q: %s\n", diag.Code, task, diag.Message)
		}
		fmt.Fprintln(os.Stderr, "rerun with --mode partial to write a partial workflow that omits unsupported constructs (--ignore-unsupported remains a deprecated alias)")
		os.Exit(3)
	}
}

func runConvertTFCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("ramen convert", flag.ExitOnError)
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	action := fs.String("action", "", "Managed resource action: create, update, delete, or replace")
	outDir := fs.String("out", "./.ramen/convert", "Output directory for draft review artifacts")
	modeFlag := fs.String("mode", "", "Conversion mode: strict or partial (default: strict)")
	strict := fs.Bool("strict", false, "Deprecated alias for --mode strict")
	var openAPIs repeatedStringFlag
	var apiSources repeatedStringFlag
	var providerSchemas repeatedStringFlag
	var targets repeatedStringFlag
	fs.Var(&openAPIs, "openapi", "Repeatable OpenAPI input as ID=PATH")
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Var(&providerSchemas, "provider-schema", "Optional offline Terraform provider schema snapshot as ID=PATH (repeatable; read-only, never obtained by running a provider)")
	fs.Var(&targets, "target", "Repeatable Terraform address target")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen convert [--config-dir DIR] --api-source KIND:ID=PATH [--openapi ID=PATH] [--provider-schema ID=PATH] [--action create|update|delete|replace] [--target ADDRESS] [--out DIR] [--mode strict|partial]\n")
		fmt.Fprintf(fs.Output(), "\nGenerates draft Ramen review scaffolding from static Terraform/OpenTofu configuration and local API source documents. API sources are required server-operation authority; provider schemas are optional client-configuration evidence and never replace them. Strict mode is the default; it exits 3 and suppresses semantic project/workflow payloads when strict diagnostics remain. --mode partial explicitly permits review-only output with symbolic or omitted semantics. --strict remains a deprecated alias for --mode strict. It does not execute Terraform, providers, API source operations, or UWS workflows.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	mode, err := resolveConversionMode(*modeFlag, "strict", *strict, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ramen convert tf: %v\n", err)
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
	schemas, err := parseProviderSchemaFlags(providerSchemas)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := tfconvert.Convert(ctx, tfconvert.Options{
		ConfigDir:       *configDir,
		OpenAPIs:        inputs,
		APISources:      sources,
		ProviderSchemas: schemas,
		Action:          *action,
		Targets:         []string(targets),
		OutDir:          *outDir,
		Mode:            mode,
		Strict:          mode == "strict",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if result != nil {
			fmt.Fprintf(os.Stderr, "diagnostics: %s\n", result.DiagnosticsJSON)
			fmt.Fprintf(os.Stderr, "review: %s\n", result.ReviewPath)
			fmt.Fprintf(os.Stderr, "manifest: %s\n", result.ManifestPath)
		}
		if tfconvert.IsStrictFailure(err) {
			os.Exit(3)
		}
		os.Exit(1)
	}
	fmt.Printf("ramen: convert wrote %s\n", result.OutDir)
	fmt.Printf("  project:     %s\n", result.ProjectPath)
	fmt.Printf("  native:      %s\n", result.NativeProjectPath)
	fmt.Printf("  native-hcl:  %s\n", result.NativeProjectHCLPath)
	fmt.Printf("  uws:         %s\n", result.UWSPath)
	fmt.Printf("  uws-hcl:     %s\n", result.UWSHCLPath)
	fmt.Printf("  conversion:  %s\n", result.ConversionPath)
	fmt.Printf("  mappings:    %s\n", result.MappingsPath)
	fmt.Printf("  plan:        %s\n", result.PlanJSONPath)
	fmt.Printf("  diagnostics: %s\n", result.DiagnosticsJSON)
	fmt.Printf("  review:      %s\n", result.ReviewPath)
	fmt.Printf("  manifest:    %s\n", result.ManifestPath)
}

func resolveConversionMode(value, defaultMode string, strictAlias, partialAlias bool) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(value))
	explicit := mode != ""
	if explicit && mode != "strict" && mode != "partial" {
		return "", fmt.Errorf("unsupported --mode %q (want strict or partial)", value)
	}
	if strictAlias && partialAlias {
		return "", fmt.Errorf("--strict and --ignore-unsupported select contradictory modes")
	}
	if explicit && strictAlias && mode != "strict" {
		return "", fmt.Errorf("--strict conflicts with --mode %s", mode)
	}
	if explicit && partialAlias && mode != "partial" {
		return "", fmt.Errorf("--ignore-unsupported conflicts with --mode %s", mode)
	}
	switch {
	case strictAlias:
		mode = "strict"
	case partialAlias:
		mode = "partial"
	case !explicit:
		mode = defaultMode
	}
	return mode, nil
}
