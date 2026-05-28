//go:build udon

package udon

import (
	"context"
	"path/filepath"
	"time"

	"github.com/OpenUdon/ramen/executor"
	"github.com/genelet/udon/generator"
	"github.com/genelet/udon/pkg/runner"
)

type Executor struct {
	OutputDir string
}

func (e Executor) Execute(ctx context.Context, req executor.Request) (executor.Result, error) {
	started := time.Now().UTC()
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
	if err := runner.ExecuteRuntimePlan(ctx, plan, outputDir); err != nil {
		return executor.Result{}, err
	}
	return executor.Result{
		Address:    req.Action.Address,
		Operation:  req.Action.Mapping.OperationID,
		Success:    true,
		StartedAt:  started,
		FinishedAt: time.Now().UTC(),
	}, nil
}
