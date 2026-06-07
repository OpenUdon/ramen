//go:build azurelive && udon

package corpus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/ramen/apply"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/executor/udon"
	tfplan "github.com/OpenUdon/ramen/plan"
)

func runAzureParityZ01Live(ctx context.Context, t *testing.T, artifact azureParityArtifact) azureParityLiveRecording {
	t.Helper()
	started := time.Now()
	requireSupportedAzureParityBaseline(t)
	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, string) azureParityRuntimeResult
		tool    string
	}{
		{runtime: "opentofu", run: runAzureParityHCLRuntimeWithTool, tool: os.Getenv(azureParityTofuEnv)},
		{runtime: "ramen", run: runAzureParityRamenRuntime, tool: ""},
	}
	if azureParityLiveBaselineMode() == "both" {
		runs = slices.Insert(runs, 1, struct {
			runtime string
			run     func(context.Context, *testing.T, string) azureParityRuntimeResult
			tool    string
		}{runtime: "terraform", run: runAzureParityHCLRuntimeWithTool, tool: os.Getenv(azureParityTerraformEnv)})
	}
	var observations []azureParityRuntimeObservation
	var failures []azureParityRuntimeFailure
	for _, run := range runs {
		result := timedAzureParityRuntime(run.runtime, func() azureParityRuntimeResult {
			return run.run(ctx, t, run.tool)
		})
		if result.Failure != nil {
			failures = append(failures, *result.Failure)
			continue
		}
		observations = append(observations, result.Observation)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Logf("%s Azure parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("Z01 Azure provider parity did not complete for all runtimes")
	}
	comparison := compareAzureParityZ01Observations(observations)
	if !comparison.Matched {
		t.Fatalf("Z01 Azure provider parity observations did not match: %#v", observations)
	}
	return azureParityLiveRecording{
		Version:      azureParityArtifactV1,
		Lane:         "Z01",
		Scenario:     artifact.Scenarios[0].Name,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		DurationMS:   time.Since(started).Milliseconds(),
		Observations: observations,
		Comparison:   comparison,
	}
}

type azureParityZ02Scope struct {
	ResourceGroup string
	Location      string
	Suffix        string
}

func TestAzureParityZ02Render(t *testing.T) {
	t.Setenv("AZURE_SUBSCRIPTION_ID", "00000000-0000-0000-0000-000000000000")
	workDir := t.TempDir()
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	openAPIPath := filepath.Join(workDir, "ramen", "openapi", "cosmos.json")
	if err := copyFixtureFile(filepath.Join(azureParityFixtureRoot, "z02", "openapi", "cosmos.json"), openAPIPath); err != nil {
		t.Fatalf("copy Z02 OpenAPI fixture: %v", err)
	}
	scope := azureParityZ02Scope{
		ResourceGroup: "ramen-parity-z02-render",
		Location:      "eastus2",
		Suffix:        "render",
	}
	accountName := azureParityZ02AccountName("ramen", scope.Suffix)
	if err := renderAzureParityZ02Project(filepath.Join(azureParityFixtureRoot, "z02", "ramen", "project.uws.yaml"), projectPath, scope, accountName); err != nil {
		t.Fatalf("render Z02 project: %v", err)
	}
	data, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatalf("read rendered Z02 project: %v", err)
	}
	if got := strings.Count(string(data), "\n      runtime_hints:"); got != 1 {
		t.Fatalf("rendered Z02 runtime_hints blocks = %d, want 1:\n%s", got, string(data))
	}
	for _, tc := range []struct {
		action      string
		operationID string
		summary     string
	}{
		{action: "create", operationID: "DatabaseAccounts_CreateOrUpdate", summary: "create"},
		{action: "read", operationID: "DatabaseAccounts_Get", summary: "read"},
		{action: "delete", operationID: "DatabaseAccounts_Delete", summary: "delete"},
	} {
		result, err := tfplan.Build(context.Background(), tfplan.Options{
			ProjectPath: projectPath,
			StatePath:   filepath.Join(workDir, tc.action+".db"),
			Action:      tc.action,
		})
		if err != nil {
			t.Fatalf("build rendered Z02 %s plan: %v", tc.action, err)
		}
		if result.Plan.Errored || len(result.Plan.Resources) != 1 {
			t.Fatalf("rendered Z02 %s plan unusable: %#v", tc.action, result.Plan)
		}
		resource := result.Plan.Resources[0]
		if resource.Mapping == nil || resource.Mapping.OperationID != tc.operationID {
			t.Fatalf("rendered Z02 %s operation = %#v, want %s", tc.action, resource.Mapping, tc.operationID)
		}
		if !azureParitySummaryHasOne(result.Plan.Summary, tc.summary) {
			t.Fatalf("rendered Z02 %s summary = %#v, want one %s action", tc.action, result.Plan.Summary, tc.summary)
		}
	}
}

