package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/OpenUdon/ramen/graph"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/state"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
)

// version is replaced in release archives with -ldflags. Module-installed
// binaries fall back to debug.BuildInfo's main module version.
var version = "devel"

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
	fmt.Printf("  summary: create=%d update=%d delete=%d post=%d put=%d patch=%d replace=%d no-op=%d read=%d diagnostics=%d\n", doc.Summary.Create, doc.Summary.Update, doc.Summary.Delete, doc.Summary.Post, doc.Summary.Put, doc.Summary.Patch, doc.Summary.Replace, doc.Summary.NoOp, doc.Summary.Read, doc.Summary.Diagnostics)
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
		Version: strings.TrimSpace(version),
		Module:  "github.com/OpenUdon/ramen",
	}
	if build, ok := debug.ReadBuildInfo(); ok {
		info.GoVersion = build.GoVersion
		if build.Main.Path != "" {
			info.MainPath = build.Main.Path
		}
		if (info.Version == "" || info.Version == "devel") &&
			build.Main.Version != "" && build.Main.Version != "(devel)" {
			info.Version = strings.TrimPrefix(build.Main.Version, "v")
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
	if info.Version == "" {
		info.Version = "devel"
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
