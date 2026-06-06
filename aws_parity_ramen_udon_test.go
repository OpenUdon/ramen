//go:build awslive && udon

package corpus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/ramen/apply"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/executor/udon"
	tfplan "github.com/OpenUdon/ramen/plan"
)

func TestAWSParityW01Render(t *testing.T) {
	workDir := t.TempDir()
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	smithyPath, err := filepath.Abs("../apitools/catalog-openapi-cache/aws-smithy/aws-iam-smithy-model.json")
	if err != nil {
		t.Fatalf("resolve IAM Smithy path: %v", err)
	}
	userName := "ramen-parity-w01-render"
	if err := renderAWSParityW01Project(filepath.Join(awsParityFixtureRoot, "w01", "ramen", "project.uws.yaml"), projectPath, userName, smithyPath); err != nil {
		t.Fatalf("render W01 project: %v", err)
	}
	data, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatalf("read rendered W01 project: %v", err)
	}
	for _, want := range []string{userName, "binding: aws_hmac", "aws_signing_name: iam", "region: us-east-1", "service: iam"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("rendered W01 project missing %q:\n%s", want, string(data))
		}
	}
	for _, tc := range []struct {
		action      string
		operationID string
		summary     string
		seed        bool
	}{
		{action: "create", operationID: "CreateUser", summary: "create"},
		{action: "read", operationID: "GetUser", summary: "read"},
		{action: "delete", operationID: "DeleteUser", summary: "delete", seed: true},
	} {
		statePath := filepath.Join(workDir, tc.action+".db")
		if tc.seed {
			seedAWSParityState(t, "w01", statePath)
		}
		result, err := tfplan.Build(context.Background(), tfplan.Options{
			ProjectPath: projectPath,
			StatePath:   statePath,
			Action:      tc.action,
		})
		if err != nil {
			t.Fatalf("build rendered W01 %s plan: %v", tc.action, err)
		}
		if result.Plan.Errored || len(result.Plan.Resources) != 1 {
			t.Fatalf("rendered W01 %s plan unusable: %#v", tc.action, result.Plan)
		}
		resource := result.Plan.Resources[0]
		if resource.Mapping == nil || resource.Mapping.OperationID != tc.operationID {
			t.Fatalf("rendered W01 %s operation = %#v, want %s", tc.action, resource.Mapping, tc.operationID)
		}
		if !awsParitySummaryHasOne(result.Plan.Summary, tc.summary) {
			t.Fatalf("rendered W01 %s summary = %#v, want one %s action", tc.action, result.Plan.Summary, tc.summary)
		}
	}
}

func runAWSParityW01Live(ctx context.Context, t *testing.T, artifact awsParityArtifact) awsParityLiveRecording {
	t.Helper()
	started := time.Now()
	suffix := awsParityW01RunSuffix()
	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, string) awsParityRuntimeResult
	}{
		{runtime: "opentofu", run: runAWSParityW01OpenTofuRuntime},
		{runtime: "ramen", run: runAWSParityW01RamenRuntime},
	}
	var observations []awsParityRuntimeObservation
	var failures []awsParityRuntimeFailure
	for _, run := range runs {
		userName := awsParityW01UserName(run.runtime, suffix)
		result := timedAWSParityRuntime(run.runtime, func() awsParityRuntimeResult {
			return run.run(ctx, t, userName)
		})
		if result.Failure != nil {
			failures = append(failures, *result.Failure)
			continue
		}
		observations = append(observations, result.Observation)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Logf("%s AWS parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("W01 AWS provider parity did not complete for all runtimes")
	}
	comparison := compareAWSParityW01Observations(observations)
	if !comparison.Matched {
		t.Fatalf("W01 AWS provider parity observations did not match: %#v", observations)
	}
	return awsParityLiveRecording{
		Version:      awsParityArtifactV1,
		Lane:         "W01",
		Scenario:     artifact.Scenarios[0].Name,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		DurationMS:   time.Since(started).Milliseconds(),
		Observations: observations,
		Comparison:   comparison,
	}
}

func timedAWSParityRuntime(runtime string, run func() awsParityRuntimeResult) awsParityRuntimeResult {
	started := time.Now()
	result := run()
	if result.Failure == nil {
		result.Observation.DurationMS = time.Since(started).Milliseconds()
	}
	return result
}

