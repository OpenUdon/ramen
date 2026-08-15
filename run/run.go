package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/OpenUdon/evidence/digest"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/governance"
	"github.com/OpenUdon/ramen/internal/asyncrecord"
	"github.com/OpenUdon/ramen/internal/redact"
	"github.com/OpenUdon/ramen/state"
	"github.com/OpenUdon/uws/schemas"
	"github.com/OpenUdon/uws/uws1"
	"github.com/OpenUdon/uws/validation"
)

const Version = "ramen.run.v1"

type Options struct {
	DocumentPath   string
	StatePath      string
	Workspace      string
	Targets        []string
	PolicyFiles    []string
	Check          bool
	AutoApprove    bool
	ApprovalDigest string
	OutDir         string
	Executor       executor.Executor
}

type Result struct {
	Version        string            `json:"version"`
	DocumentPath   string            `json:"document_path"`
	DocumentDigest string            `json:"document_digest"`
	StatePath      string            `json:"state_path,omitempty"`
	Workspace      string            `json:"workspace,omitempty"`
	RunID          int64             `json:"run_id,omitempty"`
	Check          bool              `json:"check"`
	ApprovalDigest string            `json:"approval_digest"`
	Governance     governance.Result `json:"governance,omitempty"`
	Summary        Summary           `json:"summary"`
	Executed       []ExecutedTarget  `json:"executed,omitempty"`
	Errors         []string          `json:"errors,omitempty"`
}

type Summary struct {
	Targets  int `json:"targets"`
	Executed int `json:"executed"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
}

type ExecutedTarget struct {
	Target string          `json:"target"`
	Action string          `json:"action"`
	Result executor.Result `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func Execute(ctx context.Context, opts Options) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opts.DocumentPath = strings.TrimSpace(opts.DocumentPath)
	if opts.DocumentPath == "" {
		return nil, fmt.Errorf("run.document_required: UWS document path is required")
	}
	doc, docDigest, err := loadDocument(opts.DocumentPath)
	if err != nil {
		return nil, err
	}
	targets := normalizeTargets(opts.Targets)
	if len(targets) == 0 {
		targets = []string{"default"}
	}
	result := &Result{
		Version:        Version,
		DocumentPath:   opts.DocumentPath,
		DocumentDigest: docDigest,
		StatePath:      opts.StatePath,
		Workspace:      strings.TrimSpace(opts.Workspace),
		Check:          opts.Check,
		Governance:     governance.Result{Version: governance.ResultVersion},
		Summary:        Summary{Targets: len(targets)},
	}
	result.Governance = evaluateGovernance(opts, targets)
	result.ApprovalDigest = approvalDigest(*result, targets)
	if err := rejectGovernance(result.Governance); err != nil {
		result.Errors = append(result.Errors, err.Error())
		return result, err
	}
	if opts.Check {
		result.Summary.Skipped = len(targets)
		for _, target := range targets {
			result.Executed = append(result.Executed, ExecutedTarget{Target: target, Action: "check"})
		}
		return result, nil
	}
	if strings.TrimSpace(opts.ApprovalDigest) != "" && strings.TrimSpace(opts.ApprovalDigest) != result.ApprovalDigest {
		return result, fmt.Errorf("run.approval_mismatch: approval digest does not match document, targets, workspace, or state path")
	}
	if strings.TrimSpace(opts.ApprovalDigest) == "" && !opts.AutoApprove {
		return result, fmt.Errorf("run approval required for %d target(s); rerun with --auto-approve after review or supply --approval-digest %s", len(targets), result.ApprovalDigest)
	}
	if opts.Executor == nil {
		return result, fmt.Errorf("run.executor_required: trusted executor is required")
	}
	store, err := state.Open(ctx, opts.StatePath)
	if err != nil {
		return result, err
	}
	defer func() { _ = store.Close() }()
	runID, err := store.StartRun(ctx, "run")
	if err != nil {
		return result, err
	}
	result.RunID = runID
	defer finishRun(store, result)
	recorder := asyncrecord.New(store, runID)
	for _, target := range targets {
		executed := executeTarget(ctx, opts, doc, target, runID, store, recorder)
		result.Executed = append(result.Executed, executed)
		if executed.Error != "" {
			result.Summary.Failed++
			result.Errors = append(result.Errors, executed.Error)
			continue
		}
		result.Summary.Executed++
	}
	return result, nil
}

func evaluateGovernance(opts Options, targets []string) governance.Result {
	engine, loadDecisions := governance.LoadPolicyFiles(opts.PolicyFiles)
	resources := make([]governance.Resource, 0, len(targets))
	for _, target := range targets {
		resources = append(resources, governance.Resource{Address: "run." + target, Type: "uws_run", Action: "run"})
	}
	return governance.MergeResults(governance.Result{Version: governance.ResultVersion, Decisions: loadDecisions}, engine.Evaluate(governance.Input{Action: "run", Resources: resources}))
}

func rejectGovernance(result governance.Result) error {
	for _, decision := range result.Decisions {
		if decision.Severity == "error" {
			return fmt.Errorf("%s: %s", decision.Code, decision.Message)
		}
	}
	return nil
}

