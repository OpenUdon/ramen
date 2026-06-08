//go:build githublive && udon

package corpus

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/ramen/apply"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/executor/udon"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/state"
)

func runGitHubParityH01Live(ctx context.Context, t *testing.T, artifact githubParityArtifact) githubParityLiveRecording {
	t.Helper()
	return runGitHubParityLive(ctx, t, artifact, "h01", []string{"after_create.exists", "after_create.visibility", "after_update.description", "no_op", "after_delete.exists"}, []githubParityRuntimeRun{
		{runtime: "opentofu", run: func(ctx context.Context, t *testing.T, repoName string) githubParityRuntimeResult {
			return runGitHubParityH01HCLRuntime(ctx, t, "opentofu", os.Getenv(githubParityTofuEnv), repoName)
		}},
		{runtime: "ramen", run: runGitHubParityH01RamenRuntime},
	})
}

func runGitHubParityH02Live(ctx context.Context, t *testing.T, artifact githubParityArtifact) githubParityLiveRecording {
	t.Helper()
	return runGitHubParityLive(ctx, t, artifact, "h02", []string{"after_create.exists", "after_create.color", "after_update.description", "no_op", "after_delete.exists", "after_cleanup.exists"}, []githubParityRuntimeRun{
		{runtime: "opentofu", run: func(ctx context.Context, t *testing.T, repoName string) githubParityRuntimeResult {
			return runGitHubParityH02HCLRuntime(ctx, t, "opentofu", os.Getenv(githubParityTofuEnv), repoName)
		}},
		{runtime: "ramen", run: runGitHubParityH02RamenRuntime},
	})
}

func runGitHubParityH03Live(ctx context.Context, t *testing.T, artifact githubParityArtifact) githubParityLiveRecording {
	t.Helper()
	return runGitHubParityLive(ctx, t, artifact, "h03", []string{"after_create.exists", "after_create.path", "after_update.sha_changed", "no_op", "after_delete.exists", "after_cleanup.exists"}, []githubParityRuntimeRun{
		{runtime: "opentofu", run: func(ctx context.Context, t *testing.T, repoName string) githubParityRuntimeResult {
			return runGitHubParityH03HCLRuntime(ctx, t, "opentofu", os.Getenv(githubParityTofuEnv), repoName)
		}},
		{runtime: "ramen", run: runGitHubParityH03RamenRuntime},
	})
}

type githubParityRuntimeRun struct {
	runtime string
	run     func(context.Context, *testing.T, string) githubParityRuntimeResult
}

func runGitHubParityLive(ctx context.Context, t *testing.T, artifact githubParityArtifact, lane string, fields []string, runs []githubParityRuntimeRun) githubParityLiveRecording {
	t.Helper()
	started := time.Now()
	if err := preflightGitHubParityTools(ctx); err != nil {
		t.Fatalf("%s GitHub runtime preflight failed: %v", strings.ToUpper(lane), err)
	}
	if err := preflightGitHubParity(ctx); err != nil {
		t.Fatalf("%s GitHub preflight failed: %v", strings.ToUpper(lane), err)
	}
	suffix := githubParityRunSuffix()
	var observations []githubParityRuntimeObservation
	var failures []githubParityRuntimeFailure
	for _, run := range runs {
		repoName := githubParityRepositoryName(lane, run.runtime, suffix)
		result := timedGitHubParityRuntime(run.runtime, func() githubParityRuntimeResult {
			return run.run(ctx, t, repoName)
		})
		if result.Failure != nil {
			failures = append(failures, *result.Failure)
			continue
		}
		observations = append(observations, result.Observation)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Logf("%s GitHub parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("%s GitHub provider parity did not complete for all runtimes", strings.ToUpper(lane))
	}
	for _, observation := range observations {
		assertGitHubParityLiveObservationSemantics(t, lane, observation)
		assertGitHubParityObservationSanitized(t, strings.ToUpper(lane), observation)
	}
	comparison := compareGitHubParityObservations(observations, fields)
	if !comparison.Matched {
		t.Fatalf("%s GitHub provider parity observations did not match: %#v", strings.ToUpper(lane), observations)
	}
	return githubParityLiveRecording{
		Version:      githubParityArtifactV1,
		Lane:         strings.ToUpper(lane),
		Scenario:     artifact.Scenarios[0].Name,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		DurationMS:   time.Since(started).Milliseconds(),
		Observations: observations,
		Comparison:   comparison,
	}
}

func timedGitHubParityRuntime(runtime string, run func() githubParityRuntimeResult) githubParityRuntimeResult {
	started := time.Now()
	result := run()
	if result.Failure == nil {
		result.Observation.DurationMS = time.Since(started).Milliseconds()
	}
	return result
}

func runGitHubParityH01HCLRuntime(ctx context.Context, t *testing.T, runtimeName, tool, repoName string) githubParityRuntimeResult {
	t.Helper()
	if err := validateGitHubParityRepositoryName(repoName, "h01"); err != nil {
		return githubParityFailure(runtimeName, "safety", err)
	}
	if err := deleteGitHubParityRepositoryIfExists(ctx, repoName, "h01"); err != nil {
		return githubParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteGitHubParityRepositoryIfExists(context.Background(), repoName, "h01"); err != nil {
			t.Logf("cleanup GitHub repository %s: %v", repoName, err)
		}
	})
	workDir := githubParityRuntimeWorkDir(t, runtimeName, repoName)
	mainPath := filepath.Join(workDir, "main.tf")
	if err := renderGitHubParityH01HCL(mainPath, "Ramen GitHub H01 parity fixture"); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	if err := writeGitHubParityTFVars(workDir, repoName); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), githubParityEnvForTools()...)
	if err := runGitHubParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return githubParityFailure(runtimeName, "init", err)
	}
	if err := runGitHubParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return githubParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeGitHubParityRepository(ctx, repoName, "h01")
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	planExit, _, err := runGitHubParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return githubParityFailure(runtimeName, "plan", err)
	}
	if err := renderGitHubParityH01HCL(mainPath, "Ramen GitHub H01 parity fixture updated"); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	if err := runGitHubParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return githubParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeGitHubParityRepository(ctx, repoName, "h01")
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	if err := runGitHubParityCommand(ctx, workDir, env, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return githubParityFailure(runtimeName, "destroy", err)
	}
	afterDelete, err := waitGitHubParityRepository(ctx, repoName, "h01", false)
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	return githubParityRuntimeResult{Observation: githubParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: repoName,
		Fields:   githubParityRepositoryLifecycleFields(afterCreate, afterUpdate, afterDelete, planExit == 0),
	}}
}

