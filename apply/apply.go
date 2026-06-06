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
	"github.com/OpenUdon/ramen/internal/asyncrecord"
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
	Post    int `json:"post,omitempty"`
	Put     int `json:"put,omitempty"`
	Patch   int `json:"patch,omitempty"`
	Read    int `json:"read"`
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
		action := ""
		if strings.TrimSpace(opts.ProjectPath) == "" {
			action = "create"
		}
		planResult, err = tfplan.Build(ctx, tfplan.Options{
			ConfigDir:   opts.ConfigDir,
			ProjectPath: opts.ProjectPath,
			StatePath:   opts.StatePath,
			APISources:  opts.APISources,
			VarFiles:    opts.VarFiles,
			Vars:        opts.Vars,
			Workspace:   opts.Workspace,
			Action:      action,
		})
		if err != nil {
			return nil, err
		}
	}
	result := &Result{Version: Version, StatePath: opts.StatePath, Plan: planResult.Plan}
	if err := rejectErroredPlan(planResult); err != nil {
		return result, err
	}
	executable := executableResources(planResult.Plan.Resources)
	if len(executable) == 0 {
		result.Summary.NoOp = planResult.Plan.Summary.NoOp
		result.Summary.Skipped = len(planResult.Plan.Resources)
		return result, nil
	}
	if !opts.AutoApprove {
		return result, fmt.Errorf("apply.approval_required: apply requires explicit approval for %d action(s); rerun with --auto-approve after reviewing the plan", len(executable))
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
	asyncRecorder := asyncrecord.New(store, runID)

	sourcePaths := sourcePathIndex(opts.APISources)
	attrsByAddress := loadResourceAttributes(opts.ConfigDir, opts.ProjectPath, opts.VarFiles, opts.Vars)
	readMappings := applyReadMappings(opts.ProjectPath)
	workingDir := opts.ConfigDir
	if opts.ProjectPath != "" {
		workingDir = stateBaseDir(opts.ProjectPath, opts.ConfigDir)
	}
	workingDir = executorWorkingDir(workingDir, sourcePaths)
	failed := map[string]bool{}
	for _, resource := range executable {
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
		if resource.Action == "read" {
			executed, err := executeReadPlanAction(ctx, opts.Executor, store, asyncRecorder, runID, resource, sourcePaths, attrsByAddress[resource.Address], workingDir, opts.OutDir)
			result.Feedback = append(result.Feedback, executed.Feedback...)
			if executed.Document != "" {
				result.GeneratedDocuments = append(result.GeneratedDocuments, executed.Document)
			}
			if err != nil {
				runStatus = "failed"
				result.Summary.Failed++
				failed[resource.Address] = true
				msg := fmt.Sprintf("apply.read_failed: %v", err)
				result.Errors = append(result.Errors, msg)
				if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", msg); recErr != nil {
					return result, recErr
				}
				continue
			}
			if err := recordSuccessfulRead(ctx, store, runID, resource, executed.Result); err != nil {
				runStatus = "failed"
				return result, err
			}
			result.Executed = append(result.Executed, ExecutedAction{
				Address:   resource.Address,
				Action:    resource.Action,
				Operation: resource.Mapping.OperationID,
				Document:  executed.Document,
				Result:    redactExecutorResult(executed.Result),
			})
			result.Summary.Read++
			continue
		}
		readMapping := readMappings[resource.Address]
		if resource.Action == "delete" && readMapping == nil {
			runStatus = "failed"
			result.Summary.Failed++
			failed[resource.Address] = true
			msg := fmt.Sprintf("apply.delete_confirmation_missing: delete confirmation requires a read role for %s", resource.Address)
			result.Errors = append(result.Errors, msg)
			if err := recordFailedMutation(ctx, store, runID, resource, "failed", msg); err != nil {
				return result, err
			}
			continue
		}
		if readMapping != nil {
			baseline, err := executeReadCheck(ctx, opts.Executor, store, asyncRecorder, runID, resource, readMapping, sourcePaths, attrsByAddress[resource.Address], workingDir, opts.OutDir, "baseline")
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
		if resource.Action == "delete" {
			settle, err := parseSettleBeforeDelete(resource.RuntimeHints)
			if err != nil {
				runStatus = "failed"
				result.Summary.Failed++
				failed[resource.Address] = true
				msg := fmt.Sprintf("apply.settle_failed: %v", err)
				result.Errors = append(result.Errors, msg)
				if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", msg); recErr != nil {
					return result, recErr
				}
				continue
			}
			if settle.Active {
				settled, err := executeSettleBeforeDelete(ctx, opts.Executor, store, asyncRecorder, runID, resource, readMapping, sourcePaths, attrsByAddress[resource.Address], workingDir, opts.OutDir, settle)
				result.Feedback = append(result.Feedback, settled.Feedback...)
				if err != nil {
					runStatus = "failed"
					result.Summary.Failed++
					failed[resource.Address] = true
					msg := fmt.Sprintf("apply.settle_failed: %v", err)
					result.Errors = append(result.Errors, msg)
					if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", msg); recErr != nil {
						return result, recErr
					}
					continue
				}
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
		req.Runtime = executorRuntimeHints(resource.RuntimeHints, false)
		req.Capabilities = executor.RequirementsForRuntimeHints(executor.RequirementsForAction(action), req.Runtime)
		req.Idempotency = executor.IdempotencyForAction(action)
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
		execResult, _, err := executeExecutorRequest(ctx, opts.Executor, asyncRecorder, req)
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
			if resource.Action == "delete" {
				confirmed, err := executeReadCheck(ctx, opts.Executor, store, asyncRecorder, runID, deleteConfirmationResource(resource), readMapping, sourcePaths, attrsByAddress[resource.Address], workingDir, opts.OutDir, "delete-confirmation")
				result.Feedback = append(result.Feedback, confirmed.Feedback...)
				if err != nil {
					runStatus = "failed"
					result.Summary.Failed++
					failed[resource.Address] = true
					msg := fmt.Sprintf("apply.delete_confirmation_failed: %v", err)
					result.Errors = append(result.Errors, msg)
					if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", msg); recErr != nil {
						return result, recErr
					}
					continue
				}
				if !confirmed.Result.Missing {
					runStatus = "failed"
					result.Summary.Failed++
					failed[resource.Address] = true
					msg := fmt.Sprintf("apply.delete_confirmation_exists: read-after-delete still found %s", resource.Address)
					result.Errors = append(result.Errors, msg)
					if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", msg); recErr != nil {
						return result, recErr
					}
					continue
				}
			} else {
				converged, err := executeReadCheck(ctx, opts.Executor, store, asyncRecorder, runID, resource, readMapping, sourcePaths, attrsByAddress[resource.Address], workingDir, opts.OutDir, "convergence")
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
		}
		if resource.Action == "delete" {
			if err := recordSuccessfulDelete(ctx, store, runID, resource, before); err != nil {
				runStatus = "failed"
				return result, err
			}
		} else if err := recordSuccessfulMutation(ctx, store, runID, resource, before, stateResult, stateMapping); err != nil {
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
		case "delete":
			result.Summary.Delete++
		case "post":
			result.Summary.Post++
		case "put":
			result.Summary.Put++
		case "patch":
			result.Summary.Patch++
		}
	}
	result.Summary.NoOp = planResult.Plan.Summary.NoOp
	result.Summary.Skipped = len(planResult.Plan.Resources) - len(executable)
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
	sourcePath := firstNonEmpty(sourcePaths[sourcePathKey(resource.Mapping.SourceKind, resource.Mapping.SourceID)], resource.Mapping.SourcePath, resource.Mapping.SourceID)
	attrsForRegistry := attrs
	identityForRegistry := identity
	identitiesForRegistry := resource.Mapping.IdentityAttributes
	if hasNativeRequestBindingsForAction(resource) {
		attrsForRegistry = nil
		identityForRegistry = nil
		identitiesForRegistry = nil
	}
	request := requestbinding.Build(requestbinding.Options{
		Object:      tfmapping.Object{Kind: resource.Kind, Type: resource.Type, Provider: resource.Provider},
		SourceKind:  resource.Mapping.SourceKind,
		SourceID:    resource.Mapping.SourceID,
		SourcePath:  sourcePath,
		OperationID: resource.Mapping.OperationID,
		Attributes:  attrsForRegistry,
		Identity:    identityForRegistry,
		Identities:  identitiesForRegistry,
		Extension:   "x-ramen-apply",
		Metadata: map[string]any{
			"address":      resource.Address,
			"type":         resource.Type,
			"provider":     resource.Provider,
			"action":       resource.Action,
			"desired_hash": resource.DesiredHash,
		},
	})
	applyNativeRequestBindings(request, resource, attrs, identity)
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

