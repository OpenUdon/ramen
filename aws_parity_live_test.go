//go:build awslive

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

func TestAWSProviderParityLive(t *testing.T) {
	if os.Getenv(awsParityEnv) != "1" {
		t.Skipf("set %s=1 and %s=<lane> to run the opt-in AWS provider parity harness", awsParityEnv, awsParityLaneEnv)
	}
	selectedLane := strings.ToLower(strings.TrimSpace(os.Getenv(awsParityLaneEnv)))
	if selectedLane == "" {
		t.Fatalf("%s=1 requires explicit %s selection", awsParityEnv, awsParityLaneEnv)
	}
	if !slices.Contains(awsParityLanes, selectedLane) {
		t.Fatalf("%s=%s is not a known AWS parity lane", awsParityLaneEnv, selectedLane)
	}
	artifact := loadAWSParityArtifact(t, filepath.Join(awsParityFixtureRoot, selectedLane, "observations.json"))
	assertAWSParityArtifact(t, selectedLane, artifact)
	if !artifact.Safety.LiveEnabled {
		t.Skipf("%s=%s is live-disabled by artifact metadata", awsParityLaneEnv, selectedLane)
	}
	requireAWSParityLiveEnv(t, artifact)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	var recording awsParityLiveRecording
	switch selectedLane {
	case "w01":
		recording = runAWSParityW01Live(ctx, t, artifact)
	default:
		t.Skipf("%s=%s is not live-enabled in this implementation", awsParityLaneEnv, selectedLane)
	}
	compareOrUpdateAWSParityRecording(t, recording, filepath.Join(awsParityFixtureRoot, selectedLane, "live.observations.json"))
}

func requireAWSParityLiveEnv(t *testing.T, artifact awsParityArtifact) {
	t.Helper()
	for _, envName := range artifact.Safety.RequiredEnv {
		if strings.TrimSpace(os.Getenv(envName)) == "" {
			t.Fatalf("%s is required for live AWS provider parity", envName)
		}
	}
	if strings.TrimSpace(os.Getenv(artifact.Safety.OpenTofuEnv)) == "" {
		t.Fatalf("%s is required for live AWS provider parity", artifact.Safety.OpenTofuEnv)
	}
}

func runAWSParityW01OpenTofuRuntime(ctx context.Context, t *testing.T, userName string) awsParityRuntimeResult {
	t.Helper()
	runtimeName := "opentofu"
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
	if err := copyFixtureFile(filepath.Join(awsParityFixtureRoot, "w01", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return awsParityFailure(runtimeName, "fixture", err)
	}
	tfvars := map[string]string{
		"region":    awsParityRegion(),
		"user_name": userName,
	}
	if err := writeJSONFile(filepath.Join(workDir, "terraform.tfvars.json"), tfvars); err != nil {
		return awsParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), awsParityEnvForTools()...)
	tool := os.Getenv(awsParityTofuEnv)
	if err := runAWSParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return awsParityFailure(runtimeName, "init", err)
	}
	if err := runAWSParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return awsParityFailure(runtimeName, "apply", err)
	}
	afterApply, err := observeAWSParityIAMUser(ctx, userName)
	if err != nil {
		return awsParityFailure(runtimeName, "observe", err)
	}
	planExit, _, err := runAWSParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return awsParityFailure(runtimeName, "plan", err)
	}
	if err := runAWSParityCommand(ctx, workDir, env, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return awsParityFailure(runtimeName, "destroy", err)
	}
	afterDestroy, err := waitAWSParityIAMUser(ctx, userName, false)
	if err != nil {
		return awsParityFailure(runtimeName, "observe", err)
	}
	return awsParityRuntimeResult{Observation: awsParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: userName,
		Fields:   awsParityIAMUserObservationFields(afterApply, afterDestroy, planExit == 0),
	}}
}

func runAWSParityPlan(ctx context.Context, dir string, env []string, tool string) (int, string, error) {
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

func runAWSParityCommand(ctx context.Context, dir string, env []string, tool string, args ...string) error {
	cmd := osexec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", filepath.Base(tool), strings.Join(args, " "), err, sanitizeAWSParityCommandOutput(string(out)))
	}
	return nil
}

type awsParityIAMUserObservation struct {
	Exists        bool
	UserName      string
	ArnPresent    bool
	UserIDPresent bool
}

