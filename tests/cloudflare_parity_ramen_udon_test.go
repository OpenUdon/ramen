//go:build cloudflarelive && udon

package corpus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	"github.com/genelet/udon/generator"
)

func TestCloudflareParityC01Render(t *testing.T) {
	assertRenderedCloudflareParityLane(t, "c01", []cloudflareParityRenderedPlanCase{
		{action: "create", operationID: "r2-create-bucket", summary: "create"},
		{action: "read", operationID: "r2-get-bucket", summary: "read"},
		{action: "update", operationID: "r2-patch-bucket", summary: "update", seed: true},
		{action: "delete", operationID: "r2-delete-bucket", summary: "delete", seed: true},
	})
	assertCloudflareParityC01UdonCreateShape(t)
}

func TestCloudflareParityC02Render(t *testing.T) {
	assertRenderedCloudflareParityLane(t, "c02", []cloudflareParityRenderedPlanCase{
		{action: "create", operationID: "r2-create-bucket", summary: "create"},
		{action: "read", operationID: "r2-get-bucket", summary: "read"},
		{action: "delete", operationID: "r2-delete-bucket", summary: "delete", seed: true},
	})
}

func TestCloudflareParityC03Render(t *testing.T) {
	assertRenderedCloudflareParityLane(t, "c03", []cloudflareParityRenderedPlanCase{
		{action: "create", operationID: "r2-create-bucket", summary: "create"},
		{action: "read", operationID: "r2-get-bucket", summary: "read"},
		{action: "update", operationID: "r2-patch-bucket", summary: "update", seed: true},
		{action: "delete", operationID: "r2-delete-bucket", summary: "delete", seed: true},
	})
}

func TestCloudflareParityC04Render(t *testing.T) {
	assertRenderedCloudflareParityLane(t, "c04", []cloudflareParityRenderedPlanCase{
		{action: "create", operationID: "d1-create-database", summary: "create"},
		{action: "read", operationID: "d1-get-database", summary: "read"},
	})
}

func TestCloudflareParityC05Render(t *testing.T) {
	assertRenderedCloudflareParityLane(t, "c05", []cloudflareParityRenderedPlanCase{
		{action: "create", operationID: "d1-create-database", summary: "create"},
		{action: "read", operationID: "d1-get-database", summary: "read"},
		{action: "delete", operationID: "d1-delete-database", summary: "delete", seed: true},
	})
}

func TestCloudflareParityC06Render(t *testing.T) {
	assertRenderedCloudflareParityLane(t, "c06", []cloudflareParityRenderedPlanCase{
		{action: "create", operationID: "d1-create-database", summary: "create"},
		{action: "read", operationID: "d1-get-database", summary: "read"},
		{action: "update", operationID: "d1-update-database", summary: "update", seed: true},
		{action: "delete", operationID: "d1-delete-database", summary: "delete", seed: true},
	})
}

type cloudflareParityRenderedPlanCase struct {
	action      string
	operationID string
	summary     string
	seed        bool
}

func assertRenderedCloudflareParityLane(t *testing.T, lane string, cases []cloudflareParityRenderedPlanCase) {
	t.Helper()
	workDir := t.TempDir()
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	openAPIPath, err := cloudflareParityOpenAPIPathForLane(lane)
	if err != nil {
		t.Fatalf("resolve Cloudflare OpenAPI path: %v", err)
	}
	resourceName := "ramen-parity-" + lane + "-static"
	if err := renderCloudflareParityProject(filepath.Join(cloudflareParityFixtureRoot, lane, "ramen", "project.uws.yaml"), projectPath, "cloudflare-account-placeholder", resourceName, openAPIPath); err != nil {
		t.Fatalf("render %s project: %v", strings.ToUpper(lane), err)
	}
	data, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatalf("read rendered %s project: %v", strings.ToUpper(lane), err)
	}
	for _, want := range []string{resourceName, "binding: cloudflare_api_token", "serverUrl: https://api.cloudflare.com/client/v4"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("rendered %s project missing %q:\n%s", strings.ToUpper(lane), want, string(data))
		}
	}
	for _, tc := range cases {
		statePath := filepath.Join(workDir, tc.action+tc.operationID+".db")
		if tc.seed {
			seedCloudflareParityState(t, lane, statePath)
		}
		result, err := tfplan.Build(context.Background(), tfplan.Options{
			ProjectPath: projectPath,
			StatePath:   statePath,
			Action:      tc.action,
		})
		if err != nil {
			t.Fatalf("build rendered %s %s plan: %v", strings.ToUpper(lane), tc.action, err)
		}
		if result.Plan.Errored || len(result.Plan.Resources) != 1 {
			t.Fatalf("rendered %s %s plan unusable: %#v", strings.ToUpper(lane), tc.action, result.Plan)
		}
		resource := result.Plan.Resources[0]
		if resource.Mapping == nil || resource.Mapping.OperationID != tc.operationID {
			t.Fatalf("rendered %s %s operation = %#v, want %s", strings.ToUpper(lane), tc.action, resource.Mapping, tc.operationID)
		}
		if !cloudflareParitySummaryHasOne(result.Plan.Summary, tc.summary) {
			t.Fatalf("rendered %s %s summary = %#v, want one %s action", strings.ToUpper(lane), tc.action, result.Plan.Summary, tc.summary)
		}
	}
}