func hasNativeRequestBindingsForAction(resource tfplan.ResourcePlan) bool {
	if resource.Mapping == nil {
		return false
	}
	for _, binding := range resource.Mapping.RequestBindings {
		if requestBindingApplies(binding, resource.Action, resource.Mapping.OperationID) {
			return true
		}
	}
	return false
}

func applyNativeRequestBindings(request map[string]any, resource tfplan.ResourcePlan, attrs, identity map[string]any) {
	if resource.Mapping == nil {
		return
	}
	for _, binding := range resource.Mapping.RequestBindings {
		if !requestBindingApplies(binding, resource.Action, resource.Mapping.OperationID) {
			continue
		}
		value, ok := bindingValue(binding, attrs, identity)
		if !ok {
			continue
		}
		location := strings.ToLower(strings.TrimSpace(binding.Location))
		if location == "" {
			location = "body"
		}
		requestPath := firstNonEmpty(binding.RequestPath, binding.Path)
		switch location {
		case "path", "query", "header", "cookie":
			values, _ := request[location].(map[string]any)
			if values == nil {
				values = map[string]any{}
				request[location] = values
			}
			if _, exists := values[requestPath]; !exists {
				values[requestPath] = value
			}
		default:
			body, _ := request["body"].(map[string]any)
			if body == nil {
				body = map[string]any{}
				request["body"] = body
			}
			setRequestBodyValue(body, requestPath, value)
		}
	}
}

