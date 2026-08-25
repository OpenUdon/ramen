//go:build googlelive

package corpus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestGoogleProviderParityLive(t *testing.T) {
	if os.Getenv(googleParityEnv) != "1" {
		t.Skipf("set %s=1 and %s=<lane> to run the opt-in Google Cloud provider parity harness", googleParityEnv, googleParityLaneEnv)
	}
	selectedLane := strings.ToLower(strings.TrimSpace(os.Getenv(googleParityLaneEnv)))
	if selectedLane == "" {
		t.Fatalf("%s=1 requires explicit %s selection", googleParityEnv, googleParityLaneEnv)
	}
	if !slices.Contains(googleParityLanes, selectedLane) {
		t.Fatalf("%s=%s is not a known Google parity lane", googleParityLaneEnv, selectedLane)
	}
	artifact := loadGoogleParityArtifact(t, filepath.Join(googleParityFixtureRoot, selectedLane, "observations.json"))
	assertGoogleParityArtifact(t, selectedLane, artifact)
	if !artifact.Safety.LiveEnabled {
		t.Skipf("%s=%s is live-disabled by artifact metadata", googleParityLaneEnv, selectedLane)
	}
	requireGoogleParityLiveEnv(t, artifact)
	if !slices.Contains(googleParityLiveRunnerLanes, selectedLane) {
		t.Fatalf("%s=%s is marked live-enabled but has no registered Google live runner", googleParityLaneEnv, selectedLane)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	var recording googleParityLiveRecording
	switch selectedLane {
	case "y02":
		recording = runGoogleParityY02Live(ctx, t, artifact)
	case "y03":
		recording = runGoogleParityY03Live(ctx, t, artifact)
	case "y04":
		recording = runGoogleParityY04Live(ctx, t, artifact)
	case "y05":
		recording = runGoogleParityY05Live(ctx, t, artifact)
	case "y06":
		recording = runGoogleParityY06Live(ctx, t, artifact)
	case "y08":
		recording = runGoogleParityY08Live(ctx, t, artifact)
	default:
		t.Fatalf("%s=%s is marked live-enabled but has no live runner implementation", googleParityLaneEnv, selectedLane)
	}
	compareOrUpdateGoogleParityRecording(t, recording, filepath.Join(googleParityFixtureRoot, selectedLane, "live.observations.json"))
}

func requireGoogleParityLiveEnv(t *testing.T, artifact googleParityArtifact) {
	t.Helper()
	for _, envName := range artifact.Safety.RequiredEnv {
		if strings.TrimSpace(os.Getenv(envName)) == "" {
			t.Fatalf("%s is required for live Google provider parity", envName)
		}
	}
	if strings.TrimSpace(os.Getenv(artifact.Safety.OpenTofuEnv)) == "" {
		t.Fatalf("%s is required for live Google provider parity", artifact.Safety.OpenTofuEnv)
	}
}

func runGoogleParityY02OpenTofuRuntime(ctx context.Context, t *testing.T, bucketName string) googleParityRuntimeResult {
	t.Helper()
	runtimeName := "opentofu"
	if err := validateGoogleParityBucketName(bucketName); err != nil {
		return googleParityFailure(runtimeName, "safety", err)
	}
	workDir := filepath.Join(t.TempDir(), runtimeName)
	if err := copyGoogleParityFixtureFile(filepath.Join(googleParityFixtureRoot, "y02", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	tfvars := map[string]string{
		"project":     googleParityProject(),
		"bucket_name": bucketName,
	}
	if err := writeGoogleParityJSONFile(filepath.Join(workDir, "terraform.tfvars.json"), tfvars); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), googleParityEnvForTools()...)
	tool := os.Getenv(googleParityTofuEnv)
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return googleParityFailure(runtimeName, "init", err)
	}
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
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

func runGoogleParityY03OpenTofuRuntime(ctx context.Context, t *testing.T, bucketName string) googleParityRuntimeResult {
	t.Helper()
	return runGoogleParityBucketLabelOpenTofuRuntime(ctx, t, bucketName, "y03")
}

func runGoogleParityY08OpenTofuRuntime(ctx context.Context, t *testing.T, bucketName string) googleParityRuntimeResult {
	t.Helper()
	return runGoogleParityBucketLabelOpenTofuRuntime(ctx, t, bucketName, "y08")
}

func runGoogleParityBucketLabelOpenTofuRuntime(ctx context.Context, t *testing.T, bucketName, lane string) googleParityRuntimeResult {
	t.Helper()
	runtimeName := "opentofu"
	if err := validateGoogleParityDisposableBucketName(bucketName, lane); err != nil {
		return googleParityFailure(runtimeName, "safety", err)
	}
	if observed, err := observeGoogleParityBucket(ctx, bucketName); err != nil {
		return googleParityFailure(runtimeName, "preflight", err)
	} else if observed.Exists {
		return googleParityFailure(runtimeName, "safety", fmt.Errorf("disposable bucket %s already exists", bucketName))
	}
	t.Cleanup(func() {
		if err := deleteGoogleParityBucketIfExists(context.Background(), bucketName, lane); err != nil {
			t.Logf("cleanup Google Cloud Storage bucket %s: %v", bucketName, err)
		}
	})
	workDir := filepath.Join(t.TempDir(), runtimeName)
	if err := copyGoogleParityFixtureFile(filepath.Join(googleParityFixtureRoot, lane, "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), googleParityEnvForTools()...)
	tool := os.Getenv(googleParityTofuEnv)
	if err := writeGoogleParityBucketLabelTFVars(workDir, bucketName, "create"); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return googleParityFailure(runtimeName, "init", err)
	}
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return googleParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeGoogleParityBucket(ctx, bucketName)
	if err != nil {
		return googleParityFailure(runtimeName, "observe", err)
	}
	planExit, _, err := runGoogleParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return googleParityFailure(runtimeName, "plan", err)
	}
	if err := writeGoogleParityBucketLabelTFVars(workDir, bucketName, "update"); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return googleParityFailure(runtimeName, "update", err)
	}
	afterUpdate, err := observeGoogleParityBucket(ctx, bucketName)
	if err != nil {
		return googleParityFailure(runtimeName, "observe", err)
	}
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return googleParityFailure(runtimeName, "destroy", err)
	}
	afterDestroy, err := waitGoogleParityBucket(ctx, bucketName, false)
	if err != nil {
		return googleParityFailure(runtimeName, "observe", err)
	}
	return googleParityRuntimeResult{Observation: googleParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: bucketName,
		Fields:   googleParityBucketMutationFields(afterCreate, afterUpdate, afterDestroy, planExit == 0),
	}}
}