func runGitHubParityH02HCLRuntime(ctx context.Context, t *testing.T, runtimeName, tool, repoName string) githubParityRuntimeResult {
	t.Helper()
	if err := setupGitHubParityRepository(ctx, t, repoName, "h02"); err != nil {
		return githubParityFailure(runtimeName, "setup", err)
	}
	workDir := githubParityRuntimeWorkDir(t, runtimeName, repoName)
	mainPath := filepath.Join(workDir, "main.tf")
	if err := renderGitHubParityH02HCL(mainPath, "0e8a16", "Ramen GitHub H02 parity fixture"); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	if err := writeGitHubParityTFVars(workDir, repoName); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), githubParityEnvForTools()...)
	if err := runGitHubParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return githubParityFailure(runtimeName, "init", err)
	}
	if err := runGitHubParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return githubParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeGitHubParityLabel(ctx, repoName, githubParityH02LabelName(), "h02")
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	planExit, _, err := runGitHubParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return githubParityFailure(runtimeName, "plan", err)
	}
	if err := renderGitHubParityH02HCL(mainPath, "b60205", "Ramen GitHub H02 parity fixture updated"); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	if err := runGitHubParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return githubParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeGitHubParityLabel(ctx, repoName, githubParityH02LabelName(), "h02")
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	if err := runGitHubParityCommand(ctx, workDir, env, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return githubParityFailure(runtimeName, "destroy", err)
	}
	afterDelete, err := waitGitHubParityLabel(ctx, repoName, githubParityH02LabelName(), "h02", false)
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	afterCleanup, err := cleanupGitHubParityRepository(ctx, repoName, "h02")
	if err != nil {
		return githubParityFailure(runtimeName, "cleanup", err)
	}
	return githubParityRuntimeResult{Observation: githubParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: repoName,
		Fields:   githubParityLabelLifecycleFields(afterCreate, afterUpdate, afterDelete, afterCleanup, planExit == 0),
	}}
}

func runGitHubParityH03HCLRuntime(ctx context.Context, t *testing.T, runtimeName, tool, repoName string) githubParityRuntimeResult {
	t.Helper()
	if err := setupGitHubParityRepository(ctx, t, repoName, "h03"); err != nil {
		return githubParityFailure(runtimeName, "setup", err)
	}
	workDir := githubParityRuntimeWorkDir(t, runtimeName, repoName)
	mainPath := filepath.Join(workDir, "main.tf")
	if err := renderGitHubParityH03HCL(mainPath, "ramen h03 create", "Create Ramen H03 parity file"); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	if err := writeGitHubParityTFVars(workDir, repoName); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), githubParityEnvForTools()...)
	if err := runGitHubParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return githubParityFailure(runtimeName, "init", err)
	}
	if err := runGitHubParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return githubParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeGitHubParityFile(ctx, repoName, githubParityH03FilePath(), "h03")
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	planExit, _, err := runGitHubParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return githubParityFailure(runtimeName, "plan", err)
	}
	if err := renderGitHubParityH03HCL(mainPath, "ramen h03 update", "Update Ramen H03 parity file"); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	if err := runGitHubParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return githubParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeGitHubParityFile(ctx, repoName, githubParityH03FilePath(), "h03")
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	if err := runGitHubParityCommand(ctx, workDir, env, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return githubParityFailure(runtimeName, "destroy", err)
	}
	afterDelete, err := waitGitHubParityFile(ctx, repoName, githubParityH03FilePath(), "h03", false)
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	afterCleanup, err := cleanupGitHubParityRepository(ctx, repoName, "h03")
	if err != nil {
		return githubParityFailure(runtimeName, "cleanup", err)
	}
	return githubParityRuntimeResult{Observation: githubParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: repoName,
		Fields:   githubParityFileLifecycleFields(afterCreate, afterUpdate, afterDelete, afterCleanup, planExit == 0),
	}}
}