func requestBindingApplies(binding project.RequestBinding, action, operationID string) bool {
	role := strings.TrimSpace(binding.OperationRole)
	if role != "" && role != action {
		return false
	}
	op := strings.TrimSpace(binding.OperationID)
	return op == "" || op == operationID
}

func bindingValue(binding project.RequestBinding, attrs, identity map[string]any) (any, bool) {
	for _, key := range []string{binding.Path, binding.RequestPath} {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if value, ok := lookupBindingValue(attrs, key); ok {
			return value, true
		}
		if value, ok := lookupBindingValue(identity, key); ok {
			return value, true
		}
	}
	return nil, false
}

func lookupBindingValue(values map[string]any, path string) (any, bool) {
	if len(values) == 0 {
		return nil, false
	}
	if value, ok := values[path]; ok {
		return value, true
	}
	parts := strings.Split(path, ".")
	var current any = values
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func setRequestBodyValue(target map[string]any, path string, value any) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	parts := strings.Split(path, ".")
	current := target
	for _, part := range parts[:len(parts)-1] {
		part = strings.TrimSpace(part)
		if part == "" {
			return
		}
		next, ok := current[part]
		if !ok {
			nested := map[string]any{}
			current[part] = nested
			current = nested
			continue
		}
		nested, ok := next.(map[string]any)
		if !ok {
			return
		}
		current = nested
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return
	}
	if _, exists := current[last]; !exists {
		current[last] = value
	}
}