func runAWSParityW01RamenRuntime(ctx context.Context, t *testing.T, userName string) awsParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := validateAWSParityW01UserName(userName); err != nil {
		return awsParityFailure(runtimeName, "safety", err)
	}
	if err := deleteAWSParityIAMUserIfExists(ctx, userName); err != nil {
		return awsParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteAWSParityIAMUserIfExists(context.Background(), userName); err != nil {
			t.Logf("cleanup AWS IAM user %s: %v", userName, err)
		}
	})
	workDir := filepath.Join(t.TempDir(), runtimeName)
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	smithyPath, err := filepath.Abs("../apitools/catalog-openapi-cache/aws-smithy/aws-iam-smithy-model.json")
	if err != nil {
		return awsParityFailure(runtimeName, "fixture", err)
	}
	if err := renderAWSParityW01Project(filepath.Join(awsParityFixtureRoot, "w01", "ramen", "project.uws.yaml"), projectPath, userName, smithyPath); err != nil {
		return awsParityFailure(runtimeName, "fixture", err)
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
			observed, err := observeAWSParityIAMUser(projectorCtx, userName)
			if err != nil {
				return executor.Result{}, err
			}
			if !observed.Exists {
				result.Missing = true
				return result, nil
			}
			result.Identity = map[string]any{"user_name": observed.UserName}
			result.Computed = map[string]any{
				"arn_present":     observed.ArnPresent,
				"user_id_present": observed.UserIDPresent,
			}
			return result, nil
		},
	}
	if err := buildAndApplyAWSParityPlan(ctx, projectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), udonExecutor); err != nil {
		return awsParityFailure(runtimeName, "apply", err)
	}
	afterApply, err := observeAWSParityIAMUser(ctx, userName)
	if err != nil {
		return awsParityFailure(runtimeName, "observe", err)
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ProjectPath: projectPath, StatePath: statePath})
	if err != nil {
		return awsParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1
	if err := buildAndApplyAWSParityPlan(ctx, projectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), udonExecutor); err != nil {
		return awsParityFailure(runtimeName, "read", err)
	}
	if err := buildAndApplyAWSParityPlan(ctx, projectPath, statePath, "delete", filepath.Join(workDir, "delete-plan.json"), udonExecutor); err != nil {
		return awsParityFailure(runtimeName, "delete", err)
	}
	afterDestroy, err := waitAWSParityIAMUser(ctx, userName, false)
	if err != nil {
		return awsParityFailure(runtimeName, "observe", err)
	}
	return awsParityRuntimeResult{Observation: awsParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: userName,
		Fields:   awsParityIAMUserObservationFields(afterApply, afterDestroy, noOp),
	}}
}

func buildAndApplyAWSParityPlan(ctx context.Context, projectPath, statePath, action, planPath string, udonExecutor udon.Executor) error {
	if _, err := tfplan.Build(ctx, tfplan.Options{
		ProjectPath: projectPath,
		StatePath:   statePath,
		Action:      action,
		OutPath:     planPath,
	}); err != nil {
		return fmt.Errorf("build %s plan: %w", action, err)
	}
	result, err := apply.Apply(ctx, apply.Options{
		PlanPath:    planPath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    udonExecutor,
		OutDir:      filepath.Join(filepath.Dir(planPath), "apply-"+action),
	})
	if err != nil {
		return fmt.Errorf("apply %s: %w; errors=%v", action, err, result.Errors)
	}
	if result.Summary.Failed != 0 || result.Summary.Blocked != 0 {
		return fmt.Errorf("apply %s summary failed=%d blocked=%d", action, result.Summary.Failed, result.Summary.Blocked)
	}
	return nil
}

func renderAWSParityW01Project(src, dst, userName, smithyPath string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := validateAWSParityW01UserName(userName); err != nil {
		return err
	}
	out := strings.ReplaceAll(string(data), "ramen-parity-w01-static", userName)
	if strings.TrimSpace(smithyPath) != "" {
		out = strings.ReplaceAll(out, "../../../../../../apitools/catalog-openapi-cache/aws-smithy/aws-iam-smithy-model.json", filepath.ToSlash(smithyPath))
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}
