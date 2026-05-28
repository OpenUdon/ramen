package reconcile

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tfapply "github.com/OpenUdon/ramen/apply"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/graph"
	"github.com/OpenUdon/ramen/internal/redact"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/state"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
)

type APISourceInput = tfplan.APISourceInput

type Options struct {
	ConfigDir   string
	ProjectPath string
	StatePath   string
	APISources  []APISourceInput
	AutoApprove bool
	OutDir      string
	Executor    executor.Executor
}

type Result struct {
	StatePath string
	RunID     int64
	Summary   Summary
	Actions   []ActionResult
}

type Summary struct {
	Read     int `json:"read"`
	Delete   int `json:"delete"`
	Imported int `json:"imported"`
	Failed   int `json:"failed"`
}

type ActionResult struct {
	Address  string `json:"address"`
	Action   string `json:"action"`
	Document string `json:"document,omitempty"`
}

type ImportOptions struct {
	StatePath   string
	ConfigDir   string
	ProjectPath string
	APISources  []APISourceInput
	Address     string
	Type        string
	Provider    string
	Identity    map[string]any
	Attributes  map[string]any
	SourceKind  string
	SourceID    string
	OperationID string
}

func Refresh(ctx context.Context, opts Options) (*Result, error) {
	opts = normalizeOptions(opts)
	if opts.Executor == nil {
		return nil, fmt.Errorf("refresh requires a trusted executor; pass --mock for recorded/mock execution in public builds")
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ConfigDir: opts.ConfigDir, ProjectPath: opts.ProjectPath, StatePath: opts.StatePath, APISources: opts.APISources, Action: "create"})
	if err != nil {
		return nil, err
	}
	if err := rejectPlanExecution(planResult); err != nil {
		return &Result{StatePath: opts.StatePath}, err
	}
	result, store, runID, finish, err := startMutation(ctx, opts.StatePath, "refresh")
	if err != nil {
		return result, err
	}
	defer finish(&result.Summary)
	defer store.Close()
	sourcePaths := sourcePathIndex(opts.APISources)
	workingDir := opts.ConfigDir
	if opts.ProjectPath != "" {
		workingDir = stateBaseDir(opts.ProjectPath, opts.ConfigDir)
	}
	for _, resource := range planResult.Plan.Resources {
		if resource.Action != "no-op" && resource.Action != "update" {
			continue
		}
		if resource.Mapping == nil || resource.Mapping.OperationID == "" {
			continue
		}
		identity, err := stateIdentity(ctx, store, resource.Address)
		if err != nil {
			result.Summary.Failed++
			return result, err
		}
		doc, err := tfapply.BuildActionDocumentWithBindings(asAction(resource, "read"), sourcePaths, nil, identity)
		if err != nil {
			result.Summary.Failed++
			return result, err
		}
		docPath, err := maybeWriteDocument(opts.OutDir, resource.Address, "refresh", doc)
		if err != nil {
			result.Summary.Failed++
			return result, err
		}
		execResult, err := opts.Executor.Execute(ctx, executor.Request{
			RunID:      runID,
			Action:     executorAction(resource, "read"),
			Document:   doc,
			WorkingDir: workingDir,
			OutDir:     opts.OutDir,
		})
		if err != nil {
			result.Summary.Failed++
			_ = recordFailedAction(ctx, store, runID, resource, "refresh_failed", err.Error())
			return result, err
		}
		if !execResult.Success {
			result.Summary.Failed++
			msg := fmt.Sprintf("executor reported unsuccessful read for %s", resource.Address)
			if err := recordFailedAction(ctx, store, runID, resource, "refresh_failed", msg); err != nil {
				return result, err
			}
			return result, fmt.Errorf("%s", msg)
		}
		if err := recordRefresh(ctx, store, runID, resource, execResult); err != nil {
			result.Summary.Failed++
			return result, err
		}
		result.Summary.Read++
		result.Actions = append(result.Actions, ActionResult{Address: resource.Address, Action: "read", Document: docPath})
	}
	return result, nil
}