func runGitHubParityH01RamenRuntime(ctx context.Context, t *testing.T, repoName string) githubParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := validateGitHubParityRepositoryName(repoName, "h01"); err != nil {
		return githubParityFailure(runtimeName, "safety", err)
	}
	if err := deleteGitHubParityRepositoryIfExists(ctx, repoName, "h01"); err != nil {
		return githubParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteGitHubParityRepositoryIfExists(context.Background(), repoName, "h01"); err != nil {
			t.Logf("cleanup GitHub repository %s: %v", repoName, err)
		}
	})
	workDir := githubParityRuntimeWorkDir(t, runtimeName, repoName)
	createProjectPath := filepath.Join(workDir, "create", "project.uws.yaml")
	updateProjectPath := filepath.Join(workDir, "update", "project.uws.yaml")
	openAPIPath, err := filepath.Abs("testdata/api-sources/github-repos-openapi.json")
	if err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	if err := renderGitHubParityProject(filepath.Join(githubParityFixtureRoot, "h01", "ramen", "project.uws.yaml"), createProjectPath, repoName, "Ramen GitHub H01 parity fixture", "", "", openAPIPath); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	if err := renderGitHubParityProject(filepath.Join(githubParityFixtureRoot, "h01", "ramen", "project.uws.yaml"), updateProjectPath, repoName, "Ramen GitHub H01 parity fixture updated", "", "", openAPIPath); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	exec := githubParityUdonExecutor(workDir, repoName, "h01")
	if err := buildAndApplyGitHubParityPlan(ctx, createProjectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), exec); err != nil {
		return githubParityFailure(runtimeName, "apply", err)
	}
	if err := requireGitHubParityStateResource(ctx, statePath, "github_repository.repo"); err != nil {
		return githubParityFailure(runtimeName, "state", err)
	}
	afterCreate, err := observeGitHubParityRepository(ctx, repoName, "h01")
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ProjectPath: createProjectPath, StatePath: statePath, Action: "plan", OutPath: filepath.Join(workDir, "noop-plan.json")})
	if err != nil {
		return githubParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1
	if !noOp && strings.TrimSpace(os.Getenv("RAMEN_GITHUB_DEBUG_DIR")) != "" {
		return githubParityFailure(runtimeName, "plan-noop", fmt.Errorf("Ramen no-op plan summary = %+v", planResult.Plan.Summary))
	}
	if err := buildAndApplyGitHubParityPlan(ctx, createProjectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), exec); err != nil {
		return githubParityFailure(runtimeName, "read", err)
	}
	if err := requireGitHubParityStateResource(ctx, statePath, "github_repository.repo"); err != nil {
		return githubParityFailure(runtimeName, "state-after-read", err)
	}
	if err := buildAndApplyGitHubParityPlan(ctx, updateProjectPath, statePath, "update", filepath.Join(workDir, "update-plan.json"), exec); err != nil {
		return githubParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeGitHubParityRepository(ctx, repoName, "h01")
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	if err := buildAndApplyGitHubParityPlan(ctx, updateProjectPath, statePath, "delete", filepath.Join(workDir, "delete-plan.json"), exec); err != nil {
		return githubParityFailure(runtimeName, "delete", err)
	}
	afterDelete, err := waitGitHubParityRepository(ctx, repoName, "h01", false)
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	return githubParityRuntimeResult{Observation: githubParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: repoName,
		Fields:   githubParityRepositoryLifecycleFields(afterCreate, afterUpdate, afterDelete, noOp),
	}}
}

func runGitHubParityH02RamenRuntime(ctx context.Context, t *testing.T, repoName string) githubParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := setupGitHubParityRepository(ctx, t, repoName, "h02"); err != nil {
		return githubParityFailure(runtimeName, "setup", err)
	}
	workDir := githubParityRuntimeWorkDir(t, runtimeName, repoName)
	createProjectPath := filepath.Join(workDir, "create", "project.uws.yaml")
	updateProjectPath := filepath.Join(workDir, "update", "project.uws.yaml")
	openAPIPath, err := filepath.Abs("testdata/api-sources/github-repos-openapi.json")
	if err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	if err := renderGitHubParityProject(filepath.Join(githubParityFixtureRoot, "h02", "ramen", "project.uws.yaml"), createProjectPath, repoName, "", "0e8a16", "Ramen GitHub H02 parity fixture", openAPIPath); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	if err := renderGitHubParityProject(filepath.Join(githubParityFixtureRoot, "h02", "ramen", "project.uws.yaml"), updateProjectPath, repoName, "", "b60205", "Ramen GitHub H02 parity fixture updated", openAPIPath); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	exec := githubParityUdonExecutor(workDir, repoName, "h02")
	if err := buildAndApplyGitHubParityPlan(ctx, createProjectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), exec); err != nil {
		return githubParityFailure(runtimeName, "apply", err)
	}
	if err := requireGitHubParityStateResource(ctx, statePath, "github_issue_label.label"); err != nil {
		return githubParityFailure(runtimeName, "state", err)
	}
	afterCreate, err := observeGitHubParityLabel(ctx, repoName, githubParityH02LabelName(), "h02")
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ProjectPath: createProjectPath, StatePath: statePath, Action: "plan", OutPath: filepath.Join(workDir, "noop-plan.json")})
	if err != nil {
		return githubParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1
	if err := buildAndApplyGitHubParityPlan(ctx, createProjectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), exec); err != nil {
		return githubParityFailure(runtimeName, "read", err)
	}
	if err := requireGitHubParityStateResource(ctx, statePath, "github_issue_label.label"); err != nil {
		return githubParityFailure(runtimeName, "state-after-read", err)
	}
	if err := buildAndApplyGitHubParityPlan(ctx, updateProjectPath, statePath, "update", filepath.Join(workDir, "update-plan.json"), exec); err != nil {
		return githubParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeGitHubParityLabel(ctx, repoName, githubParityH02LabelName(), "h02")
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	if err := buildAndApplyGitHubParityPlan(ctx, updateProjectPath, statePath, "delete", filepath.Join(workDir, "delete-plan.json"), exec); err != nil {
		return githubParityFailure(runtimeName, "delete", err)
	}
	afterDelete, err := waitGitHubParityLabel(ctx, repoName, githubParityH02LabelName(), "h02", false)
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	afterCleanup, err := cleanupGitHubParityRepository(ctx, repoName, "h02")
	if err != nil {
		return githubParityFailure(runtimeName, "cleanup", err)
	}
	return githubParityRuntimeResult{Observation: githubParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: repoName,
		Fields:   githubParityLabelLifecycleFields(afterCreate, afterUpdate, afterDelete, afterCleanup, noOp),
	}}
}