func observeAWSParityIAMUser(ctx context.Context, userName string) (awsParityIAMUserObservation, error) {
	if err := validateAWSParityW01UserName(userName); err != nil {
		return awsParityIAMUserObservation{}, err
	}
	args := []string{"iam", "get-user", "--user-name", userName, "--output", "json"}
	cmd := osexec.CommandContext(ctx, "aws", args...)
	cmd.Env = append(os.Environ(), awsParityEnvForTools()...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if isAWSParityNotFound(string(out)) {
			return awsParityIAMUserObservation{Exists: false}, nil
		}
		return awsParityIAMUserObservation{}, fmt.Errorf("aws iam get-user failed: %w: %s", err, sanitizeAWSParityCommandOutput(string(out)))
	}
	var doc struct {
		User struct {
			UserName string `json:"UserName"`
			Arn      string `json:"Arn"`
			UserID   string `json:"UserId"`
		} `json:"User"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return awsParityIAMUserObservation{}, fmt.Errorf("decode AWS IAM user observation: %w", err)
	}
	return awsParityIAMUserObservation{
		Exists:        true,
		UserName:      doc.User.UserName,
		ArnPresent:    strings.TrimSpace(doc.User.Arn) != "",
		UserIDPresent: strings.TrimSpace(doc.User.UserID) != "",
	}, nil
}

func waitAWSParityIAMUser(ctx context.Context, userName string, wantExists bool) (awsParityIAMUserObservation, error) {
	var last awsParityIAMUserObservation
	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		observed, err := observeAWSParityIAMUser(ctx, userName)
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
	return last, fmt.Errorf("timed out waiting for AWS IAM user %s exists=%t", userName, wantExists)
}

func deleteAWSParityIAMUserIfExists(ctx context.Context, userName string) error {
	observed, err := observeAWSParityIAMUser(ctx, userName)
	if err != nil {
		return err
	}
	if !observed.Exists {
		return nil
	}
	args := []string{"iam", "delete-user", "--user-name", userName}
	cmd := osexec.CommandContext(ctx, "aws", args...)
	cmd.Env = append(os.Environ(), awsParityEnvForTools()...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("aws iam delete-user failed: %w: %s", err, sanitizeAWSParityCommandOutput(string(out)))
	}
	_, err = waitAWSParityIAMUser(ctx, userName, false)
	return err
}

func validateAWSParityW01UserName(name string) error {
	if !strings.HasPrefix(name, "ramen-parity-w01-") {
		return fmt.Errorf("IAM user name %q must use ramen-parity-w01-* prefix", name)
	}
	if len(name) > 64 {
		return fmt.Errorf("IAM user name %q is too long", name)
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			continue
		}
		return fmt.Errorf("IAM user name %q contains unsupported character %q", name, r)
	}
	return nil
}

func awsParityW01RunSuffix() string {
	return fmt.Sprintf("%x", time.Now().UTC().UnixNano())[:8]
}

func awsParityW01UserName(runtime, suffix string) string {
	return "ramen-parity-w01-" + runtime + "-" + suffix
}

func awsParityRegion() string {
	if region := strings.TrimSpace(os.Getenv("RAMEN_AWS_REGION")); region != "" {
		return region
	}
	return "us-east-1"
}

func awsParityEnvForTools() []string {
	region := awsParityRegion()
	out := []string{
		"AWS_REGION=" + region,
		"AWS_DEFAULT_REGION=" + region,
	}
	for _, envName := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"} {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			out = append(out, envName+"="+value)
		}
	}
	return out
}

func awsParityIAMUserObservationFields(afterApply, afterDestroy awsParityIAMUserObservation, noOp bool) map[string]any {
	return map[string]any{
		"after_apply.exists":          afterApply.Exists,
		"after_apply.arn_present":     afterApply.ArnPresent,
		"after_apply.user_id_present": afterApply.UserIDPresent,
		"no_op":                       noOp,
		"after_destroy.exists":        afterDestroy.Exists,
	}
}

func isAWSParityNotFound(out string) bool {
	msg := strings.ToLower(out)
	return strings.Contains(msg, "nosuchentity") || strings.Contains(msg, "no such entity") || strings.Contains(msg, "not found")
}

func sanitizeAWSParityCommandOutput(out string) string {
	for _, envName := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"} {
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