func renderCloudflareParityProject(src, dst, accountID, resourceName, openAPIPath string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	out := strings.ReplaceAll(string(data), "cloudflare-account-placeholder", accountID)
	out = strings.ReplaceAll(out, "ramen-parity-c01-static", resourceName)
	out = strings.ReplaceAll(out, "ramen-parity-c02-static", resourceName)
	out = strings.ReplaceAll(out, "ramen-parity-c03-static", resourceName)
	out = strings.ReplaceAll(out, "ramen-parity-c04-static", resourceName)
	out = strings.ReplaceAll(out, "ramen-parity-c05-static", resourceName)
	out = strings.ReplaceAll(out, "ramen-parity-c06-static", resourceName)
	if strings.TrimSpace(openAPIPath) != "" {
		out = strings.ReplaceAll(out, "../../../../api-sources/cloudflare-r2-d1-openapi.json", filepath.ToSlash(openAPIPath))
		out = strings.ReplaceAll(out, "../../../../api-sources/cloudflare-r2-d1-delete-openapi.json", filepath.ToSlash(openAPIPath))
		out = strings.ReplaceAll(out, "../../../../api-sources/cloudflare-r2-d1-update-openapi.json", filepath.ToSlash(openAPIPath))
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}

func cloudflareParityOpenAPIPathForLane(lane string) (string, error) {
	source := "testdata/api-sources/cloudflare-r2-d1-openapi.json"
	if strings.ToLower(strings.TrimSpace(lane)) == "c05" {
		source = "testdata/api-sources/cloudflare-r2-d1-delete-openapi.json"
	}
	if strings.ToLower(strings.TrimSpace(lane)) == "c06" {
		source = "testdata/api-sources/cloudflare-r2-d1-update-openapi.json"
	}
	return filepath.Abs(source)
}

func runCloudflareParityC01Live(ctx context.Context, t *testing.T, artifact cloudflareParityArtifact) cloudflareParityLiveRecording {
	t.Helper()
	started := time.Now()
	suffix := cloudflareParityRunSuffix()
	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, string) cloudflareParityRuntimeResult
	}{
		{runtime: "opentofu", run: runCloudflareParityC01OpenTofuRuntime},
		{runtime: "terraform", run: runCloudflareParityC01TerraformRuntime},
		{runtime: "ramen", run: runCloudflareParityC01RamenRuntime},
	}
	var observations []cloudflareParityRuntimeObservation
	var failures []cloudflareParityRuntimeFailure
	for _, run := range runs {
		bucketName := cloudflareParityBucketName("c01", run.runtime, suffix)
		result := timedCloudflareParityRuntime(run.runtime, func() cloudflareParityRuntimeResult {
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
			t.Logf("%s Cloudflare parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("C01 Cloudflare provider parity did not complete for all runtimes")
	}
	fields := []string{"after_create.exists", "after_update.storage_class", "no_op", "after_delete.exists"}
	comparison := compareCloudflareParityObservations(observations, fields)
	if !comparison.Matched {
		t.Fatalf("C01 Cloudflare provider parity observations did not match: %#v", observations)
	}
	return cloudflareParityLiveRecording{
		Version:      cloudflareParityArtifactV1,
		Lane:         "C01",
		Scenario:     artifact.Scenarios[0].Name,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		DurationMS:   time.Since(started).Milliseconds(),
		Observations: observations,
		Comparison:   comparison,
	}
}

func runCloudflareParityC02Live(ctx context.Context, t *testing.T, artifact cloudflareParityArtifact) cloudflareParityLiveRecording {
	t.Helper()
	started := time.Now()
	suffix := cloudflareParityRunSuffix()
	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, string) cloudflareParityRuntimeResult
	}{
		{runtime: "opentofu", run: runCloudflareParityC02OpenTofuRuntime},
		{runtime: "terraform", run: runCloudflareParityC02TerraformRuntime},
		{runtime: "ramen", run: runCloudflareParityC02RamenRuntime},
	}
	var observations []cloudflareParityRuntimeObservation
	var failures []cloudflareParityRuntimeFailure
	for _, run := range runs {
		bucketName := cloudflareParityBucketName("c02", run.runtime, suffix)
		result := timedCloudflareParityRuntime(run.runtime, func() cloudflareParityRuntimeResult {
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
			t.Logf("%s Cloudflare parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("C02 Cloudflare provider parity did not complete for all runtimes")
	}
	fields := []string{"after_create.exists", "after_out_of_band_delete.exists", "read_missing.missing", "after_cleanup.exists"}
	comparison := compareCloudflareParityObservations(observations, fields)
	if !comparison.Matched {
		t.Fatalf("C02 Cloudflare provider parity observations did not match: %#v", observations)
	}
	return cloudflareParityLiveRecording{
		Version:      cloudflareParityArtifactV1,
		Lane:         "C02",
		Scenario:     artifact.Scenarios[0].Name,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		DurationMS:   time.Since(started).Milliseconds(),
		Observations: observations,
		Comparison:   comparison,
	}
}

func runCloudflareParityC03Live(ctx context.Context, t *testing.T, artifact cloudflareParityArtifact) cloudflareParityLiveRecording {
	t.Helper()
	started := time.Now()
	suffix := cloudflareParityRunSuffix()
	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, string) cloudflareParityRuntimeResult
	}{
		{runtime: "opentofu", run: runCloudflareParityC03OpenTofuRuntime},
		{runtime: "terraform", run: runCloudflareParityC03TerraformRuntime},
		{runtime: "ramen", run: runCloudflareParityC03RamenRuntime},
	}
	var observations []cloudflareParityRuntimeObservation
	var failures []cloudflareParityRuntimeFailure
	for _, run := range runs {
		bucketName := cloudflareParityBucketName("c03", run.runtime, suffix)
		result := timedCloudflareParityRuntime(run.runtime, func() cloudflareParityRuntimeResult {
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
			t.Logf("%s Cloudflare parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("C03 Cloudflare provider parity did not complete for all runtimes")
	}
	fields := []string{"after_create.exists", "after_create.location", "after_create.storage_class", "after_create.jurisdiction", "after_update.storage_class", "no_op", "after_delete.exists"}
	comparison := compareCloudflareParityObservations(observations, fields)
	if !comparison.Matched {
		t.Fatalf("C03 Cloudflare provider parity observations did not match: %#v", observations)
	}
	return cloudflareParityLiveRecording{
		Version:      cloudflareParityArtifactV1,
		Lane:         "C03",
		Scenario:     artifact.Scenarios[0].Name,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		DurationMS:   time.Since(started).Milliseconds(),
		Observations: observations,
		Comparison:   comparison,
	}
}

func runCloudflareParityC04Live(ctx context.Context, t *testing.T, artifact cloudflareParityArtifact) cloudflareParityLiveRecording {
	t.Helper()
	return runCloudflareParityD1Live(ctx, t, artifact, "c04", false)
}

func runCloudflareParityC05Live(ctx context.Context, t *testing.T, artifact cloudflareParityArtifact) cloudflareParityLiveRecording {
	t.Helper()
	return runCloudflareParityD1Live(ctx, t, artifact, "c05", true)
}

func runCloudflareParityC06Live(ctx context.Context, t *testing.T, artifact cloudflareParityArtifact) cloudflareParityLiveRecording {
	t.Helper()
	return runCloudflareParityD1UpdateLive(ctx, t, artifact)
}

func runCloudflareParityD1Live(ctx context.Context, t *testing.T, artifact cloudflareParityArtifact, lane string, exerciseRamenDelete bool) cloudflareParityLiveRecording {
	t.Helper()
	started := time.Now()
	suffix := cloudflareParityRunSuffix()
	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, string) cloudflareParityRuntimeResult
	}{
		{runtime: "opentofu", run: func(ctx context.Context, t *testing.T, databaseName string) cloudflareParityRuntimeResult {
			return runCloudflareParityD1HCLRuntime(ctx, t, lane, "opentofu", os.Getenv(cloudflareParityTofuEnv), databaseName)
		}},
		{runtime: "terraform", run: func(ctx context.Context, t *testing.T, databaseName string) cloudflareParityRuntimeResult {
			return runCloudflareParityD1HCLRuntime(ctx, t, lane, "terraform", os.Getenv(cloudflareParityTerraformEnv), databaseName)
		}},
		{runtime: "ramen", run: func(ctx context.Context, t *testing.T, databaseName string) cloudflareParityRuntimeResult {
			return runCloudflareParityD1RamenRuntime(ctx, t, lane, databaseName, exerciseRamenDelete)
		}},
	}
	var observations []cloudflareParityRuntimeObservation
	var failures []cloudflareParityRuntimeFailure
	for _, run := range runs {
		databaseName := cloudflareParityD1DatabaseName(lane, run.runtime, suffix)
		result := timedCloudflareParityRuntime(run.runtime, func() cloudflareParityRuntimeResult {
			return run.run(ctx, t, databaseName)
		})
		if result.Failure != nil {
			failures = append(failures, *result.Failure)
			continue
		}
		observations = append(observations, result.Observation)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Logf("%s Cloudflare parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("%s Cloudflare provider parity did not complete for all runtimes", strings.ToUpper(lane))
	}
	fields := []string{"after_create.exists", "after_create.uuid_present", "after_cleanup.exists"}
	if exerciseRamenDelete {
		assertCloudflareParityD1RamenDeleteObserved(t, observations)
	}
	comparison := compareCloudflareParityObservations(observations, fields)
	if !comparison.Matched {
		t.Fatalf("%s Cloudflare provider parity observations did not match: %#v", strings.ToUpper(lane), observations)
	}
	return cloudflareParityLiveRecording{
		Version:      cloudflareParityArtifactV1,
		Lane:         strings.ToUpper(lane),
		Scenario:     artifact.Scenarios[0].Name,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		DurationMS:   time.Since(started).Milliseconds(),
		Observations: observations,
		Comparison:   comparison,
	}
}

func runCloudflareParityD1UpdateLive(ctx context.Context, t *testing.T, artifact cloudflareParityArtifact) cloudflareParityLiveRecording {
	t.Helper()
	started := time.Now()
	lane := "c06"
	suffix := cloudflareParityRunSuffix()
	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, string) cloudflareParityRuntimeResult
	}{
		{runtime: "opentofu", run: func(ctx context.Context, t *testing.T, databaseName string) cloudflareParityRuntimeResult {
			return runCloudflareParityD1UpdateHCLRuntime(ctx, t, "opentofu", os.Getenv(cloudflareParityTofuEnv), databaseName)
		}},
		{runtime: "terraform", run: func(ctx context.Context, t *testing.T, databaseName string) cloudflareParityRuntimeResult {
			return runCloudflareParityD1UpdateHCLRuntime(ctx, t, "terraform", os.Getenv(cloudflareParityTerraformEnv), databaseName)
		}},
		{runtime: "ramen", run: runCloudflareParityD1UpdateRamenRuntime},
	}
	var observations []cloudflareParityRuntimeObservation
	var failures []cloudflareParityRuntimeFailure
	for _, run := range runs {
		databaseName := cloudflareParityD1DatabaseName(lane, run.runtime, suffix)
		result := timedCloudflareParityRuntime(run.runtime, func() cloudflareParityRuntimeResult {
			return run.run(ctx, t, databaseName)
		})
		if result.Failure != nil {
			failures = append(failures, *result.Failure)
			continue
		}
		observations = append(observations, result.Observation)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Logf("%s Cloudflare parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("C06 Cloudflare provider parity did not complete for all runtimes")
	}
	assertCloudflareParityD1RamenDeleteObserved(t, observations)
	fields := []string{"after_create.exists", "after_create.uuid_present", "after_update.read_replication_mode", "no_op", "after_cleanup.exists"}
	comparison := compareCloudflareParityObservations(observations, fields)
	if !comparison.Matched {
		t.Fatalf("C06 Cloudflare provider parity observations did not match: %#v", observations)
	}
	return cloudflareParityLiveRecording{
		Version:      cloudflareParityArtifactV1,
		Lane:         "C06",
		Scenario:     artifact.Scenarios[0].Name,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		DurationMS:   time.Since(started).Milliseconds(),
		Observations: observations,
		Comparison:   comparison,
	}
}

func assertCloudflareParityD1RamenDeleteObserved(t *testing.T, observations []cloudflareParityRuntimeObservation) {
	t.Helper()
	for _, observation := range observations {
		if observation.Runtime != "ramen" {
			continue
		}
		if observation.Fields["delete_via_runtime"] != true {
			t.Fatalf("C05 Ramen D1 delete was not observed: %#v", observation)
		}
		return
	}
	t.Fatalf("C05 Ramen D1 observation missing from %#v", observations)
}

func timedCloudflareParityRuntime(runtime string, run func() cloudflareParityRuntimeResult) cloudflareParityRuntimeResult {
	started := time.Now()
	result := run()
	if result.Failure == nil {
		result.Observation.DurationMS = time.Since(started).Milliseconds()
	}
	return result
}

func runCloudflareParityC01OpenTofuRuntime(ctx context.Context, t *testing.T, bucketName string) cloudflareParityRuntimeResult {
	t.Helper()
	return runCloudflareParityC01HCLRuntime(ctx, t, "opentofu", os.Getenv(cloudflareParityTofuEnv), bucketName)
}

func runCloudflareParityC01TerraformRuntime(ctx context.Context, t *testing.T, bucketName string) cloudflareParityRuntimeResult {
	t.Helper()
	return runCloudflareParityC01HCLRuntime(ctx, t, "terraform", os.Getenv(cloudflareParityTerraformEnv), bucketName)
}

func runCloudflareParityC01HCLRuntime(ctx context.Context, t *testing.T, runtimeName, tool, bucketName string) cloudflareParityRuntimeResult {
	t.Helper()
	if err := validateCloudflareParityBucketName(bucketName, "c01"); err != nil {
		return cloudflareParityFailure(runtimeName, "safety", err)
	}
	if err := deleteCloudflareParityR2BucketIfExists(ctx, bucketName, "c01"); err != nil {
		return cloudflareParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteCloudflareParityR2BucketIfExists(context.Background(), bucketName, "c01"); err != nil {
			t.Logf("cleanup Cloudflare R2 bucket %s: %v", bucketName, err)
		}
	})
	workDir := cloudflareParityRuntimeWorkDir(t, runtimeName, bucketName)
	if err := copyCloudflareParityFixtureFile(filepath.Join(cloudflareParityFixtureRoot, "c01", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := writeCloudflareParityTFVars(workDir, bucketName, "Standard"); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), cloudflareParityEnvForTools()...)
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return cloudflareParityFailure(runtimeName, "init", err)
	}
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return cloudflareParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeCloudflareParityR2Bucket(ctx, bucketName)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	planExit, _, err := runCloudflareParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "plan", err)
	}
	if err := writeCloudflareParityTFVars(workDir, bucketName, "InfrequentAccess"); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return cloudflareParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeCloudflareParityR2Bucket(ctx, bucketName)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return cloudflareParityFailure(runtimeName, "destroy", err)
	}
	afterDelete, err := waitCloudflareParityR2Bucket(ctx, bucketName, false)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	return cloudflareParityRuntimeResult{Observation: cloudflareParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: bucketName,
		Fields:   cloudflareParityR2LifecycleFields(afterCreate, afterUpdate, afterDelete, planExit == 0),
	}}
}