func runGitHubParityH03RamenRuntime(ctx context.Context, t *testing.T, repoName string) githubParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := setupGitHubParityRepository(ctx, t, repoName, "h03"); err != nil {
		return githubParityFailure(runtimeName, "setup", err)
	}
	workDir := githubParityRuntimeWorkDir(t, runtimeName, repoName)
	createProjectPath := filepath.Join(workDir, "create", "project.uws.yaml")
	updateProjectPath := filepath.Join(workDir, "update", "project.uws.yaml")
	openAPIPath, err := filepath.Abs("testdata/api-sources/github-repos-openapi.json")
	if err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	if err := renderGitHubParityProject(filepath.Join(githubParityFixtureRoot, "h03", "ramen", "project.uws.yaml"), createProjectPath, repoName, "ramen h03 create", "", "Create Ramen H03 parity file", openAPIPath); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	if err := renderGitHubParityProject(filepath.Join(githubParityFixtureRoot, "h03", "ramen", "project.uws.yaml"), updateProjectPath, repoName, "ramen h03 update", "", "Update Ramen H03 parity file", openAPIPath); err != nil {
		return githubParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	exec := githubParityUdonExecutor(workDir, repoName, "h03")
	if err := buildAndApplyGitHubParityPlan(ctx, createProjectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), exec); err != nil {
		return githubParityFailure(runtimeName, "apply", err)
	}
	if err := requireGitHubParityStateResource(ctx, statePath, "github_repository_file.file"); err != nil {
		return githubParityFailure(runtimeName, "state", err)
	}
	afterCreate, err := observeGitHubParityFile(ctx, repoName, githubParityH03FilePath(), "h03")
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ProjectPath: createProjectPath, StatePath: statePath, Action: "plan", OutPath: filepath.Join(workDir, "noop-plan.json")})
	if err != nil {
		return githubParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1
	if err := buildAndApplyGitHubParityPlan(ctx, createProjectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), exec); err != nil {
		return githubParityFailure(runtimeName, "read", err)
	}
	if err := requireGitHubParityStateResource(ctx, statePath, "github_repository_file.file"); err != nil {
		return githubParityFailure(runtimeName, "state-after-read", err)
	}
	if err := buildAndApplyGitHubParityPlan(ctx, updateProjectPath, statePath, "update", filepath.Join(workDir, "update-plan.json"), exec); err != nil {
		return githubParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeGitHubParityFile(ctx, repoName, githubParityH03FilePath(), "h03")
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	if err := buildAndApplyGitHubParityPlan(ctx, updateProjectPath, statePath, "delete", filepath.Join(workDir, "delete-plan.json"), exec); err != nil {
		return githubParityFailure(runtimeName, "delete", err)
	}
	afterDelete, err := waitGitHubParityFile(ctx, repoName, githubParityH03FilePath(), "h03", false)
	if err != nil {
		return githubParityFailure(runtimeName, "observe", err)
	}
	afterCleanup, err := cleanupGitHubParityRepository(ctx, repoName, "h03")
	if err != nil {
		return githubParityFailure(runtimeName, "cleanup", err)
	}
	return githubParityRuntimeResult{Observation: githubParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: repoName,
		Fields:   githubParityFileLifecycleFields(afterCreate, afterUpdate, afterDelete, afterCleanup, noOp),
	}}
}