func Destroy(ctx context.Context, opts Options) (*Result, error) {
	opts = normalizeOptions(opts)
	if !opts.AutoApprove {
		return &Result{StatePath: opts.StatePath}, fmt.Errorf("destroy requires explicit approval; rerun with --auto-approve after reviewing tracked resources")
	}
	if opts.Executor == nil {
		return nil, fmt.Errorf("destroy requires a trusted executor; pass --mock for recorded/mock execution in public builds")
	}
	depPlan, err := tfplan.Build(ctx, tfplan.Options{ConfigDir: opts.ConfigDir, ProjectPath: opts.ProjectPath, StatePath: opts.StatePath, APISources: opts.APISources, Action: "dependency"})
	if err != nil {
		return nil, err
	}
	if err := rejectPlanExecution(depPlan); err != nil {
		return &Result{StatePath: opts.StatePath}, err
	}
	dependencies := map[string][]string{}
	for _, resource := range depPlan.Plan.Resources {
		dependencies[resource.Address] = slices.Clone(resource.Dependencies)
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ConfigDir: opts.ConfigDir, ProjectPath: opts.ProjectPath, StatePath: opts.StatePath, APISources: opts.APISources, Action: "delete"})
	if err != nil {
		return nil, err
	}
	if err := rejectPlanExecution(planResult); err != nil {
		return &Result{StatePath: opts.StatePath}, err
	}
	result, store, runID, finish, err := startMutation(ctx, opts.StatePath, "destroy")
	if err != nil {
		return result, err
	}
	defer finish(&result.Summary)
	defer store.Close()
	for i := range planResult.Plan.Resources {
		if deps := dependencies[planResult.Plan.Resources[i].Address]; len(deps) > 0 {
			planResult.Plan.Resources[i].Dependencies = deps
		}
	}
	resources := destroyOrder(planResult.Plan.Resources)
	sourcePaths := sourcePathIndex(opts.APISources)
	workingDir := opts.ConfigDir
	if opts.ProjectPath != "" {
		workingDir = stateBaseDir(opts.ProjectPath, opts.ConfigDir)
	}
	for _, resource := range resources {
		if resource.Action != "delete" {
			continue
		}
		if resource.Mapping == nil || resource.Mapping.OperationID == "" {
			continue
		}
		resource = asAction(resource, "delete")
		identity, err := stateIdentity(ctx, store, resource.Address)
		if err != nil {
			result.Summary.Failed++
			return result, err
		}
		doc, err := tfapply.BuildActionDocumentWithBindings(resource, sourcePaths, nil, identity)
		if err != nil {
			result.Summary.Failed++
			return result, err
		}
		docPath, err := maybeWriteDocument(opts.OutDir, resource.Address, "destroy", doc)
		if err != nil {
			result.Summary.Failed++
			return result, err
		}
		execResult, err := opts.Executor.Execute(ctx, executor.Request{
			RunID:      runID,
			Action:     executorAction(resource, "delete"),
			Document:   doc,
			WorkingDir: workingDir,
			OutDir:     opts.OutDir,
		})
		if err != nil {
			result.Summary.Failed++
			_ = recordFailedAction(ctx, store, runID, resource, "delete_failed", err.Error())
			return result, err
		}
		if !execResult.Success {
			result.Summary.Failed++
			msg := fmt.Sprintf("executor reported unsuccessful delete for %s", resource.Address)
			if err := recordFailedAction(ctx, store, runID, resource, "delete_failed", msg); err != nil {
				return result, err
			}
			return result, fmt.Errorf("%s", msg)
		}
		if err := recordSuccessfulDestroy(ctx, store, runID, resource); err != nil {
			result.Summary.Failed++
			return result, err
		}
		result.Summary.Delete++
		result.Actions = append(result.Actions, ActionResult{Address: resource.Address, Action: "delete", Document: docPath})
	}
	return result, nil
}