func runAzureParityZ02Live(ctx context.Context, t *testing.T, artifact azureParityArtifact) azureParityLiveRecording {
	t.Helper()
	started := time.Now()
	requireSupportedAzureParityBaseline(t)
	scope := azureParityZ02Scope{
		ResourceGroup: "ramen-parity-z02-" + azureParityZ02RunSuffix(),
		Location:      strings.TrimSpace(os.Getenv("RAMEN_AZURE_COSMOS_LOCATION")),
	}
	if scope.Location == "" {
		scope.Location = "eastus"
	}
	scope.Suffix = strings.TrimPrefix(scope.ResourceGroup, "ramen-parity-z02-")
	if err := ensureAzureParityProviderRegistered(ctx, "Microsoft.DocumentDB"); err != nil {
		t.Fatalf("Z02 Azure provider registration check failed: %v", err)
	}
	if err := createAzureParityResourceGroup(ctx, scope.ResourceGroup, scope.Location); err != nil {
		t.Fatalf("create Z02 isolated resource group: %v", err)
	}
	t.Cleanup(func() {
		if err := deleteAzureParityResourceGroup(context.Background(), scope.ResourceGroup); err != nil {
			t.Logf("cleanup Azure resource group %s: %v", scope.ResourceGroup, err)
		}
	})

	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, azureParityZ02Scope) azureParityRuntimeResult
	}{
		{runtime: "opentofu", run: runAzureParityZ02OpenTofuRuntime},
		{runtime: "ramen", run: runAzureParityZ02RamenRuntime},
	}
	if azureParityLiveBaselineMode() == "both" {
		runs = slices.Insert(runs, 1, struct {
			runtime string
			run     func(context.Context, *testing.T, azureParityZ02Scope) azureParityRuntimeResult
		}{runtime: "terraform", run: runAzureParityZ02TerraformRuntime})
	}
	var observations []azureParityRuntimeObservation
	var failures []azureParityRuntimeFailure
	for _, run := range runs {
		result := timedAzureParityRuntime(run.runtime, func() azureParityRuntimeResult {
			return run.run(ctx, t, scope)
		})
		if result.Failure != nil {
			failures = append(failures, *result.Failure)
			continue
		}
		observations = append(observations, result.Observation)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Logf("%s Azure Cosmos parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		if err := deleteAzureParityResourceGroup(ctx, scope.ResourceGroup); err != nil {
			t.Logf("cleanup failed Z02 resource group %s: %v", scope.ResourceGroup, err)
		}
		t.Fatalf("Z02 Azure provider parity did not complete for all runtimes")
	}
	if err := deleteAzureParityResourceGroup(ctx, scope.ResourceGroup); err != nil {
		t.Fatalf("delete Z02 isolated resource group: %v", err)
	}
	groupExists, err := observeAzureParityResourceGroup(ctx, scope.ResourceGroup)
	if err != nil {
		t.Fatalf("observe Z02 isolated resource group after cleanup: %v", err)
	}
	for i := range observations {
		observations[i].Fields["resource_group_after_cleanup.exists"] = groupExists
	}
	comparison := compareAzureParityZ02Observations(observations)
	if !comparison.Matched {
		t.Fatalf("Z02 Azure provider parity observations did not match: %#v", observations)
	}
	return azureParityLiveRecording{
		Version:      azureParityArtifactV1,
		Lane:         "Z02",
		Scenario:     artifact.Scenarios[0].Name,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		DurationMS:   time.Since(started).Milliseconds(),
		Observations: observations,
		Comparison:   comparison,
	}
}

type azureParityZ04Scope struct {
	ResourceGroup string
	Location      string
	Suffix        string
}

func runAzureParityZ04Live(ctx context.Context, t *testing.T, artifact azureParityArtifact) azureParityLiveRecording {
	t.Helper()
	started := time.Now()
	requireSupportedAzureParityBaseline(t)
	scope := azureParityZ04Scope{
		ResourceGroup: "ramen-parity-z04-" + azureParityZ02RunSuffix(),
		Location:      strings.TrimSpace(os.Getenv("RAMEN_AZURE_STORAGE_LOCATION")),
	}
	if scope.Location == "" {
		scope.Location = "eastus"
	}
	scope.Suffix = strings.TrimPrefix(scope.ResourceGroup, "ramen-parity-z04-")
	if err := ensureAzureParityProviderRegistered(ctx, "Microsoft.Storage"); err != nil {
		t.Fatalf("Z04 Azure provider registration check failed: %v", err)
	}
	if err := createAzureParityResourceGroup(ctx, scope.ResourceGroup, scope.Location); err != nil {
		t.Fatalf("create Z04 isolated resource group: %v", err)
	}
	t.Cleanup(func() {
		if err := deleteAzureParityResourceGroup(context.Background(), scope.ResourceGroup); err != nil {
			t.Logf("cleanup Azure resource group %s: %v", scope.ResourceGroup, err)
		}
	})

	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, azureParityZ04Scope) azureParityRuntimeResult
	}{
		{runtime: "opentofu", run: runAzureParityZ04OpenTofuRuntime},
		{runtime: "ramen", run: runAzureParityZ04RamenRuntime},
	}
	if azureParityLiveBaselineMode() == "both" {
		runs = slices.Insert(runs, 1, struct {
			runtime string
			run     func(context.Context, *testing.T, azureParityZ04Scope) azureParityRuntimeResult
		}{runtime: "terraform", run: runAzureParityZ04TerraformRuntime})
	}
	var observations []azureParityRuntimeObservation
	var failures []azureParityRuntimeFailure
	for _, run := range runs {
		result := timedAzureParityRuntime(run.runtime, func() azureParityRuntimeResult {
			return run.run(ctx, t, scope)
		})
		if result.Failure != nil {
			failures = append(failures, *result.Failure)
			continue
		}
		observations = append(observations, result.Observation)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Logf("%s Azure Storage parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		if err := deleteAzureParityResourceGroup(ctx, scope.ResourceGroup); err != nil {
			t.Logf("cleanup failed Z04 resource group %s: %v", scope.ResourceGroup, err)
		}
		t.Fatalf("Z04 Azure provider parity did not complete for all runtimes")
	}
	if err := deleteAzureParityResourceGroup(ctx, scope.ResourceGroup); err != nil {
		t.Fatalf("delete Z04 isolated resource group: %v", err)
	}
	groupExists, err := observeAzureParityResourceGroup(ctx, scope.ResourceGroup)
	if err != nil {
		t.Fatalf("observe Z04 isolated resource group after cleanup: %v", err)
	}
	for i := range observations {
		observations[i].Fields["resource_group_after_cleanup.exists"] = groupExists
	}
	comparison := compareAzureParityZ04Observations(observations)
	if !comparison.Matched {
		t.Fatalf("Z04 Azure provider parity observations did not match: %#v", observations)
	}
	return azureParityLiveRecording{
		Version:      azureParityArtifactV1,
		Lane:         "Z04",
		Scenario:     artifact.Scenarios[0].Name,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		DurationMS:   time.Since(started).Milliseconds(),
		Observations: observations,
		Comparison:   comparison,
	}
}