func runCloudflareParityC02OpenTofuRuntime(ctx context.Context, t *testing.T, bucketName string) cloudflareParityRuntimeResult {
	t.Helper()
	return runCloudflareParityC02HCLRuntime(ctx, t, "opentofu", os.Getenv(cloudflareParityTofuEnv), bucketName)
}

func runCloudflareParityC02TerraformRuntime(ctx context.Context, t *testing.T, bucketName string) cloudflareParityRuntimeResult {
	t.Helper()
	return runCloudflareParityC02HCLRuntime(ctx, t, "terraform", os.Getenv(cloudflareParityTerraformEnv), bucketName)
}

func runCloudflareParityC02HCLRuntime(ctx context.Context, t *testing.T, runtimeName, tool, bucketName string) cloudflareParityRuntimeResult {
	t.Helper()
	if err := validateCloudflareParityBucketName(bucketName, "c02"); err != nil {
		return cloudflareParityFailure(runtimeName, "safety", err)
	}
	if err := deleteCloudflareParityR2BucketIfExists(ctx, bucketName, "c02"); err != nil {
		return cloudflareParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteCloudflareParityR2BucketIfExists(context.Background(), bucketName, "c02"); err != nil {
			t.Logf("cleanup Cloudflare R2 bucket %s: %v", bucketName, err)
		}
	})
	workDir := cloudflareParityRuntimeWorkDir(t, runtimeName, bucketName)
	if err := copyCloudflareParityFixtureFile(filepath.Join(cloudflareParityFixtureRoot, "c02", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := writeCloudflareParityTFVars(workDir, bucketName, ""); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), cloudflareParityEnvForTools()...)
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return cloudflareParityFailure(runtimeName, "init", err)
	}
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return cloudflareParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeCloudflareParityR2Bucket(ctx, bucketName)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	if err := deleteCloudflareParityR2BucketIfExists(ctx, bucketName, "c02"); err != nil {
		return cloudflareParityFailure(runtimeName, "out-of-band-delete", err)
	}
	afterOutOfBandDelete, err := waitCloudflareParityR2Bucket(ctx, bucketName, false)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	missingExit, _, err := runCloudflareParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "missing-plan", err)
	}
	return cloudflareParityRuntimeResult{Observation: cloudflareParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: bucketName,
		Fields:   cloudflareParityR2ReadMissingFields(afterCreate, afterOutOfBandDelete, missingExit == 2),
	}}
}

func runCloudflareParityC03OpenTofuRuntime(ctx context.Context, t *testing.T, bucketName string) cloudflareParityRuntimeResult {
	t.Helper()
	return runCloudflareParityC03HCLRuntime(ctx, t, "opentofu", os.Getenv(cloudflareParityTofuEnv), bucketName)
}

func runCloudflareParityC03TerraformRuntime(ctx context.Context, t *testing.T, bucketName string) cloudflareParityRuntimeResult {
	t.Helper()
	return runCloudflareParityC03HCLRuntime(ctx, t, "terraform", os.Getenv(cloudflareParityTerraformEnv), bucketName)
}

func runCloudflareParityC03HCLRuntime(ctx context.Context, t *testing.T, runtimeName, tool, bucketName string) cloudflareParityRuntimeResult {
	t.Helper()
	if err := validateCloudflareParityBucketName(bucketName, "c03"); err != nil {
		return cloudflareParityFailure(runtimeName, "safety", err)
	}
	if err := deleteCloudflareParityR2BucketIfExists(ctx, bucketName, "c03"); err != nil {
		return cloudflareParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteCloudflareParityR2BucketIfExists(context.Background(), bucketName, "c03"); err != nil {
			t.Logf("cleanup Cloudflare R2 bucket %s: %v", bucketName, err)
		}
	})
	workDir := cloudflareParityRuntimeWorkDir(t, runtimeName, bucketName)
	mainPath := filepath.Join(workDir, "main.tf")
	if err := renderCloudflareParityC03HCL(mainPath, "InfrequentAccess"); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := writeCloudflareParityTFVars(workDir, bucketName, ""); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), cloudflareParityEnvForTools()...)
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return cloudflareParityFailure(runtimeName, "init", err)
	}
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return cloudflareParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeCloudflareParityR2Bucket(ctx, bucketName)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	planExit, _, err := runCloudflareParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "plan", err)
	}
	if err := renderCloudflareParityC03HCL(mainPath, "Standard"); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return cloudflareParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeCloudflareParityR2Bucket(ctx, bucketName)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return cloudflareParityFailure(runtimeName, "destroy", err)
	}
	afterDelete, err := waitCloudflareParityR2Bucket(ctx, bucketName, false)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	return cloudflareParityRuntimeResult{Observation: cloudflareParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: bucketName,
		Fields:   cloudflareParityR2LifecycleFields(afterCreate, afterUpdate, afterDelete, planExit == 0),
	}}
}

func runCloudflareParityC01RamenRuntime(ctx context.Context, t *testing.T, bucketName string) cloudflareParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := validateCloudflareParityBucketName(bucketName, "c01"); err != nil {
		return cloudflareParityFailure(runtimeName, "safety", err)
	}
	if err := deleteCloudflareParityR2BucketIfExists(ctx, bucketName, "c01"); err != nil {
		return cloudflareParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteCloudflareParityR2BucketIfExists(context.Background(), bucketName, "c01"); err != nil {
			t.Logf("cleanup Cloudflare R2 bucket %s: %v", bucketName, err)
		}
	})
	workDir := cloudflareParityRuntimeWorkDir(t, runtimeName, bucketName)
	createProjectPath := filepath.Join(workDir, "create", "project.uws.yaml")
	updateProjectPath := filepath.Join(workDir, "update", "project.uws.yaml")
	openAPIPath, err := cloudflareParityOpenAPIPathForLane("c01")
	if err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := renderCloudflareParityR2Project(filepath.Join(cloudflareParityFixtureRoot, "c01", "ramen", "project.uws.yaml"), createProjectPath, cloudflareParityAccountID(), bucketName, "Standard", openAPIPath); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := renderCloudflareParityR2Project(filepath.Join(cloudflareParityFixtureRoot, "c01", "ramen", "project.uws.yaml"), updateProjectPath, cloudflareParityAccountID(), bucketName, "InfrequentAccess", openAPIPath); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	udonExecutor := cloudflareParityUdonExecutor(workDir, bucketName)
	if err := buildAndApplyCloudflareParityPlan(ctx, createProjectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeCloudflareParityR2Bucket(ctx, bucketName)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	planResult, err := buildCloudflareParityNoOpPlan(ctx, createProjectPath, statePath)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1
	if err := buildAndApplyCloudflareParityPlan(ctx, createProjectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "read", err)
	}
	if err := buildAndApplyCloudflareParityPlan(ctx, updateProjectPath, statePath, "update", filepath.Join(workDir, "update-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeCloudflareParityR2Bucket(ctx, bucketName)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	if err := buildAndApplyCloudflareParityPlan(ctx, updateProjectPath, statePath, "delete", filepath.Join(workDir, "delete-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "delete", err)
	}
	afterDelete, err := waitCloudflareParityR2Bucket(ctx, bucketName, false)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	return cloudflareParityRuntimeResult{Observation: cloudflareParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: bucketName,
		Fields:   cloudflareParityR2LifecycleFields(afterCreate, afterUpdate, afterDelete, noOp),
	}}
}

