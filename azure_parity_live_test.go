//go:build azurelive

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

func TestAzureProviderParityLive(t *testing.T) {
	if os.Getenv(azureParityEnv) != "1" {
		t.Skipf("set %s=1 and %s=<lane> to run the opt-in Azure provider parity harness", azureParityEnv, azureParityLaneEnv)
	}
	selectedLane := strings.ToLower(strings.TrimSpace(os.Getenv(azureParityLaneEnv)))
	if selectedLane == "" {
		t.Fatalf("%s=1 requires explicit %s selection", azureParityEnv, azureParityLaneEnv)
	}
	if !slices.Contains(azureParityLanes, selectedLane) {
		t.Fatalf("%s=%s is not a known Azure parity lane", azureParityLaneEnv, selectedLane)
	}
	artifact := loadAzureParityArtifact(t, filepath.Join(azureParityFixtureRoot, selectedLane, "observations.json"))
	assertAzureParityArtifact(t, selectedLane, artifact)
	if !artifact.Safety.LiveEnabled {
		t.Skipf("%s=%s is live-disabled by artifact metadata", azureParityLaneEnv, selectedLane)
	}
	requireAzureParityLiveEnv(t, artifact)
	if !slices.Contains(azureParityLiveRunnerLanes, selectedLane) {
		t.Fatalf("%s=%s is marked live-enabled but has no registered Azure live runner", azureParityLaneEnv, selectedLane)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()
	var recording azureParityLiveRecording
	switch selectedLane {
	case "z01":
		recording = runAzureParityZ01Live(ctx, t, artifact)
	case "z02":
		recording = runAzureParityZ02Live(ctx, t, artifact)
	default:
		t.Fatalf("%s=%s is marked live-enabled but has no live runner implementation", azureParityLaneEnv, selectedLane)
	}
	compareOrUpdateAzureParityRecording(t, recording, filepath.Join(azureParityFixtureRoot, selectedLane, "live.observations.json"))
}

func requireAzureParityLiveEnv(t *testing.T, artifact azureParityArtifact) {
	t.Helper()
	for _, envName := range artifact.Safety.RequiredEnv {
		if strings.TrimSpace(os.Getenv(envName)) == "" {
			t.Fatalf("%s is required for live Azure provider parity", envName)
		}
	}
	for _, envName := range []string{artifact.Safety.CredentialEnv, azureParityTofuEnv} {
		if strings.TrimSpace(os.Getenv(envName)) == "" {
			t.Fatalf("%s is required for live Azure provider parity", envName)
		}
	}
	if azureParityLiveBaselineMode() == "both" && strings.TrimSpace(os.Getenv(azureParityTerraformEnv)) == "" {
		t.Fatalf("%s is required for live Azure provider parity when %s=both", azureParityTerraformEnv, azureParityBaselineEnv)
	}
}

func azureParityLiveBaselineMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(azureParityBaselineEnv)))
	switch mode {
	case "", "opentofu":
		return "opentofu"
	case "both":
		return "both"
	default:
		return mode
	}
}

func requireSupportedAzureParityBaseline(t *testing.T) {
	t.Helper()
	switch azureParityLiveBaselineMode() {
	case "opentofu", "both":
	default:
		t.Fatalf("%s=%q is unsupported; use opentofu or both", azureParityBaselineEnv, os.Getenv(azureParityBaselineEnv))
	}
}

func runAzureParityHCLRuntime(ctx context.Context, t *testing.T, tool string) azureParityRuntimeResult {
	t.Helper()
	runtimeName := "terraform"
	if strings.Contains(filepath.Base(tool), "tofu") {
		runtimeName = "opentofu"
	}
	resourceGroup := strings.TrimSpace(os.Getenv("RAMEN_AZURE_SQL_RESOURCE_GROUP"))
	serverName := strings.TrimSpace(os.Getenv("RAMEN_AZURE_SQL_SERVER"))
	subscriptionID := strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID"))
	databaseName := azureParityZ01DatabaseName(runtimeName)
	if err := validateAzureParityZ01DatabaseName(databaseName); err != nil {
		return azureParityFailure(runtimeName, "safety", err)
	}
	if err := deleteAzureParitySQLDatabaseIfExists(ctx, resourceGroup, serverName, databaseName); err != nil {
		return azureParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteAzureParitySQLDatabaseIfExists(context.Background(), resourceGroup, serverName, databaseName); err != nil {
			t.Logf("cleanup Azure SQL database %s: %v", databaseName, err)
		}
	})
	workDir := filepath.Join(t.TempDir(), runtimeName)
	if err := copyFixtureFile(filepath.Join(azureParityFixtureRoot, "z01", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	tfvars := map[string]string{
		"subscription_id":     subscriptionID,
		"resource_group_name": resourceGroup,
		"server_name":         serverName,
		"database_name":       databaseName,
	}
	if err := writeJSONFile(filepath.Join(workDir, "terraform.tfvars.json"), tfvars); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	env := append(os.Environ(), azureParityARMEnvFromProfile()...)
	if err := runAzureParityCommand(ctx, workDir, env, tool, "init", "-input=false", "-no-color"); err != nil {
		return azureParityFailure(runtimeName, "init", err)
	}
	if err := runAzureParityCommand(ctx, workDir, env, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return azureParityFailure(runtimeName, "apply", err)
	}
	afterApply, err := observeAzureParitySQLDatabase(ctx, resourceGroup, serverName, databaseName)
	if err != nil {
		return azureParityFailure(runtimeName, "observe", err)
	}
	planExit, _, err := runAzureParityPlan(ctx, workDir, env, tool)
	if err != nil {
		return azureParityFailure(runtimeName, "plan", err)
	}
	if err := runAzureParityCommand(ctx, workDir, env, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return azureParityFailure(runtimeName, "destroy", err)
	}
	afterDestroy, err := observeAzureParitySQLDatabase(ctx, resourceGroup, serverName, databaseName)
	if err != nil {
		return azureParityFailure(runtimeName, "observe", err)
	}
	return azureParityRuntimeResult{Observation: azureParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: databaseName,
		Fields:   azureParitySQLObservationFields(afterApply, afterDestroy, planExit == 0),
	}}
}