func runAzureParityZ05Live(ctx context.Context, t *testing.T, artifact azureParityArtifact) azureParityLiveRecording {
	t.Helper()
	started := time.Now()
	requireSupportedAzureParityBaseline(t)
	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, string) azureParityRuntimeResult
		tool    string
	}{
		{runtime: "opentofu", run: runAzureParityZ05HCLRuntimeWithTool, tool: os.Getenv(azureParityTofuEnv)},
		{runtime: "ramen", run: runAzureParityZ05RamenRuntime, tool: ""},
	}
	if azureParityLiveBaselineMode() == "both" {
		runs = slices.Insert(runs, 1, struct {
			runtime string
			run     func(context.Context, *testing.T, string) azureParityRuntimeResult
			tool    string
		}{runtime: "terraform", run: runAzureParityZ05HCLRuntimeWithTool, tool: os.Getenv(azureParityTerraformEnv)})
	}
	var observations []azureParityRuntimeObservation
	var failures []azureParityRuntimeFailure
	for _, run := range runs {
		result := timedAzureParityRuntime(run.runtime, func() azureParityRuntimeResult {
			return run.run(ctx, t, run.tool)
		})
		if result.Failure != nil {
			failures = append(failures, *result.Failure)
			continue
		}
		observations = append(observations, result.Observation)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Logf("%s Azure SQL Z05 parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("Z05 Azure provider parity did not complete for all runtimes")
	}
	comparison := compareAzureParityZ05Observations(observations)
	if !comparison.Matched {
		t.Fatalf("Z05 Azure provider parity observations did not match: %#v", observations)
	}
	return azureParityLiveRecording{
		Version:      azureParityArtifactV1,
		Lane:         "Z05",
		Scenario:     artifact.Scenarios[0].Name,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		DurationMS:   time.Since(started).Milliseconds(),
		Observations: observations,
		Comparison:   comparison,
	}
}

func timedAzureParityRuntime(runtime string, run func() azureParityRuntimeResult) azureParityRuntimeResult {
	started := time.Now()
	result := run()
	if result.Failure == nil {
		result.Observation.DurationMS = time.Since(started).Milliseconds()
	}
	return result
}

func runAzureParityHCLRuntimeWithTool(ctx context.Context, t *testing.T, tool string) azureParityRuntimeResult {
	t.Helper()
	return runAzureParityHCLRuntime(ctx, t, tool)
}

func runAzureParityZ02OpenTofuRuntime(ctx context.Context, t *testing.T, scope azureParityZ02Scope) azureParityRuntimeResult {
	t.Helper()
	return runAzureParityZ02HCLRuntime(ctx, t, "opentofu", os.Getenv(azureParityTofuEnv), scope)
}

func runAzureParityZ02TerraformRuntime(ctx context.Context, t *testing.T, scope azureParityZ02Scope) azureParityRuntimeResult {
	t.Helper()
	return runAzureParityZ02HCLRuntime(ctx, t, "terraform", os.Getenv(azureParityTerraformEnv), scope)
}

