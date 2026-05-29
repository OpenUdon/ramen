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
	"github.com/OpenUdon/ramen/tfmapping"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
)

const Version = "ramen.reconcile.v1"

type APISourceInput = tfplan.APISourceInput

type Options struct {
	ConfigDir   string
	ProjectPath string
	StatePath   string
	APISources  []APISourceInput
	VarFiles    []string
	Vars        []string
	Workspace   string
	PlanPath    string
	AutoApprove bool
	OutDir      string
	Executor    executor.Executor
}

type Result struct {
	Version   string                    `json:"version"`
	StatePath string                    `json:"state_path"`
	RunID     int64                     `json:"run_id,omitempty"`
	Summary   Summary                   `json:"summary"`
	Actions   []ActionResult            `json:"actions,omitempty"`
	Feedback  []executor.FeedbackRecord `json:"feedback,omitempty"`
}

type Summary struct {
	Read      int `json:"read"`
	Delete    int `json:"delete"`
	Imported  int `json:"imported"`
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
	Missing   int `json:"missing"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
}

type ActionResult struct {
	Address  string `json:"address"`
	Action   string `json:"action"`
	Document string `json:"document,omitempty"`
}

func newResult(statePath string) *Result {
	return &Result{Version: Version, StatePath: statePath}
}

type ImportOptions struct {
	StatePath   string
	ConfigDir   string
	ProjectPath string
	APISources  []APISourceInput
	VarFiles    []string
	Vars        []string
	Workspace   string
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
	readMappings, err := refreshReadMappings(ctx, opts)
	if err != nil {
		return newResult(opts.StatePath), err
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ConfigDir: opts.ConfigDir, ProjectPath: opts.ProjectPath, StatePath: opts.StatePath, APISources: opts.APISources, VarFiles: opts.VarFiles, Vars: opts.Vars, Workspace: opts.Workspace, Action: "create"})
	if err != nil {
		return nil, err
	}
	if err := rejectPlanExecution(planResult); err != nil {
		return newResult(opts.StatePath), err
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
			result.Summary.Skipped++
			continue
		}
		if mapping := readMappings[resource.Address]; mapping != nil {
			resource.Mapping = mapping
		}
		if resource.Mapping == nil || resource.Mapping.OperationID == "" {
			result.Summary.Failed++
			return result, fmt.Errorf("refresh.read_operation_missing: %s has no usable read operation metadata", resource.Address)
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
		action := executorAction(resource, "read")
		req := executor.Request{
			RunID:      runID,
			Action:     action,
			Document:   doc,
			WorkingDir: workingDir,
			OutDir:     opts.OutDir,
		}
		req.Capabilities = executor.RequirementsForAction(action)
		req.Idempotency = executor.IdempotencyForAction(action)
		req.Events = tfapply.ExecutorEventSink(store)
		if err := executor.EnsureSupported(opts.Executor, req); err != nil {
			result.Summary.Failed++
			_ = recordFailedAction(ctx, store, runID, resource, "refresh_failed", err.Error())
			return result, err
		}
		execResult, err := opts.Executor.Execute(ctx, req)
		result.Feedback = append(result.Feedback, executor.FeedbackFromResult(req, execResult, err))
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
		result.Summary.Read++
		if execResult.Missing {
			if err := recordMissingRefresh(ctx, store, runID, resource); err != nil {
				result.Summary.Failed++
				return result, err
			}
			result.Summary.Missing++
			result.Actions = append(result.Actions, ActionResult{Address: resource.Address, Action: "read", Document: docPath})
			continue
		}
		changed, err := recordRefresh(ctx, store, runID, resource, execResult)
		if err != nil {
			result.Summary.Failed++
			return result, err
		}
		if changed {
			result.Summary.Changed++
		} else {
			result.Summary.Unchanged++
		}
		result.Actions = append(result.Actions, ActionResult{Address: resource.Address, Action: "read", Document: docPath})
	}
	return result, nil
}

func Destroy(ctx context.Context, opts Options) (*Result, error) {
	opts.PlanPath = strings.TrimSpace(opts.PlanPath)
	var artifact *tfplan.Document
	if opts.PlanPath != "" {
		loaded, err := loadPlanArtifact(opts.PlanPath)
		if err != nil {
			return nil, err
		}
		artifact = loaded
		opts = reconcileArtifactDefaults(opts, *artifact)
	}
	opts = normalizeOptions(opts)
	if !opts.AutoApprove {
		return newResult(opts.StatePath), fmt.Errorf("destroy requires explicit approval; rerun with --auto-approve after reviewing tracked resources")
	}
	if opts.Executor == nil {
		return nil, fmt.Errorf("destroy requires a trusted executor; pass --mock for recorded/mock execution in public builds")
	}
	dependencies := map[string][]string{}
	var planResult *tfplan.Result
	if artifact != nil {
		if err := validateDestroyArtifact(*artifact); err != nil {
			return newResult(opts.StatePath), err
		}
		current, err := verifyDestroyArtifact(ctx, opts, *artifact)
		if err != nil {
			return newResult(opts.StatePath), err
		}
		for _, resource := range current.Plan.Resources {
			dependencies[resource.Address] = slices.Clone(resource.Dependencies)
		}
		planResult = &tfplan.Result{StatePath: current.StatePath, Plan: *artifact, Diagnostics: artifact.Diagnostics}
	} else {
		depPlan, err := tfplan.Build(ctx, tfplan.Options{ConfigDir: opts.ConfigDir, ProjectPath: opts.ProjectPath, StatePath: opts.StatePath, APISources: opts.APISources, VarFiles: opts.VarFiles, Vars: opts.Vars, Workspace: opts.Workspace, Action: "dependency"})
		if err != nil {
			return nil, err
		}
		if err := rejectPlanExecution(depPlan); err != nil {
			return newResult(opts.StatePath), err
		}
		for _, resource := range depPlan.Plan.Resources {
			dependencies[resource.Address] = slices.Clone(resource.Dependencies)
		}
		deletePlan, err := tfplan.Build(ctx, tfplan.Options{ConfigDir: opts.ConfigDir, ProjectPath: opts.ProjectPath, StatePath: opts.StatePath, APISources: opts.APISources, VarFiles: opts.VarFiles, Vars: opts.Vars, Workspace: opts.Workspace, Action: "delete"})
		if err != nil {
			return nil, err
		}
		planResult = deletePlan
		if err := rejectPlanExecution(planResult); err != nil {
			return newResult(opts.StatePath), err
		}
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
			result.Summary.Failed++
			msg := fmt.Sprintf("destroy.operation_missing: %s has no safe destroy operation role", resource.Address)
			_ = recordFailedAction(ctx, store, runID, resource, "delete_failed", msg)
			return result, fmt.Errorf("%s", msg)
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
		action := executorAction(resource, "delete")
		req := executor.Request{
			RunID:      runID,
			Action:     action,
			Document:   doc,
			WorkingDir: workingDir,
			OutDir:     opts.OutDir,
		}
		req.Capabilities = executor.RequirementsForAction(action)
		req.Idempotency = executor.IdempotencyForAction(action)
		req.Events = tfapply.ExecutorEventSink(store)
		if err := executor.EnsureSupported(opts.Executor, req); err != nil {
			result.Summary.Failed++
			_ = recordFailedAction(ctx, store, runID, resource, "delete_failed", err.Error())
			return result, err
		}
		execResult, err := opts.Executor.Execute(ctx, req)
		result.Feedback = append(result.Feedback, executor.FeedbackFromResult(req, execResult, err))
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
	stopRenewal := store.StartLockRenewal(ctx, "state", lockHolder, 30*time.Minute, 0)
	defer stopRenewal()
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
	if err := store.AttachLockRun(ctx, "state", lockHolder, runID); err != nil {
		return nil, err
	}
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
	return &Result{Version: Version, StatePath: opts.StatePath, RunID: runID, Summary: summary, Actions: []ActionResult{{Address: opts.Address, Action: "import"}}}, nil
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
	allowed := importIdentityFields(*resource)
	if len(allowed) == 0 {
		return fmt.Errorf("import.identity_schema_missing: %s has no importable identity metadata", opts.Address)
	}
	for name, field := range allowed {
		if field.Required {
			if _, ok := opts.Identity[name]; !ok {
				return fmt.Errorf("import.identity_missing: %s requires identity attribute %s", opts.Address, name)
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

type importIdentityField struct {
	Required bool
}

func importIdentityFields(resource project.Resource) map[string]importIdentityField {
	out := map[string]importIdentityField{}
	for _, attr := range resource.IdentityAttributes {
		name := strings.TrimSpace(attr.Name)
		if name == "" {
			continue
		}
		out[name] = importIdentityField{Required: attr.Required}
	}
	for _, schema := range resource.Schema {
		if !schema.Identity && !schema.ResponseDerivedIdentity {
			continue
		}
		name := strings.TrimSpace(schema.Path)
		if name == "" {
			continue
		}
		field := out[name]
		field.Required = field.Required || schema.Required
		out[name] = field
	}
	for _, binding := range resource.ResponseBindings {
		if !binding.Identity && !binding.ResponseDerivedIdentity {
			continue
		}
		name := strings.TrimSpace(binding.StatePath)
		if name == "" {
			continue
		}
		if _, ok := out[name]; !ok {
			out[name] = importIdentityField{}
		}
	}
	return out
}

func validateAPISources(inputs []APISourceInput) []ramenvalidate.APISourceInput {
	out := make([]ramenvalidate.APISourceInput, len(inputs))
	for i, input := range inputs {
		out[i] = ramenvalidate.APISourceInput(input)
	}
	return out
}

func loadPlanArtifact(path string) (*tfplan.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("destroy.plan_read_error: %w", err)
	}
	var doc tfplan.Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("destroy.plan_parse_error: %w", err)
	}
	if doc.Version != tfplan.Version {
		return nil, fmt.Errorf("destroy.plan_version_invalid: got %q, want %q", doc.Version, tfplan.Version)
	}
	return &doc, nil
}

func validateDestroyArtifact(doc tfplan.Document) error {
	if err := tfplan.VerifyApproval(doc); err != nil {
		return fmt.Errorf("destroy.approval_invalid: %w", err)
	}
	if doc.Action != "delete" && !doc.Controls.Destroy {
		return fmt.Errorf("destroy.plan_action_invalid: destroy requires a destroy plan artifact")
	}
	return nil
}

func reconcileArtifactDefaults(opts Options, artifact tfplan.Document) Options {
	if strings.TrimSpace(opts.ConfigDir) == "" {
		opts.ConfigDir = artifact.ConfigDir
	}
	if strings.TrimSpace(opts.ProjectPath) == "" {
		opts.ProjectPath = artifact.ProjectPath
	}
	if strings.TrimSpace(opts.StatePath) == "" {
		opts.StatePath = artifact.StatePath
	}
	if strings.TrimSpace(opts.Workspace) == "" {
		opts.Workspace = artifact.Workspace
	}
	if len(opts.APISources) == 0 {
		opts.APISources = reconcileAPISourceInputsFromPlan(artifact.APISources)
	}
	return opts
}

func reconcileAPISourceInputsFromPlan(refs []tfplan.APISourceRef) []APISourceInput {
	out := make([]APISourceInput, 0, len(refs))
	for _, ref := range refs {
		out = append(out, APISourceInput{Kind: ref.Kind, ID: ref.ID, Path: ref.Path})
	}
	return out
}

func verifyDestroyArtifact(ctx context.Context, opts Options, artifact tfplan.Document) (*tfplan.Result, error) {
	current, err := tfplan.Build(ctx, tfplan.Options{
		ConfigDir:   opts.ConfigDir,
		ProjectPath: opts.ProjectPath,
		StatePath:   opts.StatePath,
		APISources:  opts.APISources,
		VarFiles:    opts.VarFiles,
		Vars:        opts.Vars,
		Workspace:   opts.Workspace,
		Action:      artifact.Action,
		Targets:     artifact.Controls.Targets,
		Excludes:    artifact.Controls.Excludes,
		Replaces:    artifact.Controls.Replaces,
		Destroy:     artifact.Controls.Destroy,
	})
	if err != nil {
		return nil, err
	}
	if err := rejectPlanExecution(current); err != nil {
		return current, err
	}
	if current.Plan.Approval == nil || artifact.Approval == nil || current.Plan.Approval.Digest != artifact.Approval.Digest {
		return current, fmt.Errorf("destroy.approval_mismatch: plan artifact no longer matches current project, API sources, state baseline, or controls")
	}
	return current, nil
}

func refreshReadMappings(ctx context.Context, opts Options) (map[string]*tfplan.MappingPlan, error) {
	if strings.TrimSpace(opts.ProjectPath) == "" {
		return nil, nil
	}
	result, err := ramenvalidate.Run(ctx, ramenvalidate.Options{ProjectPath: opts.ProjectPath, APISources: validateAPISources(opts.APISources)})
	if err != nil {
		return nil, err
	}
	if !result.Valid {
		if len(result.Diagnostics) > 0 {
			diag := result.Diagnostics[0]
			return nil, fmt.Errorf("%s: %s", diag.Code, diag.Message)
		}
		return nil, fmt.Errorf("refresh.project_invalid: native project is invalid")
	}
	doc, err := project.Load(opts.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("refresh.project_load_error: %w", err)
	}
	out := map[string]*tfplan.MappingPlan{}
	for _, resource := range doc.Profile.Resources {
		if resource.Kind != "resource" {
			continue
		}
		role, ok := resource.Operations["read"]
		if !ok || strings.TrimSpace(role.OperationID) == "" {
			continue
		}
		source := project.SourceForRole(doc.Profile, role)
		out[resource.Address] = &tfplan.MappingPlan{
			Purpose:            "read",
			SourceKind:         firstNonEmpty(role.SourceKind, source.Kind),
			SourceID:           firstNonEmpty(role.SourceID, source.ID),
			SourcePath:         firstNonEmpty(role.SourcePath, source.Path),
			OperationID:        role.OperationID,
			IdentityAttributes: reconcileProjectIdentityAttributes(resource.IdentityAttributes),
			ResponseBindings:   slices.Clone(resource.ResponseBindings),
			Normalizers:        slices.Clone(resource.Normalizers),
			MappingLifecycle:   cloneProjectMappingLifecycle(resource.MappingLifecycle),
		}
	}
	return out, nil
}

func reconcileProjectIdentityAttributes(attrs []project.IdentityAttribute) []tfmapping.IdentityAttribute {
	out := make([]tfmapping.IdentityAttribute, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, tfmapping.IdentityAttribute{
			Name:          attr.Name,
			TerraformPath: attr.Path,
			RequestKeys:   slices.Clone(attr.RequestKeys),
			ResponsePaths: slices.Clone(attr.ResponsePaths),
			Required:      attr.Required,
		})
	}
	return out
}

func cloneProjectMappingLifecycle(lifecycle *project.MappingLifecycle) *project.MappingLifecycle {
	if lifecycle == nil {
		return nil
	}
	return &project.MappingLifecycle{
		OperationRoles: slices.Clone(lifecycle.OperationRoles),
		Paths:          slices.Clone(lifecycle.Paths),
	}
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
		VarFiles:    opts.VarFiles,
		Vars:        opts.Vars,
		Workspace:   opts.Workspace,
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
		return newResult(statePath), nil, 0, nil, err
	}
	lockHolder := fmt.Sprintf("%s-%d", command, time.Now().UTC().UnixNano())
	if err := store.AcquireLock(ctx, "state", lockHolder, 30*time.Minute); err != nil {
		_ = store.Close()
		return newResult(statePath), nil, 0, nil, err
	}
	stopRenewal := store.StartLockRenewal(ctx, "state", lockHolder, 30*time.Minute, 0)
	runID, err := store.StartRun(ctx, command)
	if err != nil {
		stopRenewal()
		_ = store.ReleaseLock(ctx, "state", lockHolder)
		_ = store.Close()
		return newResult(statePath), nil, 0, nil, err
	}
	if err := store.AttachLockRun(ctx, "state", lockHolder, runID); err != nil {
		stopRenewal()
		_ = store.FinishRun(context.Background(), runID, "failed", "")
		_ = store.ReleaseLock(ctx, "state", lockHolder)
		_ = store.Close()
		return newResult(statePath), nil, 0, nil, err
	}
	result := &Result{Version: Version, StatePath: statePath, RunID: runID}
	finish := func(summary *Summary) {
		status := "completed"
		if summary != nil && summary.Failed > 0 {
			status = "failed"
		}
		data, _ := json.Marshal(summary)
		_ = store.FinishRun(context.Background(), runID, status, string(data))
		stopRenewal()
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
		if len(resource.Mapping.IdentityAttributes) > 0 {
			attrs, err := json.Marshal(resource.Mapping.IdentityAttributes)
			if err == nil {
				out.Metadata = map[string]string{"identity_attributes": string(attrs)}
			}
		}
	}
	return out
}

func recordRefresh(ctx context.Context, store *state.Store, runID int64, resource tfplan.ResourcePlan, execResult executor.Result) (bool, error) {
	current, err := store.CurrentResource(ctx, resource.Address)
	if err != nil {
		return false, err
	}
	if current == nil {
		return false, nil
	}
	identity, computed := projectedRefreshState(resource.Mapping, execResult)
	changed := false
	if len(identity) > 0 {
		data, _ := json.Marshal(redact.Map(identity))
		if current.IdentityJSON != string(data) {
			changed = true
		}
		current.IdentityJSON = string(data)
	}
	if len(computed) > 0 {
		data, _ := json.Marshal(redact.Map(computed))
		if current.AttributesJSON != string(data) {
			changed = true
		}
		current.AttributesJSON = string(data)
	}
	current.UpdatedRunID = runID
	current.UpdatedAt = time.Now().UTC()
	afterJSON, _ := json.Marshal(current)
	err = store.WithTx(ctx, func(tx *state.Tx) error {
		if err := tx.RecordResource(ctx, *current); err != nil {
			return err
		}
		return tx.RecordRevision(ctx, state.Revision{ResourceAddress: resource.Address, RunID: runID, Action: "refresh", AfterJSON: string(afterJSON)})
	})
	return changed, err
}

func projectedRefreshState(mapping *tfplan.MappingPlan, execResult executor.Result) (map[string]any, map[string]any) {
	identity := cloneAnyMap(execResult.Identity)
	computed := cloneAnyMap(execResult.Computed)
	if mapping == nil {
		return identity, computed
	}
	for _, binding := range mapping.ResponseBindings {
		value, ok := responseBindingValue(binding, execResult)
		if !ok {
			continue
		}
		if binding.ResponsePath != binding.StatePath {
			deleteDottedAny(identity, binding.ResponsePath)
			deleteDottedAny(computed, binding.ResponsePath)
		}
		if binding.Identity || binding.ResponseDerivedIdentity {
			setDottedAny(identity, binding.StatePath, value)
		}
		if binding.Computed || binding.Observed {
			setDottedAny(computed, binding.StatePath, value)
		}
		if binding.Sensitive {
			if binding.Identity || binding.ResponseDerivedIdentity {
				setDottedAny(identity, binding.StatePath, "${redacted}")
			}
			if binding.Computed || binding.Observed {
				setDottedAny(computed, binding.StatePath, "${redacted}")
			}
		}
	}
	applyRefreshNormalizers(identity, mapping.Normalizers)
	applyRefreshNormalizers(computed, mapping.Normalizers)
	return identity, computed
}

func responseBindingValue(binding project.ResponseBinding, execResult executor.Result) (any, bool) {
	for _, values := range []map[string]any{execResult.Identity, execResult.Computed} {
		if value, ok := dottedAny(values, binding.ResponsePath); ok {
			return value, true
		}
		if value, ok := dottedAny(values, binding.StatePath); ok {
			return value, true
		}
	}
	return nil, false
}

func cloneAnyMap(values map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func dottedAny(values map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	var cur any = values
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func setDottedAny(values map[string]any, path string, value any) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || parts[0] == "" {
		return
	}
	cur := values
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[part] = next
		}
		cur = next
	}
	cur[parts[len(parts)-1]] = value
}

func deleteDottedAny(values map[string]any, path string) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || parts[0] == "" {
		return
	}
	cur := values
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]any)
		if !ok {
			return
		}
		cur = next
	}
	delete(cur, parts[len(parts)-1])
}

func applyRefreshNormalizers(values map[string]any, normalizers []project.Normalizer) {
	for _, normalizer := range normalizers {
		value, ok := dottedAny(values, normalizer.Path)
		if !ok {
			continue
		}
		normalized, keep := normalizeRefreshValue(value, normalizer.Kind)
		if keep {
			setDottedAny(values, normalizer.Path, normalized)
		} else {
			deleteDottedAny(values, normalizer.Path)
		}
	}
}

func normalizeRefreshValue(value any, kind string) (any, bool) {
	switch strings.TrimSpace(kind) {
	case "json_semantic":
		text, ok := value.(string)
		if !ok {
			return value, true
		}
		var decoded any
		if err := json.Unmarshal([]byte(text), &decoded); err != nil {
			return value, true
		}
		data, err := json.Marshal(decoded)
		if err != nil {
			return value, true
		}
		return string(data), true
	case "case_fold":
		text, ok := value.(string)
		if !ok {
			return value, true
		}
		return strings.ToLower(text), true
	case "unordered_collection":
		values, ok := value.([]any)
		if !ok {
			return value, true
		}
		out := slices.Clone(values)
		slices.SortFunc(out, func(a, b any) int {
			left, _ := json.Marshal(a)
			right, _ := json.Marshal(b)
			return strings.Compare(string(left), string(right))
		})
		return out, true
	case "empty_null_absent_equivalent":
		if value == nil {
			return nil, false
		}
		switch v := value.(type) {
		case string:
			return v, strings.TrimSpace(v) != ""
		case []any:
			return v, len(v) > 0
		case map[string]any:
			return v, len(v) > 0
		default:
			return value, true
		}
	case "sensitive_placeholder":
		return "${redacted}", true
	default:
		return value, true
	}
}

func recordMissingRefresh(ctx context.Context, store *state.Store, runID int64, resource tfplan.ResourcePlan) error {
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
		"status":       "missing",
		"desired_hash": resource.DesiredHash,
	})
	if err != nil {
		return err
	}
	return store.RecordRevision(ctx, state.Revision{ResourceAddress: resource.Address, RunID: runID, Action: "refresh_missing", BeforeJSON: string(beforeJSON), DiffJSON: string(diffJSON)})
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