func executorStepBody(request map[string]any) map[string]any {
	out := map[string]any{}
	for _, component := range []string{"path", "query", "header", "cookie", "body"} {
		values, _ := request[component].(map[string]any)
		for key, value := range cloneRequestMap(values) {
			if component == "body" {
				out[key] = value
			} else {
				out[component+"."+key] = value
			}
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

func executableResources(resources []tfplan.ResourcePlan) []tfplan.ResourcePlan {
	var out []tfplan.ResourcePlan
	for _, resource := range resources {
		switch resource.Action {
		case "read", "create", "update", "delete", "post", "put", "patch":
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
			Method:      resource.Mapping.Method,
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

type readActionResult struct {
	Result   executor.Result
	Feedback []executor.FeedbackRecord
	Document string
}

func executeReadPlanAction(ctx context.Context, exec executor.Executor, store *state.Store, recorder *asyncrecord.Recorder, runID int64, resource tfplan.ResourcePlan, sourcePaths map[string]string, attrs map[string]any, workingDir, outDir string) (readActionResult, error) {
	identity, err := currentIdentity(ctx, store, resource.Address)
	if err != nil {
		return readActionResult{}, err
	}
	doc, err := buildActionDocument(resource, sourcePaths, attrs, identity)
	if err != nil {
		return readActionResult{}, fmt.Errorf("document invalid: %w", err)
	}
	docPath := ""
	if outDir != "" {
		docPath, err = writeActionDocument(outDir, resource.Address, doc)
		if err != nil {
			return readActionResult{}, err
		}
	}
	action := executorAction(resource)
	req := executor.Request{
		RunID:      runID,
		Action:     action,
		Document:   doc,
		WorkingDir: workingDir,
		OutDir:     outDir,
	}
	req.Runtime = executorRuntimeHints(resource.RuntimeHints, true)
	req.Capabilities = executor.RequirementsForRuntimeHints(executor.RequirementsForAction(action), req.Runtime)
	req.Idempotency = executor.IdempotencyForAction(action)
	if err := executor.EnsureSupported(exec, req); err != nil {
		return readActionResult{Document: docPath}, err
	}
	execResult, requestEvidenceID, err := executeReadRequest(ctx, exec, recorder, req, true)
	execResult, err = stateprojection.ClassifyReadResult(execResult, err)
	if requestEvidenceID != "" {
		if recordErr := recorder.RecordConfirmationRead(ctx, req, execResult, err, requestEvidenceID); recordErr != nil {
			return readActionResult{Document: docPath}, recordErr
		}
	}
	feedback := executor.FeedbackFromResult(req, execResult, err)
	out := readActionResult{Result: execResult, Feedback: []executor.FeedbackRecord{feedback}, Document: docPath}
	if err != nil {
		return out, err
	}
	if !execResult.Success {
		return out, fmt.Errorf("executor reported unsuccessful read for %s", resource.Address)
	}
	return out, nil
}

func executeReadCheck(ctx context.Context, exec executor.Executor, store *state.Store, recorder *asyncrecord.Recorder, runID int64, resource tfplan.ResourcePlan, readMapping *tfplan.MappingPlan, sourcePaths map[string]string, attrs map[string]any, workingDir, outDir, phase string) (readCheckResult, error) {
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
	req.Runtime = executorRuntimeHints(resource.RuntimeHints, phase != "baseline" && phase != "settle")
	req.Capabilities = executor.RequirementsForRuntimeHints(executor.RequirementsForAction(action), req.Runtime)
	req.Idempotency = executor.IdempotencyForAction(action)
	if phase != "" {
		req.Idempotency.Key += "-" + phase
	}
	if err := executor.EnsureSupported(exec, req); err != nil {
		return readCheckResult{}, err
	}
	execResult, requestEvidenceID, err := executeReadRequest(ctx, exec, recorder, req, phase != "baseline")
	execResult, err = stateprojection.ClassifyReadResult(execResult, err)
	if requestEvidenceID != "" {
		if recordErr := recorder.RecordConfirmationRead(ctx, req, execResult, err, requestEvidenceID); recordErr != nil {
			return readCheckResult{}, recordErr
		}
	}
	feedback := executor.FeedbackFromResult(req, execResult, err)
	if err != nil {
		return readCheckResult{Result: execResult, Feedback: []executor.FeedbackRecord{feedback}}, err
	}
	if !execResult.Success {
		return readCheckResult{Result: execResult, Feedback: []executor.FeedbackRecord{feedback}}, fmt.Errorf("executor reported unsuccessful read for %s", resource.Address)
	}
	return readCheckResult{Result: execResult, Feedback: []executor.FeedbackRecord{feedback}}, nil
}

func executeSettleBeforeDelete(ctx context.Context, exec executor.Executor, store *state.Store, recorder *asyncrecord.Recorder, runID int64, resource tfplan.ResourcePlan, readMapping *tfplan.MappingPlan, sourcePaths map[string]string, attrs map[string]any, workingDir, outDir string, policy settlePolicy) (readCheckResult, error) {
	deadline := time.Now().Add(policy.Duration)
	var out readCheckResult
	for {
		checked, err := executeReadCheck(ctx, exec, store, recorder, runID, resource, readMapping, sourcePaths, attrs, workingDir, outDir, "settle")
		out.Feedback = append(out.Feedback, checked.Feedback...)
		out.Result = checked.Result
		if err != nil {
			return out, err
		}
		if checked.Result.Missing {
			return out, fmt.Errorf("settle read reported %s missing", resource.Address)
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return out, nil
		}
		sleepFor := policy.Interval
		if remaining < sleepFor {
			sleepFor = remaining
		}
		if err := sleepRuntimeInterval(ctx, sleepFor); err != nil {
			return out, err
		}
	}
}

func executeExecutorRequest(ctx context.Context, exec executor.Executor, recorder *asyncrecord.Recorder, req executor.Request) (executor.Result, string, error) {
	requestEvidenceID, recordErr := recorder.RecordRequest(ctx, req)
	if recordErr != nil {
		return executor.Result{}, "", recordErr
	}
	req.Events = recorder.EventSink(ctx, req, requestEvidenceID, ExecutorEventSink(recorder.Store))
	attempts := hintAttempts(req.Runtime.Retry, "max_attempts", 1)
	interval := hintInterval(req.Runtime.Retry)
	var last executor.Result
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		last, lastErr = exec.Execute(ctx, req)
		if lastErr == nil && last.Success {
			if err := recorder.RecordResponse(ctx, req, last, nil, requestEvidenceID); err != nil {
				return last, requestEvidenceID, err
			}
			return last, requestEvidenceID, nil
		}
		if attempt == attempts {
			break
		}
		if err := sleepRuntimeInterval(ctx, interval); err != nil {
			return last, requestEvidenceID, err
		}
	}
	if attempts > 1 {
		if lastErr != nil {
			lastErr = fmt.Errorf("apply.retry_exhausted: executor failed after %d attempt(s): %w", attempts, lastErr)
			if err := recorder.RecordResponse(ctx, req, last, lastErr, requestEvidenceID); err != nil {
				return last, requestEvidenceID, err
			}
			return last, requestEvidenceID, lastErr
		}
		if !last.Success {
			lastErr = fmt.Errorf("apply.retry_exhausted: executor reported unsuccessful result after %d attempt(s)", attempts)
			if err := recorder.RecordResponse(ctx, req, last, lastErr, requestEvidenceID); err != nil {
				return last, requestEvidenceID, err
			}
			return last, requestEvidenceID, lastErr
		}
	}
	if err := recorder.RecordResponse(ctx, req, last, lastErr, requestEvidenceID); err != nil {
		return last, requestEvidenceID, err
	}
	return last, requestEvidenceID, lastErr
}

func executeReadRequest(ctx context.Context, exec executor.Executor, recorder *asyncrecord.Recorder, req executor.Request, useWaiter bool) (executor.Result, string, error) {
	if !useWaiter || len(req.Runtime.Waiter) == 0 {
		return executeExecutorRequest(ctx, exec, recorder, req)
	}
	until := strings.ToLower(strings.TrimSpace(fmt.Sprint(req.Runtime.Waiter["until"])))
	if until == "" {
		return executeExecutorRequest(ctx, exec, recorder, req)
	}
	if until != "exists" && until != "missing" && until != "success" {
		return executor.Result{}, "", fmt.Errorf("apply.waiter_unsupported: unsupported waiter predicate %q", until)
	}
	attempts := hintAttempts(req.Runtime.Waiter, "max_attempts", hintAttempts(req.Runtime.Retry, "max_attempts", 1))
	interval := hintInterval(req.Runtime.Waiter)
	var last executor.Result
	var lastRequestEvidenceID string
	for attempt := 1; attempt <= attempts; attempt++ {
		result, requestEvidenceID, err := executeExecutorRequest(ctx, exec, recorder, req)
		result, err = stateprojection.ClassifyReadResult(result, err)
		last = result
		lastRequestEvidenceID = requestEvidenceID
		if err != nil {
			return result, requestEvidenceID, err
		}
		if waiterSatisfied(until, result) {
			return result, requestEvidenceID, nil
		}
		if attempt == attempts {
			break
		}
		if err := sleepRuntimeInterval(ctx, interval); err != nil {
			return result, requestEvidenceID, err
		}
	}
	return last, lastRequestEvidenceID, fmt.Errorf("apply.waiter_timeout: read waiter %q not satisfied for %s after %d attempt(s)", until, req.Action.Address, attempts)
}

func waiterSatisfied(until string, result executor.Result) bool {
	switch until {
	case "exists":
		return result.Success && !result.Missing
	case "missing":
		return result.Success && result.Missing
	case "success":
		return result.Success
	default:
		return false
	}
}

func deleteConfirmationResource(resource tfplan.ResourcePlan) tfplan.ResourcePlan {
	out := resource
	hints := project.RuntimeHints{}
	if resource.RuntimeHints != nil {
		hints.Retry = cloneAnyMap(resource.RuntimeHints.Retry)
		hints.Waiter = cloneAnyMap(resource.RuntimeHints.Waiter)
		hints.Settle = cloneAnyMap(resource.RuntimeHints.Settle)
	}
	if hints.Waiter == nil {
		hints.Waiter = map[string]any{}
	}
	hints.Waiter["until"] = "missing"
	out.RuntimeHints = &hints
	return out
}

type settlePolicy struct {
	Active   bool
	Duration time.Duration
	Interval time.Duration
}

func parseSettleBeforeDelete(hints *project.RuntimeHints) (settlePolicy, error) {
	if hints == nil || len(hints.Settle) == 0 {
		return settlePolicy{}, nil
	}
	before := strings.ToLower(strings.TrimSpace(fmt.Sprint(hints.Settle["before"])))
	if before == "" {
		before = "delete"
	}
	if before != "delete" {
		return settlePolicy{}, fmt.Errorf("settle.before %q is not supported", before)
	}
	readExpect := strings.ToLower(strings.TrimSpace(fmt.Sprint(hints.Settle["read_expect"])))
	if readExpect == "" {
		readExpect = "exists"
	}
	if readExpect != "exists" {
		return settlePolicy{}, fmt.Errorf("settle.read_expect %q is not supported", readExpect)
	}
	duration := hintDuration(hints.Settle, "duration")
	if duration <= 0 {
		return settlePolicy{}, fmt.Errorf("settle.duration must be a positive duration")
	}
	interval := hintInterval(hints.Settle)
	if interval <= 0 {
		return settlePolicy{}, fmt.Errorf("settle.interval must be a positive duration")
	}
	return settlePolicy{Active: true, Duration: duration, Interval: interval}, nil
}

func hintAttempts(hints map[string]any, key string, fallback int) int {
	value, ok := hints[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case json.Number:
		if parsed, err := typed.Int64(); err == nil && parsed > 0 {
			return int(parsed)
		}
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func hintInterval(hints map[string]any) time.Duration {
	value, ok := hints["interval"]
	if !ok {
		value = hints["delay"]
	}
	if value == nil {
		return 0
	}
	switch typed := value.(type) {
	case time.Duration:
		if typed > 0 {
			return typed
		}
	case int:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	case int64:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	case float64:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	case json.Number:
		if parsed, err := typed.Int64(); err == nil && parsed > 0 {
			return time.Duration(parsed) * time.Millisecond
		}
	case string:
		parsed, err := time.ParseDuration(strings.TrimSpace(typed))
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func hintDuration(hints map[string]any, key string) time.Duration {
	value, ok := hints[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case time.Duration:
		if typed > 0 {
			return typed
		}
	case int:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	case int64:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	case float64:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	case json.Number:
		if parsed, err := typed.Int64(); err == nil && parsed > 0 {
			return time.Duration(parsed) * time.Millisecond
		}
	case string:
		parsed, err := time.ParseDuration(strings.TrimSpace(typed))
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func sleepRuntimeInterval(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
	case "delete":
		if execResult.Missing {
			return fmt.Errorf("apply.baseline_missing: read-before-delete reported missing remote object for planned delete %s", resource.Address)
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
			Schema:             slices.Clone(resource.Schema),
			RequestBindings:    slices.Clone(resource.RequestBindings),
			ResponseBindings:   slices.Clone(resource.ResponseBindings),
			Normalizers:        slices.Clone(resource.Normalizers),
			MappingLifecycle:   cloneMappingLifecycle(resource.MappingLifecycle),
			RequiredOperations: slices.Clone(resource.RequiredOperations),
		}
	}
	return out
}

func executorRuntimeHints(hints *project.RuntimeHints, includeWaiter bool) executor.RuntimeHints {
	if hints == nil {
		return executor.RuntimeHints{}
	}
	out := executor.RuntimeHints{
		Retry: cloneAnyMap(hints.Retry),
	}
	if includeWaiter && waiterPredicate(hints.Waiter) != "" {
		out.Waiter = cloneAnyMap(hints.Waiter)
	}
	return out
}

func waiterPredicate(waiter map[string]any) string {
	if len(waiter) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(fmt.Sprint(waiter["until"])))
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = cloneAny(value)
	}
	return out
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneAny(item)
		}
		return out
	default:
		return value
	}
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

func recordSuccessfulDelete(ctx context.Context, store *state.Store, runID int64, resource tfplan.ResourcePlan, before *state.ResourceSnapshot) error {
	beforeJSON := []byte(nil)
	if before != nil {
		var err error
		beforeJSON, err = json.Marshal(before)
		if err != nil {
			return err
		}
	}
	diffJSON, err := json.Marshal(map[string]any{
		"operation": mappingOperationID(resource.Mapping),
	})
	if err != nil {
		return err
	}
	return store.WithTx(ctx, func(tx *state.Tx) error {
		if err := tx.DeleteResource(ctx, resource.Address); err != nil {
			return err
		}
		return tx.RecordRevision(ctx, state.Revision{
			ResourceAddress: resource.Address,
			RunID:           runID,
			Action:          "delete",
			BeforeJSON:      string(beforeJSON),
			DiffJSON:        string(diffJSON),
		})
	})
}

func recordSuccessfulRead(ctx context.Context, store *state.Store, runID int64, resource tfplan.ResourcePlan, execResult executor.Result) error {
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
	if execResult.Missing {
		diffJSON, err := json.Marshal(map[string]any{
			"status":       "missing",
			"desired_hash": resource.DesiredHash,
			"operation":    mappingOperationID(resource.Mapping),
		})
		if err != nil {
			return err
		}
		return store.RecordRevision(ctx, state.Revision{
			ResourceAddress: resource.Address,
			RunID:           runID,
			Action:          "read_missing",
			BeforeJSON:      string(beforeJSON),
			DiffJSON:        string(diffJSON),
		})
	}
	redacted := redactExecutorResult(execResult)
	if resource.Mapping != nil {
		identity, computed := stateprojection.Project(resource.Mapping, execResult)
		if len(identity) > 0 {
			redacted.Identity = redactMap(identity)
		}
		if len(computed) > 0 {
			redacted.Computed = redactMap(computed)
		}
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
			Action:          "read",
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

func mappingOperationID(mapping *tfplan.MappingPlan) string {
	if mapping == nil {
		return ""
	}
	return mapping.OperationID
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
		out[sourcePathKey(input.Kind, input.ID)] = executorSourcePath(input.Path)
	}
	return out
}

func executorSourcePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "://") || strings.HasPrefix(path, "urn:") {
		return path
	}
	abs, err := filepath.Abs(filepath.FromSlash(path))
	if err != nil {
		return path
	}
	return abs
}

func executorWorkingDir(base string, sourcePaths map[string]string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "."
	}
	root, err := filepath.Abs(filepath.FromSlash(base))
	if err != nil {
		return base
	}
	for _, sourcePath := range sourcePaths {
		sourcePath = strings.TrimSpace(sourcePath)
		if sourcePath == "" || strings.Contains(sourcePath, "://") || strings.HasPrefix(sourcePath, "urn:") {
			continue
		}
		absSource, err := filepath.Abs(filepath.FromSlash(sourcePath))
		if err != nil {
			continue
		}
		root = commonPath(root, filepath.Dir(absSource))
	}
	return root
}

func commonPath(left, right string) string {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	for {
		if sameOrChild(right, left) {
			return left
		}
		parent := filepath.Dir(left)
		if parent == left {
			return left
		}
		left = parent
	}
}

func sameOrChild(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
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
