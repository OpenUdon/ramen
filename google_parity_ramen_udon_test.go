//go:build googlelive && udon

package corpus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/ramen/apply"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/executor/udon"
	tfplan "github.com/OpenUdon/ramen/plan"
	"golang.org/x/oauth2/google"
)

func TestGoogleParityY02Render(t *testing.T) {
	workDir := t.TempDir()
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	discoveryPath, err := filepath.Abs("../apitools/catalog-openapi-cache/google-discovery/google-cloud-storage-discovery-v1.json")
	if err != nil {
		t.Fatalf("resolve Google Discovery path: %v", err)
	}
	if err := renderGoogleParityProject(filepath.Join(googleParityFixtureRoot, "y02", "ramen", "project.uws.yaml"), projectPath, "ramen-parity-y02-render", "ramen-parity-render-project", "/tmp/google-service-account.json", discoveryPath, ""); err != nil {
		t.Fatalf("render Y02 project: %v", err)
	}
	assertRenderedGoogleParityProject(t, projectPath, []string{"ramen-parity-y02-render", "google_service_account_file", "storage.buckets.get"})
	result, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: projectPath,
		StatePath:   filepath.Join(workDir, "state.db"),
		Action:      "read",
	})
	if err != nil {
		t.Fatalf("build rendered Y02 read plan: %v", err)
	}
	if result.Plan.Errored || len(result.Plan.Resources) != 1 || result.Plan.Resources[0].Mapping == nil || result.Plan.Resources[0].Mapping.OperationID != "storage.buckets.get" {
		t.Fatalf("rendered Y02 read plan unusable: %#v", result.Plan)
	}
}

func TestGoogleParityY03Render(t *testing.T) {
	workDir := t.TempDir()
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	discoveryPath, err := filepath.Abs("../apitools/catalog-openapi-cache/google-discovery/google-cloud-storage-discovery-v1.json")
	if err != nil {
		t.Fatalf("resolve Google Discovery path: %v", err)
	}
	if err := renderGoogleParityProject(filepath.Join(googleParityFixtureRoot, "y03", "ramen", "project.uws.yaml"), projectPath, "ramen-parity-y03-render", "ramen-parity-render-project", "/tmp/google-service-account.json", discoveryPath, "update"); err != nil {
		t.Fatalf("render Y03 project: %v", err)
	}
	assertRenderedGoogleParityProject(t, projectPath, []string{"ramen-parity-y03-render", "ramen-parity-render-project", "google_service_account_file", "storage.buckets.insert", "storage.buckets.patch"})
	for _, tc := range []struct {
		action      string
		operationID string
		summary     string
		seed        bool
	}{
		{action: "create", operationID: "storage.buckets.insert", summary: "create"},
		{action: "read", operationID: "storage.buckets.get", summary: "read"},
		{action: "create", operationID: "storage.buckets.patch", summary: "update", seed: true},
		{action: "delete", operationID: "storage.buckets.delete", summary: "delete", seed: true},
	} {
		statePath := filepath.Join(workDir, tc.action+tc.operationID+".db")
		if tc.seed {
			seedGoogleParityState(t, "y03", statePath)
		}
		result, err := tfplan.Build(context.Background(), tfplan.Options{
			ProjectPath: projectPath,
			StatePath:   statePath,
			Action:      tc.action,
		})
		if err != nil {
			t.Fatalf("build rendered Y03 %s plan: %v", tc.action, err)
		}
		if result.Plan.Errored || len(result.Plan.Resources) != 1 {
			t.Fatalf("rendered Y03 %s plan unusable: %#v", tc.action, result.Plan)
		}
		resource := result.Plan.Resources[0]
		if resource.Mapping == nil || resource.Mapping.OperationID != tc.operationID {
			t.Fatalf("rendered Y03 %s operation = %#v, want %s", tc.action, resource.Mapping, tc.operationID)
		}
		if !googleParitySummaryHasOne(result.Plan.Summary, tc.summary) {
			t.Fatalf("rendered Y03 %s summary = %#v, want one %s action", tc.action, result.Plan.Summary, tc.summary)
		}
	}
}

func assertRenderedGoogleParityProject(t *testing.T, path string, expected []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered Google project: %v", err)
	}
	text := string(data)
	for _, want := range expected {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered Google project missing %q:\n%s", want, text)
		}
	}
}