func runCloudflareParityC02RamenRuntime(ctx context.Context, t *testing.T, bucketName string) cloudflareParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := validateCloudflareParityBucketName(bucketName, "c02"); err != nil {
		return cloudflareParityFailure(runtimeName, "safety", err)
	}
	if err := deleteCloudflareParityR2BucketIfExists(ctx, bucketName, "c02"); err != nil {
		return cloudflareParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteCloudflareParityR2BucketIfExists(context.Background(), bucketName, "c02"); err != nil {
			t.Logf("cleanup Cloudflare R2 bucket %s: %v", bucketName, err)
		}
	})
	workDir := cloudflareParityRuntimeWorkDir(t, runtimeName, bucketName)
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	openAPIPath, err := cloudflareParityOpenAPIPathForLane("c02")
	if err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := renderCloudflareParityProject(filepath.Join(cloudflareParityFixtureRoot, "c02", "ramen", "project.uws.yaml"), projectPath, cloudflareParityAccountID(), bucketName, openAPIPath); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	udonExecutor := cloudflareParityUdonExecutor(workDir, bucketName)
	if err := buildAndApplyCloudflareParityPlan(ctx, projectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeCloudflareParityR2Bucket(ctx, bucketName)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	if err := deleteCloudflareParityR2BucketIfExists(ctx, bucketName, "c02"); err != nil {
		return cloudflareParityFailure(runtimeName, "out-of-band-delete", err)
	}
	afterOutOfBandDelete, err := waitCloudflareParityR2Bucket(ctx, bucketName, false)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	if err := buildAndApplyCloudflareParityPlan(ctx, projectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "read-missing", err)
	}
	return cloudflareParityRuntimeResult{Observation: cloudflareParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: bucketName,
		Fields:   cloudflareParityR2ReadMissingFields(afterCreate, afterOutOfBandDelete, true),
	}}
}

func runCloudflareParityC03RamenRuntime(ctx context.Context, t *testing.T, bucketName string) cloudflareParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := validateCloudflareParityBucketName(bucketName, "c03"); err != nil {
		return cloudflareParityFailure(runtimeName, "safety", err)
	}
	if err := deleteCloudflareParityR2BucketIfExists(ctx, bucketName, "c03"); err != nil {
		return cloudflareParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteCloudflareParityR2BucketIfExists(context.Background(), bucketName, "c03"); err != nil {
			t.Logf("cleanup Cloudflare R2 bucket %s: %v", bucketName, err)
		}
	})
	workDir := cloudflareParityRuntimeWorkDir(t, runtimeName, bucketName)
	createProjectPath := filepath.Join(workDir, "create", "project.uws.yaml")
	updateProjectPath := filepath.Join(workDir, "update", "project.uws.yaml")
	openAPIPath, err := cloudflareParityOpenAPIPathForLane("c03")
	if err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := renderCloudflareParityR2Project(filepath.Join(cloudflareParityFixtureRoot, "c03", "ramen", "project.uws.yaml"), createProjectPath, cloudflareParityAccountID(), bucketName, "InfrequentAccess", openAPIPath); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := renderCloudflareParityR2Project(filepath.Join(cloudflareParityFixtureRoot, "c03", "ramen", "project.uws.yaml"), updateProjectPath, cloudflareParityAccountID(), bucketName, "Standard", openAPIPath); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	udonExecutor := cloudflareParityUdonExecutor(workDir, bucketName)
	if err := buildAndApplyCloudflareParityPlan(ctx, createProjectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeCloudflareParityR2Bucket(ctx, bucketName)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	planResult, err := buildCloudflareParityNoOpPlan(ctx, createProjectPath, statePath)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1
	if err := buildAndApplyCloudflareParityPlan(ctx, createProjectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "read", err)
	}
	if err := buildAndApplyCloudflareParityPlan(ctx, updateProjectPath, statePath, "update", filepath.Join(workDir, "update-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeCloudflareParityR2Bucket(ctx, bucketName)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	if err := buildAndApplyCloudflareParityPlan(ctx, updateProjectPath, statePath, "delete", filepath.Join(workDir, "delete-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "delete", err)
	}
	afterDelete, err := waitCloudflareParityR2Bucket(ctx, bucketName, false)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	return cloudflareParityRuntimeResult{Observation: cloudflareParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: bucketName,
		Fields:   cloudflareParityR2LifecycleFields(afterCreate, afterUpdate, afterDelete, noOp),
	}}
}

func runCloudflareParityD1HCLRuntime(ctx context.Context, t *testing.T, lane, runtimeName, tool, databaseName string) cloudflareParityRuntimeResult {
	t.Helper()
	if err := validateCloudflareParityD1DatabaseName(databaseName, lane); err != nil {
		return cloudflareParityFailure(runtimeName, "safety", err)
	}
	if err := deleteCloudflareParityD1DatabasesByName(ctx, databaseName, lane); err != nil {
		return cloudflareParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteCloudflareParityD1DatabasesByName(context.Background(), databaseName, lane); err != nil {
			t.Logf("cleanup Cloudflare D1 database %s: %v", databaseName, err)
		}
	})
	workDir := cloudflareParityRuntimeWorkDir(t, runtimeName, databaseName)
	if err := copyCloudflareParityFixtureFile(filepath.Join(cloudflareParityFixtureRoot, lane, "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := writeCloudflareParityD1TFVars(workDir, databaseName); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), cloudflareParityEnvForTools()...)
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return cloudflareParityFailure(runtimeName, "init", err)
	}
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return cloudflareParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := waitCloudflareParityD1DatabaseByName(ctx, databaseName, lane, true)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	planExit, _, err := runCloudflareParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "plan", err)
	}
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return cloudflareParityFailure(runtimeName, "destroy", err)
	}
	afterCleanup, err := waitCloudflareParityD1DatabaseByName(ctx, databaseName, lane, false)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	return cloudflareParityRuntimeResult{Observation: cloudflareParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: databaseName,
		Fields:   cloudflareParityD1Fields(afterCreate, afterCleanup, planExit == 0, false),
	}}
}

func runCloudflareParityD1UpdateHCLRuntime(ctx context.Context, t *testing.T, runtimeName, tool, databaseName string) cloudflareParityRuntimeResult {
	t.Helper()
	lane := "c06"
	if err := validateCloudflareParityD1DatabaseName(databaseName, lane); err != nil {
		return cloudflareParityFailure(runtimeName, "safety", err)
	}
	if err := deleteCloudflareParityD1DatabasesByName(ctx, databaseName, lane); err != nil {
		return cloudflareParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteCloudflareParityD1DatabasesByName(context.Background(), databaseName, lane); err != nil {
			t.Logf("cleanup Cloudflare D1 database %s: %v", databaseName, err)
		}
	})
	workDir := cloudflareParityRuntimeWorkDir(t, runtimeName, databaseName)
	if err := copyCloudflareParityFixtureFile(filepath.Join(cloudflareParityFixtureRoot, lane, "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := writeCloudflareParityD1UpdateTFVars(workDir, databaseName, "disabled"); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), cloudflareParityEnvForTools()...)
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return cloudflareParityFailure(runtimeName, "init", err)
	}
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return cloudflareParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := waitCloudflareParityD1DatabaseByName(ctx, databaseName, lane, true)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	afterCreate, err = observeCloudflareParityD1DatabaseByID(ctx, afterCreate.UUID, databaseName, lane)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	planExit, _, err := runCloudflareParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "plan", err)
	}
	if err := writeCloudflareParityD1UpdateTFVars(workDir, databaseName, "auto"); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return cloudflareParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeCloudflareParityD1DatabaseByID(ctx, afterCreate.UUID, databaseName, lane)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	if err := runCloudflareParityCommand(ctx, workDir, env, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return cloudflareParityFailure(runtimeName, "destroy", err)
	}
	afterCleanup, err := waitCloudflareParityD1DatabaseByName(ctx, databaseName, lane, false)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	return cloudflareParityRuntimeResult{Observation: cloudflareParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: databaseName,
		Fields:   cloudflareParityD1UpdateFields(afterCreate, afterUpdate, afterCleanup, planExit == 0, false),
	}}
}

