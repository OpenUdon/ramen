package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"
	"time"

	tfapply "github.com/OpenUdon/ramen/apply"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/graph"
	"github.com/OpenUdon/ramen/internal/tfconvert"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/reconcile"
	"github.com/OpenUdon/ramen/state"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
	"github.com/OpenUdon/tfconfig"
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
		fmt.Fprintf(flag.CommandLine.Output(), "  force-unlock release a local Ramen state lock by exact holder token\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  graph     emit the native resource dependency graph\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  import    attach an existing resource identity to state\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  init      create or migrate local Ramen state\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  plan      emit a static desired-state plan without mutation\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  refresh   read tracked resources and update state through a trusted executor\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  show      inspect Ramen plan and approval artifacts\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  state     inspect local Ramen state\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  validate  validate a native UWS/Ramen project without mutation\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  version   print version\n")
	}
	flag.Parse()

	command := "help"
	if flag.NArg() > 0 {
		command = flag.Arg(0)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch command {
	case "apply":
		runApplyCommand(ctx, flag.Args()[1:])
	case "convert":
		runConvertCommand(ctx, flag.Args()[1:])
	case "destroy":
		runDestroyCommand(ctx, flag.Args()[1:])
	case "force-unlock":
		runForceUnlockCommand(ctx, flag.Args()[1:])
	case "graph":
		runGraphCommand(ctx, flag.Args()[1:])
	case "import":
		runImportCommand(ctx, flag.Args()[1:])
	case "init":
		runInitCommand(ctx, flag.Args()[1:])
	case "plan":
		runPlanCommand(ctx, flag.Args()[1:])
	case "refresh":
		runRefreshCommand(ctx, flag.Args()[1:])
	case "show":
		runShowCommand(flag.Args()[1:])
	case "state":
		runStateCommand(ctx, flag.Args()[1:])
	case "validate":
		runValidateCommand(ctx, flag.Args()[1:])
	case "version":
		runVersionCommand(flag.Args()[1:])
	case "-h", "--help", "help":
		flag.Usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", command)
		flag.Usage()
		os.Exit(2)
	}
}

func runForceUnlockCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("force-unlock", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	lockName := fs.String("name", "state", "Lock name to release")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen force-unlock LOCK_HOLDER [--state PATH] [--name state]\n")
		fmt.Fprintf(fs.Output(), "\nReleases a local SQLite state lock only when LOCK_HOLDER exactly matches the stored holder. It does not modify resources, revisions, runs, project files, API source documents, or remote systems.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(positionalFirstLast(args)); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		fs.Usage()
		os.Exit(2)
	}
	path := strings.TrimSpace(*statePath)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "state path %s does not exist\n", path)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	store, err := state.Open(ctx, path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	lock, err := store.ForceUnlock(ctx, *lockName, fs.Arg(0))
	if err != nil {
		var missing state.LockNotFoundError
		var mismatch state.LockHolderMismatchError
		switch {
		case errors.As(err, &missing):
			fmt.Fprintln(os.Stderr, missing.Error())
		case errors.As(err, &mismatch):
			fmt.Fprintln(os.Stderr, mismatch.Error())
		default:
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
	fmt.Printf("ramen: force-unlocked %s held by %s since %s\n", lock.Name, lock.Holder, lock.AcquiredAt.Format(time.RFC3339Nano))
}

func runShowCommand(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "Emit JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen show PLAN_OR_APPROVAL [--json]\n")
		fmt.Fprintf(fs.Output(), "\nInspects a Ramen plan/approval artifact without reading state, executing workflows, or mutating files.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(positionalFirstLast(args)); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	doc, err := loadPlanForShow(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(doc)
		return
	}
	fmt.Printf("ramen: show version=%s action=%s errored=%t resources=%d diagnostics=%d\n", doc.Version, doc.Action, doc.Errored, len(doc.Resources), len(doc.Diagnostics))
	fmt.Printf("  summary: create=%d update=%d delete=%d replace=%d no-op=%d read=%d diagnostics=%d\n", doc.Summary.Create, doc.Summary.Update, doc.Summary.Delete, doc.Summary.Replace, doc.Summary.NoOp, doc.Summary.Read, doc.Summary.Diagnostics)
	if doc.Approval != nil {
		fmt.Printf("  approval: version=%s digest=%s project=%s state=%s\n", doc.Approval.Version, doc.Approval.Digest, doc.Approval.ProjectDigest, doc.Approval.StateDigest)
	}
	if len(doc.Controls.Targets) > 0 || len(doc.Controls.Excludes) > 0 || len(doc.Controls.Replaces) > 0 || doc.Controls.Destroy {
		fmt.Printf("  controls: targets=%d excludes=%d replaces=%d destroy=%t\n", len(doc.Controls.Targets), len(doc.Controls.Excludes), len(doc.Controls.Replaces), doc.Controls.Destroy)
	}
}

func loadPlanForShow(path string) (tfplan.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return tfplan.Document{}, fmt.Errorf("show.plan_read_error: %w", err)
	}
	var doc tfplan.Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return tfplan.Document{}, fmt.Errorf("show.plan_parse_error: %w", err)
	}
	if doc.Version != tfplan.Version {
		return tfplan.Document{}, fmt.Errorf("show.plan_version_invalid: got %q, want %q", doc.Version, tfplan.Version)
	}
	return doc, nil
}