func runGoogleParityY04OpenTofuRuntime(ctx context.Context, t *testing.T, bucketName string) googleParityRuntimeResult {
	t.Helper()
	runtimeName := "opentofu"
	if err := validateGoogleParityDisposableBucketName(bucketName, "y04"); err != nil {
		return googleParityFailure(runtimeName, "safety", err)
	}
	if observed, err := observeGoogleParityBucket(ctx, bucketName); err != nil {
		return googleParityFailure(runtimeName, "preflight", err)
	} else if observed.Exists {
		return googleParityFailure(runtimeName, "safety", fmt.Errorf("disposable bucket %s already exists", bucketName))
	}
	t.Cleanup(func() {
		if err := deleteGoogleParityBucketIfExists(context.Background(), bucketName, "y04"); err != nil {
			t.Logf("cleanup Google Cloud Storage bucket %s: %v", bucketName, err)
		}
	})
	workDir := filepath.Join(t.TempDir(), runtimeName)
	if err := copyGoogleParityFixtureFile(filepath.Join(googleParityFixtureRoot, "y04", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	if err := writeGoogleParityBucketTFVars(workDir, bucketName); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), googleParityEnvForTools()...)
	tool := os.Getenv(googleParityTofuEnv)
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return googleParityFailure(runtimeName, "init", err)
	}
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return googleParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeGoogleParityBucket(ctx, bucketName)
	if err != nil {
		return googleParityFailure(runtimeName, "observe", err)
	}
	noOpExit, _, err := runGoogleParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return googleParityFailure(runtimeName, "plan", err)
	}
	if err := deleteGoogleParityBucketIfExists(ctx, bucketName, "y04"); err != nil {
		return googleParityFailure(runtimeName, "out_of_band_delete", err)
	}
	afterDelete, err := waitGoogleParityBucket(ctx, bucketName, false)
	if err != nil {
		return googleParityFailure(runtimeName, "observe", err)
	}
	missingExit, _, err := runGoogleParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return googleParityFailure(runtimeName, "missing_plan", err)
	}
	return googleParityRuntimeResult{Observation: googleParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: bucketName,
		Fields:   googleParityBucketReadMissingFields(afterCreate, afterDelete, noOpExit == 0, missingExit == 2),
	}}
}