func runCloudflareParityD1RamenRuntime(ctx context.Context, t *testing.T, lane, databaseName string, exerciseDelete bool) cloudflareParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := validateCloudflareParityD1DatabaseName(databaseName, lane); err != nil {
		return cloudflareParityFailure(runtimeName, "safety", err)
	}
	if err := deleteCloudflareParityD1DatabasesByName(ctx, databaseName, lane); err != nil {
		return cloudflareParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteCloudflareParityD1DatabasesByName(context.Background(), databaseName, lane); err != nil {
			t.Logf("cleanup Cloudflare D1 database %s: %v", databaseName, err)
		}
	})
	workDir := cloudflareParityRuntimeWorkDir(t, runtimeName, databaseName)
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	openAPIPath, err := cloudflareParityOpenAPIPathForLane(lane)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := renderCloudflareParityProject(filepath.Join(cloudflareParityFixtureRoot, lane, "ramen", "project.uws.yaml"), projectPath, cloudflareParityAccountID(), databaseName, openAPIPath); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	udonExecutor := cloudflareParityD1UdonExecutor(workDir, databaseName, lane)
	if err := buildAndApplyCloudflareParityPlan(ctx, projectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := waitCloudflareParityD1DatabaseByName(ctx, databaseName, lane, true)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	planResult, err := buildCloudflareParityNoOpPlan(ctx, projectPath, statePath)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1
	if err := buildAndApplyCloudflareParityPlan(ctx, projectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "read", err)
	}
	deleteViaRuntime := false
	if exerciseDelete {
		if err := buildAndApplyCloudflareParityPlan(ctx, projectPath, statePath, "delete", filepath.Join(workDir, "delete-plan.json"), udonExecutor); err != nil {
			return cloudflareParityFailure(runtimeName, "delete", err)
		}
		deleteViaRuntime = true
	} else if err := deleteCloudflareParityD1DatabaseByID(ctx, afterCreate.UUID, databaseName, lane); err != nil {
		return cloudflareParityFailure(runtimeName, "cleanup", err)
	}
	afterCleanup, err := waitCloudflareParityD1DatabaseByName(ctx, databaseName, lane, false)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	return cloudflareParityRuntimeResult{Observation: cloudflareParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: databaseName,
		Fields:   cloudflareParityD1Fields(afterCreate, afterCleanup, noOp, deleteViaRuntime),
	}}
}

func runCloudflareParityD1UpdateRamenRuntime(ctx context.Context, t *testing.T, databaseName string) cloudflareParityRuntimeResult {
	t.Helper()
	lane := "c06"
	runtimeName := "ramen"
	if err := validateCloudflareParityD1DatabaseName(databaseName, lane); err != nil {
		return cloudflareParityFailure(runtimeName, "safety", err)
	}
	if err := deleteCloudflareParityD1DatabasesByName(ctx, databaseName, lane); err != nil {
		return cloudflareParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteCloudflareParityD1DatabasesByName(context.Background(), databaseName, lane); err != nil {
			t.Logf("cleanup Cloudflare D1 database %s: %v", databaseName, err)
		}
	})
	workDir := cloudflareParityRuntimeWorkDir(t, runtimeName, databaseName)
	createProjectPath := filepath.Join(workDir, "create", "project.uws.yaml")
	updateProjectPath := filepath.Join(workDir, "update", "project.uws.yaml")
	openAPIPath, err := cloudflareParityOpenAPIPathForLane(lane)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := renderCloudflareParityD1UpdateProject(filepath.Join(cloudflareParityFixtureRoot, lane, "ramen", "project.uws.yaml"), createProjectPath, cloudflareParityAccountID(), databaseName, openAPIPath, "disabled"); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	if err := renderCloudflareParityD1UpdateProject(filepath.Join(cloudflareParityFixtureRoot, lane, "ramen", "project.uws.yaml"), updateProjectPath, cloudflareParityAccountID(), databaseName, openAPIPath, "auto"); err != nil {
		return cloudflareParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	udonExecutor := cloudflareParityD1UdonExecutor(workDir, databaseName, lane)
	if err := buildAndApplyCloudflareParityPlan(ctx, createProjectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := waitCloudflareParityD1DatabaseByName(ctx, databaseName, lane, true)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	afterCreate, err = observeCloudflareParityD1DatabaseByID(ctx, afterCreate.UUID, databaseName, lane)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	planResult, err := buildCloudflareParityNoOpPlan(ctx, createProjectPath, statePath)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1
	if err := buildAndApplyCloudflareParityPlan(ctx, createProjectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "read", err)
	}
	if err := buildAndApplyCloudflareParityPlan(ctx, updateProjectPath, statePath, "update", filepath.Join(workDir, "update-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeCloudflareParityD1DatabaseByID(ctx, afterCreate.UUID, databaseName, lane)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	if err := buildAndApplyCloudflareParityPlan(ctx, updateProjectPath, statePath, "delete", filepath.Join(workDir, "delete-plan.json"), udonExecutor); err != nil {
		return cloudflareParityFailure(runtimeName, "delete", err)
	}
	afterCleanup, err := waitCloudflareParityD1DatabaseByName(ctx, databaseName, lane, false)
	if err != nil {
		return cloudflareParityFailure(runtimeName, "observe", err)
	}
	return cloudflareParityRuntimeResult{Observation: cloudflareParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: databaseName,
		Fields:   cloudflareParityD1UpdateFields(afterCreate, afterUpdate, afterCleanup, noOp, true),
	}}
}