func runAzureParityZ02HCLRuntime(ctx context.Context, t *testing.T, runtimeName, tool string, scope azureParityZ02Scope) azureParityRuntimeResult {
	t.Helper()
	accountName := azureParityZ02AccountName(runtimeName, scope.Suffix)
	if err := validateAzureParityZ02AccountName(accountName); err != nil {
		return azureParityFailure(runtimeName, "safety", err)
	}
	if err := deleteAzureParityCosmosAccountIfExists(ctx, scope.ResourceGroup, accountName); err != nil {
		return azureParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteAzureParityCosmosAccountIfExists(context.Background(), scope.ResourceGroup, accountName); err != nil {
			t.Logf("cleanup Azure Cosmos account %s: %v", accountName, err)
		}
	})
	workDir := filepath.Join(t.TempDir(), runtimeName)
	if err := copyFixtureFile(filepath.Join(azureParityFixtureRoot, "z02", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	tfvars := map[string]string{
		"resource_group_name": scope.ResourceGroup,
		"account_name":        accountName,
		"location":            scope.Location,
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
	afterApply, err := waitAzureParityCosmosAccount(ctx, scope.ResourceGroup, accountName, true)
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
	afterDestroy, err := waitAzureParityCosmosAccount(ctx, scope.ResourceGroup, accountName, false)
	if err != nil {
		return azureParityFailure(runtimeName, "observe", err)
	}
	return azureParityRuntimeResult{Observation: azureParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: accountName,
		Fields:   azureParityCosmosObservationFields(afterApply, afterDestroy, planExit == 0),
	}}
}

func runAzureParityZ04OpenTofuRuntime(ctx context.Context, t *testing.T, scope azureParityZ04Scope) azureParityRuntimeResult {
	t.Helper()
	return runAzureParityZ04HCLRuntime(ctx, t, "opentofu", os.Getenv(azureParityTofuEnv), scope)
}

func runAzureParityZ04TerraformRuntime(ctx context.Context, t *testing.T, scope azureParityZ04Scope) azureParityRuntimeResult {
	t.Helper()
	return runAzureParityZ04HCLRuntime(ctx, t, "terraform", os.Getenv(azureParityTerraformEnv), scope)
}

func runAzureParityZ04HCLRuntime(ctx context.Context, t *testing.T, runtimeName, tool string, scope azureParityZ04Scope) azureParityRuntimeResult {
	t.Helper()
	accountName := azureParityZ04AccountName(runtimeName, scope.Suffix)
	if err := validateAzureParityZ04AccountName(accountName); err != nil {
		return azureParityFailure(runtimeName, "safety", err)
	}
	if err := deleteAzureParityStorageAccountIfExists(ctx, scope.ResourceGroup, accountName); err != nil {
		return azureParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteAzureParityStorageAccountIfExists(context.Background(), scope.ResourceGroup, accountName); err != nil {
			t.Logf("cleanup Azure Storage account %s: %v", accountName, err)
		}
	})
	workDir := filepath.Join(t.TempDir(), runtimeName)
	if err := copyFixtureFile(filepath.Join(azureParityFixtureRoot, "z04", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	tfvars := map[string]string{
		"resource_group_name": scope.ResourceGroup,
		"account_name":        accountName,
		"location":            scope.Location,
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
	afterApply, err := waitAzureParityStorageAccount(ctx, scope.ResourceGroup, accountName, true)
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
	afterDestroy, err := waitAzureParityStorageAccount(ctx, scope.ResourceGroup, accountName, false)
	if err != nil {
		return azureParityFailure(runtimeName, "observe", err)
	}
	return azureParityRuntimeResult{Observation: azureParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: accountName,
		Fields:   azureParityStorageObservationFields(afterApply, afterDestroy, planExit == 0),
	}}
}

func runAzureParityZ05HCLRuntimeWithTool(ctx context.Context, t *testing.T, tool string) azureParityRuntimeResult {
	t.Helper()
	runtimeName := "terraform"
	if strings.Contains(filepath.Base(tool), "tofu") {
		runtimeName = "opentofu"
	}
	resourceGroup := strings.TrimSpace(os.Getenv("RAMEN_AZURE_SQL_RESOURCE_GROUP"))
	serverName := strings.TrimSpace(os.Getenv("RAMEN_AZURE_SQL_SERVER"))
	subscriptionID := strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID"))
	databaseName := azureParityZ05DatabaseName(runtimeName)
	if err := validateAzureParitySQLDatabaseName(databaseName); err != nil {
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
	if err := copyFixtureFile(filepath.Join(azureParityFixtureRoot, "z05", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
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

func runAzureParityRamenRuntime(ctx context.Context, t *testing.T, _ string) azureParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := refreshAzureParityUdonToken(ctx); err != nil {
		return azureParityFailure(runtimeName, "credential", err)
	}
	resourceGroup := strings.TrimSpace(os.Getenv("RAMEN_AZURE_SQL_RESOURCE_GROUP"))
	serverName := strings.TrimSpace(os.Getenv("RAMEN_AZURE_SQL_SERVER"))
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
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	deleteProjectPath := filepath.Join(workDir, "ramen", "project.delete.uws.yaml")
	openAPIPath := filepath.Join(workDir, "ramen", "openapi", "sql.json")
	if err := copyFixtureFile("../azure-rest-api-specs/specification/sql/resource-manager/Microsoft.Sql/SQL/preview/2023-08-01-preview/Databases.json", openAPIPath); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	if err := renderAzureParityZ01Project(filepath.Join(azureParityFixtureRoot, "z01", "ramen", "project.uws.yaml"), projectPath, databaseName, false); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	if err := renderAzureParityZ01Project(filepath.Join(azureParityFixtureRoot, "z01", "ramen", "project.uws.yaml"), deleteProjectPath, databaseName, true); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	udonExecutor := udon.Executor{
		OutputDir:           filepath.Join(workDir, "udon"),
		CredentialResolvers: azureParityUdonCredentialResolvers(),
		OutputProjector: func(projectorCtx context.Context, req executor.Request, _ string) (executor.Result, error) {
			result := executor.Result{
				Address:   req.Action.Address,
				Operation: req.Action.Mapping.OperationID,
				Success:   true,
			}
			if req.Action.Action == "delete" {
				return result, nil
			}
			observed, err := observeAzureParitySQLDatabase(projectorCtx, resourceGroup, serverName, databaseName)
			if err != nil {
				return executor.Result{}, err
			}
			if !observed.Exists {
				result.Missing = true
				return result, nil
			}
			result.Identity = map[string]any{"database_name": observed.Name}
			result.Computed = map[string]any{
				"id_present": observed.IDPresent,
				"name":       observed.Name,
				"location":   observed.Location,
				"sku.name":   observed.SKUName,
				"sku.tier":   observed.SKUTier,
			}
			return result, nil
		},
	}
	if err := buildAndApplyAzureParityPlan(ctx, projectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), udonExecutor); err != nil {
		return azureParityFailure(runtimeName, "apply", err)
	}
	afterApply, err := observeAzureParitySQLDatabase(ctx, resourceGroup, serverName, databaseName)
	if err != nil {
		return azureParityFailure(runtimeName, "observe", err)
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ProjectPath: projectPath, StatePath: statePath})
	if err != nil {
		return azureParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1
	if err := buildAndApplyAzureParityPlan(ctx, projectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), udonExecutor); err != nil {
		return azureParityFailure(runtimeName, "read", err)
	}
	if err := buildAndApplyAzureParityPlan(ctx, deleteProjectPath, statePath, "delete", filepath.Join(workDir, "delete-plan.json"), udonExecutor); err != nil {
		return azureParityFailure(runtimeName, "delete", err)
	}
	afterDestroy, err := observeAzureParitySQLDatabase(ctx, resourceGroup, serverName, databaseName)
	if err != nil {
		return azureParityFailure(runtimeName, "observe", err)
	}
	return azureParityRuntimeResult{Observation: azureParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: databaseName,
		Fields:   azureParitySQLObservationFields(afterApply, afterDestroy, noOp),
	}}
}

func runAzureParityZ02RamenRuntime(ctx context.Context, t *testing.T, scope azureParityZ02Scope) azureParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := refreshAzureParityUdonToken(ctx); err != nil {
		return azureParityFailure(runtimeName, "credential", err)
	}
	accountName := azureParityZ02AccountName(runtimeName, scope.Suffix)
	if err := validateAzureParityZ02AccountName(accountName); err != nil {
		return azureParityFailure(runtimeName, "safety", err)
	}
	if err := deleteAzureParityCosmosAccountIfExists(ctx, scope.ResourceGroup, accountName); err != nil {
		return azureParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteAzureParityCosmosAccountIfExists(context.Background(), scope.ResourceGroup, accountName); err != nil {
			t.Logf("cleanup Azure Cosmos account %s: %v", accountName, err)
		}
	})
	workDir := filepath.Join(t.TempDir(), runtimeName)
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	deleteProjectPath := filepath.Join(workDir, "ramen", "project.delete.uws.yaml")
	openAPIPath := filepath.Join(workDir, "ramen", "openapi", "cosmos.json")
	if err := copyFixtureFile(filepath.Join(azureParityFixtureRoot, "z02", "openapi", "cosmos.json"), openAPIPath); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	if err := renderAzureParityZ02Project(filepath.Join(azureParityFixtureRoot, "z02", "ramen", "project.uws.yaml"), projectPath, scope, accountName); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	if err := renderAzureParityZ02Project(filepath.Join(azureParityFixtureRoot, "z02", "ramen", "project.uws.yaml"), deleteProjectPath, scope, accountName); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	udonExecutor := udon.Executor{
		OutputDir:           filepath.Join(workDir, "udon"),
		CredentialResolvers: azureParityUdonCredentialResolvers(),
		OutputProjector: func(projectorCtx context.Context, req executor.Request, _ string) (executor.Result, error) {
			result := executor.Result{
				Address:   req.Action.Address,
				Operation: req.Action.Mapping.OperationID,
				Success:   true,
			}
			if req.Action.Action == "delete" {
				return result, nil
			}
			observed, err := observeAzureParityCosmosAccount(projectorCtx, scope.ResourceGroup, accountName)
			if err != nil {
				return executor.Result{}, err
			}
			if !observed.Exists {
				result.Missing = true
				return result, nil
			}
			result.Identity = map[string]any{"account_name": observed.Name}
			result.Computed = map[string]any{
				"id_present": observed.IDPresent,
				"name":       observed.Name,
				"location":   observed.Location,
				"kind":       observed.Kind,
				"offer_type": observed.OfferType,
			}
			return result, nil
		},
	}
	if err := buildAndApplyAzureParityPlan(ctx, projectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), udonExecutor); err != nil {
		return azureParityFailure(runtimeName, "apply", err)
	}
	afterApply, err := waitAzureParityCosmosAccount(ctx, scope.ResourceGroup, accountName, true)
	if err != nil {
		return azureParityFailure(runtimeName, "observe", err)
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ProjectPath: projectPath, StatePath: statePath})
	if err != nil {
		return azureParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1
	if err := buildAndApplyAzureParityPlan(ctx, projectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), udonExecutor); err != nil {
		return azureParityFailure(runtimeName, "read", err)
	}
	if err := buildAndApplyAzureParityPlan(ctx, deleteProjectPath, statePath, "delete", filepath.Join(workDir, "delete-plan.json"), udonExecutor); err != nil {
		return azureParityFailure(runtimeName, "delete", err)
	}
	afterDestroy, err := waitAzureParityCosmosAccount(ctx, scope.ResourceGroup, accountName, false)
	if err != nil {
		return azureParityFailure(runtimeName, "observe", err)
	}
	return azureParityRuntimeResult{Observation: azureParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: accountName,
		Fields:   azureParityCosmosObservationFields(afterApply, afterDestroy, noOp),
	}}
}