func runGoogleParityY05OpenTofuRuntime(ctx context.Context, t *testing.T, bucketName string) googleParityRuntimeResult {
	t.Helper()
	runtimeName := "opentofu"
	objectName := googleParityY05ObjectName()
	if err := validateGoogleParityDisposableBucketName(bucketName, "y05"); err != nil {
		return googleParityFailure(runtimeName, "safety", err)
	}
	if err := createGoogleParitySupportBucket(ctx, bucketName, "y05", false); err != nil {
		return googleParityFailure(runtimeName, "support_bucket", err)
	}
	t.Cleanup(func() {
		if err := deleteGoogleParityBucketIfExists(context.Background(), bucketName, "y05"); err != nil {
			t.Logf("cleanup Google Cloud Storage bucket %s: %v", bucketName, err)
		}
	})
	workDir := filepath.Join(t.TempDir(), runtimeName)
	if err := copyGoogleParityFixtureFile(filepath.Join(googleParityFixtureRoot, "y05", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	if err := writeGoogleParityBucketOnlyTFVars(workDir, bucketName); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), googleParityEnvForTools()...)
	tool := os.Getenv(googleParityTofuEnv)
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return googleParityFailure(runtimeName, "init", err)
	}
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return googleParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeGoogleParityObject(ctx, bucketName, objectName)
	if err != nil {
		return googleParityFailure(runtimeName, "observe", err)
	}
	planExit, _, err := runGoogleParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return googleParityFailure(runtimeName, "plan", err)
	}
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return googleParityFailure(runtimeName, "destroy", err)
	}
	afterDestroy, err := waitGoogleParityObject(ctx, bucketName, objectName, false)
	if err != nil {
		return googleParityFailure(runtimeName, "observe", err)
	}
	return googleParityRuntimeResult{Observation: googleParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: bucketName + "/" + objectName,
		Fields:   googleParityObjectMutationFields(afterCreate, afterDestroy, planExit == 0),
	}}
}

func runGoogleParityY06OpenTofuRuntime(ctx context.Context, t *testing.T, bucketName string) googleParityRuntimeResult {
	t.Helper()
	runtimeName := "opentofu"
	folderName := googleParityY06FolderName()
	if err := validateGoogleParityDisposableBucketName(bucketName, "y06"); err != nil {
		return googleParityFailure(runtimeName, "safety", err)
	}
	if err := createGoogleParitySupportBucket(ctx, bucketName, "y06", true); err != nil {
		return googleParityFailure(runtimeName, "support_bucket", err)
	}
	t.Cleanup(func() {
		if err := deleteGoogleParityBucketIfExists(context.Background(), bucketName, "y06"); err != nil {
			t.Logf("cleanup Google Cloud Storage bucket %s: %v", bucketName, err)
		}
	})
	workDir := filepath.Join(t.TempDir(), runtimeName)
	if err := copyGoogleParityFixtureFile(filepath.Join(googleParityFixtureRoot, "y06", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	if err := writeGoogleParityBucketOnlyTFVars(workDir, bucketName); err != nil {
		return googleParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), googleParityEnvForTools()...)
	tool := os.Getenv(googleParityTofuEnv)
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return googleParityFailure(runtimeName, "init", err)
	}
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return googleParityFailure(runtimeName, "apply", err)
	}
	afterCreate, err := observeGoogleParityManagedFolder(ctx, bucketName, folderName)
	if err != nil {
		return googleParityFailure(runtimeName, "observe", err)
	}
	planExit, _, err := runGoogleParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return googleParityFailure(runtimeName, "plan", err)
	}
	if err := runGoogleParityCommand(ctx, workDir, env, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return googleParityFailure(runtimeName, "destroy", err)
	}
	afterDestroy, err := waitGoogleParityManagedFolder(ctx, bucketName, folderName, false)
	if err != nil {
		return googleParityFailure(runtimeName, "observe", err)
	}
	return googleParityRuntimeResult{Observation: googleParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: bucketName + "/" + folderName,
		Fields:   googleParityManagedFolderMutationFields(afterCreate, afterDestroy, planExit == 0),
	}}
}