func positionalFirstLast(args []string) []string {
	if len(args) > 1 && !strings.HasPrefix(args[0], "-") {
		return append(slices.Clone(args[1:]), args[0])
	}
	return args
}

func runStateCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		stateUsage(os.Stderr)
		os.Exit(2)
	}
	switch args[0] {
	case "list":
		runStateListCommand(ctx, args[1:])
	case "show":
		runStateShowCommand(ctx, args[1:])
	case "history":
		runStateHistoryCommand(ctx, args[1:])
	case "runs":
		runStateRunsCommand(ctx, args[1:])
	case "-h", "--help", "help":
		stateUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown state subcommand %q\n", args[0])
		stateUsage(os.Stderr)
		os.Exit(2)
	}
}

func stateUsage(out *os.File) {
	fmt.Fprintf(out, "Usage: ramen state <list|show|history> [args]\n\n")
	fmt.Fprintf(out, "Read-only local SQLite state inspection. No state surgery, backend access, provider execution, or Terraform/OpenTofu state compatibility is performed.\n\n")
	fmt.Fprintf(out, "Subcommands:\n")
	fmt.Fprintf(out, "  list              list current resource addresses\n")
	fmt.Fprintf(out, "  show ADDRESS      show one current resource\n")
	fmt.Fprintf(out, "  history [ADDRESS] show revision history\n")
	fmt.Fprintf(out, "  runs              show command run history\n")
}

func runStateListCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state list", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state list [--state PATH] [--json]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if store == nil {
		if *jsonOut {
			writeJSONOutput([]state.ResourceSnapshot{})
		} else {
			fmt.Println("ramen: state resources=0")
		}
		return
	}
	defer func() { _ = store.Close() }()
	resources, err := store.ListCurrentResources(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(resources)
		return
	}
	fmt.Printf("ramen: state resources=%d\n", len(resources))
	for _, resource := range resources {
		fmt.Printf("  %s %s status=%s\n", resource.Address, resource.Type, resource.Status)
	}
}

func runStateShowCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state show", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state show ADDRESS [--state PATH] [--json]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(positionalFirstLast(args)); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		fs.Usage()
		os.Exit(2)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if store == nil {
		fmt.Fprintf(os.Stderr, "state.resource_not_found: %s\n", fs.Arg(0))
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	resource, err := store.CurrentResource(ctx, fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if resource == nil {
		fmt.Fprintf(os.Stderr, "state.resource_not_found: %s\n", fs.Arg(0))
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(resource)
		return
	}
	fmt.Printf("ramen: state address=%s type=%s status=%s\n", resource.Address, resource.Type, resource.Status)
	fmt.Printf("  provider=%s source=%s:%s operation=%s run=%d updated=%s\n", resource.Provider, resource.SourceKind, resource.SourceID, resource.OperationID, resource.UpdatedRunID, resource.UpdatedAt.Format(time.RFC3339Nano))
}

func runStateHistoryCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state history", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state history [ADDRESS] [--state PATH] [--json]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(positionalFirstLast(args)); err != nil {
		os.Exit(2)
	}
	if fs.NArg() > 1 {
		fs.Usage()
		os.Exit(2)
	}
	address := ""
	if fs.NArg() == 1 {
		address = fs.Arg(0)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if store == nil {
		if *jsonOut {
			writeJSONOutput([]state.Revision{})
		} else {
			fmt.Println("ramen: state revisions=0")
		}
		return
	}
	defer func() { _ = store.Close() }()
	revisions, err := store.ListRevisions(ctx, address)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(revisions)
		return
	}
	fmt.Printf("ramen: state revisions=%d\n", len(revisions))
	for _, rev := range revisions {
		fmt.Printf("  #%d %s action=%s run=%d at=%s\n", rev.ID, rev.ResourceAddress, rev.Action, rev.RunID, rev.CreatedAt.Format(time.RFC3339Nano))
	}
}

func runStateRunsCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state runs", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	status := fs.String("status", "", "Optional run status filter")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state runs [--state PATH] [--status STATUS] [--json]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if store == nil {
		if *jsonOut {
			writeJSONOutput([]state.Run{})
		} else {
			fmt.Println("ramen: state runs=0")
		}
		return
	}
	defer func() { _ = store.Close() }()
	runs, err := store.ListRuns(ctx, *status)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(runs)
		return
	}
	fmt.Printf("ramen: state runs=%d\n", len(runs))
	for _, run := range runs {
		finished := ""
		if !run.FinishedAt.IsZero() {
			finished = " finished=" + run.FinishedAt.Format(time.RFC3339Nano)
		}
		fmt.Printf("  #%d %s status=%s started=%s%s\n", run.ID, run.Command, run.Status, run.StartedAt.Format(time.RFC3339Nano), finished)
	}
}

func openStateReadOnly(ctx context.Context, path string) (*state.Store, error) {
	store, err := state.OpenReadOnly(ctx, strings.TrimSpace(path))
	if err != nil {
		return nil, fmt.Errorf("state.open_read_error: %w", err)
	}
	return store, nil
}

func writeJSONOutput(value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(append(data, '\n'))
}

func runGraphCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("graph", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	format := fs.String("format", "dot", "Output format: dot or json")
	outPath := fs.String("out", "", "Optional output path")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen graph --project DIR|FILE [--format dot|json] [--out PATH]\n")
		fmt.Fprintf(fs.Output(), "\nEmits the native UWS/Ramen resource dependency graph, including operation-role references, without Terraform/OpenTofu graph compatibility, provider execution, state access, or mutation.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	normalizedFormat := strings.ToLower(strings.TrimSpace(*format))
	if normalizedFormat != "dot" && normalizedFormat != "json" {
		fmt.Fprintf(os.Stderr, "--format must be dot or json, got %q\n", *format)
		os.Exit(2)
	}
	validation, err := ramenvalidate.Run(ctx, ramenvalidate.Options{ProjectPath: *projectPath})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var graphDoc graph.Document
	if validation.Valid {
		proj, err := project.Load(*projectPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		graphDoc = graph.BuildProject(proj)
	} else {
		graphDoc = graph.Document{Version: graph.DocumentVersion, ProjectPath: validation.ProjectPath}
	}
	graphDoc.Diagnostics = append(graphDoc.Diagnostics, graphDiagnostics(validation.Diagnostics)...)
	if normalizedFormat == "json" {
		writeGraphOutput(*outPath, mustGraphJSON(graphDoc))
	} else {
		writeGraphOutput(*outPath, graph.DOT(graphDoc))
	}
	for _, diag := range graphDoc.Diagnostics {
		fmt.Fprintf(os.Stderr, "%s: %s\n", diag.Code, diag.Message)
	}
	if hasGraphErrors(graphDoc.Diagnostics) {
		os.Exit(1)
	}
}

func graphDiagnostics(diagnostics []ramenvalidate.Diagnostic) []graph.Diagnostic {
	out := make([]graph.Diagnostic, 0, len(diagnostics))
	for _, diag := range diagnostics {
		out = append(out, graph.Diagnostic{
			Code:          diag.Code,
			Severity:      diag.Severity,
			Message:       diag.Message,
			Address:       diag.Address,
			APISourceKind: diag.APISourceKind,
			APISourceID:   diag.APISourceID,
			OperationID:   diag.OperationID,
		})
	}
	return out
}

func mustGraphJSON(doc graph.Document) string {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return string(data) + "\n"
}

func writeGraphOutput(path, content string) {
	if strings.TrimSpace(path) == "" {
		fmt.Print(content)
		return
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func hasGraphErrors(diagnostics []graph.Diagnostic) bool {
	for _, diag := range diagnostics {
		if diag.Severity == "error" {
			return true
		}
	}
	return false
}

func runValidateCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	jsonOutput := fs.Bool("json", false, "Print machine-readable validation diagnostics")
	strict := fs.Bool("strict", false, "Treat validation warnings as errors")
	var apiSources repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen validate --project DIR|FILE [--api-source KIND:ID=PATH] [--json] [--strict]\n")
		fmt.Fprintf(fs.Output(), "\nValidates a native UWS/Ramen project, optional local API source operation references, and diagnostics without planning, executing, touching state, reading Terraform/OpenTofu HCL, or performing network access.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	sources, err := parseValidateAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := ramenvalidate.Run(ctx, ramenvalidate.Options{ProjectPath: *projectPath, APISources: sources, Strict: *strict})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOutput {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		fmt.Printf("ramen: validate valid=%t errors=%d warnings=%d diagnostics=%d\n", result.Valid, result.Summary.Errors, result.Summary.Warnings, result.Summary.Diagnostics)
		for _, diag := range result.Diagnostics {
			fmt.Fprintf(os.Stderr, "%s: %s\n", diag.Code, diag.Message)
		}
	}
	if !result.Valid {
		os.Exit(1)
	}
}

type versionInfo struct {
	Version   string            `json:"version"`
	Module    string            `json:"module"`
	MainPath  string            `json:"main_path,omitempty"`
	GoVersion string            `json:"go_version,omitempty"`
	Revision  string            `json:"revision,omitempty"`
	BuildTags []string          `json:"build_tags,omitempty"`
	Settings  map[string]string `json:"settings,omitempty"`
}

func runVersionCommand(args []string) {
	fs := flag.NewFlagSet("version", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "Print version and local build metadata as JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen version [--json]\n")
		fmt.Fprintf(fs.Output(), "\nPrints the Ramen CLI version. With --json, prints local build metadata only; it does not check networks, releases, updates, or telemetry.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	info := collectVersionInfo()
	if *jsonOutput {
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}
	fmt.Println(info.Version)
}

func collectVersionInfo() versionInfo {
	info := versionInfo{
		Version: version,
		Module:  "github.com/OpenUdon/ramen",
	}
	if build, ok := debug.ReadBuildInfo(); ok {
		info.GoVersion = build.GoVersion
		if build.Main.Path != "" {
			info.MainPath = build.Main.Path
		}
		settings := make(map[string]string)
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				info.Revision = setting.Value
			case "-tags":
				if strings.TrimSpace(setting.Value) != "" {
					info.BuildTags = splitBuildTags(setting.Value)
				}
			case "vcs.modified", "vcs.time", "vcs":
				settings[setting.Key] = setting.Value
			}
		}
		if len(settings) > 0 {
			info.Settings = settings
		}
	}
	return info
}

func splitBuildTags(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' '
	})
	tags := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags
}

func runRefreshCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("refresh", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	mock := fs.Bool("mock", false, "Use the public mock executor instead of a live trusted executor")
	outDir := fs.String("out", "", "Optional directory for generated read UWS action documents")
	var apiSources repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen refresh [--project DIR|FILE | --config-dir DIR] [--state PATH] [--api-source KIND:ID=PATH] --mock [--out DIR]\n")
		fmt.Fprintf(fs.Output(), "\nReads tracked resources through a trusted executor and records redacted refresh revisions. Public builds only include the mock executor.\n\n")
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
	exec := reconcileExecutor(*mock)
	result, err := reconcile.Refresh(ctx, reconcile.Options{ConfigDir: *configDir, ProjectPath: *projectPath, StatePath: statePathOrDefault(*statePath, *projectPath, *configDir), APISources: sources, OutDir: *outDir, Executor: exec})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: refresh read=%d changed=%d unchanged=%d missing=%d skipped=%d failed=%d\n", result.Summary.Read, result.Summary.Changed, result.Summary.Unchanged, result.Summary.Missing, result.Summary.Skipped, result.Summary.Failed)
}

func runDestroyCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("destroy", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	planPath := fs.String("plan", "", "Digest-bound Ramen destroy plan artifact to verify and execute")
	autoApprove := fs.Bool("auto-approve", false, "Approve planned delete mutations without an interactive prompt")
	mock := fs.Bool("mock", false, "Use the public mock executor instead of a live trusted executor")
	outDir := fs.String("out", "", "Optional directory for generated delete UWS action documents")
	var apiSources repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen destroy [--plan PLAN.json | --project DIR|FILE | --config-dir DIR] [--state PATH] [--api-source KIND:ID=PATH] --auto-approve --mock [--out DIR]\n")
		fmt.Fprintf(fs.Output(), "\nVerifies a digest-bound destroy plan artifact or builds the same approval contract from project inputs, then deletes tracked resources through a trusted executor in deterministic reverse order. Public builds only include the mock executor.\n\n")
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
	sources, err := parseReconcileAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	stateValue := *statePath
	if strings.TrimSpace(stateValue) == "" && strings.TrimSpace(*planPath) == "" {
		stateValue = statePathOrDefault(*statePath, *projectPath, *configDir)
	}
	configDirValue := *configDir
	if strings.TrimSpace(*planPath) != "" && !configDirSet {
		configDirValue = ""
	}
	result, err := reconcile.Destroy(ctx, reconcile.Options{ConfigDir: configDirValue, ProjectPath: *projectPath, StatePath: stateValue, APISources: sources, PlanPath: *planPath, AutoApprove: *autoApprove, OutDir: *outDir, Executor: reconcileExecutor(*mock)})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: destroy delete=%d failed=%d\n", result.Summary.Delete, result.Summary.Failed)
}

func runImportCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory used to compute plan-compatible desired hashes")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory used to compute plan-compatible desired hashes")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	typeName := fs.String("type", "", "Terraform/OpenTofu resource type")
	provider := fs.String("provider", "", "Provider address")
	identity := fs.String("identity", "{}", "Identity JSON object")
	sourceKind := fs.String("source-kind", "", "Optional API source kind")
	sourceID := fs.String("source-id", "", "Optional API source ID")
	operationID := fs.String("operation-id", "", "Optional read/import operation ID")
	var apiSources repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; when ADDRESS exists in config, import records the plan-compatible desired hash")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen import ADDRESS --type TYPE --identity JSON [--project DIR|FILE | --config-dir DIR] [--state PATH] [--api-source KIND:ID=PATH]\n")
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
		StatePath:   statePathOrDefault(*statePath, *projectPath, *configDir),
		APISources:  sources,
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

func reconcileExecutor(mock bool) executor.Executor {
	if mock {
		return &executor.MockExecutor{}
	}
	return nil
}

func statePathOrDefault(path, projectPath, configDir string) string {
	if strings.TrimSpace(path) != "" {
		return path
	}
	return state.DefaultPath(stateBaseDir(projectPath, configDir))
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
	return doc.Summary.Create != 0 || doc.Summary.Update != 0 || doc.Summary.Delete != 0 || doc.Summary.Replace != 0
}

func runApplyCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	planPath := fs.String("plan", "", "Digest-bound Ramen plan artifact to verify and execute")
	autoApprove := fs.Bool("auto-approve", false, "Approve planned create/update mutations without an interactive prompt")
	mock := fs.Bool("mock", false, "Use the public mock executor instead of a live trusted executor")
	outDir := fs.String("out", "", "Optional directory for generated executor-ready UWS action documents")
	var apiSources repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen apply [--plan PLAN.json | --project DIR|FILE | --config-dir DIR] [--state PATH] [--api-source KIND:ID=PATH] --auto-approve --mock [--out DIR]\n")
		fmt.Fprintf(fs.Output(), "\nVerifies a digest-bound plan artifact or builds the same approval contract from project inputs, requires explicit mutation approval, generates executor-ready UWS action documents, and hands approved mutations to a trusted executor. Public builds only include the mock executor; live execution requires an opt-in adapter build.\n\n")
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
	if strings.TrimSpace(path) == "" && strings.TrimSpace(*planPath) == "" {
		path = state.DefaultPath(stateBaseDir(*projectPath, *configDir))
	}
	configDirValue := *configDir
	if strings.TrimSpace(*planPath) != "" && !configDirSet {
		configDirValue = ""
	}
	var exec executor.Executor
	if *mock {
		exec = &executor.MockExecutor{}
	}
	result, err := tfapply.Apply(ctx, tfapply.Options{
		ConfigDir:   configDirValue,
		ProjectPath: *projectPath,
		StatePath:   path,
		APISources:  sources,
		PlanPath:    *planPath,
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

func runInitCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen init [--project DIR|FILE | --config-dir DIR] [--state PATH]\n")
		fmt.Fprintf(fs.Output(), "\nValidates a native UWS/Ramen project when --project is supplied, otherwise validates Terraform/OpenTofu configuration with tfconfig, then creates or migrates local Ramen SQLite state. It does not execute Terraform, providers, API source operations, module downloads, backend initialization, or UWS workflows.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	path := *statePath
	if strings.TrimSpace(path) == "" {
		path = state.DefaultPath(stateBaseDir(*projectPath, *configDir))
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

func runPlanCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	action := fs.String("action", "create", "Desired managed-resource action for absent resources")
	destroy := fs.Bool("destroy", false, "Plan deletes for managed resources")
	outPath := fs.String("out", "", "Optional JSON plan output path")
	detailedExitCode := fs.Bool("detailed-exitcode", false, "Return 2 when the plan has changes, 1 on errors, and 0 when empty")
	var apiSources repeatedStringFlag
	var targets repeatedStringFlag
	var excludes repeatedStringFlag
	var replaces repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Var(&targets, "target", "Repeatable native resource address to include with dependency closure")
	fs.Var(&excludes, "exclude", "Repeatable native resource address to exclude with dependent closure")
	fs.Var(&replaces, "replace", "Repeatable native resource address to force replacement in the plan")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen plan [--project DIR|FILE | --config-dir DIR] [--state PATH] [--api-source KIND:ID=PATH] [--target ADDRESS] [--exclude ADDRESS] [--replace ADDRESS] [--destroy] [--out PATH]\n")
		fmt.Fprintf(fs.Output(), "\nBuilds a deterministic desired-state plan from native UWS/Ramen project artifacts or transitional Terraform/OpenTofu facts, API source metadata, and recorded SQLite state. It does not execute Terraform, providers, API source operations, refresh, apply, destroy, or UWS workflows.\n\n")
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
		path = state.DefaultPath(stateBaseDir(*projectPath, *configDir))
	}
	result, err := tfplan.Build(ctx, tfplan.Options{
		ConfigDir:   *configDir,
		ProjectPath: *projectPath,
		StatePath:   path,
		APISources:  sources,
		Action:      *action,
		OutPath:     *outPath,
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
	fmt.Printf("ramen: plan create=%d update=%d delete=%d replace=%d no-op=%d diagnostics=%d\n", summary.Create, summary.Update, summary.Delete, summary.Replace, summary.NoOp, summary.Diagnostics)
	if result.OutPath != "" {
		fmt.Printf("  plan: %s\n", result.OutPath)
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

func runConvertCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		convertUsage(os.Stderr)
		os.Exit(2)
	}
	switch args[0] {
	case "tf":
		runConvertTFCommand(ctx, args[1:])
	case "-h", "--help", "help":
		convertUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown convert subcommand %q\n", args[0])
		convertUsage(os.Stderr)
		os.Exit(2)
	}
}

func convertUsage(out *os.File) {
	fmt.Fprintf(out, "Usage: ramen convert <tf> [args]\n\n")
	fmt.Fprintf(out, "Generates Ramen review scaffolding from supported authoring and migration inputs.\n\n")
	fmt.Fprintf(out, "Subcommands:\n")
	fmt.Fprintf(out, "  tf        convert Terraform/OpenTofu configuration into native Ramen/UWS project artifacts\n")
}

func runConvertTFCommand(ctx context.Context, args []string) {
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
	fmt.Printf("  native:      %s\n", result.NativeProjectPath)
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