func Import(ctx context.Context, opts ImportOptions) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(opts.StatePath) == "" {
		opts.StatePath = state.DefaultPath(stateBaseDir(opts.ProjectPath, firstNonEmpty(opts.ConfigDir, ".")))
	}
	if strings.TrimSpace(opts.ConfigDir) == "" {
		opts.ConfigDir = "."
	}
	opts.ProjectPath = strings.TrimSpace(opts.ProjectPath)
	if strings.TrimSpace(opts.Address) == "" || strings.TrimSpace(opts.Type) == "" {
		return nil, fmt.Errorf("import requires address and type")
	}
	if err := validateImportMetadata(ctx, opts); err != nil {
		return nil, err
	}
	store, err := state.Open(ctx, opts.StatePath)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	lockHolder := fmt.Sprintf("import-%d", time.Now().UTC().UnixNano())
	if err := store.AcquireLock(ctx, "state", lockHolder, 30*time.Minute); err != nil {
		return nil, err
	}
	defer func() { _ = store.ReleaseLock(context.Background(), "state", lockHolder) }()
	current, err := store.CurrentResource(ctx, opts.Address)
	if err != nil {
		return nil, err
	}
	if current != nil {
		return nil, fmt.Errorf("import.state_conflict: %s is already present in state", opts.Address)
	}
	runID, err := store.StartRun(ctx, "import")
	if err != nil {
		return nil, err
	}
	runFinished := false
	defer func() {
		if !runFinished {
			_ = store.FinishRun(context.Background(), runID, "failed", "")
		}
	}()
	identityJSON, err := json.Marshal(redact.Map(opts.Identity))
	if err != nil {
		return nil, err
	}
	attrsJSON, err := json.Marshal(redact.Map(opts.Attributes))
	if err != nil {
		return nil, err
	}
	hash, mapping := importDesiredHash(ctx, opts, "import:"+opts.Address+":"+string(identityJSON))
	if opts.SourceKind == "" && mapping != nil {
		opts.SourceKind = mapping.SourceKind
	}
	if opts.SourceID == "" && mapping != nil {
		opts.SourceID = mapping.SourceID
	}
	if opts.OperationID == "" && mapping != nil {
		opts.OperationID = mapping.OperationID
	}
	snap := state.ResourceSnapshot{
		Address:        opts.Address,
		Type:           opts.Type,
		Provider:       opts.Provider,
		DesiredHash:    hash,
		IdentityJSON:   string(identityJSON),
		AttributesJSON: string(attrsJSON),
		Status:         "imported",
		SourceKind:     opts.SourceKind,
		SourceID:       opts.SourceID,
		OperationID:    opts.OperationID,
		UpdatedRunID:   runID,
		UpdatedAt:      time.Now().UTC(),
	}
	afterJSON, _ := json.Marshal(snap)
	if err := store.WithTx(ctx, func(tx *state.Tx) error {
		if err := tx.RecordResource(ctx, snap); err != nil {
			return err
		}
		return tx.RecordRevision(ctx, state.Revision{ResourceAddress: opts.Address, RunID: runID, Action: "import", AfterJSON: string(afterJSON)})
	}); err != nil {
		return nil, err
	}
	summary := Summary{Imported: 1}
	data, _ := json.Marshal(summary)
	if err := store.FinishRun(ctx, runID, "completed", string(data)); err != nil {
		return nil, err
	}
	runFinished = true
	return &Result{StatePath: opts.StatePath, RunID: runID, Summary: summary, Actions: []ActionResult{{Address: opts.Address, Action: "import"}}}, nil
}

func validateImportMetadata(ctx context.Context, opts ImportOptions) error {
	if strings.TrimSpace(opts.ProjectPath) == "" {
		return nil
	}
	result, err := ramenvalidate.Run(ctx, ramenvalidate.Options{ProjectPath: opts.ProjectPath, APISources: validateAPISources(opts.APISources)})
	if err != nil {
		return err
	}
	if !result.Valid {
		if len(result.Diagnostics) > 0 {
			diag := result.Diagnostics[0]
			return fmt.Errorf("%s: %s", diag.Code, diag.Message)
		}
		return fmt.Errorf("import.project_invalid: native project is invalid")
	}
	doc, err := project.Load(opts.ProjectPath)
	if err != nil {
		return fmt.Errorf("import.project_load_error: %w", err)
	}
	var resource *project.Resource
	for i := range doc.Profile.Resources {
		if doc.Profile.Resources[i].Address == opts.Address {
			resource = &doc.Profile.Resources[i]
			break
		}
	}
	if resource == nil {
		return fmt.Errorf("import.address_unknown: %s does not exist in native project", opts.Address)
	}
	if resource.Type != "" && opts.Type != "" && resource.Type != opts.Type {
		return fmt.Errorf("import.type_mismatch: %s has type %s in native project, got %s", opts.Address, resource.Type, opts.Type)
	}
	allowed := map[string]project.IdentityAttribute{}
	for _, attr := range resource.IdentityAttributes {
		allowed[attr.Name] = attr
		if attr.Required {
			if _, ok := opts.Identity[attr.Name]; !ok {
				return fmt.Errorf("import.identity_missing: %s requires identity attribute %s", opts.Address, attr.Name)
			}
		}
	}
	for key := range opts.Identity {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("import.identity_unknown: %s identity attribute %s is not declared in native project", opts.Address, key)
		}
	}
	if _, ok := resource.Operations["import"]; !ok {
		if _, ok := resource.Operations["read"]; !ok {
			return fmt.Errorf("import.operation_missing: %s requires an import or read operation role", opts.Address)
		}
	}
	return nil
}

