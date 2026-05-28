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
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/state"
	"github.com/OpenUdon/ramen/tfmapping"
	"github.com/OpenUdon/tfconfig"
	"github.com/OpenUdon/uws/uws1"
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
	StatePath          string
	RunID              int64
	Plan               tfplan.Document
	Summary            Summary
	Executed           []ExecutedAction
	GeneratedDocuments []string
	Errors             []string
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
	opts = normalizeOptions(opts)
	planResult, err := tfplan.Build(ctx, tfplan.Options{
		ConfigDir:   opts.ConfigDir,
		ProjectPath: opts.ProjectPath,
		StatePath:   opts.StatePath,
		APISources:  opts.APISources,
		Action:      "create",
	})
	if err != nil {
		return nil, err
	}
	result := &Result{StatePath: opts.StatePath, Plan: planResult.Plan}
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
		return result, fmt.Errorf("apply requires explicit approval for %d mutation(s); rerun with --auto-approve after reviewing the plan", len(mutations))
	}
	if opts.Executor == nil {
		return result, fmt.Errorf("apply requires a trusted executor; pass --mock for recorded/mock execution in public builds")
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

	sourcePaths := sourcePathIndex(opts.APISources)
	attrsByAddress := loadResourceAttributes(opts.ConfigDir, opts.ProjectPath)
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
			msg := fmt.Sprintf("apply delete for %s is handled by ramen destroy", resource.Address)
			result.Errors = append(result.Errors, msg)
			if err := recordFailedMutation(ctx, store, runID, resource, "failed", msg); err != nil {
				return result, err
			}
			continue
		}
		doc, err := buildActionDocument(resource, sourcePaths, attrsByAddress[resource.Address], nil)
		if err != nil {
			runStatus = "failed"
			result.Summary.Failed++
			failed[resource.Address] = true
			result.Errors = append(result.Errors, err.Error())
			if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", err.Error()); recErr != nil {
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
		req := executor.Request{
			RunID:      runID,
			Action:     executorAction(resource),
			Document:   doc,
			WorkingDir: workingDir,
			OutDir:     opts.OutDir,
		}
		before, err := store.CurrentResource(ctx, resource.Address)
		if err != nil {
			runStatus = "failed"
			return result, err
		}
		execResult, err := opts.Executor.Execute(ctx, req)
		if err != nil {
			runStatus = "failed"
			result.Summary.Failed++
			failed[resource.Address] = true
			result.Errors = append(result.Errors, err.Error())
			if recErr := recordFailedMutation(ctx, store, runID, resource, "failed", err.Error()); recErr != nil {
				return result, recErr
			}
			continue
		}
		if !execResult.Success {
			runStatus = "failed"
			result.Summary.Failed++
			failed[resource.Address] = true
			msg := fmt.Sprintf("executor reported unsuccessful %s for %s", resource.Action, resource.Address)
			result.Errors = append(result.Errors, msg)
			if err := recordFailedMutation(ctx, store, runID, resource, "failed", msg); err != nil {
				return result, err
			}
			continue
		}
		if err := recordSuccessfulMutation(ctx, store, runID, resource, before, execResult); err != nil {
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
	result.Summary.Skipped = planResult.Plan.Summary.NoOp
	if len(result.Errors) > 0 {
		return result, fmt.Errorf("apply failed for %d resource(s) and blocked %d resource(s)", result.Summary.Failed, result.Summary.Blocked)
	}
	return result, nil
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
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID:  "main",
			Type:        uws1.WorkflowTypeSequence,
			Description: "Execute one approved Ramen apply action.",
			Steps: []*uws1.Step{{
				StepID:       operationID,
				OperationRef: operationID,
				Body: map[string]any{
					"ramen_address": resource.Address,
					"ramen_action":  resource.Action,
				},
			}},
		}},
		Extensions: map[string]any{
			"x-ramen-plan-version": tfplan.Version,
		},
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

func loadResourceAttributes(configDir, projectPath string) map[string]map[string]any {
	if strings.TrimSpace(projectPath) != "" {
		proj, err := project.Load(projectPath)
		if err != nil {
			return nil
		}
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
	return rejectErrorDiagnostics(planResult.Diagnostics)
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
	}
	return action
}

func recordSuccessfulMutation(ctx context.Context, store *state.Store, runID int64, resource tfplan.ResourcePlan, before *state.ResourceSnapshot, execResult executor.Result) error {
	redacted := redactExecutorResult(execResult)
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