func runAzureParityZ04RamenRuntime(ctx context.Context, t *testing.T, scope azureParityZ04Scope) azureParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := refreshAzureParityUdonToken(ctx); err != nil {
		return azureParityFailure(runtimeName, "credential", err)
	}
	accountName := azureParityZ04AccountName(runtimeName, scope.Suffix)
	if err := validateAzureParityZ04AccountName(accountName); err != nil {
		return azureParityFailure(runtimeName, "safety", err)
	}
	if err := deleteAzureParityStorageAccountIfExists(ctx, scope.ResourceGroup, accountName); err != nil {
		return azureParityFailure(runtimeName, "pre-cleanup", err)
	}
	t.Cleanup(func() {
		if err := deleteAzureParityStorageAccountIfExists(context.Background(), scope.ResourceGroup, accountName); err != nil {
			t.Logf("cleanup Azure Storage account %s: %v", accountName, err)
		}
	})
	workDir := filepath.Join(t.TempDir(), runtimeName)
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	deleteProjectPath := filepath.Join(workDir, "ramen", "project.delete.uws.yaml")
	openAPIPath := filepath.Join(workDir, "ramen", "openapi", "storage.json")
	if err := copyFixtureFile(filepath.Join(azureParityFixtureRoot, "z04", "openapi", "storage.json"), openAPIPath); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	if err := renderAzureParityZ04Project(filepath.Join(azureParityFixtureRoot, "z04", "ramen", "project.uws.yaml"), projectPath, scope, accountName, false); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	if err := renderAzureParityZ04Project(filepath.Join(azureParityFixtureRoot, "z04", "ramen", "project.uws.yaml"), deleteProjectPath, scope, accountName, true); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	udonExecutor := udon.Executor{
		OutputDir:           filepath.Join(workDir, "udon"),
		CredentialResolvers: azureParityUdonCredentialResolvers(),
		OutputProjector: func(projectorCtx context.Context, req executor.Request, _ string) (executor.Result, error) {
			result := executor.Result{
				Address:   req.Action.Address,
				Operation: req.Action.Mapping.OperationID,
				Success:   true,
			}
			if req.Action.Action == "delete" {
				return result, nil
			}
			observed, err := observeAzureParityStorageAccount(projectorCtx, scope.ResourceGroup, accountName)
			if err != nil {
				return executor.Result{}, err
			}
			if !observed.Exists {
				result.Missing = true
				return result, nil
			}
			result.Identity = map[string]any{"account_name": observed.Name, "resource_group_name": scope.ResourceGroup}
			result.Computed = map[string]any{
				"id_present": observed.IDPresent,
				"name":       observed.Name,
				"location":   observed.Location,
				"kind":       observed.Kind,
				"sku.name":   observed.SKUName,
			}
			return result, nil
		},
	}
	if err := buildAndApplyAzureParityPlan(ctx, projectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), udonExecutor); err != nil {
		return azureParityFailure(runtimeName, "apply", err)
	}
	afterApply, err := waitAzureParityStorageAccount(ctx, scope.ResourceGroup, accountName, true)
	if err != nil {
		return azureParityFailure(runtimeName, "observe", err)
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ProjectPath: projectPath, StatePath: statePath})
	if err != nil {
		return azureParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1
	if err := buildAndApplyAzureParityPlan(ctx, projectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), udonExecutor); err != nil {
		return azureParityFailure(runtimeName, "read", err)
	}
	if err := buildAndApplyAzureParityPlan(ctx, deleteProjectPath, statePath, "delete", filepath.Join(workDir, "delete-plan.json"), udonExecutor); err != nil {
		return azureParityFailure(runtimeName, "delete", err)
	}
	afterDestroy, err := waitAzureParityStorageAccount(ctx, scope.ResourceGroup, accountName, false)
	if err != nil {
		return azureParityFailure(runtimeName, "observe", err)
	}
	return azureParityRuntimeResult{Observation: azureParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: accountName,
		Fields:   azureParityStorageObservationFields(afterApply, afterDestroy, noOp),
	}}
}