func validateAPISources(inputs []APISourceInput) []ramenvalidate.APISourceInput {
	out := make([]ramenvalidate.APISourceInput, len(inputs))
	for i, input := range inputs {
		out[i] = ramenvalidate.APISourceInput(input)
	}
	return out
}

func importDesiredHash(ctx context.Context, opts ImportOptions, fallback string) (string, *tfplan.MappingPlan) {
	if strings.TrimSpace(opts.ProjectPath) == "" && (strings.TrimSpace(opts.ConfigDir) == "" || len(opts.APISources) == 0) {
		return fallback, nil
	}
	result, err := tfplan.Build(ctx, tfplan.Options{
		ConfigDir:   opts.ConfigDir,
		ProjectPath: opts.ProjectPath,
		StatePath:   opts.StatePath,
		APISources:  opts.APISources,
		Action:      "create",
	})
	if err != nil || result == nil || result.Plan.Errored {
		return fallback, nil
	}
	for _, resource := range result.Plan.Resources {
		if resource.Address == opts.Address && resource.DesiredHash != "" {
			return resource.DesiredHash, resource.Mapping
		}
	}
	return fallback, nil
}

func normalizeOptions(opts Options) Options {
	if strings.TrimSpace(opts.ConfigDir) == "" {
		opts.ConfigDir = "."
	}
	if strings.TrimSpace(opts.StatePath) == "" {
		opts.StatePath = state.DefaultPath(stateBaseDir(opts.ProjectPath, opts.ConfigDir))
	}
	opts.ProjectPath = strings.TrimSpace(opts.ProjectPath)
	return opts
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

func rejectPlanExecution(planResult *tfplan.Result) error {
	if planResult != nil && planResult.Plan.Errored {
		return fmt.Errorf("plan is marked errored and cannot be executed")
	}
	if planResult == nil {
		return nil
	}
	for _, diag := range planResult.Diagnostics {
		if diag.Severity == "error" {
			return fmt.Errorf("plan has error diagnostic %s: %s", diag.Code, diag.Message)
		}
	}
	return tfplan.VerifyApproval(planResult.Plan)
}

func startMutation(ctx context.Context, statePath, command string) (*Result, *state.Store, int64, func(*Summary), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	store, err := state.Open(ctx, statePath)
	if err != nil {
		return &Result{StatePath: statePath}, nil, 0, nil, err
	}
	lockHolder := fmt.Sprintf("%s-%d", command, time.Now().UTC().UnixNano())
	if err := store.AcquireLock(ctx, "state", lockHolder, 30*time.Minute); err != nil {
		_ = store.Close()
		return &Result{StatePath: statePath}, nil, 0, nil, err
	}
	runID, err := store.StartRun(ctx, command)
	if err != nil {
		_ = store.ReleaseLock(ctx, "state", lockHolder)
		_ = store.Close()
		return &Result{StatePath: statePath}, nil, 0, nil, err
	}
	result := &Result{StatePath: statePath, RunID: runID}
	finish := func(summary *Summary) {
		status := "completed"
		if summary != nil && summary.Failed > 0 {
			status = "failed"
		}
		data, _ := json.Marshal(summary)
		_ = store.FinishRun(context.Background(), runID, status, string(data))
		_ = store.ReleaseLock(context.Background(), "state", lockHolder)
	}
	return result, store, runID, finish, nil
}

func destroyOrder(resources []tfplan.ResourcePlan) []tfplan.ResourcePlan {
	byAddress := map[string]tfplan.ResourcePlan{}
	var nodes []graph.Node
	for _, resource := range resources {
		if resource.Action == "error" {
			continue
		}
		byAddress[resource.Address] = resource
		nodes = append(nodes, graph.Node{Address: resource.Address, DependsOn: resource.Dependencies})
	}
	sorted, err := graph.Sort(nodes)
	if err != nil {
		out := make([]tfplan.ResourcePlan, 0, len(byAddress))
		for _, resource := range byAddress {
			out = append(out, resource)
		}
		slices.SortStableFunc(out, func(a, b tfplan.ResourcePlan) int { return cmp.Compare(b.Address, a.Address) })
		return out
	}
	out := make([]tfplan.ResourcePlan, 0, len(sorted))
	for i := len(sorted) - 1; i >= 0; i-- {
		out = append(out, byAddress[sorted[i].Address])
	}
	return out
}

func asAction(resource tfplan.ResourcePlan, action string) tfplan.ResourcePlan {
	resource.Action = action
	if resource.Mapping != nil {
		resource.Mapping.Purpose = action
	}
	return resource
}

func executorAction(resource tfplan.ResourcePlan, action string) executor.Action {
	out := executor.Action{Address: resource.Address, Type: resource.Type, Provider: resource.Provider, Action: action, DesiredHash: resource.DesiredHash}
	if resource.Mapping != nil {
		out.Mapping = executor.ActionMapping{SourceKind: resource.Mapping.SourceKind, SourceID: resource.Mapping.SourceID, SourcePath: resource.Mapping.SourcePath, OperationID: resource.Mapping.OperationID}
	}
	return out
}

func recordRefresh(ctx context.Context, store *state.Store, runID int64, resource tfplan.ResourcePlan, execResult executor.Result) error {
	current, err := store.CurrentResource(ctx, resource.Address)
	if err != nil {
		return err
	}
	if current == nil {
		return nil
	}
	if len(execResult.Identity) > 0 {
		data, _ := json.Marshal(redact.Map(execResult.Identity))
		current.IdentityJSON = string(data)
	}
	if len(execResult.Computed) > 0 {
		data, _ := json.Marshal(redact.Map(execResult.Computed))
		current.AttributesJSON = string(data)
	}
	current.UpdatedRunID = runID
	current.UpdatedAt = time.Now().UTC()
	afterJSON, _ := json.Marshal(current)
	return store.WithTx(ctx, func(tx *state.Tx) error {
		if err := tx.RecordResource(ctx, *current); err != nil {
			return err
		}
		return tx.RecordRevision(ctx, state.Revision{ResourceAddress: resource.Address, RunID: runID, Action: "refresh", AfterJSON: string(afterJSON)})
	})
}

func recordSuccessfulDestroy(ctx context.Context, store *state.Store, runID int64, resource tfplan.ResourcePlan) error {
	before, err := store.CurrentResource(ctx, resource.Address)
	if err != nil {
		return err
	}
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return err
	}
	return store.WithTx(ctx, func(tx *state.Tx) error {
		if err := tx.DeleteResource(ctx, resource.Address); err != nil {
			return err
		}
		return tx.RecordRevision(ctx, state.Revision{ResourceAddress: resource.Address, RunID: runID, Action: "delete", BeforeJSON: string(beforeJSON)})
	})
}