func cloudflareParityUdonExecutor(workDir, bucketName string) udon.Executor {
	return udon.Executor{
		OutputDir: filepath.Join(workDir, "udon"),
		CredentialResolvers: map[string]func(context.Context) (string, error){
			"api_token": func(context.Context) (string, error) {
				return strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")), nil
			},
			"cloudflare_api_token": func(context.Context) (string, error) {
				return strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")), nil
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
			observed, err := observeCloudflareParityR2Bucket(projectorCtx, bucketName)
			if err != nil {
				return executor.Result{}, err
			}
			if !observed.Exists {
				result.Missing = true
				return result, nil
			}
			result.Identity = map[string]any{
				"account_id":  cloudflareParityAccountID(),
				"bucket_name": observed.Name,
			}
			result.Computed = map[string]any{
				"location":      observed.Location,
				"storage_class": observed.StorageClass,
				"jurisdiction":  observed.Jurisdiction,
			}
			return result, nil
		},
	}
}

func cloudflareParityD1UdonExecutor(workDir, databaseName, lane string) udon.Executor {
	return udon.Executor{
		OutputDir: filepath.Join(workDir, "udon"),
		CredentialResolvers: map[string]func(context.Context) (string, error){
			"api_token": func(context.Context) (string, error) {
				return strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")), nil
			},
			"cloudflare_api_token": func(context.Context) (string, error) {
				return strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")), nil
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
			observed, err := waitCloudflareParityD1DatabaseByName(projectorCtx, databaseName, lane, true)
			if err != nil {
				return executor.Result{}, err
			}
			if !observed.Exists {
				result.Missing = true
				return result, nil
			}
			result.Identity = map[string]any{
				"account_id":  cloudflareParityAccountID(),
				"database_id": observed.UUID,
			}
			result.Computed = map[string]any{
				"result": map[string]any{
					"name": observed.Name,
					"uuid": observed.UUID,
					"read_replication": map[string]any{
						"mode": normalizeCloudflareParityD1ReadReplicationMode(observed.ReadReplicationMode),
					},
				},
			}
			return result, nil
		},
	}
}

func buildAndApplyCloudflareParityPlan(ctx context.Context, projectPath, statePath, action, planPath string, udonExecutor udon.Executor) error {
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

func buildCloudflareParityNoOpPlan(ctx context.Context, projectPath, statePath string) (*tfplan.Result, error) {
	return tfplan.Build(ctx, tfplan.Options{
		ProjectPath: projectPath,
		StatePath:   statePath,
		Action:      "plan",
	})
}

func renderCloudflareParityR2Project(src, dst, accountID, bucketName, storageClass, openAPIPath string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	out := strings.ReplaceAll(string(data), "cloudflare-account-placeholder", accountID)
	out = strings.ReplaceAll(out, "ramen-parity-c01-static", bucketName)
	out = strings.ReplaceAll(out, "ramen-parity-c02-static", bucketName)
	out = strings.ReplaceAll(out, "ramen-parity-c03-static", bucketName)
	if strings.TrimSpace(storageClass) != "" {
		out = strings.ReplaceAll(out, "storage_class: InfrequentAccess", "storage_class: "+storageClass)
	}
	if strings.TrimSpace(openAPIPath) != "" {
		out = strings.ReplaceAll(out, "../../../../api-sources/cloudflare-r2-d1-openapi.json", filepath.ToSlash(openAPIPath))
		out = strings.ReplaceAll(out, "../../../../api-sources/cloudflare-r2-d1-delete-openapi.json", filepath.ToSlash(openAPIPath))
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}

func renderCloudflareParityD1UpdateProject(src, dst, accountID, databaseName, openAPIPath, readReplicationMode string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	out := strings.ReplaceAll(string(data), "cloudflare-account-placeholder", accountID)
	out = strings.ReplaceAll(out, "ramen-parity-c06-static", databaseName)
	if strings.TrimSpace(readReplicationMode) != "" {
		out = strings.ReplaceAll(out, "mode: disabled", "mode: "+readReplicationMode)
	}
	if strings.TrimSpace(openAPIPath) != "" {
		out = strings.ReplaceAll(out, "../../../../api-sources/cloudflare-r2-d1-update-openapi.json", filepath.ToSlash(openAPIPath))
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}

func renderCloudflareParityC03HCL(dst, storageClass string) error {
	data, err := os.ReadFile(filepath.Join(cloudflareParityFixtureRoot, "c03", "hcl", "main.tf"))
	if err != nil {
		return err
	}
	out := strings.ReplaceAll(string(data), `storage_class = "InfrequentAccess"`, `storage_class = "`+storageClass+`"`)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}

func cloudflareParityRuntimeWorkDir(t *testing.T, runtimeName, bucketName string) string {
	t.Helper()
	debugRoot := strings.TrimSpace(os.Getenv("RAMEN_CLOUDFLARE_DEBUG_DIR"))
	if debugRoot == "" {
		return filepath.Join(t.TempDir(), runtimeName)
	}
	workDir := filepath.Join(debugRoot, bucketName, runtimeName)
	if err := os.RemoveAll(workDir); err != nil {
		t.Fatalf("clear Cloudflare debug workdir: %v", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("create Cloudflare debug workdir: %v", err)
	}
	t.Logf("Cloudflare parity debug workdir: %s", workDir)
	return workDir
}

func assertCloudflareParityC01UdonCreateShape(t *testing.T) {
	t.Helper()
	workDir := t.TempDir()
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	openAPIPath, err := filepath.Abs("testdata/api-sources/cloudflare-r2-d1-openapi.json")
	if err != nil {
		t.Fatalf("resolve Cloudflare OpenAPI path: %v", err)
	}
	bucketName := "ramen-parity-c01-udon-shape"
	if err := renderCloudflareParityR2Project(filepath.Join(cloudflareParityFixtureRoot, "c01", "ramen", "project.uws.yaml"), projectPath, "cloudflare-account-placeholder", bucketName, "Standard", openAPIPath); err != nil {
		t.Fatalf("render C01 udon shape project: %v", err)
	}
	result, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: projectPath,
		StatePath:   filepath.Join(workDir, "state.db"),
		Action:      "create",
	})
	if err != nil {
		t.Fatalf("build C01 udon shape plan: %v", err)
	}
	if result.Plan.Errored || len(result.Plan.Resources) != 1 {
		t.Fatalf("C01 udon shape plan unusable: %#v", result.Plan)
	}
	attrs := map[string]any{
		"account_id":    "cloudflare-account-placeholder",
		"name":          bucketName,
		"location":      "ENAM",
		"storage_class": "Standard",
		"jurisdiction":  "default",
	}
	doc, err := apply.BuildActionDocumentWithBindings(result.Plan.Resources[0], nil, attrs, nil)
	if err != nil {
		t.Fatalf("build C01 udon shape action document: %v", err)
	}
	plan, err := generator.NewRuntimePlanFromUWSDocument(doc, "/")
	if err != nil {
		t.Fatalf("compile C01 udon shape action document: %v", err)
	}
	cfg := plan.ExecCache()
	if cfg == nil || len(cfg.Operations) != 1 {
		t.Fatalf("C01 udon shape exec cache = %#v", cfg)
	}
	op := cfg.Operations[0]
	if op.Path != "/client/v4/accounts/{account_id}/r2/buckets" {
		t.Fatalf("C01 udon create path = %q", op.Path)
	}
	if op.PathPars == nil || op.PathPars.Properties["account_id"] == nil {
		t.Fatalf("C01 udon create path params = %#v", op.PathPars)
	}
	for _, key := range []string{"name", "locationHint", "storageClass"} {
		if op.PayloadPars == nil || op.PayloadPars.Properties[key] == nil {
			t.Fatalf("C01 udon create payload params missing %s: %#v", key, op.PayloadPars)
		}
	}
	requests, err := plan.OperationRequests()
	if err != nil {
		t.Fatalf("read C01 udon operation requests: %v", err)
	}
	request := requests[op.Name]
	for key, want := range map[string]any{
		"account_id":   "cloudflare-account-placeholder",
		"name":         bucketName,
		"locationHint": "ENAM",
		"storageClass": "Standard",
	} {
		if request[key] != want {
			t.Fatalf("C01 udon request[%s] = %#v, want %#v; request=%#v", key, request[key], want, request)
		}
	}
}

func TestCloudflareParityC01UdonCreateHTTPShape(t *testing.T) {
	bucketName := "ramen-parity-c01-http-shape"
	var created bool
	var sawCreate bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/client/v4/accounts/cloudflare-account-placeholder/r2/buckets/"+bucketName:
			if !created {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":10006,"message":"The specified bucket does not exist."}],"messages":[],"result":null}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"messages":[],"result":{"name":"` + bucketName + `","location":"ENAM","storageClass":"Standard","jurisdiction":"default"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/client/v4/accounts/cloudflare-account-placeholder/r2/buckets":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read create body: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Errorf("decode create body %q: %v", string(body), err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			for key, want := range map[string]any{"name": bucketName, "locationHint": "ENAM", "storageClass": "Standard"} {
				if payload[key] != want {
					t.Errorf("create payload[%s] = %#v, want %#v; payload=%#v", key, payload[key], want, payload)
				}
			}
			sawCreate = true
			created = true
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"messages":[],"result":{"name":"` + bucketName + `","creation_date":"2026-06-07T00:00:00Z","location":"ENAM","storage_class":"Standard","jurisdiction":"default"}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":10404,"message":"No route for that URI"}],"messages":[],"result":null}`))
		}
	}))
	defer server.Close()

	workDir := t.TempDir()
	sourceData, err := os.ReadFile("testdata/api-sources/cloudflare-r2-d1-openapi.json")
	if err != nil {
		t.Fatalf("read Cloudflare OpenAPI source: %v", err)
	}
	openAPIPath := filepath.Join(workDir, "cloudflare-r2-d1-openapi.json")
	sourceText := strings.ReplaceAll(string(sourceData), "https://api.cloudflare.com", server.URL)
	if err := os.WriteFile(openAPIPath, []byte(sourceText), 0o644); err != nil {
		t.Fatalf("write local Cloudflare OpenAPI source: %v", err)
	}
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	if err := renderCloudflareParityR2Project(filepath.Join(cloudflareParityFixtureRoot, "c01", "ramen", "project.uws.yaml"), projectPath, "cloudflare-account-placeholder", bucketName, "Standard", openAPIPath); err != nil {
		t.Fatalf("render local C01 project: %v", err)
	}
	exec := udon.Executor{
		OutputDir: filepath.Join(workDir, "udon"),
		CredentialResolvers: map[string]func(context.Context) (string, error){
			"api_token":            func(context.Context) (string, error) { return "test-token", nil },
			"cloudflare_api_token": func(context.Context) (string, error) { return "test-token", nil },
		},
		OutputProjector: func(_ context.Context, req executor.Request, _ string) (executor.Result, error) {
			result := executor.Result{
				Address:   req.Action.Address,
				Operation: req.Action.Mapping.OperationID,
				Success:   true,
			}
			if !created {
				result.Missing = true
				return result, nil
			}
			result.Identity = map[string]any{"account_id": "cloudflare-account-placeholder", "bucket_name": bucketName}
			result.Computed = map[string]any{"location": "ENAM", "storage_class": "Standard", "jurisdiction": "default"}
			return result, nil
		},
	}
	statePath := filepath.Join(workDir, "state.db")
	if err := buildAndApplyCloudflareParityPlan(context.Background(), projectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), exec); err != nil {
		t.Fatalf("local C01 create apply failed: %v", err)
	}
	if !sawCreate {
		t.Fatalf("local C01 create request was not observed")
	}
	planResult, err := buildCloudflareParityNoOpPlan(context.Background(), projectPath, statePath)
	if err != nil {
		t.Fatalf("local C01 post-create plan failed: %v", err)
	}
	if planResult.Plan.Errored || planResult.Plan.Summary.NoOp != 1 || len(planResult.Plan.Resources) != 1 {
		t.Fatalf("local C01 post-create plan summary = %+v resources=%d errored=%v, want one no-op", planResult.Plan.Summary, len(planResult.Plan.Resources), planResult.Plan.Errored)
	}
}

func runCloudflareParityPlan(ctx context.Context, dir string, env []string, tool string) (int, string, error) {
	cmd := osexec.CommandContext(ctx, tool, "plan", "-input=false", "-no-color", "-detailed-exitcode")
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	summary := lastNonEmptyCloudflareParityLine(string(out))
	if err == nil {
		return 0, summary, nil
	}
	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
		return 2, summary, nil
	}
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), summary, fmt.Errorf("%s plan failed with exit %d: %w", filepath.Base(tool), exitErr.ExitCode(), err)
	}
	return -1, summary, fmt.Errorf("%s plan failed: %w", filepath.Base(tool), err)
}

