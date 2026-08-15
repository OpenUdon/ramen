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
	fmt.Fprintf(out, "Usage: %s [tf] [--config-dir DIR] --api-source KIND:ID=PATH [--openapi ID=PATH] [--action create|update|delete|replace] [--target ADDRESS] [--out DIR] [--strict]\n", command)
	fmt.Fprintf(out, "       %s ansible --playbook FILE [--argspec ID=PATH] [--project-dir DIR] [--roles-path DIR] [--collections-path DIR] [--inventory FILE] [--extra-var NAME=VALUE] [--target-uws 1.5|1.6|1.7] [--out DIR] [--ignore-unsupported]\n\n", command)
	fmt.Fprintf(out, "Converts Terraform/OpenTofu configuration (default or `tf`) or an Ansible playbook (`ansible`) into native Ramen/UWS project artifacts. It does not execute Terraform, providers, Ansible modules, API source operations, or UWS workflows.\n\n")
}

func runConvertAnsibleCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("ramen convert ansible", flag.ExitOnError)
	playbook := fs.String("playbook", "", "Ansible playbook YAML file")
	outDir := fs.String("out", ".ramen/convert-ansible", "Output directory for converted artifacts")
	projectDir := fs.String("project-dir", "", "Static Ansible project root (defaults to the playbook directory)")
	strict := fs.Bool("strict", false, "Deprecated for Ansible conversion; unsupported constructs fail by default")
	ignoreUnsupported := fs.Bool("ignore-unsupported", false, "Write a partial workflow that omits unsupported Ansible constructs")
	targetUWS := fs.String("target-uws", "1.5", "UWS version declared by the emitted document: 1.5, 1.6, or 1.7. Module leaves are extension-owned at every version, so the shape does not change")
	var argspecs repeatedStringFlag
	var rolesPaths repeatedStringFlag
	var collectionsPaths repeatedStringFlag
	var inventoryPaths repeatedStringFlag
	var extraVars repeatedStringFlag
	fs.Var(&argspecs, "argspec", "Collection argspec document as ID=PATH (repeatable; ramen.ansible.1.0 shape)")
	fs.Var(&rolesPaths, "roles-path", "Static Ansible roles search path for resolving play roles/import_role (repeatable)")
	fs.Var(&collectionsPaths, "collections-path", "Static Ansible collections search path for resolving FQCN collection roles (repeatable)")
	fs.Var(&inventoryPaths, "inventory", "Inventory file or directory input; when supplied, non-local plays lower as host fan-out over $inputs.hosts (repeatable)")
	fs.Var(&extraVars, "extra-var", "Static extra variable NAME=VALUE or @file (repeatable; recorded for review, not used for static expression lowering)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen convert ansible --playbook FILE [--argspec ID=PATH] [--project-dir DIR] [--roles-path DIR] [--collections-path DIR] [--inventory FILE] [--extra-var NAME=VALUE] [--target-uws 1.5|1.6|1.7] [--out DIR] [--ignore-unsupported]\n\n")
		fmt.Fprintf(fs.Output(), "Converts an Ansible playbook into a reviewable UWS workflow. Ansible module leaves are emitted as extension-owned operations carrying ramen.ansible-module-call.1.0; --target-uws only selects the uws version the document declares, defaulting to 1.5. Unsupported constructs are reported explicitly and fail the command unless --ignore-unsupported is set.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if strings.TrimSpace(*playbook) == "" {
		fmt.Fprintln(os.Stderr, "ramen convert ansible: --playbook is required")
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
		OutDir:            *outDir,
		Strict:            *strict,
		ProjectDir:        *projectDir,
		RolesPaths:        []string(rolesPaths),
		CollectionsPaths:  []string(collectionsPaths),
		InventoryPaths:    []string(inventoryPaths),
		ExtraVars:         []string(extraVars),
		TargetUWS:         target,
		IgnoreUnsupported: *ignoreUnsupported,
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
	if result.StrictFailures > 0 && !*ignoreUnsupported {
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
		fmt.Fprintln(os.Stderr, "rerun with --ignore-unsupported to write a partial workflow that omits unsupported constructs")
		os.Exit(3)
	}
}

func runConvertTFCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("ramen convert", flag.ExitOnError)
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
		fmt.Fprintf(fs.Output(), "Usage: ramen convert [--config-dir DIR] --api-source KIND:ID=PATH [--openapi ID=PATH] [--action create|update|delete|replace] [--target ADDRESS] [--out DIR] [--strict]\n")
		fmt.Fprintf(fs.Output(), "\nGenerates draft Ramen review scaffolding from static Terraform/OpenTofu configuration and local API source documents. It does not execute Terraform, providers, API source operations, or UWS workflows.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
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