func executeTarget(ctx context.Context, opts Options, doc *uws1.Document, target string, runID int64, store *state.Store, recorder *asyncrecord.Recorder) ExecutedTarget {
	action := executor.Action{
		Address:  "run." + target,
		Type:     "uws_run",
		Action:   "run",
		Mapping:  executor.ActionMapping{SourceKind: "uws", OperationID: filepath.Base(opts.DocumentPath)},
		Metadata: map[string]string{"target": target},
	}
	var events []executor.Event
	var eventsMu sync.Mutex
	req := executor.Request{
		RunID:        runID,
		Action:       action,
		Document:     doc,
		WorkingDir:   filepath.Dir(opts.DocumentPath),
		OutDir:       opts.OutDir,
		Capabilities: executor.RequirementsForAction(action),
		Idempotency:  executor.IdempotencyForAction(action),
	}
	if err := executor.EnsureSupported(opts.Executor, req); err != nil {
		recordRunEvent(store, runID, target, action, "failed", err.Error(), nil)
		return ExecutedTarget{Target: target, Action: "run", Error: err.Error()}
	}
	requestEvidenceID, recordErr := recorder.RecordRequest(ctx, req)
	if recordErr != nil {
		recordRunEvent(store, runID, target, action, "failed", recordErr.Error(), nil)
		return ExecutedTarget{Target: target, Action: "run", Error: recordErr.Error()}
	}
	req.Events = recorder.EventSink(ctx, req, requestEvidenceID, func(event executor.Event) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
		_ = store.RecordRunEvent(context.Background(), state.RunEvent{
			RunID:           runID,
			ResourceAddress: "run." + target,
			Action:          "run",
			OperationID:     action.Mapping.OperationID,
			Phase:           event.Phase,
			Message:         event.Message,
			MetadataJSON:    mustJSON(event.Metadata),
			CreatedAt:       event.Time,
		})
	})
	result, err := opts.Executor.Execute(ctx, req)
	if recordErr := recorder.RecordResponse(ctx, req, result, err, requestEvidenceID); recordErr != nil {
		recordRunEvent(store, runID, target, action, "failed", recordErr.Error(), nil)
		return ExecutedTarget{Target: target, Action: "run", Result: result, Error: recordErr.Error()}
	}
	if err != nil {
		recordRunEvent(store, runID, target, action, "failed", err.Error(), nil)
		return ExecutedTarget{Target: target, Action: "run", Error: err.Error()}
	}
	eventsMu.Lock()
	result.Events = append(result.Events, events...)
	eventsMu.Unlock()
	if !result.Success {
		message := strings.Join(result.Messages, "; ")
		recordRunEvent(store, runID, target, action, "failed", message, nil)
		return ExecutedTarget{Target: target, Action: "run", Result: result, Error: message}
	}
	return ExecutedTarget{Target: target, Action: "run", Result: result}
}

func recordRunEvent(store *state.Store, runID int64, target string, action executor.Action, phase, message string, metadata map[string]any) {
	_ = store.RecordRunEvent(context.Background(), state.RunEvent{
		RunID:           runID,
		ResourceAddress: "run." + target,
		Action:          "run",
		OperationID:     action.Mapping.OperationID,
		Phase:           phase,
		Message:         message,
		MetadataJSON:    mustJSON(metadata),
		CreatedAt:       timeNow(),
	})
}

func loadDocument(path string) (*uws1.Document, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("run.document_load_error: %w", err)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	default:
		return nil, "", fmt.Errorf("run.document_parse_error: unsupported UWS document extension %q", filepath.Ext(path))
	case ".json", ".yaml", ".yml":
	}
	doc, err := validation.LoadDocumentFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("run.document_parse_error: %w", err)
	}
	schemaVersion := strings.TrimSpace(doc.UWS)
	if schemaVersion == "" {
		schemaVersion = "1.0.0"
	}
	if err := validation.ValidateFile(schemas.PathForVersion(filepath.Dir(path), schemaVersion), path); err != nil {
		return nil, "", fmt.Errorf("run.document_invalid: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return nil, "", fmt.Errorf("run.document_invalid: %w", err)
	}
	return doc, digest.SHA256String(data), nil
}

func approvalDigest(result Result, targets []string) string {
	payload := struct {
		Version        string            `json:"version"`
		DocumentDigest string            `json:"document_digest"`
		StatePath      string            `json:"state_path,omitempty"`
		Workspace      string            `json:"workspace,omitempty"`
		Targets        []string          `json:"targets"`
		Governance     governance.Result `json:"governance,omitempty"`
	}{
		Version:        result.Version,
		DocumentDigest: result.DocumentDigest,
		StatePath:      result.StatePath,
		Workspace:      result.Workspace,
		Targets:        targets,
		Governance:     result.Governance,
	}
	data, _ := json.Marshal(payload)
	return digest.SHA256String(data)
}

func normalizeTargets(targets []string) []string {
	out := make([]string, 0, len(targets))
	seen := map[string]bool{}
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		out = append(out, target)
	}
	slices.Sort(out)
	return out
}

func finishRun(store *state.Store, result *Result) {
	status := "completed"
	if result.Summary.Failed > 0 {
		status = "failed"
	}
	data, _ := json.Marshal(result.Summary)
	_ = store.FinishRun(context.Background(), result.RunID, status, string(data))
}

func mustJSON(value any) string {
	if value == nil {
		return ""
	}
	if typed, ok := value.(map[string]any); ok {
		value = redact.Map(typed)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func timeNow() time.Time {
	return time.Now().UTC()
}