func runCloudflareParityCommand(ctx context.Context, dir string, env []string, tool string, args ...string) error {
	if strings.TrimSpace(tool) == "" {
		return fmt.Errorf("empty tool path for %s", strings.Join(args, " "))
	}
	cmd := osexec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", filepath.Base(tool), strings.Join(args, " "), err, sanitizeCloudflareParityCommandOutput(string(out)))
	}
	return nil
}

func copyCloudflareParityFixtureFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func writeCloudflareParityTFVars(dir, bucketName, storageClass string) error {
	vars := map[string]string{
		"account_id":  cloudflareParityAccountID(),
		"bucket_name": bucketName,
	}
	if strings.TrimSpace(storageClass) != "" {
		vars["storage_class"] = storageClass
	}
	data, err := json.MarshalIndent(vars, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, "terraform.tfvars.json"), data, 0o644)
}

func writeCloudflareParityD1TFVars(dir, databaseName string) error {
	vars := map[string]string{
		"account_id":    cloudflareParityAccountID(),
		"database_name": databaseName,
	}
	data, err := json.MarshalIndent(vars, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, "terraform.tfvars.json"), data, 0o644)
}

func writeCloudflareParityD1UpdateTFVars(dir, databaseName, readReplicationMode string) error {
	vars := map[string]string{
		"account_id":            cloudflareParityAccountID(),
		"database_name":         databaseName,
		"read_replication_mode": readReplicationMode,
	}
	data, err := json.MarshalIndent(vars, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, "terraform.tfvars.json"), data, 0o644)
}

type cloudflareParityR2BucketObservation struct {
	Exists       bool
	Name         string
	Location     string
	StorageClass string
	Jurisdiction string
}

func observeCloudflareParityR2Bucket(ctx context.Context, bucketName string) (cloudflareParityR2BucketObservation, error) {
	if !strings.HasPrefix(bucketName, "ramen-parity-c") {
		return cloudflareParityR2BucketObservation{}, fmt.Errorf("refusing to observe non-parity R2 bucket %q", bucketName)
	}
	status, body, err := cloudflareParityAPIRequest(ctx, http.MethodGet, "/accounts/"+cloudflareParityAccountID()+"/r2/buckets/"+bucketName)
	if err != nil {
		return cloudflareParityR2BucketObservation{}, err
	}
	if status == http.StatusNotFound {
		return cloudflareParityR2BucketObservation{Exists: false}, nil
	}
	if status < 200 || status > 299 {
		return cloudflareParityR2BucketObservation{}, fmt.Errorf("Cloudflare R2 bucket get returned HTTP %d: %s", status, sanitizeCloudflareParityCommandOutput(string(body)))
	}
	var doc struct {
		Success bool `json:"success"`
		Result  struct {
			Name              string `json:"name"`
			Location          string `json:"location"`
			StorageClass      string `json:"storageClass"`
			StorageClassSnake string `json:"storage_class"`
			Jurisdiction      string `json:"jurisdiction"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return cloudflareParityR2BucketObservation{}, fmt.Errorf("decode Cloudflare R2 bucket observation: %w", err)
	}
	if !doc.Success {
		return cloudflareParityR2BucketObservation{}, fmt.Errorf("Cloudflare R2 bucket get was not successful")
	}
	return cloudflareParityR2BucketObservation{
		Exists:       true,
		Name:         doc.Result.Name,
		Location:     doc.Result.Location,
		StorageClass: firstNonEmptyCloudflareParityString(doc.Result.StorageClass, doc.Result.StorageClassSnake),
		Jurisdiction: doc.Result.Jurisdiction,
	}, nil
}

func firstNonEmptyCloudflareParityString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func waitCloudflareParityR2Bucket(ctx context.Context, bucketName string, wantExists bool) (cloudflareParityR2BucketObservation, error) {
	var last cloudflareParityR2BucketObservation
	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		observed, err := observeCloudflareParityR2Bucket(ctx, bucketName)
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
	return last, fmt.Errorf("timed out waiting for Cloudflare R2 bucket %s exists=%t", bucketName, wantExists)
}

func deleteCloudflareParityR2BucketIfExists(ctx context.Context, bucketName, lane string) error {
	if err := validateCloudflareParityBucketName(bucketName, lane); err != nil {
		return err
	}
	observed, err := observeCloudflareParityR2Bucket(ctx, bucketName)
	if err != nil {
		return err
	}
	if !observed.Exists {
		return nil
	}
	status, body, err := cloudflareParityAPIRequest(ctx, http.MethodDelete, "/accounts/"+cloudflareParityAccountID()+"/r2/buckets/"+bucketName)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status < 200 || status > 299 {
		return fmt.Errorf("Cloudflare R2 bucket delete returned HTTP %d: %s", status, sanitizeCloudflareParityCommandOutput(string(body)))
	}
	_, err = waitCloudflareParityR2Bucket(ctx, bucketName, false)
	return err
}

type cloudflareParityD1DatabaseObservation struct {
	Exists              bool
	Name                string
	UUID                string
	ReadReplicationMode string
}

type cloudflareParityD1DatabaseListItem struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
	ID   string `json:"id"`
}

func observeCloudflareParityD1DatabaseByName(ctx context.Context, databaseName, lane string) (cloudflareParityD1DatabaseObservation, error) {
	if err := validateCloudflareParityD1DatabaseName(databaseName, lane); err != nil {
		return cloudflareParityD1DatabaseObservation{}, err
	}
	values := url.Values{}
	values.Set("per_page", "100")
	values.Set("name", databaseName)
	status, body, err := cloudflareParityAPIRequest(ctx, http.MethodGet, "/accounts/"+cloudflareParityAccountID()+"/d1/database?"+values.Encode())
	if err != nil {
		return cloudflareParityD1DatabaseObservation{}, err
	}
	if status < 200 || status > 299 {
		return cloudflareParityD1DatabaseObservation{}, fmt.Errorf("Cloudflare D1 database list returned HTTP %d: %s", status, sanitizeCloudflareParityCommandOutput(string(body)))
	}
	databases, err := decodeCloudflareParityD1DatabaseList(body)
	if err != nil {
		return cloudflareParityD1DatabaseObservation{}, err
	}
	for _, database := range databases {
		if database.Name == databaseName {
			return database, nil
		}
	}
	return cloudflareParityD1DatabaseObservation{Exists: false}, nil
}

func observeCloudflareParityD1DatabaseByID(ctx context.Context, databaseID, databaseName, lane string) (cloudflareParityD1DatabaseObservation, error) {
	if strings.TrimSpace(databaseID) == "" {
		return cloudflareParityD1DatabaseObservation{Exists: false}, nil
	}
	if err := validateCloudflareParityD1DatabaseName(databaseName, lane); err != nil {
		return cloudflareParityD1DatabaseObservation{}, err
	}
	status, body, err := cloudflareParityAPIRequest(ctx, http.MethodGet, "/accounts/"+cloudflareParityAccountID()+"/d1/database/"+url.PathEscape(databaseID))
	if err != nil {
		return cloudflareParityD1DatabaseObservation{}, err
	}
	if status == http.StatusNotFound {
		return cloudflareParityD1DatabaseObservation{Exists: false}, nil
	}
	if status < 200 || status > 299 {
		return cloudflareParityD1DatabaseObservation{}, fmt.Errorf("Cloudflare D1 database get returned HTTP %d: %s", status, sanitizeCloudflareParityCommandOutput(string(body)))
	}
	database, err := decodeCloudflareParityD1DatabaseGet(body)
	if err != nil {
		return cloudflareParityD1DatabaseObservation{}, err
	}
	if database.Name != databaseName {
		return cloudflareParityD1DatabaseObservation{}, fmt.Errorf("Cloudflare D1 database UUID %s resolved to unexpected name %q", databaseID, database.Name)
	}
	return database, nil
}

func waitCloudflareParityD1DatabaseByName(ctx context.Context, databaseName, lane string, wantExists bool) (cloudflareParityD1DatabaseObservation, error) {
	var last cloudflareParityD1DatabaseObservation
	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		observed, err := observeCloudflareParityD1DatabaseByName(ctx, databaseName, lane)
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
	return last, fmt.Errorf("timed out waiting for Cloudflare D1 database %s exists=%t", databaseName, wantExists)
}

func deleteCloudflareParityD1DatabasesByName(ctx context.Context, databaseName, lane string) error {
	if err := validateCloudflareParityD1DatabaseName(databaseName, lane); err != nil {
		return err
	}
	for {
		observed, err := observeCloudflareParityD1DatabaseByName(ctx, databaseName, lane)
		if err != nil {
			return err
		}
		if !observed.Exists {
			return nil
		}
		if err := deleteCloudflareParityD1DatabaseByID(ctx, observed.UUID, databaseName, lane); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func deleteCloudflareParityD1DatabaseByID(ctx context.Context, databaseID, databaseName, lane string) error {
	if strings.TrimSpace(databaseID) == "" {
		return nil
	}
	if err := validateCloudflareParityD1DatabaseName(databaseName, lane); err != nil {
		return err
	}
	if _, err := observeCloudflareParityD1DatabaseByID(ctx, databaseID, databaseName, lane); err != nil {
		return err
	}
	status, body, err := cloudflareParityAPIRequest(ctx, http.MethodDelete, "/accounts/"+cloudflareParityAccountID()+"/d1/database/"+url.PathEscape(databaseID))
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status < 200 || status > 299 {
		return fmt.Errorf("Cloudflare D1 database delete returned HTTP %d: %s", status, sanitizeCloudflareParityCommandOutput(string(body)))
	}
	return nil
}

func decodeCloudflareParityD1DatabaseList(body []byte) ([]cloudflareParityD1DatabaseObservation, error) {
	var doc struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode Cloudflare D1 database list: %w", err)
	}
	if !doc.Success {
		return nil, fmt.Errorf("Cloudflare D1 database list was not successful")
	}
	var items []cloudflareParityD1DatabaseListItem
	if err := json.Unmarshal(doc.Result, &items); err == nil {
		return cloudflareParityD1ObservationsFromItems(items), nil
	}
	var wrapped struct {
		Databases []cloudflareParityD1DatabaseListItem `json:"databases"`
	}
	if err := json.Unmarshal(doc.Result, &wrapped); err != nil {
		return nil, fmt.Errorf("decode Cloudflare D1 database list result: %w", err)
	}
	return cloudflareParityD1ObservationsFromItems(wrapped.Databases), nil
}

func decodeCloudflareParityD1DatabaseGet(body []byte) (cloudflareParityD1DatabaseObservation, error) {
	var doc struct {
		Success bool `json:"success"`
		Result  struct {
			Name            string `json:"name"`
			UUID            string `json:"uuid"`
			ID              string `json:"id"`
			ReadReplication struct {
				Mode string `json:"mode"`
			} `json:"read_replication"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return cloudflareParityD1DatabaseObservation{}, fmt.Errorf("decode Cloudflare D1 database get: %w", err)
	}
	if !doc.Success {
		return cloudflareParityD1DatabaseObservation{}, fmt.Errorf("Cloudflare D1 database get was not successful")
	}
	return cloudflareParityD1DatabaseObservation{
		Exists:              true,
		Name:                doc.Result.Name,
		UUID:                firstNonEmptyCloudflareParityString(doc.Result.UUID, doc.Result.ID),
		ReadReplicationMode: doc.Result.ReadReplication.Mode,
	}, nil
}

func cloudflareParityD1ObservationsFromItems(items []cloudflareParityD1DatabaseListItem) []cloudflareParityD1DatabaseObservation {
	out := make([]cloudflareParityD1DatabaseObservation, 0, len(items))
	for _, item := range items {
		out = append(out, cloudflareParityD1DatabaseObservation{
			Exists: true,
			Name:   item.Name,
			UUID:   firstNonEmptyCloudflareParityString(item.UUID, item.ID),
		})
	}
	return out
}

func cloudflareParityAPIRequest(ctx context.Context, method, path string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, "https://api.cloudflare.com/client/v4"+path, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv("CLOUDFLARE_API_TOKEN"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	var body []byte
	if resp.Body != nil {
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return resp.StatusCode, nil, err
		}
	}
	return resp.StatusCode, body, nil
}

func cloudflareParityR2LifecycleFields(afterCreate, afterUpdate, afterDelete cloudflareParityR2BucketObservation, noOp bool) map[string]any {
	return map[string]any{
		"after_create.exists":        afterCreate.Exists,
		"after_create.name":          afterCreate.Name,
		"after_create.location":      normalizeCloudflareParityR2Location(afterCreate.Location),
		"after_create.storage_class": normalizeCloudflareParityR2StorageClass(afterCreate.StorageClass),
		"after_create.jurisdiction":  normalizeCloudflareParityR2Jurisdiction(afterCreate.Jurisdiction),
		"after_update.exists":        afterUpdate.Exists,
		"after_update.storage_class": normalizeCloudflareParityR2StorageClass(afterUpdate.StorageClass),
		"no_op":                      noOp,
		"after_delete.exists":        afterDelete.Exists,
	}
}

func cloudflareParityD1Fields(afterCreate, afterCleanup cloudflareParityD1DatabaseObservation, noOp, deleteViaRuntime bool) map[string]any {
	return map[string]any{
		"after_create.exists":       afterCreate.Exists,
		"after_create.name":         afterCreate.Name,
		"after_create.uuid_present": strings.TrimSpace(afterCreate.UUID) != "",
		"no_op":                     noOp,
		"delete_via_runtime":        deleteViaRuntime,
		"after_cleanup.exists":      afterCleanup.Exists,
	}
}

func cloudflareParityD1UpdateFields(afterCreate, afterUpdate, afterCleanup cloudflareParityD1DatabaseObservation, noOp, deleteViaRuntime bool) map[string]any {
	return map[string]any{
		"after_create.exists":                afterCreate.Exists,
		"after_create.name":                  afterCreate.Name,
		"after_create.uuid_present":          strings.TrimSpace(afterCreate.UUID) != "",
		"after_create.read_replication_mode": normalizeCloudflareParityD1ReadReplicationMode(afterCreate.ReadReplicationMode),
		"after_update.exists":                afterUpdate.Exists,
		"after_update.read_replication_mode": normalizeCloudflareParityD1ReadReplicationMode(afterUpdate.ReadReplicationMode),
		"no_op":                              noOp,
		"delete_via_runtime":                 deleteViaRuntime,
		"after_cleanup.exists":               afterCleanup.Exists,
	}
}

func cloudflareParityR2ReadMissingFields(afterCreate, afterDelete cloudflareParityR2BucketObservation, missing bool) map[string]any {
	return map[string]any{
		"after_create.exists":             afterCreate.Exists,
		"after_create.name":               afterCreate.Name,
		"after_out_of_band_delete.exists": afterDelete.Exists,
		"read_missing.missing":            missing,
		"after_cleanup.exists":            afterDelete.Exists,
	}
}

func normalizeCloudflareParityR2Location(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return strings.ToUpper(value)
}

func normalizeCloudflareParityR2StorageClass(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "standard":
		return "Standard"
	case "infrequentaccess", "infrequent_access":
		return "InfrequentAccess"
	default:
		return value
	}
}