func runGoogleParityY02Live(ctx context.Context, t *testing.T, artifact googleParityArtifact) googleParityLiveRecording {
	t.Helper()
	started := time.Now()
	bucketName := googleParityExistingBucket()
	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, string) googleParityRuntimeResult
	}{
		{runtime: "opentofu", run: runGoogleParityY02OpenTofuRuntime},
		{runtime: "ramen", run: runGoogleParityY02RamenRuntime},
	}
	var observations []googleParityRuntimeObservation
	var failures []googleParityRuntimeFailure
	for _, run := range runs {
		result := timedGoogleParityRuntime(run.runtime, func() googleParityRuntimeResult {
			return run.run(ctx, t, bucketName)
		})
		if result.Failure != nil {
			failures = append(failures, *result.Failure)
			continue
		}
		observations = append(observations, result.Observation)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Logf("%s Google parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("Y02 Google provider parity did not complete for all runtimes")
	}
	fields := []string{"exists", "name_matches", "id_present", "location_present", "uniform_bucket_level_access_field_present"}
	comparison := compareGoogleParityObservations(observations, fields)
	if !comparison.Matched {
		t.Fatalf("Y02 Google provider parity observations did not match: %#v", observations)
	}
	return googleParityLiveRecording{
		Version:      googleParityArtifactV1,
		Lane:         "Y02",
		Scenario:     artifact.Scenarios[0].Name,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		DurationMS:   time.Since(started).Milliseconds(),
		Observations: observations,
		Comparison:   comparison,
	}
}

func runGoogleParityY03Live(ctx context.Context, t *testing.T, artifact googleParityArtifact) googleParityLiveRecording {
	t.Helper()
	started := time.Now()
	suffix := googleParityY03RunSuffix()
	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, string) googleParityRuntimeResult
	}{
		{runtime: "opentofu", run: runGoogleParityY03OpenTofuRuntime},
		{runtime: "ramen", run: runGoogleParityY03RamenRuntime},
	}
	var observations []googleParityRuntimeObservation
	var failures []googleParityRuntimeFailure
	for _, run := range runs {
		bucketName := googleParityY03BucketName(run.runtime, suffix)
		result := timedGoogleParityRuntime(run.runtime, func() googleParityRuntimeResult {
			return run.run(ctx, t, bucketName)
		})
		if result.Failure != nil {
			failures = append(failures, *result.Failure)
			continue
		}
		observations = append(observations, result.Observation)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Logf("%s Google parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("Y03 Google provider parity did not complete for all runtimes")
	}
	fields := []string{"exists", "after_create.exists", "after_update.exists", "labels.ramen_parity_phase_update_observable", "no_op", "after_destroy.exists"}
	comparison := compareGoogleParityObservations(observations, fields)
	if !comparison.Matched {
		t.Fatalf("Y03 Google provider parity observations did not match: %#v", observations)
	}
	return googleParityLiveRecording{
		Version:      googleParityArtifactV1,
		Lane:         "Y03",
		Scenario:     artifact.Scenarios[0].Name,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		DurationMS:   time.Since(started).Milliseconds(),
		Observations: observations,
		Comparison:   comparison,
	}
}

func timedGoogleParityRuntime(runtime string, run func() googleParityRuntimeResult) googleParityRuntimeResult {
	started := time.Now()
	result := run()
	if result.Failure == nil {
		result.Observation.DurationMS = time.Since(started).Milliseconds()
	}
	return result
}

func runGoogleParityY02RamenRuntime(ctx context.Context, t *testing.T, bucketName string) googleParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := validateGoogleParityBucketName(bucketName); err != nil {
		return googleParityFailure(runtimeName, "safety", err)
	}
	workDir := filepath.Join(t.TempDir(), runtimeName)
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	discoveryPath, err := filepath.Abs("../apitools/catalog-openapi-cache/google-discovery/google-cloud-storage-discovery-v1.json")
	if err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	if err := renderGoogleParityProject(filepath.Join(googleParityFixtureRoot, "y02", "ramen", "project.uws.yaml"), projectPath, bucketName, googleParityProject(), os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"), discoveryPath, ""); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	udonExecutor := googleParityUdonExecutor(workDir, bucketName)
	if err := buildAndApplyGoogleParityPlan(ctx, projectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), udonExecutor); err != nil {
		return googleParityFailure(runtimeName, "read", err)
	}
	observed, err := observeGoogleParityBucket(ctx, bucketName)
	if err != nil {
		return googleParityFailure(runtimeName, "observe", err)
	}
	return googleParityRuntimeResult{Observation: googleParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: bucketName,
		Fields:   googleParityBucketReadFields(observed, bucketName),
	}}
}

