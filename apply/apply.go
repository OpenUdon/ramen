package apply

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

	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/internal/requestbinding"
	"github.com/OpenUdon/ramen/internal/stateprojection"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/state"
	"github.com/OpenUdon/ramen/tfmapping"
	"github.com/OpenUdon/tfconfig"
	"github.com/OpenUdon/uws/uws1"
)

const Version = "ramen.apply.v1"

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
	Version            string                    `json:"version"`
	StatePath          string                    `json:"state_path"`
	RunID              int64                     `json:"run_id,omitempty"`
	Plan               tfplan.Document           `json:"plan"`
	Summary            Summary                   `json:"summary"`
	Executed           []ExecutedAction          `json:"executed,omitempty"`
	Feedback           []executor.FeedbackRecord `json:"feedback,omitempty"`
	GeneratedDocuments []string                  `json:"generated_documents,omitempty"`
	Errors             []string                  `json:"errors,omitempty"`
}

type Summary struct {
	Create  int `json:"create"`
	Update  int `json:"update"`
	Delete  int `json:"delete"`
	NoOp    int `json:"no_op"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
	Blocked int `json:"blocked"`
}

type ExecutedAction struct {
	Address   string          `json:"address"`
	Action    string          `json:"action"`
	Operation string          `json:"operation,omitempty"`
	Document  string          `json:"document,omitempty"`
	Result    executor.Result `json:"result"`
}

func Apply(ctx context.Context, opts Options) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opts.PlanPath = strings.TrimSpace(opts.PlanPath)
	var artifact *tfplan.Document
	if opts.PlanPath != "" {
		loaded, err := loadPlanArtifact(opts.PlanPath)
		if err != nil {
			return nil, err
		}
		artifact = loaded
		opts = applyArtifactDefaults(opts, *artifact)
	}
	opts = normalizeOptions(opts)
	var planResult *tfplan.Result
	var err error
	if artifact != nil {
		if err := validateLoadedPlanArtifact(*artifact); err != nil {
			return &Result{Version: Version, StatePath: opts.StatePath, Plan: *artifact}, err
		}
		planResult, err = verifyPlanArtifact(ctx, opts, *artifact)
		if err != nil {
			return &Result{Version: Version, StatePath: opts.StatePath, Plan: *artifact}, err
		}
	} else {
		planResult, err = tfplan.Build(ctx, tfplan.Options{
			ConfigDir:   opts.ConfigDir,
			ProjectPath: opts.ProjectPath,
			StatePath:   opts.StatePath,
			APISources:  opts.APISources,
			VarFiles:    opts.VarFiles,
			Vars:        opts.Vars,
			Workspace:   opts.Workspace,
			Action:      "create",
		})
		if err != nil {
			return nil, err
		}
	}
	result := &Result{Version: Version, StatePath: opts.StatePath, Plan: planResult.Plan}
	if err := rejectErroredPlan(planResult); err != nil {
		return result, err
	}
	mutations := mutableResources(planResult.Plan.Resources)
	if len(mutations) == 0 {
		result.Summary.NoOp = planResult.Plan.Summary.NoOp
		result.Summary.Skipped = len(planResult.Plan.Resources)
		return result, nil
	}
	if !opts.AutoApprove {
		return result, fmt.Errorf("apply.approval_required: apply requires explicit approval for %d mutation(s); rerun with --auto-approve after reviewing the plan", len(mutations))
	}
	if opts.Executor == nil {
		return result, fmt.Errorf("apply.executor_required: apply requires a trusted executor; pass --mock for recorded/mock execution in public builds")
	}

	store, err := state.Open(ctx, opts.StatePath)
	if err != nil {
		return result, err
	}
	defer store.Close()
	lockHolder := fmt.Sprintf("apply-%d", time.Now().UTC().UnixNano())
	if err := store.AcquireLock(ctx, "state", lockHolder, 30*time.Minute); err != nil {
		return result, err
	}
	defer func() { _ = store.ReleaseLock(context.Background(), "state", lockHolder) }()
	stopRenewal := store.StartLockRenewal(ctx, "state", lockHolder, 30*time.Minute, 0)
	defer stopRenewal()
	runID, err := store.StartRun(ctx, "apply")
	if err != nil {
		return result, err
	}
	result.RunID = runID
	runStatus := "completed"
	defer func() {
		summary, _ := json.Marshal(result.Summary)
		_ = store.FinishRun(context.Background(), runID, runStatus, string(summary))
	}()
	if err := store.AttachLockRun(ctx, "state", lockHolder, runID); err != nil {
		runStatus = "failed"
		return result, err
	}

	sourcePaths := sourcePathIndex(opts.APISources)
	attrsByAddress := loadResourceAttributes(opts.ConfigDir, opts.ProjectPath, opts.VarFiles, opts.Vars)
	readMappings := applyReadMappings(opts.ProjectPath)
	workingDir := opts.ConfigDir
	if opts.ProjectPath != "" {
		workingDir = stateBaseDir(opts.ProjectPath, opts.ConfigDir)
	}
	failed := map[string]bool{}
	for _, resource := range mutations {
		if blockedByFailure(resource, failed) {
			runStatus = "failed"
			result.Summary.Blocked++
			failed[resource.Address] = true
			msg := fmt.Sprintf("blocked %s because a dependency failed", resource.Address)
			result.Errors = append(result.Errors, msg)
			if err := recordFailedMutation(ctx, store, runID, resource, "blocked", msg); err != nil {
				return result, err
			}
			continue
		}
		if resource.Action == "delete" {
			runStatus = "failed"
			result.Summary.Failed++
			failed[resource.Address] = true
			msg := fmt.Sprintf("apply.delete_unsupported: apply delete for %s is handled by ramen destroy", resource.Address)
			result.Errors = append(result.Errors, msg)
			if err := recordFailedMutation(ctx, store, runID, resource, "failed", msg); err != nil {
				return result, err
			}
			continue
		}
		readMapping := readMappings[resource.Address]
		if readMapping != nil {
			baseline, err := executeReadCheck(ctx, opts.Executor, store, runID, resource, readMapping, sourcePaths, attrsByAddress[resource.Address], workingDir, opts.OutDir, "baseline")
			result.Feedback = append(result.Feedback, baseline.Feedback...)
			if err != nil {
				runStatus = "failed"
				result.Summary.Failed++
				failed[resource.Address] = true
				msg := fmt.Sprintf("apply.baseline_failed: %v", err)
				result.Errors = append(result.Errors, msg)
				if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", msg); recErr != nil {
					return result, recErr
				}
				continue
			}
			if err := validateReadBeforeWrite(ctx, store, resource, readMapping, baseline.Result); err != nil {
				runStatus = "failed"
				result.Summary.Failed++
				failed[resource.Address] = true
				msg := err.Error()
				result.Errors = append(result.Errors, msg)
				if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", msg); recErr != nil {
					return result, recErr
				}
				continue
			}
		}
		doc, err := buildActionDocument(resource, sourcePaths, attrsByAddress[resource.Address], nil)
		if err != nil {
			runStatus = "failed"
			result.Summary.Failed++
			failed[resource.Address] = true
			msg := fmt.Sprintf("apply.document_invalid: %v", err)
			result.Errors = append(result.Errors, msg)
			if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", msg); recErr != nil {
				return result, recErr
			}
			continue
		}
		docPath := ""
		if opts.OutDir != "" {
			docPath, err = writeActionDocument(opts.OutDir, resource.Address, doc)
			if err != nil {
				runStatus = "failed"
				return result, err
			}
			result.GeneratedDocuments = append(result.GeneratedDocuments, docPath)
		}
		action := executorAction(resource)
		req := executor.Request{
			RunID:      runID,
			Action:     action,
			Document:   doc,
			WorkingDir: workingDir,
			OutDir:     opts.OutDir,
		}
		req.Capabilities = executor.RequirementsForAction(action)
		req.Idempotency = executor.IdempotencyForAction(action)
		req.Events = ExecutorEventSink(store)
		if err := executor.EnsureSupported(opts.Executor, req); err != nil {
			runStatus = "failed"
			result.Summary.Failed++
			failed[resource.Address] = true
			msg := fmt.Sprintf("apply.executor_unsupported: %v", err)
			result.Errors = append(result.Errors, msg)
			if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", msg); recErr != nil {
				return result, recErr
			}
			continue
		}
		before, err := store.CurrentResource(ctx, resource.Address)
		if err != nil {
			runStatus = "failed"
			return result, err
		}
		execResult, err := opts.Executor.Execute(ctx, req)
		result.Feedback = append(result.Feedback, executor.FeedbackFromResult(req, execResult, err))
		if err != nil {
			runStatus = "failed"
			result.Summary.Failed++
			failed[resource.Address] = true
			msg := fmt.Sprintf("apply.executor_failed: %v", err)
			result.Errors = append(result.Errors, msg)
			if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", msg); recErr != nil {
				return result, recErr
			}
			continue
		}
		if !execResult.Success {
			runStatus = "failed"
			result.Summary.Failed++
			failed[resource.Address] = true
			msg := fmt.Sprintf("apply.executor_unsuccessful: executor reported unsuccessful %s for %s", resource.Action, resource.Address)
			result.Errors = append(result.Errors, msg)
			if err := recordFailedMutation(ctx, store, runID, resource, "failed", msg); err != nil {
				return result, err
			}
			continue
		}
		stateResult := execResult
		stateMapping := (*tfplan.MappingPlan)(nil)
		if readMapping != nil {
			converged, err := executeReadCheck(ctx, opts.Executor, store, runID, resource, readMapping, sourcePaths, attrsByAddress[resource.Address], workingDir, opts.OutDir, "convergence")
			result.Feedback = append(result.Feedback, converged.Feedback...)
			if err != nil {
				runStatus = "failed"
				result.Summary.Failed++
				failed[resource.Address] = true
				msg := fmt.Sprintf("apply.convergence_failed: %v", err)
				result.Errors = append(result.Errors, msg)
				if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", msg); recErr != nil {
					return result, recErr
				}
				continue
			}
			if converged.Result.Missing {
				runStatus = "failed"
				result.Summary.Failed++
				failed[resource.Address] = true
				msg := fmt.Sprintf("apply.convergence_missing: read-after-write reported missing for %s", resource.Address)
				result.Errors = append(result.Errors, msg)
				if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", msg); recErr != nil {
					return result, recErr
				}
				continue
			}
			stateResult = converged.Result
			stateMapping = readMapping
		}
		if err := recordSuccessfulMutation(ctx, store, runID, resource, before, stateResult, stateMapping); err != nil {
			runStatus = "failed"
			return result, err
		}
		result.Executed = append(result.Executed, ExecutedAction{
			Address:   resource.Address,
			Action:    resource.Action,
			Operation: resource.Mapping.OperationID,
			Document:  docPath,
			Result:    redactExecutorResult(execResult),
		})
		switch resource.Action {
		case "create":
			result.Summary.Create++
		case "update":
			result.Summary.Update++
		}
	}
	result.Summary.NoOp = planResult.Plan.Summary.NoOp
	result.Summary.Skipped = len(planResult.Plan.Resources) - len(mutations)
	if len(result.Errors) > 0 {
		return result, fmt.Errorf("apply.failed: apply failed for %d resource(s) and blocked %d resource(s)", result.Summary.Failed, result.Summary.Blocked)
	}
	return result, nil
}

func ExecutorEventSink(store *state.Store) executor.EventSink {
	return func(event executor.Event) {
		if store == nil {
			return
		}
		metadataJSON := ""
		if len(event.Metadata) > 0 {
			data, err := json.Marshal(redactMap(event.Metadata))
			if err == nil {
				metadataJSON = string(data)
			}
		}
		_ = store.RecordRunEvent(context.Background(), state.RunEvent{
			RunID:           event.RunID,
			ResourceAddress: event.Address,
			Action:          event.Action,
			OperationID:     event.Operation,
			Phase:           event.Phase,
			Message:         redactString(event.Message),
			MetadataJSON:    metadataJSON,
			CreatedAt:       event.Time,
		})
	}
}

func BuildActionDocument(resource tfplan.ResourcePlan, sourcePaths map[string]string) (*uws1.Document, error) {
	return buildActionDocument(resource, sourcePaths, nil, nil)
}

func BuildActionDocumentWithBindings(resource tfplan.ResourcePlan, sourcePaths map[string]string, attrs, identity map[string]any) (*uws1.Document, error) {
	return buildActionDocument(resource, sourcePaths, attrs, identity)
}

func buildActionDocument(resource tfplan.ResourcePlan, sourcePaths map[string]string, attrs, identity map[string]any) (*uws1.Document, error) {
	if resource.Mapping == nil {
		return nil, fmt.Errorf("resource %s has no API source mapping", resource.Address)
	}
	if strings.TrimSpace(resource.Mapping.OperationID) == "" {
		return nil, fmt.Errorf("resource %s has no API source operation", resource.Address)
	}
	sourceName := normalizeName(firstNonEmpty(resource.Mapping.SourceID, resource.Mapping.SourceKind, "api_source"))
	operationID := normalizeName(resource.Address + "_" + resource.Action)
	if operationID == "" {
		operationID = "apply_action"
	}
	sourcePath := firstNonEmpty(resource.Mapping.SourcePath, sourcePaths[sourcePathKey(resource.Mapping.SourceKind, resource.Mapping.SourceID)], resource.Mapping.SourceID)
	request := requestbinding.Build(requestbinding.Options{
		Object:      tfmapping.Object{Kind: resource.Kind, Type: resource.Type, Provider: resource.Provider},
		SourceKind:  resource.Mapping.SourceKind,
		SourceID:    resource.Mapping.SourceID,
		SourcePath:  sourcePath,
		OperationID: resource.Mapping.OperationID,
		Attributes:  attrs,
		Identity:    identity,
		Identities:  resource.Mapping.IdentityAttributes,
		Extension:   "x-ramen-apply",
		Metadata: map[string]any{
			"address":      resource.Address,
			"type":         resource.Type,
			"provider":     resource.Provider,
			"action":       resource.Action,
			"desired_hash": resource.DesiredHash,
		},
	})
	stepBody := executorStepBody(request)
	stepBody["ramen_address"] = resource.Address
	stepBody["ramen_action"] = resource.Action
	doc := &uws1.Document{
		UWS: "1.4.0",
		Info: &uws1.Info{
			Title:       "ramen_apply_action",
			Description: "Executor-ready Ramen apply action generated from an approved desired-state plan.",
			Version:     "1.0.0",
		},
		SourceDescriptions: []*uws1.SourceDescription{{
			Name: sourceName,
			URL:  filepath.ToSlash(sourcePath),
			Type: sourceDescriptionType(resource.Mapping.SourceKind),
		}},
		Operations: []*uws1.Operation{{
			OperationID:       operationID,
			SourceDescription: sourceName,
			SourceOperationID: resource.Mapping.OperationID,
			Description:       fmt.Sprintf("Apply %s for %s", resource.Action, resource.Address),
			Request:           request,
			Outputs: map[string]string{
				"body": "$response.body",
			},
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID:  "main",
			Type:        uws1.WorkflowTypeSequence,
			Description: "Execute one approved Ramen apply action.",
			Steps: []*uws1.Step{{
				StepID:       operationID,
				OperationRef: operationID,
				Body:         stepBody,
			}},
		}},
		Extensions: map[string]any{
			"x-ramen-plan-version": tfplan.Version,
			"x-ramen-executor": map[string]any{
				"idempotency": executor.IdempotencyForAction(executorAction(resource)),
			},
		},
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

func executorStepBody(request map[string]any) map[string]any {
	out := map[string]any{}
	for _, component := range []string{"path", "query", "header", "cookie", "body"} {
		values, _ := request[component].(map[string]any)
		for key, value := range cloneRequestMap(values) {
			out[key] = value
		}
	}
	return out
}

func cloneRequestMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	data, err := json.Marshal(in)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func loadResourceAttributes(configDir, projectPath string, varFiles, vars []string) map[string]map[string]any {
	if strings.TrimSpace(projectPath) != "" {
		proj, err := project.Load(projectPath)
		if err != nil {
			return nil
		}
		profile, _, diags := project.ResolveProfile(proj.Profile, proj.Dir, project.ValuesOptions{VarFiles: varFiles, Vars: vars})
		if len(diags) > 0 {
			return nil
		}
		proj.Profile = profile
		out := map[string]map[string]any{}
		for _, resource := range proj.Profile.Resources {
			if len(resource.Attributes) > 0 {
				out[resource.Address] = resource.Attributes
			}
		}
		return out
	}
	doc, err := tfconfig.LoadDir(configDir)
	if err != nil {
		return nil
	}
	out := map[string]map[string]any{}
	for _, mod := range doc.Modules {
		for _, res := range mod.Resources {
			attrs := map[string]any{}
			for _, attr := range res.Config {
				if strings.TrimSpace(attr.Path) == "" {
					continue
				}
				attrs[attr.Path] = valueAny(attr.Value)
			}
			out[fullAddress(mod.Address, res.Address)] = attrs
		}
	}
	return out
}

func valueAny(value tfconfig.Value) any {
	if value.Sensitive || value.Redacted || value.SensitiveCandidate != nil {
		return redactedValue
	}
	if value.Expression != "" {
		return value.Expression
	}
	if value.UnknownReason != "" {
		return "${unknown:" + value.UnknownReason + "}"
	}
	if value.Literal != nil {
		return value.Literal
	}
	return string(value.Kind)
}

func fullAddress(moduleAddress, objectAddress string) string {
	moduleAddress = strings.TrimSpace(moduleAddress)
	objectAddress = strings.TrimSpace(objectAddress)
	if moduleAddress == "" {
		return objectAddress
	}
	if objectAddress == "" {
		return moduleAddress
	}
	return moduleAddress + "." + objectAddress
}

func normalizeOptions(opts Options) Options {
	if strings.TrimSpace(opts.ConfigDir) == "" {
		opts.ConfigDir = "."
	}
	if strings.TrimSpace(opts.StatePath) == "" {
		opts.StatePath = state.DefaultPath(stateBaseDir(opts.ProjectPath, opts.ConfigDir))
	}
	opts.ProjectPath = strings.TrimSpace(opts.ProjectPath)
	for i := range opts.APISources {
		opts.APISources[i].Kind = strings.TrimSpace(opts.APISources[i].Kind)
		opts.APISources[i].ID = strings.TrimSpace(opts.APISources[i].ID)
		opts.APISources[i].Path = strings.TrimSpace(opts.APISources[i].Path)
	}
	slices.SortFunc(opts.APISources, func(a, b APISourceInput) int {
		left := a.Kind + "\x00" + a.ID + "\x00" + a.Path
		right := b.Kind + "\x00" + b.ID + "\x00" + b.Path
		return cmp.Compare(left, right)
	})
	return opts
}

func loadPlanArtifact(path string) (*tfplan.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("apply.plan_read_error: %w", err)
	}
	var doc tfplan.Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("apply.plan_parse_error: %w", err)
	}
	if doc.Version != tfplan.Version {
		return nil, fmt.Errorf("apply.plan_version_invalid: got %q, want %q", doc.Version, tfplan.Version)
	}
	return &doc, nil
}

func validateLoadedPlanArtifact(doc tfplan.Document) error {
	if err := tfplan.VerifyApproval(doc); err != nil {
		return fmt.Errorf("apply.approval_invalid: %w", err)
	}
	if doc.Action == "delete" || doc.Controls.Destroy {
		return fmt.Errorf("apply.plan_action_invalid: apply requires a non-destroy plan artifact")
	}
	return nil
}

func applyArtifactDefaults(opts Options, artifact tfplan.Document) Options {
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
		opts.APISources = apiSourceInputsFromPlan(artifact.APISources)
	}
	return opts
}

func apiSourceInputsFromPlan(refs []tfplan.APISourceRef) []APISourceInput {
	out := make([]APISourceInput, 0, len(refs))
	for _, ref := range refs {
		out = append(out, APISourceInput{Kind: ref.Kind, ID: ref.ID, Path: ref.Path})
	}
	return out
}

func verifyPlanArtifact(ctx context.Context, opts Options, artifact tfplan.Document) (*tfplan.Result, error) {
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
	if err := rejectErroredPlan(current); err != nil {
		return current, err
	}
	if current.Plan.Approval == nil || artifact.Approval == nil || current.Plan.Approval.Digest != artifact.Approval.Digest {
		return current, fmt.Errorf("apply.approval_mismatch: plan artifact no longer matches current project, API sources, state baseline, or controls")
	}
	return &tfplan.Result{StatePath: current.StatePath, OutPath: current.OutPath, Plan: artifact, Diagnostics: artifact.Diagnostics}, nil
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

func rejectErrorDiagnostics(diags []tfplan.Diagnostic) error {
	for _, diag := range diags {
		if diag.Severity == "error" {
			return fmt.Errorf("plan has error diagnostic %s: %s", diag.Code, diag.Message)
		}
	}
	return nil
}

func rejectErroredPlan(planResult *tfplan.Result) error {
	if planResult != nil && planResult.Plan.Errored {
		return fmt.Errorf("plan is marked errored and cannot be executed")
	}
	if planResult == nil {
		return nil
	}
	if err := rejectErrorDiagnostics(planResult.Diagnostics); err != nil {
		return err
	}
	return tfplan.VerifyApproval(planResult.Plan)
}

func mutableResources(resources []tfplan.ResourcePlan) []tfplan.ResourcePlan {
	var out []tfplan.ResourcePlan
	for _, resource := range resources {
		switch resource.Action {
		case "create", "update", "delete":
			out = append(out, resource)
		}
	}
	return out
}

func blockedByFailure(resource tfplan.ResourcePlan, failed map[string]bool) bool {
	for _, dependency := range resource.Dependencies {
		if failed[dependency] {
			return true
		}
	}
	return false
}

func executorAction(resource tfplan.ResourcePlan) executor.Action {
	action := executor.Action{
		Address:     resource.Address,
		Type:        resource.Type,
		Provider:    resource.Provider,
		Action:      resource.Action,
		DesiredHash: resource.DesiredHash,
	}
	if resource.Mapping != nil {
		action.Mapping = executor.ActionMapping{
			SourceKind:  resource.Mapping.SourceKind,
			SourceID:    resource.Mapping.SourceID,
			SourcePath:  resource.Mapping.SourcePath,
			OperationID: resource.Mapping.OperationID,
		}
		if len(resource.Mapping.IdentityAttributes) > 0 {
			attrs, err := json.Marshal(resource.Mapping.IdentityAttributes)
			if err == nil {
				action.Metadata = map[string]string{"identity_attributes": string(attrs)}
			}
		}
	}
	return action
}

type readCheckResult struct {
	Result   executor.Result
	Feedback []executor.FeedbackRecord
}

func executeReadCheck(ctx context.Context, exec executor.Executor, store *state.Store, runID int64, resource tfplan.ResourcePlan, readMapping *tfplan.MappingPlan, sourcePaths map[string]string, attrs map[string]any, workingDir, outDir, phase string) (readCheckResult, error) {
	readResource := resource
	readResource.Action = "read"
	readResource.Mapping = readMapping
	identity, err := currentIdentity(ctx, store, resource.Address)
	if err != nil {
		return readCheckResult{}, err
	}
	doc, err := buildActionDocument(readResource, sourcePaths, attrs, identity)
	if err != nil {
		return readCheckResult{}, err
	}
	action := executorAction(readResource)
	req := executor.Request{
		RunID:      runID,
		Action:     action,
		Document:   doc,
		WorkingDir: workingDir,
		OutDir:     outDir,
	}
	req.Capabilities = executor.RequirementsForAction(action)
	req.Idempotency = executor.IdempotencyForAction(action)
	if phase != "" {
		req.Idempotency.Key += "-" + phase
	}
	req.Events = ExecutorEventSink(store)
	if err := executor.EnsureSupported(exec, req); err != nil {
		return readCheckResult{}, err
	}
	execResult, err := exec.Execute(ctx, req)
	execResult, err = stateprojection.ClassifyReadResult(execResult, err)
	feedback := executor.FeedbackFromResult(req, execResult, err)
	if err != nil {
		return readCheckResult{Result: execResult, Feedback: []executor.FeedbackRecord{feedback}}, err
	}
	if !execResult.Success {
		return readCheckResult{Result: execResult, Feedback: []executor.FeedbackRecord{feedback}}, fmt.Errorf("executor reported unsuccessful read for %s", resource.Address)
	}
	return readCheckResult{Result: execResult, Feedback: []executor.FeedbackRecord{feedback}}, nil
}

func validateReadBeforeWrite(ctx context.Context, store *state.Store, resource tfplan.ResourcePlan, readMapping *tfplan.MappingPlan, execResult executor.Result) error {
	switch resource.Action {
	case "create":
		if !execResult.Missing {
			return fmt.Errorf("apply.baseline_exists: read-before-write found existing remote object for planned create %s", resource.Address)
		}
	case "update":
		if execResult.Missing {
			return fmt.Errorf("apply.baseline_missing: read-before-write reported missing remote object for planned update %s", resource.Address)
		}
		changed, err := readResultDiffersFromState(ctx, store, resource.Address, readMapping, execResult)
		if err != nil {
			return err
		}
		if changed {
			return fmt.Errorf("apply.baseline_drift: read-before-write drift changed the approved baseline for %s", resource.Address)
		}
	}
	return nil
}

func readResultDiffersFromState(ctx context.Context, store *state.Store, address string, mapping *tfplan.MappingPlan, execResult executor.Result) (bool, error) {
	current, err := store.CurrentResource(ctx, address)
	if err != nil {
		return false, err
	}
	if current == nil {
		return true, nil
	}
	identity, computed := stateprojection.Project(mapping, execResult)
	if len(identity) > 0 {
		data, _ := json.Marshal(redactMap(identity))
		if current.IdentityJSON != string(data) {
			return true, nil
		}
	}
	if len(computed) > 0 {
		data, _ := json.Marshal(redactMap(computed))
		if current.AttributesJSON != string(data) {
			return true, nil
		}
	}
	return false, nil
}

func currentIdentity(ctx context.Context, store *state.Store, address string) (map[string]any, error) {
	current, err := store.CurrentResource(ctx, address)
	if err != nil || current == nil || strings.TrimSpace(current.IdentityJSON) == "" {
		return nil, err
	}
	var identity map[string]any
	if err := json.Unmarshal([]byte(current.IdentityJSON), &identity); err != nil {
		return nil, err
	}
	return identity, nil
}

func applyReadMappings(projectPath string) map[string]*tfplan.MappingPlan {
	if strings.TrimSpace(projectPath) == "" {
		return nil
	}
	doc, err := project.Load(projectPath)
	if err != nil {
		return nil
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
			IdentityAttributes: projectIdentityAttributes(resource.IdentityAttributes),
			ResponseBindings:   slices.Clone(resource.ResponseBindings),
			Normalizers:        slices.Clone(resource.Normalizers),
			MappingLifecycle:   cloneMappingLifecycle(resource.MappingLifecycle),
			RequiredOperations: slices.Clone(resource.RequiredOperations),
		}
	}
	return out
}

func projectIdentityAttributes(attrs []project.IdentityAttribute) []tfmapping.IdentityAttribute {
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

func cloneMappingLifecycle(lifecycle *project.MappingLifecycle) *project.MappingLifecycle {
	if lifecycle == nil {
		return nil
	}
	return &project.MappingLifecycle{
		OperationRoles: slices.Clone(lifecycle.OperationRoles),
		Paths:          slices.Clone(lifecycle.Paths),
	}
}

func recordSuccessfulMutation(ctx context.Context, store *state.Store, runID int64, resource tfplan.ResourcePlan, before *state.ResourceSnapshot, execResult executor.Result, projection *tfplan.MappingPlan) error {
	redacted := redactExecutorResult(execResult)
	if projection != nil {
		identity, computed := stateprojection.Project(projection, execResult)
		redacted.Identity = redactMap(identity)
		redacted.Computed = redactMap(computed)
	}
	identityJSON, err := marshalMap(redacted.Identity)
	if err != nil {
		return err
	}
	attributesJSON, err := marshalMap(redacted.Computed)
	if err != nil {
		return err
	}
	snap := state.ResourceSnapshot{
		Address:        resource.Address,
		Type:           resource.Type,
		Provider:       resource.Provider,
		DesiredHash:    resource.DesiredHash,
		IdentityJSON:   identityJSON,
		AttributesJSON: attributesJSON,
		Status:         "managed",
		UpdatedRunID:   runID,
		UpdatedAt:      time.Now().UTC(),
	}
	if resource.Mapping != nil {
		snap.SourceKind = resource.Mapping.SourceKind
		snap.SourceID = resource.Mapping.SourceID
		snap.OperationID = resource.Mapping.OperationID
	}
	afterJSON, err := json.Marshal(snap)
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
		"desired_hash": resource.DesiredHash,
		"operation":    snap.OperationID,
	})
	if err != nil {
		return err
	}
	return store.WithTx(ctx, func(tx *state.Tx) error {
		if err := tx.RecordResource(ctx, snap); err != nil {
			return err
		}
		return tx.RecordRevision(ctx, state.Revision{
			ResourceAddress: resource.Address,
			RunID:           runID,
			Action:          resource.Action,
			BeforeJSON:      string(beforeJSON),
			AfterJSON:       string(afterJSON),
			DiffJSON:        string(diffJSON),
		})
	})
}

func recordFailedMutation(ctx context.Context, store *state.Store, runID int64, resource tfplan.ResourcePlan, action, message string) error {
	diffJSON, err := json.Marshal(map[string]any{
		"status":       action,
		"error":        redactString(message),
		"desired_hash": resource.DesiredHash,
	})
	if err != nil {
		return err
	}
	return store.RecordRevision(ctx, state.Revision{
		ResourceAddress: resource.Address,
		RunID:           runID,
		Action:          action,
		DiffJSON:        string(diffJSON),
	})
}

func marshalMap(value map[string]any) (string, error) {
	if len(value) == 0 {
		return "", nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func writeActionDocument(outDir, address string, doc *uws1.Document) (string, error) {
	path := filepath.Join(outDir, "actions", normalizeName(address)+".uws.json")
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
		out[sourcePathKey(input.Kind, input.ID)] = input.Path
	}
	return out
}

func sourcePathKey(kind, id string) string {
	return strings.TrimSpace(kind) + "\x00" + strings.TrimSpace(id)
}

func sourceDescriptionType(kind string) uws1.SourceDescriptionType {
	switch strings.TrimSpace(kind) {
	case "aws-smithy":
		return uws1.SourceDescriptionTypeAWSSmithy
	case "google-discovery":
		return uws1.SourceDescriptionTypeGoogleDiscovery
	default:
		return uws1.SourceDescriptionTypeOpenAPI
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