func writeGoogleParityY03TFVars(dir, bucketName, phase string) error {
	return writeGoogleParityBucketLabelTFVars(dir, bucketName, phase)
}

func writeGoogleParityBucketLabelTFVars(dir, bucketName, phase string) error {
	return writeGoogleParityJSONFile(filepath.Join(dir, "terraform.tfvars.json"), map[string]string{
		"project":      googleParityProject(),
		"bucket_name":  bucketName,
		"location":     googleParityLocation(),
		"parity_phase": phase,
	})
}

func writeGoogleParityBucketTFVars(dir, bucketName string) error {
	return writeGoogleParityJSONFile(filepath.Join(dir, "terraform.tfvars.json"), map[string]string{
		"project":     googleParityProject(),
		"bucket_name": bucketName,
		"location":    googleParityLocation(),
	})
}

func writeGoogleParityBucketOnlyTFVars(dir, bucketName string) error {
	return writeGoogleParityJSONFile(filepath.Join(dir, "terraform.tfvars.json"), map[string]string{
		"project":     googleParityProject(),
		"bucket_name": bucketName,
	})
}

func runGoogleParityPlan(ctx context.Context, dir string, env []string, tool string) (int, string, error) {
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
		return exitErr.ExitCode(), summary, fmt.Errorf("%s plan failed with exit %d: %w", filepath.Base(tool), exitErr.ExitCode(), err)
	}
	return -1, summary, fmt.Errorf("%s plan failed: %w", filepath.Base(tool), err)
}

func runGoogleParityCommand(ctx context.Context, dir string, env []string, tool string, args ...string) error {
	cmd := osexec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", filepath.Base(tool), strings.Join(args, " "), err, sanitizeGoogleParityCommandOutput(string(out)))
	}
	return nil
}

type googleParityBucketObservation struct {
	Exists                   bool
	Name                     string
	ID                       string
	Location                 string
	UniformBucketLevelAccess any
	Labels                   map[string]string
}

type googleParityObjectObservation struct {
	Exists     bool
	Name       string
	Bucket     string
	ID         string
	Generation string
	Size       string
	Metadata   map[string]string
}

type googleParityManagedFolderObservation struct {
	Exists         bool
	Name           string
	Bucket         string
	ID             string
	Metageneration string
}