func recordFailedAction(ctx context.Context, store *state.Store, runID int64, resource tfplan.ResourcePlan, action, message string) error {
	before, err := store.CurrentResource(ctx, resource.Address)
	if err != nil {
		return err
	}
	beforeJSON := []byte(nil)
	if before != nil {
		beforeJSON, err = json.Marshal(before)
		if err != nil {
			return err
		}
	}
	diffJSON, err := json.Marshal(map[string]any{
		"status":       "failed",
		"error":        redact.String(message),
		"desired_hash": resource.DesiredHash,
	})
	if err != nil {
		return err
	}
	return store.RecordRevision(ctx, state.Revision{
		ResourceAddress: resource.Address,
		RunID:           runID,
		Action:          action,
		BeforeJSON:      string(beforeJSON),
		DiffJSON:        string(diffJSON),
	})
}

func stateIdentity(ctx context.Context, store *state.Store, address string) (map[string]any, error) {
	current, err := store.CurrentResource(ctx, address)
	if err != nil || current == nil || strings.TrimSpace(current.IdentityJSON) == "" {
		return nil, err
	}
	var identity map[string]any
	if err := json.Unmarshal([]byte(current.IdentityJSON), &identity); err != nil {
		return nil, fmt.Errorf("read state identity for %s: %w", address, err)
	}
	return identity, nil
}

func maybeWriteDocument(outDir, address, action string, doc any) (string, error) {
	if strings.TrimSpace(outDir) == "" {
		return "", nil
	}
	path := filepath.Join(outDir, "actions", normalizeName(address+"_"+action)+".uws.json")
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

func sourcePathIndex(inputs []APISourceInput) map[string]string {
	out := map[string]string{}
	for _, input := range inputs {
		out[strings.TrimSpace(input.Kind)+"\x00"+strings.TrimSpace(input.ID)] = input.Path
	}
	return out
}

func normalizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