func buildAndApplyGitHubParityPlan(ctx context.Context, projectPath, statePath, action, planPath string, udonExecutor udon.Executor) error {
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

func requireGitHubParityStateResource(ctx context.Context, statePath, address string) error {
	store, err := state.Open(ctx, statePath)
	if err != nil {
		return fmt.Errorf("open state: %w", err)
	}
	defer store.Close()
	snap, err := store.CurrentResource(ctx, address)
	if err != nil {
		return fmt.Errorf("read current resource %s: %w", address, err)
	}
	if snap == nil {
		return fmt.Errorf("current resource %s was not recorded", address)
	}
	return nil
}

func githubParityUdonExecutor(workDir, repoName, lane string) udon.Executor {
	return udon.Executor{
		OutputDir: filepath.Join(workDir, "udon"),
		CredentialResolvers: map[string]func(context.Context) (string, error){
			"github_token": func(context.Context) (string, error) {
				return strings.TrimSpace(os.Getenv("UDON_CREDENTIAL_GITHUB_TOKEN")), nil
			},
		},
		OutputProjector: func(projectorCtx context.Context, req executor.Request, _ string) (executor.Result, error) {
			result := executor.Result{
				Address:   req.Action.Address,
				Operation: req.Action.Mapping.OperationID,
				Success:   true,
			}
			if req.Action.Action == "delete" {
				return result, nil
			}
			switch lane {
			case "h01":
				observed, err := observeGitHubParityRepository(projectorCtx, repoName, lane)
				if err != nil {
					return executor.Result{}, err
				}
				if !observed.Exists {
					result.Missing = true
					return result, nil
				}
				result.Identity = map[string]any{"owner": githubParityOwner(), "repository": observed.Name, "name": observed.Name}
				result.Computed = map[string]any{"full_name": observed.FullName, "visibility": observed.Visibility, "default_branch": observed.DefaultBranch}
			case "h02":
				observed, err := observeGitHubParityLabel(projectorCtx, repoName, githubParityH02LabelName(), lane)
				if err != nil {
					return executor.Result{}, err
				}
				if !observed.Exists {
					result.Missing = true
					return result, nil
				}
				result.Identity = map[string]any{"owner": githubParityOwner(), "repository": repoName, "name": observed.Name}
				result.Computed = map[string]any{"color": observed.Color, "description": observed.Description}
			case "h03":
				observed, err := observeGitHubParityFile(projectorCtx, repoName, githubParityH03FilePath(), lane)
				if err != nil {
					return executor.Result{}, err
				}
				if !observed.Exists {
					result.Missing = true
					return result, nil
				}
				result.Identity = map[string]any{"owner": githubParityOwner(), "repository": repoName, "file": observed.Path, "path": observed.Path, "sha": observed.SHA}
				result.Computed = map[string]any{"sha": observed.SHA}
			}
			return result, nil
		},
	}
}

func runGitHubParityPlan(ctx context.Context, dir string, env []string, tool string) (int, string, error) {
	if strings.TrimSpace(tool) == "" {
		return -1, "", fmt.Errorf("empty tool path")
	}
	cmd := osexec.CommandContext(ctx, tool, "plan", "-input=false", "-no-color", "-detailed-exitcode")
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	summary := lastNonEmptyLine(string(out))
	if err == nil {
		return 0, summary, nil
	}
	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
		return 2, summary, nil
	}
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), summary, fmt.Errorf("%s plan failed with exit %d: %w: %s", filepath.Base(tool), exitErr.ExitCode(), err, sanitizeGitHubParityOutput(string(out)))
	}
	return -1, summary, fmt.Errorf("%s plan failed: %w: %s", filepath.Base(tool), err, sanitizeGitHubParityOutput(string(out)))
}

func runGitHubParityCommand(ctx context.Context, dir string, env []string, tool string, args ...string) error {
	if strings.TrimSpace(tool) == "" {
		return fmt.Errorf("empty tool path for %s", strings.Join(args, " "))
	}
	cmd := osexec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", filepath.Base(tool), strings.Join(args, " "), err, sanitizeGitHubParityOutput(string(out)))
	}
	return nil
}

func renderGitHubParityH01HCL(dst, description string) error {
	data, err := os.ReadFile(filepath.Join(githubParityFixtureRoot, "h01", "hcl", "main.tf"))
	if err != nil {
		return err
	}
	out := strings.ReplaceAll(string(data), `description = "Ramen GitHub H01 parity fixture"`, `description = "`+description+`"`)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}

func renderGitHubParityH02HCL(dst, color, description string) error {
	data, err := os.ReadFile(filepath.Join(githubParityFixtureRoot, "h02", "hcl", "main.tf"))
	if err != nil {
		return err
	}
	out := strings.ReplaceAll(string(data), `color       = "0e8a16"`, `color       = "`+color+`"`)
	out = strings.ReplaceAll(out, `description = "Ramen GitHub H02 parity fixture"`, `description = "`+description+`"`)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}

func renderGitHubParityH03HCL(dst, content, message string) error {
	data, err := os.ReadFile(filepath.Join(githubParityFixtureRoot, "h03", "hcl", "main.tf"))
	if err != nil {
		return err
	}
	out := strings.ReplaceAll(string(data), `content        = "ramen h03 create"`, `content        = "`+content+`"`)
	out = strings.ReplaceAll(out, `commit_message = "Create Ramen H03 parity file"`, `commit_message = "`+message+`"`)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}

func renderGitHubParityProject(src, dst, repoName, primaryValue, color, description, openAPIPath string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	out := strings.ReplaceAll(string(data), "github-owner-placeholder", githubParityOwner())
	out = strings.ReplaceAll(out, "ramen-parity-h01-static", repoName)
	out = strings.ReplaceAll(out, "ramen-parity-h02-static", repoName)
	out = strings.ReplaceAll(out, "ramen-parity-h03-static", repoName)
	if strings.Contains(src, "/h01/") && primaryValue != "" {
		out = strings.ReplaceAll(out, "description: Ramen GitHub H01 parity fixture", "description: "+primaryValue)
	}
	if strings.Contains(src, "/h02/") {
		if color != "" {
			out = strings.ReplaceAll(out, "color: 0e8a16", "color: "+color)
		}
		if description != "" {
			out = strings.ReplaceAll(out, "description: Ramen GitHub H02 parity fixture", "description: "+description)
		}
	}
	if strings.Contains(src, "/h03/") {
		if primaryValue != "" {
			out = strings.ReplaceAll(out, "content: ramen h03 create", "content: "+primaryValue)
		}
		if description != "" {
			out = strings.ReplaceAll(out, "commit_message: Create Ramen H03 parity file", "commit_message: "+description)
		}
	}
	if strings.TrimSpace(openAPIPath) != "" {
		out = strings.ReplaceAll(out, "../../../../api-sources/github-repos-openapi.json", filepath.ToSlash(openAPIPath))
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}

func writeGitHubParityTFVars(dir, repoName string) error {
	vars := map[string]string{
		"github_owner":    githubParityOwner(),
		"repository_name": repoName,
	}
	data, err := json.MarshalIndent(vars, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, "terraform.tfvars.json"), data, 0o644)
}