func observeGoogleParityBucket(ctx context.Context, bucketName string) (googleParityBucketObservation, error) {
	if err := validateGoogleParityBucketName(bucketName); err != nil {
		return googleParityBucketObservation{}, err
	}
	args := []string{"storage", "buckets", "describe", "gs://" + bucketName, "--format=json"}
	cmd := osexec.CommandContext(ctx, "gcloud", args...)
	cmd.Env = append(os.Environ(), googleParityEnvForTools()...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if isGoogleParityNotFound(string(out)) {
			return googleParityBucketObservation{Exists: false}, nil
		}
		return googleParityBucketObservation{}, fmt.Errorf("gcloud storage buckets describe failed: %w: %s", err, sanitizeGoogleParityCommandOutput(string(out)))
	}
	var doc struct {
		Name             string            `json:"name"`
		ID               string            `json:"id"`
		Location         string            `json:"location"`
		Labels           map[string]string `json:"labels"`
		IAMConfiguration struct {
			UniformBucketLevelAccess any `json:"uniformBucketLevelAccess"`
		} `json:"iamConfiguration"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return googleParityBucketObservation{}, fmt.Errorf("decode Google bucket observation: %w", err)
	}
	name := doc.Name
	if strings.TrimSpace(name) == "" {
		name = bucketName
	}
	return googleParityBucketObservation{
		Exists:                   true,
		Name:                     name,
		ID:                       doc.ID,
		Location:                 doc.Location,
		UniformBucketLevelAccess: doc.IAMConfiguration.UniformBucketLevelAccess,
		Labels:                   doc.Labels,
	}, nil
}

func waitGoogleParityBucket(ctx context.Context, bucketName string, wantExists bool) (googleParityBucketObservation, error) {
	var last googleParityBucketObservation
	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		observed, err := observeGoogleParityBucket(ctx, bucketName)
		if err == nil && observed.Exists == wantExists {
			return observed, nil
		}
		last = observed
		lastErr = err
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	if lastErr != nil {
		return last, lastErr
	}
	return last, fmt.Errorf("timed out waiting for Google bucket %s exists=%t", bucketName, wantExists)
}

func createGoogleParitySupportBucket(ctx context.Context, bucketName, lane string, hierarchicalNamespace bool) error {
	if err := validateGoogleParityDisposableBucketName(bucketName, lane); err != nil {
		return err
	}
	observed, err := observeGoogleParityBucket(ctx, bucketName)
	if err != nil {
		return err
	}
	if observed.Exists {
		return fmt.Errorf("disposable bucket %s already exists", bucketName)
	}
	args := []string{"storage", "buckets", "create", "gs://" + bucketName, "--project", googleParityProject(), "--location", googleParityLocation(), "--uniform-bucket-level-access", "--quiet"}
	if hierarchicalNamespace {
		args = append(args, "--enable-hierarchical-namespace")
	}
	cmd := osexec.CommandContext(ctx, "gcloud", args...)
	cmd.Env = append(os.Environ(), googleParityEnvForTools()...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gcloud storage buckets create failed: %w: %s", err, sanitizeGoogleParityCommandOutput(string(out)))
	}
	_, err = waitGoogleParityBucket(ctx, bucketName, true)
	return err
}

func deleteGoogleParityBucketIfExists(ctx context.Context, bucketName, lane string) error {
	if err := validateGoogleParityDisposableBucketName(bucketName, lane); err != nil {
		return err
	}
	observed, err := observeGoogleParityBucket(ctx, bucketName)
	if err != nil {
		return err
	}
	if !observed.Exists {
		return nil
	}
	args := []string{"storage", "rm", "--recursive", "gs://" + bucketName, "--quiet"}
	cmd := osexec.CommandContext(ctx, "gcloud", args...)
	cmd.Env = append(os.Environ(), googleParityEnvForTools()...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gcloud storage rm failed: %w: %s", err, sanitizeGoogleParityCommandOutput(string(out)))
	}
	_, err = waitGoogleParityBucket(ctx, bucketName, false)
	return err
}

func observeGoogleParityObject(ctx context.Context, bucketName, objectName string) (googleParityObjectObservation, error) {
	if err := validateGoogleParityBucketName(bucketName); err != nil {
		return googleParityObjectObservation{}, err
	}
	if strings.TrimSpace(objectName) == "" {
		return googleParityObjectObservation{}, fmt.Errorf("object name is required")
	}
	cmd := osexec.CommandContext(ctx, "gcloud", "storage", "objects", "describe", "gs://"+bucketName+"/"+objectName, "--format=json")
	cmd.Env = append(os.Environ(), googleParityEnvForTools()...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if isGoogleParityNotFound(string(out)) {
			return googleParityObjectObservation{Exists: false}, nil
		}
		return googleParityObjectObservation{}, fmt.Errorf("gcloud storage objects describe failed: %w: %s", err, sanitizeGoogleParityCommandOutput(string(out)))
	}
	var doc struct {
		Name       string            `json:"name"`
		Bucket     string            `json:"bucket"`
		ID         string            `json:"id"`
		Generation string            `json:"generation"`
		Size       any               `json:"size"`
		Metadata   map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return googleParityObjectObservation{}, fmt.Errorf("decode Google object observation: %w", err)
	}
	return googleParityObjectObservation{
		Exists:     true,
		Name:       doc.Name,
		Bucket:     doc.Bucket,
		ID:         doc.ID,
		Generation: doc.Generation,
		Size:       googleParityJSONScalarString(doc.Size),
		Metadata:   doc.Metadata,
	}, nil
}

func waitGoogleParityObject(ctx context.Context, bucketName, objectName string, wantExists bool) (googleParityObjectObservation, error) {
	var last googleParityObjectObservation
	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		observed, err := observeGoogleParityObject(ctx, bucketName, objectName)
		if err == nil && observed.Exists == wantExists {
			return observed, nil
		}
		last = observed
		lastErr = err
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	if lastErr != nil {
		return last, lastErr
	}
	return last, fmt.Errorf("timed out waiting for Google object gs://%s/%s exists=%t", bucketName, objectName, wantExists)
}

func observeGoogleParityManagedFolder(ctx context.Context, bucketName, folderName string) (googleParityManagedFolderObservation, error) {
	if err := validateGoogleParityBucketName(bucketName); err != nil {
		return googleParityManagedFolderObservation{}, err
	}
	if strings.TrimSpace(folderName) == "" || !strings.HasSuffix(folderName, "/") {
		return googleParityManagedFolderObservation{}, fmt.Errorf("managed folder name %q must end with /", folderName)
	}
	cmd := osexec.CommandContext(ctx, "gcloud", "storage", "managed-folders", "describe", "gs://"+bucketName+"/"+folderName, "--format=json")
	cmd.Env = append(os.Environ(), googleParityEnvForTools()...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if isGoogleParityNotFound(string(out)) {
			return googleParityManagedFolderObservation{Exists: false}, nil
		}
		return googleParityManagedFolderObservation{}, fmt.Errorf("gcloud storage managed-folders describe failed: %w: %s", err, sanitizeGoogleParityCommandOutput(string(out)))
	}
	var doc struct {
		Name           string `json:"name"`
		Bucket         string `json:"bucket"`
		ID             string `json:"id"`
		Metageneration any    `json:"metageneration"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return googleParityManagedFolderObservation{}, fmt.Errorf("decode Google managed folder observation: %w", err)
	}
	return googleParityManagedFolderObservation{
		Exists:         true,
		Name:           doc.Name,
		Bucket:         doc.Bucket,
		ID:             doc.ID,
		Metageneration: googleParityJSONScalarString(doc.Metageneration),
	}, nil
}