func runGoogleParityY03RamenRuntime(ctx context.Context, t *testing.T, bucketName string) googleParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := validateGoogleParityDisposableBucketName(bucketName, "y03"); err != nil {
		return googleParityFailure(runtimeName, "safety", err)
	}
	if observed, err := observeGoogleParityBucket(ctx, bucketName); err != nil {
		return googleParityFailure(runtimeName, "preflight", err)
	} else if observed.Exists {
		return googleParityFailure(runtimeName, "safety", fmt.Errorf("disposable bucket %s already exists", bucketName))
	}
	t.Cleanup(func() {
		if err := deleteGoogleParityBucketIfExists(context.Background(), bucketName); err != nil {
			t.Logf("cleanup Google Cloud Storage bucket %s: %v", bucketName, err)
		}
	})
	workDir := filepath.Join(t.TempDir(), runtimeName)
	discoveryPath, err := filepath.Abs("../apitools/catalog-openapi-cache/google-discovery/google-cloud-storage-discovery-v1.json")
	if err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	createProjectPath := filepath.Join(workDir, "ramen-create", "project.uws.yaml")
	if err := renderGoogleParityProject(filepath.Join(googleParityFixtureRoot, "y03", "ramen", "project.uws.yaml"), createProjectPath, bucketName, googleParityProject(), os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"), discoveryPath, "create"); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	updateProjectPath := filepath.Join(workDir, "ramen-update", "project.uws.yaml")
	if err := renderGoogleParityProject(filepath.Join(googleParityFixtureRoot, "y03", "ramen", "project.uws.yaml"), updateProjectPath, bucketName, googleParityProject(), os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"), discoveryPath, "update"); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	udonExecutor := googleParityUdonExecutor(workDir, bucketName)
	if err := buildAndApplyGoogleParityPlan(ctx, createProjectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), udonExecutor); err != nil {
		return googleParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeGoogleParityBucket(ctx, bucketName)
	if err != nil {
		return googleParityFailure(runtimeName, "observe", err)
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ProjectPath: createProjectPath, StatePath: statePath})
	if err != nil {
		return googleParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1
	if err := buildAndApplyGoogleParityPlan(ctx, updateProjectPath, statePath, "create", filepath.Join(workDir, "update-plan.json"), udonExecutor); err != nil {
		return googleParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeGoogleParityBucket(ctx, bucketName)
	if err != nil {
		return googleParityFailure(runtimeName, "observe", err)
	}
	if err := buildAndApplyGoogleParityPlan(ctx, updateProjectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), udonExecutor); err != nil {
		return googleParityFailure(runtimeName, "read", err)
	}
	if err := buildAndApplyGoogleParityPlan(ctx, updateProjectPath, statePath, "delete", filepath.Join(workDir, "delete-plan.json"), udonExecutor); err != nil {
		return googleParityFailure(runtimeName, "delete", err)
	}
	afterDestroy, err := waitGoogleParityBucket(ctx, bucketName, false)
	if err != nil {
		return googleParityFailure(runtimeName, "observe", err)
	}
	return googleParityRuntimeResult{Observation: googleParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: bucketName,
		Fields:   googleParityBucketMutationFields(afterCreate, afterUpdate, afterDestroy, noOp),
	}}
}

func googleParityUdonExecutor(workDir, bucketName string) udon.Executor {
	return udon.Executor{
		OutputDir: filepath.Join(workDir, "udon"),
		CredentialResolvers: map[string]func(context.Context) (string, error){
			"google_oauth2": func(ctx context.Context) (string, error) {
				return googleParityAccessToken(ctx)
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
			observed, err := observeGoogleParityBucket(projectorCtx, bucketName)
			if err != nil {
				return executor.Result{}, err
			}
			if !observed.Exists {
				result.Missing = true
				return result, nil
			}
			result.Identity = map[string]any{"bucket_name": observed.Name}
			result.Computed = map[string]any{
				"id_present":       strings.TrimSpace(observed.ID) != "",
				"location_present": strings.TrimSpace(observed.Location) != "",
			}
			return result, nil
		},
	}
}

func googleParityAccessToken(ctx context.Context) (string, error) {
	data, err := os.ReadFile(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	if err != nil {
		return "", err
	}
	creds, err := google.CredentialsFromJSON(ctx, data, "https://www.googleapis.com/auth/devstorage.full_control")
	if err != nil {
		return "", err
	}
	token, err := creds.TokenSource.Token()
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func buildAndApplyGoogleParityPlan(ctx context.Context, projectPath, statePath, action, planPath string, udonExecutor udon.Executor) error {
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

func renderGoogleParityProject(src, dst, bucketName, projectID, serviceAccountFile, discoveryPath, phase string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := validateGoogleParityBucketName(bucketName); err != nil {
		return err
	}
	if strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("Google project ID is required")
	}
	if strings.TrimSpace(serviceAccountFile) == "" {
		return fmt.Errorf("Google service account file is required")
	}
	out := string(data)
	for _, placeholder := range []string{"ramen-parity-y02-existing", "ramen-parity-y03-static"} {
		out = strings.ReplaceAll(out, placeholder, bucketName)
	}
	out = strings.ReplaceAll(out, "ramen-parity-y03-fixture-project", projectID)
	if phase != "" {
		out = strings.ReplaceAll(out, "ramen_parity_phase: create", "ramen_parity_phase: "+phase)
	}
	if strings.TrimSpace(discoveryPath) != "" {
		out = strings.ReplaceAll(out, "../../../../../../apitools/catalog-openapi-cache/google-discovery/google-cloud-storage-discovery-v1.json", filepath.ToSlash(discoveryPath))
	}
	appendix := "serverUrl: https://storage.googleapis.com/storage/v1\n    appendices:\n      google_service_account_file: " + strconv.Quote(filepath.ToSlash(serviceAccountFile))
	out = strings.Replace(out, "serverUrl: https://storage.googleapis.com/storage/v1", appendix, 1)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}
