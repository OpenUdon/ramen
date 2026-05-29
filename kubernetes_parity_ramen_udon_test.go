//go:build udon

package corpus

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/OpenUdon/ramen/apply"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/executor/udon"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/reconcile"
)

func runKubernetesParityRamenRuntime(ctx context.Context, t *testing.T, env kubernetesParityLiveEnv, _ string) kubernetesParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	namespace := "ramen-parity-k01-" + runtimeName
	workDir := filepath.Join(t.TempDir(), runtimeName)
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	openAPIPath := filepath.Join(workDir, "ramen", "openapi", "core.json")
	if err := copyFixtureFile(filepath.Join(kubernetesParityFixtureRoot, "k01", "openapi", "core.json"), openAPIPath); err != nil {
		return kubernetesParityFailure(runtimeName, "fixture", err)
	}
	if err := renderKubernetesParityProject(filepath.Join(kubernetesParityFixtureRoot, "k01", "ramen", "project.uws.yaml"), projectPath, namespace, "openapi/core.json"); err != nil {
		return kubernetesParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	udonExecutor := udon.Executor{
		OutputDir: filepath.Join(workDir, "udon"),
		OutputProjector: func(projectorCtx context.Context, req executor.Request, _ string) (executor.Result, error) {
			result := executor.Result{
				Address:   req.Action.Address,
				Operation: req.Action.Mapping.OperationID,
				Success:   true,
			}
			if req.Action.Action == "delete" {
				return result, nil
			}
			observed, err := observeKubernetesParityNamespace(projectorCtx, env, namespace)
			if err != nil {
				if isKubernetesParityNotFound(err) {
					result.Missing = true
					return result, nil
				}
				return executor.Result{}, err
			}
			if !observed.Exists {
				result.Missing = true
				return result, nil
			}
			result.Identity = map[string]any{"namespace_name": observed.Name}
			result.Computed = map[string]any{
				"metadata.name":   observed.Name,
				"metadata.labels": observed.Labels,
				"status.phase":    observed.Phase,
			}
			return result, nil
		},
	}
	applyResult, err := apply.Apply(ctx, apply.Options{
		ProjectPath: projectPath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    udonExecutor,
	})
	if err != nil {
		return kubernetesParityFailure(runtimeName, "apply", fmt.Errorf("%w; errors=%v feedback=%v", err, applyResultErrors(applyResult), applyResultFeedbackMessages(applyResult)))
	}
	if applyResult.Summary.Create != 1 || applyResult.Summary.Failed != 0 {
		return kubernetesParityFailure(runtimeName, "apply", errUnexpectedKubernetesParitySummary("apply", applyResult.Summary))
	}
	afterApply, err := observeKubernetesParityNamespace(ctx, env, namespace)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "observe", err)
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ProjectPath: projectPath, StatePath: statePath})
	if err != nil {
		return kubernetesParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1 && len(planResult.Plan.Resources) == 1
	refreshResult, err := reconcile.Refresh(ctx, reconcile.Options{
		ProjectPath: projectPath,
		StatePath:   statePath,
		Executor:    udonExecutor,
	})
	if err != nil {
		return kubernetesParityFailure(runtimeName, "refresh", fmt.Errorf("%w; feedback=%v", err, reconcileFeedbackMessages(refreshResult)))
	}
	if refreshResult.Summary.Read != 1 || refreshResult.Summary.Unchanged != 1 || refreshResult.Summary.Failed != 0 {
		return kubernetesParityFailure(runtimeName, "refresh", errUnexpectedKubernetesParitySummary("refresh", refreshResult.Summary))
	}
	destroyResult, err := reconcile.Destroy(ctx, reconcile.Options{
		ProjectPath: projectPath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    udonExecutor,
	})
	if err != nil {
		return kubernetesParityFailure(runtimeName, "destroy", fmt.Errorf("%w; feedback=%v", err, reconcileFeedbackMessages(destroyResult)))
	}
	if destroyResult.Summary.Delete != 1 || destroyResult.Summary.Failed != 0 {
		return kubernetesParityFailure(runtimeName, "destroy", errUnexpectedKubernetesParitySummary("destroy", destroyResult.Summary))
	}
	afterDestroy, err := observeKubernetesParityNamespace(ctx, env, namespace)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "observe", err)
	}
	return kubernetesParityRuntimeResult{Observation: kubernetesParityRuntimeObservation{
		Runtime:      runtimeName,
		Namespace:    namespace,
		AfterApply:   afterApply,
		NoOpPlan:     kubernetesParityNoOpObservation{NoOp: noOp, Summary: fmt.Sprintf("%+v", planResult.Plan.Summary)},
		AfterDestroy: afterDestroy,
	}}
}

func errUnexpectedKubernetesParitySummary(phase string, summary any) error {
	return &kubernetesParitySummaryError{phase: phase, summary: summary}
}

func applyResultErrors(result *apply.Result) []string {
	if result == nil {
		return nil
	}
	return result.Errors
}

func applyResultFeedbackMessages(result *apply.Result) []string {
	if result == nil {
		return nil
	}
	var messages []string
	for _, feedback := range result.Feedback {
		messages = append(messages, feedback.Messages...)
		if feedback.ErrorClass != "" {
			messages = append(messages, feedback.ErrorClass)
		}
	}
	return messages
}

func reconcileFeedbackMessages(result *reconcile.Result) []string {
	if result == nil {
		return nil
	}
	var messages []string
	for _, feedback := range result.Feedback {
		messages = append(messages, feedback.Messages...)
		if feedback.ErrorClass != "" {
			messages = append(messages, feedback.ErrorClass)
		}
	}
	return messages
}

type kubernetesParitySummaryError struct {
	phase   string
	summary any
}

func (e *kubernetesParitySummaryError) Error() string {
	return fmt.Sprintf("%s summary did not match K01 parity expectations: %#v", e.phase, e.summary)
}