func setupGitHubParityRepository(ctx context.Context, t *testing.T, repoName, lane string) error {
	t.Helper()
	if err := validateGitHubParityRepositoryName(repoName, lane); err != nil {
		return err
	}
	if err := deleteGitHubParityRepositoryIfExists(ctx, repoName, lane); err != nil {
		return err
	}
	t.Cleanup(func() {
		if err := deleteGitHubParityRepositoryIfExists(context.Background(), repoName, lane); err != nil {
			t.Logf("cleanup GitHub repository %s: %v", repoName, err)
		}
	})
	payload := map[string]any{
		"name":        repoName,
		"description": "Ramen GitHub " + strings.ToUpper(lane) + " parity setup repository",
		"visibility":  "private",
		"private":     true,
		"auto_init":   true,
		"has_issues":  true,
	}
	status, body, err := githubParityAPIRequest(ctx, http.MethodPost, "/orgs/"+url.PathEscape(githubParityOwner())+"/repos", payload)
	if err != nil {
		return err
	}
	if status < 200 || status > 299 {
		return fmt.Errorf("GitHub repository setup returned HTTP %d: %s", status, sanitizeGitHubParityOutput(string(body)))
	}
	_, err = waitGitHubParityRepository(ctx, repoName, lane, true)
	return err
}

func cleanupGitHubParityRepository(ctx context.Context, repoName, lane string) (githubParityRepositoryObservation, error) {
	if err := deleteGitHubParityRepositoryIfExists(ctx, repoName, lane); err != nil {
		return githubParityRepositoryObservation{}, err
	}
	return waitGitHubParityRepository(ctx, repoName, lane, false)
}

type githubParityRepositoryObservation struct {
	Exists        bool
	Name          string
	FullName      string
	Visibility    string
	Description   string
	DefaultBranch string
}

type githubParityLabelObservation struct {
	Exists      bool
	Name        string
	Color       string
	Description string
}

type githubParityFileObservation struct {
	Exists bool
	Path   string
	SHA    string
}

func observeGitHubParityRepository(ctx context.Context, repoName, lane string) (githubParityRepositoryObservation, error) {
	if err := validateGitHubParityRepositoryName(repoName, lane); err != nil {
		return githubParityRepositoryObservation{}, err
	}
	status, body, err := githubParityAPIRequest(ctx, http.MethodGet, "/repos/"+url.PathEscape(githubParityOwner())+"/"+url.PathEscape(repoName), nil)
	if err != nil {
		return githubParityRepositoryObservation{}, err
	}
	if status == http.StatusNotFound {
		return githubParityRepositoryObservation{Exists: false}, nil
	}
	if status < 200 || status > 299 {
		return githubParityRepositoryObservation{}, fmt.Errorf("GitHub repository get returned HTTP %d: %s", status, sanitizeGitHubParityOutput(string(body)))
	}
	var doc struct {
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Visibility    string `json:"visibility"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return githubParityRepositoryObservation{}, fmt.Errorf("decode GitHub repository observation: %w", err)
	}
	return githubParityRepositoryObservation{Exists: true, Name: doc.Name, FullName: doc.FullName, Visibility: doc.Visibility, Description: doc.Description, DefaultBranch: doc.DefaultBranch}, nil
}

func waitGitHubParityRepository(ctx context.Context, repoName, lane string, wantExists bool) (githubParityRepositoryObservation, error) {
	var last githubParityRepositoryObservation
	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		observed, err := observeGitHubParityRepository(ctx, repoName, lane)
		if err == nil && observed.Exists == wantExists {
			return observed, nil
		}
		last = observed
		lastErr = err
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if lastErr != nil {
		return last, lastErr
	}
	return last, fmt.Errorf("timed out waiting for GitHub repository %s exists=%t", repoName, wantExists)
}

func deleteGitHubParityRepositoryIfExists(ctx context.Context, repoName, lane string) error {
	if err := validateGitHubParityRepositoryName(repoName, lane); err != nil {
		return err
	}
	observed, err := observeGitHubParityRepository(ctx, repoName, lane)
	if err != nil {
		return err
	}
	if !observed.Exists {
		return nil
	}
	status, body, err := githubParityAPIRequest(ctx, http.MethodDelete, "/repos/"+url.PathEscape(githubParityOwner())+"/"+url.PathEscape(repoName), nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status < 200 || status > 299 {
		return fmt.Errorf("GitHub repository delete returned HTTP %d: %s", status, sanitizeGitHubParityOutput(string(body)))
	}
	_, err = waitGitHubParityRepository(ctx, repoName, lane, false)
	return err
}

func observeGitHubParityLabel(ctx context.Context, repoName, labelName, lane string) (githubParityLabelObservation, error) {
	if err := validateGitHubParityRepositoryName(repoName, lane); err != nil {
		return githubParityLabelObservation{}, err
	}
	status, body, err := githubParityAPIRequest(ctx, http.MethodGet, "/repos/"+url.PathEscape(githubParityOwner())+"/"+url.PathEscape(repoName)+"/labels/"+url.PathEscape(labelName), nil)
	if err != nil {
		return githubParityLabelObservation{}, err
	}
	if status == http.StatusNotFound {
		return githubParityLabelObservation{Exists: false}, nil
	}
	if status < 200 || status > 299 {
		return githubParityLabelObservation{}, fmt.Errorf("GitHub label get returned HTTP %d: %s", status, sanitizeGitHubParityOutput(string(body)))
	}
	var doc struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return githubParityLabelObservation{}, fmt.Errorf("decode GitHub label observation: %w", err)
	}
	return githubParityLabelObservation{Exists: true, Name: doc.Name, Color: normalizeGitHubParityColor(doc.Color), Description: doc.Description}, nil
}

func waitGitHubParityLabel(ctx context.Context, repoName, labelName, lane string, wantExists bool) (githubParityLabelObservation, error) {
	var last githubParityLabelObservation
	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		observed, err := observeGitHubParityLabel(ctx, repoName, labelName, lane)
		if err == nil && observed.Exists == wantExists {
			return observed, nil
		}
		last = observed
		lastErr = err
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if lastErr != nil {
		return last, lastErr
	}
	return last, fmt.Errorf("timed out waiting for GitHub label %s/%s exists=%t", repoName, labelName, wantExists)
}

func observeGitHubParityFile(ctx context.Context, repoName, path, lane string) (githubParityFileObservation, error) {
	if err := validateGitHubParityRepositoryName(repoName, lane); err != nil {
		return githubParityFileObservation{}, err
	}
	status, body, err := githubParityAPIRequest(ctx, http.MethodGet, "/repos/"+url.PathEscape(githubParityOwner())+"/"+url.PathEscape(repoName)+"/contents/"+url.PathEscape(path), nil)
	if err != nil {
		return githubParityFileObservation{}, err
	}
	if status == http.StatusNotFound {
		return githubParityFileObservation{Exists: false}, nil
	}
	if status < 200 || status > 299 {
		return githubParityFileObservation{}, fmt.Errorf("GitHub file get returned HTTP %d: %s", status, sanitizeGitHubParityOutput(string(body)))
	}
	var doc struct {
		Path string `json:"path"`
		SHA  string `json:"sha"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return githubParityFileObservation{}, fmt.Errorf("decode GitHub file observation: %w", err)
	}
	return githubParityFileObservation{Exists: true, Path: doc.Path, SHA: doc.SHA}, nil
}