func runAzureParityPlan(ctx context.Context, dir string, env []string, tool string) (int, string, error) {
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

func runAzureParityCommand(ctx context.Context, dir string, env []string, tool string, args ...string) error {
	cmd := osexec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", filepath.Base(tool), strings.Join(args, " "), err, sanitizeAzureParityCommandOutput(string(out)))
	}
	return nil
}

type azureParitySQLDatabaseObservation struct {
	Exists    bool
	Name      string
	Location  string
	SKUName   string
	SKUTier   string
	IDPresent bool
}

func observeAzureParitySQLDatabase(ctx context.Context, resourceGroup, serverName, databaseName string) (azureParitySQLDatabaseObservation, error) {
	if err := validateAzureParityZ01DatabaseName(databaseName); err != nil {
		return azureParitySQLDatabaseObservation{}, err
	}
	args := []string{
		"sql", "db", "show",
		"--resource-group", resourceGroup,
		"--server", serverName,
		"--name", databaseName,
		"--subscription", strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID")),
		"-o", "json",
	}
	out, err := osexec.CommandContext(ctx, "az", args...).CombinedOutput()
	if err != nil {
		if isAzureParityNotFound(string(out)) {
			return azureParitySQLDatabaseObservation{Exists: false}, nil
		}
		return azureParitySQLDatabaseObservation{}, fmt.Errorf("az sql db show failed: %w: %s", err, sanitizeAzureParityCommandOutput(string(out)))
	}
	var doc struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Location string `json:"location"`
		SKU      struct {
			Name string `json:"name"`
			Tier string `json:"tier"`
		} `json:"sku"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return azureParitySQLDatabaseObservation{}, fmt.Errorf("decode Azure SQL observation: %w", err)
	}
	return azureParitySQLDatabaseObservation{
		Exists:    true,
		Name:      doc.Name,
		Location:  doc.Location,
		SKUName:   doc.SKU.Name,
		SKUTier:   doc.SKU.Tier,
		IDPresent: strings.TrimSpace(doc.ID) != "",
	}, nil
}

func deleteAzureParitySQLDatabaseIfExists(ctx context.Context, resourceGroup, serverName, databaseName string) error {
	observed, err := observeAzureParitySQLDatabase(ctx, resourceGroup, serverName, databaseName)
	if err != nil {
		return err
	}
	if !observed.Exists {
		return nil
	}
	args := []string{
		"sql", "db", "delete",
		"--resource-group", resourceGroup,
		"--server", serverName,
		"--name", databaseName,
		"--subscription", strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID")),
		"--yes",
	}
	out, err := osexec.CommandContext(ctx, "az", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("az sql db delete failed: %w: %s", err, sanitizeAzureParityCommandOutput(string(out)))
	}
	return nil
}

func azureParityZ01DatabaseName(runtime string) string {
	return "ramen-parity-z01-" + runtime
}

func validateAzureParityZ01DatabaseName(name string) error {
	if !strings.HasPrefix(name, "ramen-parity-z01-") {
		return fmt.Errorf("database name %q must use ramen-parity-z01-* prefix", name)
	}
	if len(name) > 63 {
		return fmt.Errorf("database name %q is too long", name)
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			continue
		}
		return fmt.Errorf("database name %q contains unsupported character %q", name, r)
	}
	return nil
}

func azureParitySQLObservationFields(afterApply, afterDestroy azureParitySQLDatabaseObservation, noOp bool) map[string]any {
	return map[string]any{
		"after_apply.exists":     afterApply.Exists,
		"after_apply.id_present": afterApply.IDPresent,
		"after_apply.name":       afterApply.Name,
		"after_apply.location":   afterApply.Location,
		"after_apply.sku.name":   afterApply.SKUName,
		"after_apply.sku.tier":   afterApply.SKUTier,
		"no_op":                  noOp,
		"after_destroy.exists":   afterDestroy.Exists,
	}
}

func isAzureParityNotFound(out string) bool {
	msg := strings.ToLower(out)
	return strings.Contains(msg, "not found") || strings.Contains(msg, "notfound") || strings.Contains(msg, "resourcenotfound") || strings.Contains(msg, "does not exist")
}

func sanitizeAzureParityCommandOutput(out string) string {
	out = strings.ReplaceAll(out, strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID")), "<subscription-id>")
	out = strings.ReplaceAll(out, strings.TrimSpace(os.Getenv("AZURE_TENANT_ID")), "<tenant-id>")
	out = strings.ReplaceAll(out, strings.TrimSpace(os.Getenv("AZURE_CLIENT_ID")), "<client-id>")
	out = strings.ReplaceAll(out, strings.TrimSpace(os.Getenv("AZURE_CLIENT_SECRET")), "<client-secret>")
	out = strings.ReplaceAll(out, strings.TrimSpace(os.Getenv("UDON_CREDENTIAL_AZURE_AUTH")), "<azure-token>")
	out = strings.TrimSpace(out)
	if len(out) > 2000 {
		out = out[:2000] + "...<truncated>"
	}
	return out
}