func googleParityJSONScalarString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return fmt.Sprint(typed)
	}
}

func waitGoogleParityManagedFolder(ctx context.Context, bucketName, folderName string, wantExists bool) (googleParityManagedFolderObservation, error) {
	var last googleParityManagedFolderObservation
	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		observed, err := observeGoogleParityManagedFolder(ctx, bucketName, folderName)
		if err == nil && observed.Exists == wantExists {
			return observed, nil
		}
		last = observed
		lastErr = err
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	if lastErr != nil {
		return last, lastErr
	}
	return last, fmt.Errorf("timed out waiting for Google managed folder gs://%s/%s exists=%t", bucketName, folderName, wantExists)
}

func validateGoogleParityBucketName(name string) error {
	name = strings.TrimSpace(name)
	if len(name) < 3 || len(name) > 63 {
		return fmt.Errorf("bucket name %q must be 3-63 characters", name)
	}
	if name[0] == '-' || name[len(name)-1] == '-' {
		return fmt.Errorf("bucket name %q must not start or end with hyphen", name)
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_' {
			continue
		}
		return fmt.Errorf("bucket name %q contains unsupported character %q", name, r)
	}
	return nil
}

func validateGoogleParityDisposableBucketName(name, lane string) error {
	if err := validateGoogleParityBucketName(name); err != nil {
		return err
	}
	if !strings.HasPrefix(name, "ramen-parity-"+lane+"-") {
		return fmt.Errorf("bucket name %q must use ramen-parity-%s-* prefix", name, lane)
	}
	if strings.Contains(name, ".") || strings.Contains(name, "_") {
		return fmt.Errorf("disposable bucket name %q must avoid dots and underscores", name)
	}
	return nil
}

func googleParityY03RunSuffix() string {
	return fmt.Sprintf("%x", time.Now().UTC().UnixNano())[:8]
}

func googleParityY03BucketName(runtime, suffix string) string {
	return "ramen-parity-y03-" + runtime + "-" + suffix
}

func googleParityBucketName(lane, runtime, suffix string) string {
	return "ramen-parity-" + lane + "-" + runtime + "-" + suffix
}

func googleParityY05ObjectName() string {
	return "ramen-parity-y05-object.txt"
}

func googleParityY06FolderName() string {
	return "managed/y06/"
}

func googleParityProject() string {
	return strings.TrimSpace(os.Getenv("RAMEN_GOOGLE_PROJECT"))
}

func googleParityLocation() string {
	if location := strings.TrimSpace(os.Getenv("RAMEN_GOOGLE_LOCATION")); location != "" {
		return location
	}
	return "US"
}

func googleParityExistingBucket() string {
	return strings.TrimSpace(os.Getenv("RAMEN_GOOGLE_EXISTING_BUCKET"))
}

func googleParityEnvForTools() []string {
	out := []string{
		"GOOGLE_APPLICATION_CREDENTIALS=" + strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
		"CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE=" + strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
		"CLOUDSDK_CORE_PROJECT=" + googleParityProject(),
		"GOOGLE_PROJECT=" + googleParityProject(),
	}
	return out
}