func waitGitHubParityFile(ctx context.Context, repoName, path, lane string, wantExists bool) (githubParityFileObservation, error) {
	var last githubParityFileObservation
	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		observed, err := observeGitHubParityFile(ctx, repoName, path, lane)
		if err == nil && observed.Exists == wantExists {
			return observed, nil
		}
		last = observed
		lastErr = err
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if lastErr != nil {
		return last, lastErr
	}
	return last, fmt.Errorf("timed out waiting for GitHub file %s/%s exists=%t", repoName, path, wantExists)
}

func preflightGitHubParity(ctx context.Context) error {
	tokens := []struct {
		envName string
		value   string
	}{
		{envName: "GITHUB_TOKEN", value: os.Getenv("GITHUB_TOKEN")},
		{envName: "UDON_CREDENTIAL_GITHUB_TOKEN", value: os.Getenv("UDON_CREDENTIAL_GITHUB_TOKEN")},
	}
	seen := map[string]bool{}
	for _, token := range tokens {
		value := strings.TrimSpace(token.value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		if err := preflightGitHubParityToken(ctx, token.envName, value); err != nil {
			return err
		}
	}
	return nil
}

func preflightGitHubParityTools(ctx context.Context) error {
	tool, err := resolveGitHubParityExecutable(os.Getenv(githubParityTofuEnv))
	if err != nil {
		return fmt.Errorf("%s: %w", githubParityTofuEnv, err)
	}
	cmd := osexec.CommandContext(ctx, tool, "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s version failed: %w: %s", filepath.Base(tool), err, sanitizeGitHubParityOutput(string(out)))
	}
	return nil
}

func resolveGitHubParityExecutable(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("empty executable path")
	}
	if strings.ContainsRune(value, os.PathSeparator) {
		info, err := os.Stat(value)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", fmt.Errorf("%s is a directory", value)
		}
		if info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("%s is not executable", value)
		}
		return value, nil
	}
	return osexec.LookPath(value)
}

func preflightGitHubParityToken(ctx context.Context, envName, token string) error {
	if status, body, err := githubParityAPIRequestWithToken(ctx, token, http.MethodGet, "/user", nil); err != nil {
		return err
	} else if status < 200 || status > 299 {
		return fmt.Errorf("%s GitHub /user returned HTTP %d: %s", envName, status, sanitizeGitHubParityOutput(string(body)))
	}
	if status, body, err := githubParityAPIRequestWithToken(ctx, token, http.MethodGet, "/orgs/"+url.PathEscape(githubParityOwner()), nil); err != nil {
		return err
	} else if status < 200 || status > 299 {
		return fmt.Errorf("%s GitHub org preflight returned HTTP %d: %s", envName, status, sanitizeGitHubParityOutput(string(body)))
	}
	if status, body, err := githubParityAPIRequestWithToken(ctx, token, http.MethodGet, "/orgs/"+url.PathEscape(githubParityOwner())+"/repos?per_page=1&type=private", nil); err != nil {
		return err
	} else if status < 200 || status > 299 {
		return fmt.Errorf("%s GitHub org repos preflight returned HTTP %d: %s", envName, status, sanitizeGitHubParityOutput(string(body)))
	}
	return nil
}

