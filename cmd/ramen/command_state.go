package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OpenUdon/ramen/state"
)

func runStateCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		stateUsage(os.Stderr)
		os.Exit(2)
	}
	switch args[0] {
	case "async-evidence":
		runStateAsyncEvidenceCommand(ctx, args[1:])
	case "audit":
		runStateAuditCommand(ctx, args[1:])
	case "backup":
		runStateBackupCommand(ctx, args[1:])
	case "export":
		runStateExportCommand(ctx, args[1:])
	case "list":
		runStateListCommand(ctx, args[1:])
	case "restore":
		runStateRestoreCommand(ctx, args[1:])
	case "show":
		runStateShowCommand(ctx, args[1:])
	case "history":
		runStateHistoryCommand(ctx, args[1:])
	case "runs":
		runStateRunsCommand(ctx, args[1:])
	case "vacuum":
		runStateVacuumCommand(ctx, args[1:])
	case "-h", "--help", "help":
		stateUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown state subcommand %q\n", args[0])
		stateUsage(os.Stderr)
		os.Exit(2)
	}
}

func stateUsage(out *os.File) {
	fmt.Fprintf(out, "Usage: ramen state <async-evidence|audit|backup|export|list|restore|show|history|runs|vacuum> [args]\n\n")
	fmt.Fprintf(out, "Local SQLite state inspection and maintenance. No backend access, provider execution, or Terraform/OpenTofu state compatibility is performed.\n\n")
	fmt.Fprintf(out, "Subcommands:\n")
	fmt.Fprintf(out, "  async-evidence    list neutral async execution evidence records\n")
	fmt.Fprintf(out, "  audit             export tamper-evident audit summary\n")
	fmt.Fprintf(out, "  backup            write a consistent SQLite backup snapshot\n")
	fmt.Fprintf(out, "  export            export redacted state metadata as JSON\n")
	fmt.Fprintf(out, "  list              list current resource addresses\n")
	fmt.Fprintf(out, "  restore           replace local state from a backup with --force\n")
	fmt.Fprintf(out, "  show ADDRESS      show one current resource\n")
	fmt.Fprintf(out, "  history [ADDRESS] show revision history\n")
	fmt.Fprintf(out, "  runs              show command run history\n")
	fmt.Fprintf(out, "  vacuum            compact local SQLite state\n")
}

func runStateAsyncEvidenceCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state async-evidence", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	runID := fs.Int64("run", 0, "Optional run ID filter")
	address := fs.String("address", "", "Optional resource address filter")
	kind := fs.String("kind", "", "Optional record kind filter")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state async-evidence [--state PATH] [--run ID] [--address ADDRESS] [--kind KIND] [--json]\n")
		fmt.Fprintf(fs.Output(), "\nLists durable neutral async evidence records attached to local runs. These records are execution observations only; accepted executor responses do not imply convergence or state success.\n\n")
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
			writeJSONOutput([]state.AsyncEvidenceRecord{})
		} else {
			fmt.Println("ramen: state async-evidence=0")
		}
		return
	}
	defer func() { _ = store.Close() }()
	records, err := store.ListAsyncEvidence(ctx, state.AsyncEvidenceFilter{RunID: *runID, ResourceAddress: *address, RecordKind: *kind})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(records)
		return
	}
	fmt.Printf("ramen: state async-evidence=%d\n", len(records))
	for _, record := range records {
		fmt.Printf("  #%d run=%d %s %s kind=%s phase=%s evidence=%s\n", record.ID, record.RunID, record.ResourceAddress, record.Action, record.RecordKind, record.Phase, record.EvidenceID)
	}
}

func runStateAuditCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state audit", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state audit [--state PATH] [--json]\n")
		fmt.Fprintf(fs.Output(), "\nExports a tamper-evident local audit summary derived from runs, revisions, resources, locks, and migration records without remote backend access.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "state path %s does not exist\n", strings.TrimSpace(*statePath))
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	doc, err := store.Audit(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(doc)
		return
	}
	fmt.Printf("ramen: state audit version=%s digest=%s resources=%d revisions=%d runs=%d events=%d async_evidence=%d locks=%d\n", doc.Version, doc.Digest, doc.Counts["resources"], doc.Counts["revisions"], doc.Counts["runs"], doc.Counts["run_events"], doc.Counts["async_evidence"], doc.Counts["locks"])
}

func runStateBackupCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state backup", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	outPath := fs.String("out", "", "Backup output path")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state backup --state PATH --out PATH\n")
		fmt.Fprintf(fs.Output(), "\nWrites a consistent local SQLite backup snapshot. It does not read or write remote backends or Terraform/OpenTofu state.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 || strings.TrimSpace(*outPath) == "" {
		fs.Usage()
		os.Exit(2)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if store == nil {
		fmt.Fprintf(os.Stderr, "state path %s does not exist\n", strings.TrimSpace(*statePath))
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	if err := store.Backup(ctx, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: state backup written %s\n", strings.TrimSpace(*outPath))
}

func runStateExportCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state export", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state export --state PATH --json\n")
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
		fmt.Fprintf(os.Stderr, "state path %s does not exist\n", strings.TrimSpace(*statePath))
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	doc, err := store.Export(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(doc)
		return
	}
	fmt.Printf("ramen: state export version=%s resources=%d revisions=%d runs=%d async_evidence=%d locks=%d\n", doc.Version, len(doc.Resources), len(doc.Revisions), len(doc.Runs), len(doc.AsyncEvidence), len(doc.Locks))
}

func runStateRestoreCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state restore", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	fromPath := fs.String("from", "", "Backup input path")
	force := fs.Bool("force", false, "Required confirmation to overwrite local state")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state restore --state PATH --from PATH --force\n")
		fmt.Fprintf(fs.Output(), "\nReplaces the local SQLite state file from a validated Ramen backup. This is local state surgery only and never reads remote backends or Terraform/OpenTofu state.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 || strings.TrimSpace(*fromPath) == "" {
		fs.Usage()
		os.Exit(2)
	}
	if err := state.Restore(ctx, *statePath, *fromPath, *force); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: state restored %s from %s\n", strings.TrimSpace(*statePath), strings.TrimSpace(*fromPath))
}

func runStateVacuumCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state vacuum", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state vacuum [--state PATH]\n")
		fmt.Fprintf(fs.Output(), "\nCompacts local SQLite state after taking the cooperative state lock. It does not read or write remote backends or Terraform/OpenTofu state.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	store, err := state.Open(ctx, strings.TrimSpace(*statePath))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	holder := maintenanceLockHolder("vacuum")
	if err := store.AcquireLock(ctx, "state", holder, 30*time.Minute); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = store.ReleaseLock(context.Background(), "state", holder) }()
	if err := store.Vacuum(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: state vacuum completed %s\n", strings.TrimSpace(*statePath))
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