func runAzureParityZ05RamenRuntime(ctx context.Context, t *testing.T, _ string) azureParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	if err := refreshAzureParityUdonToken(ctx); err != nil {
		return azureParityFailure(runtimeName, "credential", err)
	}
	resourceGroup := strings.TrimSpace(os.Getenv("RAMEN_AZURE_SQL_RESOURCE_GROUP"))
	serverName := strings.TrimSpace(os.Getenv("RAMEN_AZURE_SQL_SERVER"))
	databaseName := azureParityZ05DatabaseName(runtimeName)
	if err := validateAzureParitySQLDatabaseName(databaseName); err != nil {
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
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	deleteProjectPath := filepath.Join(workDir, "ramen", "project.delete.uws.yaml")
	openAPIPath := filepath.Join(workDir, "ramen", "openapi", "sql.json")
	if err := copyFixtureFile(filepath.Join(azureParityFixtureRoot, "z05", "openapi", "sql.json"), openAPIPath); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	if err := renderAzureParityZ05Project(filepath.Join(azureParityFixtureRoot, "z05", "ramen", "project.uws.yaml"), projectPath, databaseName, false); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	if err := renderAzureParityZ05Project(filepath.Join(azureParityFixtureRoot, "z05", "ramen", "project.uws.yaml"), deleteProjectPath, databaseName, true); err != nil {
		return azureParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	udonExecutor := udon.Executor{
		OutputDir:           filepath.Join(workDir, "udon"),
		CredentialResolvers: azureParityUdonCredentialResolvers(),
		OutputProjector: func(projectorCtx context.Context, req executor.Request, _ string) (executor.Result, error) {
			result := executor.Result{
				Address:   req.Action.Address,
				Operation: req.Action.Mapping.OperationID,
				Success:   true,
			}
			if req.Action.Action == "delete" {
				return result, nil
			}
			observed, err := observeAzureParitySQLDatabase(projectorCtx, resourceGroup, serverName, databaseName)
			if err != nil {
				return executor.Result{}, err
			}
			if !observed.Exists {
				result.Missing = true
				return result, nil
			}
			result.Identity = map[string]any{"database_name": observed.Name}
			result.Computed = map[string]any{
				"id_present": observed.IDPresent,
				"name":       observed.Name,
				"location":   observed.Location,
				"sku.name":   observed.SKUName,
				"sku.tier":   observed.SKUTier,
			}
			return result, nil
		},
	}
	if err := buildAndApplyAzureParityPlan(ctx, projectPath, statePath, "create", filepath.Join(workDir, "create-plan.json"), udonExecutor); err != nil {
		return azureParityFailure(runtimeName, "apply", err)
	}
	afterApply, err := observeAzureParitySQLDatabase(ctx, resourceGroup, serverName, databaseName)
	if err != nil {
		return azureParityFailure(runtimeName, "observe", err)
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ProjectPath: projectPath, StatePath: statePath})
	if err != nil {
		return azureParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1
	if err := buildAndApplyAzureParityPlan(ctx, projectPath, statePath, "read", filepath.Join(workDir, "read-plan.json"), udonExecutor); err != nil {
		return azureParityFailure(runtimeName, "read", err)
	}
	if err := buildAndApplyAzureParityPlan(ctx, deleteProjectPath, statePath, "delete", filepath.Join(workDir, "delete-plan.json"), udonExecutor); err != nil {
		return azureParityFailure(runtimeName, "delete", err)
	}
	afterDestroy, err := observeAzureParitySQLDatabase(ctx, resourceGroup, serverName, databaseName)
	if err != nil {
		return azureParityFailure(runtimeName, "observe", err)
	}
	return azureParityRuntimeResult{Observation: azureParityRuntimeObservation{
		Runtime:  runtimeName,
		Resource: databaseName,
		Fields:   azureParitySQLObservationFields(afterApply, afterDestroy, noOp),
	}}
}

func refreshAzureParityUdonToken(ctx context.Context) error {
	_, err := refreshAzureParityUdonTokenValue(ctx)
	return err
}

func azureParityUdonCredentialResolvers() map[string]func(context.Context) (string, error) {
	return map[string]func(context.Context) (string, error){
		"azure_auth": refreshAzureParityUdonTokenValue,
	}
}

func refreshAzureParityUdonTokenValue(ctx context.Context) (string, error) {
	cmd := osexec.CommandContext(ctx, "az", "account", "get-access-token", "--resource", "https://management.azure.com/", "--query", "accessToken", "-o", "tsv")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("refresh Azure token: %w: %s", err, sanitizeAzureParityCommandOutput(string(out)))
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("refresh Azure token: empty token")
	}
	os.Setenv("UDON_CREDENTIAL_AZURE_AUTH", token)
	return token, nil
}