func normalizeCloudflareParityR2Jurisdiction(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return strings.ToLower(value)
}

func normalizeCloudflareParityD1ReadReplicationMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto":
		return "auto"
	case "disabled", "":
		return "disabled"
	default:
		return value
	}
}

func validateCloudflareParityBucketName(name, lane string) error {
	prefix := "ramen-parity-" + lane + "-"
	if !strings.HasPrefix(name, prefix) {
		return fmt.Errorf("Cloudflare R2 bucket name %q must use %s* prefix", name, prefix)
	}
	if len(name) > 63 {
		return fmt.Errorf("Cloudflare R2 bucket name %q is too long", name)
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			continue
		}
		return fmt.Errorf("Cloudflare R2 bucket name %q contains unsupported character %q", name, r)
	}
	return nil
}

func validateCloudflareParityD1DatabaseName(name, lane string) error {
	prefix := "ramen-parity-" + lane + "-"
	if !strings.HasPrefix(name, prefix) {
		return fmt.Errorf("Cloudflare D1 database name %q must use %s* prefix", name, prefix)
	}
	if len(name) > 63 {
		return fmt.Errorf("Cloudflare D1 database name %q is too long", name)
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			continue
		}
		return fmt.Errorf("Cloudflare D1 database name %q contains unsupported character %q", name, r)
	}
	return nil
}

func cloudflareParityRunSuffix() string {
	return fmt.Sprintf("%x", time.Now().UTC().UnixNano())[:8]
}

func cloudflareParityBucketName(lane, runtime, suffix string) string {
	return "ramen-parity-" + lane + "-" + runtime + "-" + suffix
}

func cloudflareParityD1DatabaseName(lane, runtime, suffix string) string {
	return "ramen-parity-" + lane + "-" + runtime + "-" + suffix
}

func cloudflareParityAccountID() string {
	return strings.TrimSpace(os.Getenv("CLOUDFLARE_ACCOUNT_ID"))
}

func cloudflareParityEnvForTools() []string {
	return []string{
		"CLOUDFLARE_ACCOUNT_ID=" + cloudflareParityAccountID(),
		"CLOUDFLARE_API_TOKEN=" + strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")),
	}
}

func sanitizeCloudflareParityCommandOutput(output string) string {
	replacements := []string{
		os.Getenv("CLOUDFLARE_API_TOKEN"),
		os.Getenv("UDON_CREDENTIAL_CLOUDFLARE_API_TOKEN"),
		cloudflareParityAccountID(),
	}
	for _, value := range replacements {
		if strings.TrimSpace(value) != "" {
			output = strings.ReplaceAll(output, value, "<redacted>")
		}
	}
	return output
}

func lastNonEmptyCloudflareParityLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}
