//go:build udon

package udon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/uws/uws1"
	"github.com/genelet/udon/generator"
	"github.com/genelet/udon/pkg/runner"
	"github.com/genelet/udon/pkg/uwsprofile"
)

type Executor struct {
	OutputDir       string
	OutputProjector func(context.Context, executor.Request, string) (executor.Result, error)
}

func (e Executor) Capabilities() executor.CapabilityDescriptor {
	return executor.CapabilityDescriptor{
		Protocols:   []string{"aws-smithy", "openapi", "google-discovery"},
		AuthSchemes: []string{"executor-configured"},
		Features: []string{
			executor.FeatureIdempotency,
			executor.FeatureProgressEvents,
			executor.FeatureRetry,
			executor.FeatureWaiter,
			executor.FeaturePagination,
			executor.FeatureOutputIdentity,
			executor.FeatureOutputComputed,
			executor.FeatureMissingEvidence,
		},
	}
}

func (e Executor) Execute(ctx context.Context, req executor.Request) (executor.Result, error) {
	started := time.Now().UTC()
	if err := executor.EnsureSupported(e, req); err != nil {
		return executor.Result{}, err
	}
	startEvent := executor.Emit(req, "started", "udon executor started", nil)
	ensureUdonExecutionHints(req.Document, req.WorkingDir)
	plan, err := generator.NewRuntimePlanFromUWSDocument(req.Document, req.WorkingDir)
	if err != nil {
		return executor.Result{}, err
	}
	outputDir := e.OutputDir
	if outputDir == "" {
		outputDir = req.OutDir
	}
	if outputDir == "" {
		outputDir = filepath.Join(req.WorkingDir, ".ramen", "apply", "udon")
	}
	outputDir = filepath.Join(outputDir, safeOutputName(req.Action.Address))
	if err := runner.ExecuteRuntimePlan(ctx, plan, outputDir); err != nil {
		if e.OutputProjector != nil && req.Action.Action == "read" && isProjectedMissingReadError(err) {
			projected, projectErr := e.OutputProjector(ctx, req, outputDir)
			if projectErr == nil && projected.Missing {
				events := append([]executor.Event{startEvent}, projected.Events...)
				events = append(events, executor.Emit(req, "finished", "udon executor finished with projected missing read", nil))
				return executor.Result{
					Address:    req.Action.Address,
					Operation:  req.Action.Mapping.OperationID,
					Success:    true,
					Missing:    true,
					Messages:   append(projected.Messages, err.Error()),
					Events:     events,
					StartedAt:  started,
					FinishedAt: time.Now().UTC(),
				}, nil
			}
		}
		return executor.Result{}, err
	}
	if e.OutputProjector == nil && req.Action.Action != "delete" {
		return executor.Result{}, fmt.Errorf("udon output projection is required for %s %s", req.Action.Action, req.Action.Address)
	}
	result := executor.Result{
		Address:    req.Action.Address,
		Operation:  req.Action.Mapping.OperationID,
		Success:    true,
		Events:     []executor.Event{startEvent},
		StartedAt:  started,
		FinishedAt: time.Now().UTC(),
	}
	if e.OutputProjector != nil {
		projected, err := e.OutputProjector(ctx, req, outputDir)
		if err != nil {
			return executor.Result{}, err
		}
		result.Identity = projected.Identity
		result.Computed = projected.Computed
		result.Missing = projected.Missing
		result.Messages = projected.Messages
		result.Events = append(result.Events, projected.Events...)
	}
	result.Events = append(result.Events, executor.Emit(req, "finished", "udon executor finished", nil))
	return result, nil
}

func ensureUdonExecutionHints(doc *uws1.Document, workDir string) {
	if doc == nil {
		return
	}
	sources := map[string]*uws1.SourceDescription{}
	for _, source := range doc.SourceDescriptions {
		if source != nil && strings.TrimSpace(source.Name) != "" {
			sources[source.Name] = source
		}
	}
	for _, op := range doc.Operations {
		ensureUdonOperationHints(op, sources, workDir)
	}
}

func ensureUdonOperationHints(op *uws1.Operation, sources map[string]*uws1.SourceDescription, workDir string) {
	if op == nil {
		return
	}
	config, _, err := uwsprofile.ReadOperationConfigExtension(op.Extensions)
	if err != nil {
		return
	}
	if config == nil {
		config = &uwsprofile.OperationConfig{}
	}
	changed := false
	if len(op.Request) > 0 && config.PathPars == nil {
		if schema := requestSectionSchema(op.Request, "path"); schema != nil {
			config.PathPars = schema
			changed = true
		}
	}
	if len(op.Request) > 0 && config.QueryPars == nil {
		if schema := requestSectionSchema(op.Request, "query"); schema != nil {
			config.QueryPars = schema
			changed = true
		}
	}
	if len(op.Request) > 0 && config.HeaderPars == nil {
		if schema := requestSectionSchema(op.Request, "header"); schema != nil {
			config.HeaderPars = schema
			changed = true
		}
	}
	if len(op.Request) > 0 && config.CookiePars == nil {
		if schema := requestSectionSchema(op.Request, "cookie"); schema != nil {
			config.CookiePars = schema
			changed = true
		}
	}
	if len(config.Security) == 0 && sourceHasAzureAuth(sources[op.SourceDescription], workDir) {
		config.Security = []*uwsprofile.SecurityRequirement{{
			Name:    "azure_auth",
			Binding: "azure_auth",
			Scopes:  []string{"user_impersonation"},
		}}
		changed = true
	}
	if changed {
		_ = uwsprofile.SetOperationConfigExtension(&op.Extensions, config)
	}
}

func requestSectionSchema(request map[string]any, section string) *uws1.ParamSchema {
	values, ok := request[section].(map[string]any)
	if !ok || len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		key = strings.TrimSpace(key)
		if key != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	properties := make(map[string]*uws1.ParamSchema, len(keys))
	for _, key := range keys {
		properties[key] = &uws1.ParamSchema{Type: "string"}
	}
	return &uws1.ParamSchema{Type: "object", Properties: properties, Required: keys}
}

func sourceHasAzureAuth(source *uws1.SourceDescription, workDir string) bool {
	if source == nil || !strings.EqualFold(string(source.Type), string(uws1.SourceDescriptionTypeOpenAPI)) {
		return false
	}
	path := strings.TrimSpace(source.URL)
	if path == "" {
		return false
	}
	if !filepath.IsAbs(path) && strings.TrimSpace(workDir) != "" {
		path = filepath.Join(workDir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc struct {
		SecurityDefinitions map[string]any `json:"securityDefinitions"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}
	_, ok := doc.SecurityDefinitions["azure_auth"]
	return ok
}

func isProjectedMissingReadError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "404") || strings.Contains(msg, "notfound") || strings.Contains(msg, "not found")
}

func safeOutputName(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return "action"
	}
	var b strings.Builder
	for _, r := range address {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "action"
	}
	return out
}