func githubParityAPIRequest(ctx context.Context, method, path string, payload any) (int, []byte, error) {
	return githubParityAPIRequestWithToken(ctx, os.Getenv("GITHUB_TOKEN"), method, path, payload)
}

func githubParityAPIRequestWithToken(ctx context.Context, token, method, path string, payload any) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com"+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, data, nil
}

func githubParityRepositoryLifecycleFields(afterCreate, afterUpdate, afterDelete githubParityRepositoryObservation, noOp bool) map[string]any {
	return map[string]any{
		"after_create.exists":         afterCreate.Exists,
		"after_create.name":           afterCreate.Name,
		"after_create.visibility":     normalizeGitHubParityVisibility(afterCreate.Visibility),
		"after_create.default_branch": afterCreate.DefaultBranch,
		"after_update.exists":         afterUpdate.Exists,
		"after_update.description":    afterUpdate.Description,
		"no_op":                       noOp,
		"after_delete.exists":         afterDelete.Exists,
	}
}

func githubParityLabelLifecycleFields(afterCreate, afterUpdate, afterDelete githubParityLabelObservation, afterCleanup githubParityRepositoryObservation, noOp bool) map[string]any {
	return map[string]any{
		"after_create.exists":      afterCreate.Exists,
		"after_create.name":        afterCreate.Name,
		"after_create.color":       normalizeGitHubParityColor(afterCreate.Color),
		"after_update.exists":      afterUpdate.Exists,
		"after_update.color":       normalizeGitHubParityColor(afterUpdate.Color),
		"after_update.description": afterUpdate.Description,
		"no_op":                    noOp,
		"after_delete.exists":      afterDelete.Exists,
		"after_cleanup.exists":     afterCleanup.Exists,
	}
}

func githubParityFileLifecycleFields(afterCreate, afterUpdate, afterDelete githubParityFileObservation, afterCleanup githubParityRepositoryObservation, noOp bool) map[string]any {
	return map[string]any{
		"after_create.exists":      afterCreate.Exists,
		"after_create.path":        afterCreate.Path,
		"after_create.sha_present": strings.TrimSpace(afterCreate.SHA) != "",
		"after_update.exists":      afterUpdate.Exists,
		"after_update.sha_changed": afterCreate.SHA != "" && afterUpdate.SHA != "" && afterCreate.SHA != afterUpdate.SHA,
		"no_op":                    noOp,
		"after_delete.exists":      afterDelete.Exists,
		"after_cleanup.exists":     afterCleanup.Exists,
	}
}

func validateGitHubParityRepositoryName(name, lane string) error {
	prefix := "ramen-parity-" + lane + "-"
	if !strings.HasPrefix(name, prefix) {
		return fmt.Errorf("GitHub repository name %q must use %s* prefix", name, prefix)
	}
	if len(name) > 100 {
		return fmt.Errorf("GitHub repository name %q is too long", name)
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			continue
		}
		return fmt.Errorf("GitHub repository name %q contains unsupported character %q", name, r)
	}
	return nil
}

func githubParityRunSuffix() string {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err == nil {
		return hex.EncodeToString(suffix[:])
	}
	return fmt.Sprintf("%08x", uint32(time.Now().UTC().UnixNano()))
}

func githubParityRepositoryName(lane, runtime, suffix string) string {
	return "ramen-parity-" + lane + "-" + runtime + "-" + suffix
}

func githubParityOwner() string {
	return strings.TrimSpace(os.Getenv("GITHUB_OWNER"))
}

func githubParityH02LabelName() string {
	return "ramen-parity-h02"
}

func githubParityH03FilePath() string {
	return "ramen-parity-h03.txt"
}

func githubParityEnvForTools() []string {
	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	return []string{
		"GITHUB_OWNER=" + githubParityOwner(),
		"GITHUB_TOKEN=" + token,
		"GH_TOKEN=" + token,
	}
}

func normalizeGitHubParityVisibility(value string) string {
	if strings.TrimSpace(value) == "" {
		return "private"
	}
	return strings.ToLower(value)
}

func normalizeGitHubParityColor(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "#")
}

func sanitizeGitHubParityOutput(output string) string {
	replacements := []string{
		os.Getenv("GITHUB_TOKEN"),
		os.Getenv("UDON_CREDENTIAL_GITHUB_TOKEN"),
		githubParityOwner(),
	}
	for _, value := range replacements {
		if strings.TrimSpace(value) != "" {
			output = strings.ReplaceAll(output, value, "<redacted>")
		}
	}
	return output
}

func githubParityRuntimeWorkDir(t *testing.T, runtimeName, repoName string) string {
	t.Helper()
	debugRoot := strings.TrimSpace(os.Getenv("RAMEN_GITHUB_DEBUG_DIR"))
	if debugRoot == "" {
		return filepath.Join(t.TempDir(), runtimeName)
	}
	workDir := filepath.Join(debugRoot, repoName, runtimeName)
	if err := os.RemoveAll(workDir); err != nil {
		t.Fatalf("clear GitHub debug workdir: %v", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("create GitHub debug workdir: %v", err)
	}
	t.Logf("GitHub parity debug workdir: %s", workDir)
	return workDir
}
