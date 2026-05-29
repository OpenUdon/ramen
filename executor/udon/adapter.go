//go:build udon

package udon

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenUdon/ramen/executor"
	"github.com/genelet/udon/generator"
	"github.com/genelet/udon/pkg/runner"
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