func buildAndApplyAzureParityPlan(ctx context.Context, projectPath, statePath, action, planPath string, udonExecutor udon.Executor) error {
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

func renderAzureParityZ01Project(src, dst, databaseName string, deleteWaiter bool) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	out := string(data)
	replacements := map[string]string{
		"subscription-placeholder":   strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID")),
		"placeholder-resource-group": strings.TrimSpace(os.Getenv("RAMEN_AZURE_SQL_RESOURCE_GROUP")),
		"placeholder-server":         strings.TrimSpace(os.Getenv("RAMEN_AZURE_SQL_SERVER")),
		"ramen-parity-z01-static":    databaseName,
		"../openapi/sql.json":        "openapi/sql.json",
		"\"2023-08-01\"":             "\"2023-08-01-preview\"",
	}
	for old, newValue := range replacements {
		out = strings.ReplaceAll(out, old, newValue)
	}
	out = strings.Replace(out, "retry:\n          max_attempts: 2", "retry:\n          max_attempts: 30", 1)
	out = strings.Replace(out, "waiter:\n          until: exists\n          max_attempts: 2", "waiter:\n          until: exists\n          max_attempts: 30\n          interval: 10s", 1)
	if deleteWaiter {
		out = strings.Replace(out, "until: exists", "until: missing", 1)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}

func renderAzureParityZ02Project(src, dst string, scope azureParityZ02Scope, accountName string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	out := string(data)
	replacements := [][2]string{
		{"subscription-placeholder", strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID"))},
		{"account-placeholder", accountName},
		{"resource-group-placeholder", scope.ResourceGroup},
		{"eastus", scope.Location},
		{"../openapi/cosmos.json", "openapi/cosmos.json"},
	}
	for _, replacement := range replacements {
		out = strings.ReplaceAll(out, replacement[0], replacement[1])
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}

func renderAzureParityZ04Project(src, dst string, scope azureParityZ04Scope, accountName string, deleteWaiter bool) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	out := string(data)
	replacements := [][2]string{
		{"subscription-placeholder", strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID"))},
		{"ramenparityz04static", accountName},
		{"ramen-parity-z04", scope.ResourceGroup},
		{"eastus", scope.Location},
		{"../openapi/storage.json", "openapi/storage.json"},
	}
	for _, replacement := range replacements {
		out = strings.ReplaceAll(out, replacement[0], replacement[1])
	}
	out = strings.Replace(out, "waiter:\n          until: exists\n          max_attempts: 12\n          interval: 5s", "waiter:\n          until: exists\n          max_attempts: 40\n          interval: 10s", 1)
	if deleteWaiter {
		out = strings.Replace(out, "until: exists", "until: missing", 1)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}

func renderAzureParityZ05Project(src, dst, databaseName string, deleteWaiter bool) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	out := string(data)
	replacements := map[string]string{
		"subscription-placeholder":   strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID")),
		"placeholder-resource-group": strings.TrimSpace(os.Getenv("RAMEN_AZURE_SQL_RESOURCE_GROUP")),
		"placeholder-server":         strings.TrimSpace(os.Getenv("RAMEN_AZURE_SQL_SERVER")),
		"ramen-parity-z05-static":    databaseName,
		"../openapi/sql.json":        "openapi/sql.json",
	}
	for old, newValue := range replacements {
		out = strings.ReplaceAll(out, old, newValue)
	}
	out = strings.Replace(out, "waiter:\n          until: exists\n          max_attempts: 12\n          interval: 5s", "waiter:\n          until: exists\n          max_attempts: 30\n          interval: 10s", 1)
	if deleteWaiter {
		out = strings.Replace(out, "until: exists", "until: missing", 1)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}

type azureParityCosmosAccountObservation struct {
	Exists    bool
	Name      string
	Location  string
	Kind      string
	OfferType string
	IDPresent bool
}

func azureParityZ02RunSuffix() string {
	return fmt.Sprintf("%x", time.Now().UTC().UnixNano())[:8]
}

func azureParityZ02AccountName(runtime, suffix string) string {
	return "ramen-parity-z02-" + runtime + "-" + suffix
}

func validateAzureParityZ02AccountName(name string) error {
	if !strings.HasPrefix(name, "ramen-parity-z02-") {
		return fmt.Errorf("account name %q must use ramen-parity-z02-* prefix", name)
	}
	if len(name) < 3 || len(name) > 44 {
		return fmt.Errorf("account name %q length must be between 3 and 44", name)
	}
	if name[0] == '-' || name[len(name)-1] == '-' {
		return fmt.Errorf("account name %q must start and end with alphanumeric characters", name)
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			continue
		}
		return fmt.Errorf("account name %q contains unsupported character %q", name, r)
	}
	return nil
}

func ensureAzureParityProviderRegistered(ctx context.Context, namespace string) error {
	show := osexec.CommandContext(ctx, "az", "provider", "show", "--namespace", namespace, "--query", "registrationState", "-o", "tsv")
	out, err := show.CombinedOutput()
	if err != nil {
		return fmt.Errorf("az provider show %s: %w: %s", namespace, err, sanitizeAzureParityCommandOutput(string(out)))
	}
	if strings.TrimSpace(string(out)) == "Registered" {
		return nil
	}
	register := osexec.CommandContext(ctx, "az", "provider", "register", "--namespace", namespace, "--wait")
	out, err = register.CombinedOutput()
	if err != nil {
		return fmt.Errorf("az provider register %s: %w: %s", namespace, err, sanitizeAzureParityCommandOutput(string(out)))
	}
	return nil
}

func createAzureParityResourceGroup(ctx context.Context, resourceGroup, location string) error {
	args := []string{
		"group", "create",
		"--name", resourceGroup,
		"--location", location,
		"--subscription", strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID")),
		"-o", "json",
	}
	out, err := osexec.CommandContext(ctx, "az", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("az group create failed: %w: %s", err, sanitizeAzureParityCommandOutput(string(out)))
	}
	return nil
}

func deleteAzureParityResourceGroup(ctx context.Context, resourceGroup string) error {
	if !strings.HasPrefix(resourceGroup, "ramen-parity-z02-") && !strings.HasPrefix(resourceGroup, "ramen-parity-z04-") {
		return fmt.Errorf("refusing to delete non-isolated parity resource group %q", resourceGroup)
	}
	exists, err := observeAzureParityResourceGroup(ctx, resourceGroup)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	args := []string{
		"group", "delete",
		"--name", resourceGroup,
		"--subscription", strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID")),
		"--yes",
	}
	out, err := osexec.CommandContext(ctx, "az", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("az group delete failed: %w: %s", err, sanitizeAzureParityCommandOutput(string(out)))
	}
	return nil
}

func observeAzureParityResourceGroup(ctx context.Context, resourceGroup string) (bool, error) {
	args := []string{
		"group", "exists",
		"--name", resourceGroup,
		"--subscription", strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID")),
	}
	out, err := osexec.CommandContext(ctx, "az", args...).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("az group exists failed: %w: %s", err, sanitizeAzureParityCommandOutput(string(out)))
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

type azureParityStorageAccountObservation struct {
	Exists    bool
	Name      string
	Location  string
	Kind      string
	SKUName   string
	IDPresent bool
}

func azureParityZ04AccountName(runtime, suffix string) string {
	runtimeID := map[string]string{
		"opentofu":  "ot",
		"terraform": "tf",
		"ramen":     "rm",
	}[runtime]
	if runtimeID == "" {
		runtimeID = "rt"
	}
	return "ramenparityz04" + runtimeID + suffix
}

func validateAzureParityZ04AccountName(name string) error {
	if !strings.HasPrefix(name, "ramenparityz04") {
		return fmt.Errorf("storage account name %q must use ramenparityz04* prefix", name)
	}
	if len(name) < 3 || len(name) > 24 {
		return fmt.Errorf("storage account name %q length must be between 3 and 24", name)
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		return fmt.Errorf("storage account name %q contains unsupported character %q", name, r)
	}
	return nil
}

func observeAzureParityStorageAccount(ctx context.Context, resourceGroup, accountName string) (azureParityStorageAccountObservation, error) {
	if err := validateAzureParityZ04AccountName(accountName); err != nil {
		return azureParityStorageAccountObservation{}, err
	}
	args := []string{
		"storage", "account", "show",
		"--resource-group", resourceGroup,
		"--name", accountName,
		"--subscription", strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID")),
		"-o", "json",
	}
	out, err := osexec.CommandContext(ctx, "az", args...).CombinedOutput()
	if err != nil {
		if isAzureParityNotFound(string(out)) {
			return azureParityStorageAccountObservation{Exists: false}, nil
		}
		return azureParityStorageAccountObservation{}, fmt.Errorf("az storage account show failed: %w: %s", err, sanitizeAzureParityCommandOutput(string(out)))
	}
	var doc struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Location string `json:"location"`
		Kind     string `json:"kind"`
		SKU      struct {
			Name string `json:"name"`
		} `json:"sku"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return azureParityStorageAccountObservation{}, fmt.Errorf("decode Azure Storage account observation: %w", err)
	}
	return azureParityStorageAccountObservation{
		Exists:    true,
		Name:      doc.Name,
		Location:  doc.Location,
		Kind:      doc.Kind,
		SKUName:   doc.SKU.Name,
		IDPresent: strings.TrimSpace(doc.ID) != "",
	}, nil
}

func waitAzureParityStorageAccount(ctx context.Context, resourceGroup, accountName string, wantExists bool) (azureParityStorageAccountObservation, error) {
	var last azureParityStorageAccountObservation
	var lastErr error
	for attempt := 0; attempt < 60; attempt++ {
		observed, err := observeAzureParityStorageAccount(ctx, resourceGroup, accountName)
		if err == nil && observed.Exists == wantExists {
			return observed, nil
		}
		last = observed
		lastErr = err
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
	if lastErr != nil {
		return last, lastErr
	}
	return last, fmt.Errorf("timed out waiting for Azure Storage account %s exists=%t", accountName, wantExists)
}

func deleteAzureParityStorageAccountIfExists(ctx context.Context, resourceGroup, accountName string) error {
	observed, err := observeAzureParityStorageAccount(ctx, resourceGroup, accountName)
	if err != nil {
		return err
	}
	if !observed.Exists {
		return nil
	}
	args := []string{
		"storage", "account", "delete",
		"--resource-group", resourceGroup,
		"--name", accountName,
		"--subscription", strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID")),
		"--yes",
	}
	out, err := osexec.CommandContext(ctx, "az", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("az storage account delete failed: %w: %s", err, sanitizeAzureParityCommandOutput(string(out)))
	}
	_, err = waitAzureParityStorageAccount(ctx, resourceGroup, accountName, false)
	return err
}

func observeAzureParityCosmosAccount(ctx context.Context, resourceGroup, accountName string) (azureParityCosmosAccountObservation, error) {
	if err := validateAzureParityZ02AccountName(accountName); err != nil {
		return azureParityCosmosAccountObservation{}, err
	}
	args := []string{
		"cosmosdb", "show",
		"--resource-group", resourceGroup,
		"--name", accountName,
		"--subscription", strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID")),
		"-o", "json",
	}
	out, err := osexec.CommandContext(ctx, "az", args...).CombinedOutput()
	if err != nil {
		if isAzureParityNotFound(string(out)) {
			return azureParityCosmosAccountObservation{Exists: false}, nil
		}
		return azureParityCosmosAccountObservation{}, fmt.Errorf("az cosmosdb show failed: %w: %s", err, sanitizeAzureParityCommandOutput(string(out)))
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		return azureParityCosmosAccountObservation{}, fmt.Errorf("decode Azure Cosmos observation: %w", err)
	}
	return azureParityCosmosAccountObservation{
		Exists:    true,
		Name:      azureParityStringAt(doc, "name"),
		Location:  azureParityStringAt(doc, "location"),
		Kind:      azureParityStringAt(doc, "kind"),
		OfferType: firstNonEmptyString(azureParityStringAt(doc, "properties", "databaseAccountOfferType"), azureParityStringAt(doc, "databaseAccountOfferType")),
		IDPresent: strings.TrimSpace(azureParityStringAt(doc, "id")) != "",
	}, nil
}

func waitAzureParityCosmosAccount(ctx context.Context, resourceGroup, accountName string, wantExists bool) (azureParityCosmosAccountObservation, error) {
	var last azureParityCosmosAccountObservation
	var lastErr error
	for attempt := 0; attempt < 80; attempt++ {
		observed, err := observeAzureParityCosmosAccount(ctx, resourceGroup, accountName)
		if err == nil && observed.Exists == wantExists {
			return observed, nil
		}
		last = observed
		lastErr = err
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}
	if lastErr != nil {
		return last, lastErr
	}
	return last, fmt.Errorf("timed out waiting for Azure Cosmos account %s exists=%t", accountName, wantExists)
}

func deleteAzureParityCosmosAccountIfExists(ctx context.Context, resourceGroup, accountName string) error {
	observed, err := observeAzureParityCosmosAccount(ctx, resourceGroup, accountName)
	if err != nil {
		return err
	}
	if !observed.Exists {
		return nil
	}
	args := []string{
		"cosmosdb", "delete",
		"--resource-group", resourceGroup,
		"--name", accountName,
		"--subscription", strings.TrimSpace(os.Getenv("AZURE_SUBSCRIPTION_ID")),
		"--yes",
	}
	out, err := osexec.CommandContext(ctx, "az", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("az cosmosdb delete failed: %w: %s", err, sanitizeAzureParityCommandOutput(string(out)))
	}
	_, err = waitAzureParityCosmosAccount(ctx, resourceGroup, accountName, false)
	return err
}

func azureParityCosmosObservationFields(afterApply, afterDestroy azureParityCosmosAccountObservation, noOp bool) map[string]any {
	return map[string]any{
		"after_apply.exists":     afterApply.Exists,
		"after_apply.id_present": afterApply.IDPresent,
		"after_apply.name":       afterApply.Name,
		"after_apply.location":   afterApply.Location,
		"after_apply.kind":       afterApply.Kind,
		"after_apply.offer_type": afterApply.OfferType,
		"no_op":                  noOp,
		"after_destroy.exists":   afterDestroy.Exists,
	}
}

func azureParityStorageObservationFields(afterApply, afterDestroy azureParityStorageAccountObservation, noOp bool) map[string]any {
	return map[string]any{
		"after_apply.exists":     afterApply.Exists,
		"after_apply.id_present": afterApply.IDPresent,
		"after_apply.name":       afterApply.Name,
		"after_apply.location":   afterApply.Location,
		"after_apply.kind":       afterApply.Kind,
		"after_apply.sku.name":   afterApply.SKUName,
		"no_op":                  noOp,
		"after_destroy.exists":   afterDestroy.Exists,
	}
}

func azureParityStringAt(doc map[string]any, path ...string) string {
	var current any = doc
	for _, part := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = m[part]
	}
	value, _ := current.(string)
	return value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