func googleParityBucketReadFields(observed googleParityBucketObservation, bucketName string) map[string]any {
	return map[string]any{
		"exists":           observed.Exists,
		"name_matches":     observed.Name == bucketName,
		"id_present":       strings.TrimSpace(observed.ID) != "",
		"location_present": strings.TrimSpace(observed.Location) != "",
		"uniform_bucket_level_access_field_present":   observed.UniformBucketLevelAccess != nil,
		"labels.ramen_parity_phase":                   observed.Labels["ramen_parity_phase"],
		"labels.ramen_parity_phase_update_observable": observed.Labels["ramen_parity_phase"] == "update",
	}
}

func googleParityBucketMutationFields(afterCreate, afterUpdate, afterDestroy googleParityBucketObservation, noOp bool) map[string]any {
	return map[string]any{
		"exists":                    afterCreate.Exists && !afterDestroy.Exists,
		"after_create.exists":       afterCreate.Exists,
		"after_create.name_present": strings.TrimSpace(afterCreate.Name) != "",
		"after_update.exists":       afterUpdate.Exists,
		"labels.ramen_parity_phase_update_observable": afterUpdate.Labels["ramen_parity_phase"] == "update",
		"no_op":                noOp,
		"after_destroy.exists": afterDestroy.Exists,
	}
}

func googleParityBucketReadMissingFields(afterCreate, afterDelete googleParityBucketObservation, noOpBeforeDelete, readMissingObserved bool) map[string]any {
	return map[string]any{
		"exists":                                afterCreate.Exists && !afterDelete.Exists,
		"after_create.exists":                   afterCreate.Exists,
		"after_out_of_band_delete.exists":       afterDelete.Exists,
		"no_op_before_out_of_band_delete":       noOpBeforeDelete,
		"read_missing_after_out_of_band_delete": readMissingObserved,
	}
}

func googleParityObjectMutationFields(afterCreate, afterDestroy googleParityObjectObservation, noOp bool) map[string]any {
	return map[string]any{
		"exists":                      afterCreate.Exists && !afterDestroy.Exists,
		"after_create.exists":         afterCreate.Exists,
		"after_create.name_present":   strings.TrimSpace(afterCreate.Name) != "",
		"after_create.bucket_present": strings.TrimSpace(afterCreate.Bucket) != "",
		"id_present":                  strings.TrimSpace(afterCreate.ID) != "",
		"generation_present":          strings.TrimSpace(afterCreate.Generation) != "",
		"size_present":                strings.TrimSpace(afterCreate.Size) != "",
		"metadata.ramen_parity_phase_update_observable": afterCreate.Metadata["ramen_parity_phase"] == "update",
		"no_op":                noOp,
		"after_destroy.exists": afterDestroy.Exists,
	}
}

func googleParityManagedFolderMutationFields(afterCreate, afterDestroy googleParityManagedFolderObservation, noOp bool) map[string]any {
	return map[string]any{
		"exists":                      afterCreate.Exists && !afterDestroy.Exists,
		"after_create.exists":         afterCreate.Exists,
		"after_create.name_present":   strings.TrimSpace(afterCreate.Name) != "",
		"after_create.bucket_present": strings.TrimSpace(afterCreate.Bucket) != "",
		"id_present":                  strings.TrimSpace(afterCreate.ID) != "",
		"metageneration_present":      strings.TrimSpace(afterCreate.Metageneration) != "",
		"no_op":                       noOp,
		"after_destroy.exists":        afterDestroy.Exists,
	}
}

func isGoogleParityNotFound(out string) bool {
	msg := strings.ToLower(out)
	return strings.Contains(msg, "not found") || strings.Contains(msg, "not exist") || strings.Contains(msg, "no urls matched") || strings.Contains(msg, "404")
}

func sanitizeGoogleParityCommandOutput(out string) string {
	for _, envName := range []string{"GOOGLE_APPLICATION_CREDENTIALS", "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE"} {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			out = strings.ReplaceAll(out, value, "<"+strings.ToLower(envName)+">")
		}
	}
	out = strings.TrimSpace(out)
	if len(out) > 2000 {
		out = out[:2000] + "...<truncated>"
	}
	return out
}

func copyGoogleParityFixtureFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func writeGoogleParityJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
