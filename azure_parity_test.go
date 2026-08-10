package corpus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/tfconfig"
)

const (
	azureParityEnv          = "RAMEN_AZURE_PARITY"
	azureParityRecordEnv    = "RAMEN_AZURE_PARITY_RECORD_UPDATE"
	azureParityLaneEnv      = "RAMEN_AZURE_PARITY_LANE"
	azureParityBaselineEnv  = "RAMEN_AZURE_PARITY_BASELINE"
	azureParityTerraformEnv = "RAMEN_AZURE_TERRAFORM"
	azureParityTofuEnv      = "RAMEN_AZURE_TOFU"
	azureParityArtifactV1   = "ramen.azure.provider-parity.v1"
	azureParityFixtureRoot  = "testdata/parity/azure"
)

var azureParityLanes = []string{"z01", "z02", "z03", "z04", "z05", "z06", "z08", "z09"}
var azureParityLiveRunnerLanes = []string{"z01", "z02", "z04", "z05", "z08", "z09"}

type azureParityArtifact struct {
	Version          string                `json:"version"`
	Lane             string                `json:"lane"`
	Status           string                `json:"status"`
	Provider         azureParityProvider   `json:"provider"`
	OpenAPI          azureParityOpenAPI    `json:"openapi"`
	Safety           azureParitySafety     `json:"safety"`
	Runtimes         []string              `json:"runtimes"`
	Scenarios        []azureParityScenario `json:"scenarios"`
	RecordedAt       string                `json:"recorded_at,omitempty"`
	RecordingsSource string                `json:"recordings_source,omitempty"`
	Notes            []string              `json:"notes,omitempty"`
}

type azureParityProvider struct {
	Source            string `json:"source"`
	Version           string `json:"version,omitempty"`
	VersionConstraint string `json:"version_constraint,omitempty"`
	Published         string `json:"published,omitempty"`
	RegistryLatest    *bool  `json:"registry_latest,omitempty"`
}

type azureParityOpenAPI struct {
	SourcePath string `json:"source_path"`
	Fixture    string `json:"fixture"`
}

type azureParitySafety struct {
	LiveEnv               string            `json:"live_env"`
	RecordUpdateEnv       string            `json:"record_update_env"`
	LaneEnv               string            `json:"lane_env"`
	CredentialEnv         string            `json:"credential_env,omitempty"`
	ResourcePrefix        string            `json:"resource_prefix"`
	RequiresExplicitLane  bool              `json:"requires_explicit_lane"`
	LiveEnabled           bool              `json:"live_enabled,omitempty"`
	LiveWaiterMinAttempts int               `json:"live_waiter_min_attempts,omitempty"`
	CleanupFallback       string            `json:"cleanup_fallback,omitempty"`
	RequiredEnv           []string          `json:"required_env,omitempty"`
	TerraformEnvMap       map[string]string `json:"terraform_env_map,omitempty"`
	CostGuardrails        []string          `json:"cost_guardrails,omitempty"`
}

type azureParityScenario = apiParityScenario
type azureParityLiveRecording = apiParityLiveRecording
type azureParityRuntimeObservation = apiParityRuntimeObservation
type azureParityObservationComparison = apiParityObservationComparison
type azureParityRuntimeFailure = apiParityRuntimeFailure
type azureParityRuntimeResult = apiParityRuntimeResult

func TestAzureProviderParityReplayArtifacts(t *testing.T) {
	for _, lane := range azureParityLanes {
		lane := lane
		t.Run(strings.ToUpper(lane), func(t *testing.T) {
			artifact := loadAzureParityArtifact(t, filepath.Join(azureParityFixtureRoot, lane, "observations.json"))
			assertAzureParityArtifact(t, lane, artifact)
			assertAzureParityRecordingArtifacts(t, lane, artifact)
			assertAzureParityStaticFixtures(t, lane, artifact)
		})
	}
}

func loadAzureParityArtifact(t *testing.T, path string) azureParityArtifact {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var artifact azureParityArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return artifact
}

func loadAzureParityLiveRecording(t *testing.T, path string) azureParityLiveRecording {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var recording azureParityLiveRecording
	if err := json.Unmarshal(data, &recording); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return recording
}

func assertAzureParityArtifact(t *testing.T, lane string, artifact azureParityArtifact) {
	t.Helper()
	wantLane := strings.ToUpper(lane)
	if artifact.Version != azureParityArtifactV1 {
		t.Fatalf("artifact version = %q, want %q", artifact.Version, azureParityArtifactV1)
	}
	if artifact.Lane != wantLane {
		t.Fatalf("artifact lane = %q, want %q", artifact.Lane, wantLane)
	}
	if artifact.Status != "planned" && artifact.Status != "recorded" {
		t.Fatalf("artifact status = %q, want planned or recorded", artifact.Status)
	}
	if artifact.Status == "recorded" {
		if strings.TrimSpace(artifact.RecordedAt) == "" {
			t.Fatalf("recorded artifact %s must include recorded_at", wantLane)
		}
		if strings.TrimSpace(artifact.RecordingsSource) == "" {
			t.Fatalf("recorded artifact %s must include recordings_source", wantLane)
		}
	}
	if artifact.Provider.Source != "hashicorp/azurerm" {
		t.Fatalf("provider source = %q, want hashicorp/azurerm", artifact.Provider.Source)
	}
	if strings.TrimSpace(artifact.Provider.Version) == "" && strings.TrimSpace(artifact.Provider.VersionConstraint) == "" {
		t.Fatalf("provider version or version_constraint is required")
	}
	if artifact.OpenAPI.Fixture == "" {
		t.Fatalf("openapi fixture path is required")
	}
	if artifact.Safety.LiveEnv != azureParityEnv {
		t.Fatalf("live env = %q, want %q", artifact.Safety.LiveEnv, azureParityEnv)
	}
	if artifact.Safety.RecordUpdateEnv != azureParityRecordEnv {
		t.Fatalf("record update env = %q, want %q", artifact.Safety.RecordUpdateEnv, azureParityRecordEnv)
	}
	if artifact.Safety.LaneEnv != azureParityLaneEnv {
		t.Fatalf("lane env = %q, want %q", artifact.Safety.LaneEnv, azureParityLaneEnv)
	}
	if !artifact.Safety.RequiresExplicitLane {
		t.Fatalf("Azure parity artifacts must require explicit lane selection")
	}
	if artifact.Safety.CredentialEnv != "UDON_CREDENTIAL_AZURE_AUTH" {
		t.Fatalf("credential env = %q, want UDON_CREDENTIAL_AZURE_AUTH", artifact.Safety.CredentialEnv)
	}
	wantPrefix := "ramen-parity-" + lane + "-"
	if lane == "z04" {
		wantPrefix = "ramenparityz04"
	}
	if lane == "z06" {
		wantPrefix = "ramen-parity-z02-"
	}
	if !strings.HasPrefix(artifact.Safety.ResourcePrefix, wantPrefix) {
		t.Fatalf("resource prefix = %q, want ramen-parity-%s-*", artifact.Safety.ResourcePrefix, lane)
	}
	assertAzureParitySafetyContract(t, lane, artifact.Safety)
	if artifact.Safety.LiveEnabled && !slices.Contains(azureParityLiveRunnerLanes, lane) {
		t.Fatalf("%s is marked live-enabled but has no registered Azure live runner", wantLane)
	}
	for _, runtime := range []string{"opentofu", "terraform", "ramen"} {
		if !slices.Contains(artifact.Runtimes, runtime) {
			t.Fatalf("artifact runtimes %v missing %s", artifact.Runtimes, runtime)
		}
	}
	if len(artifact.Scenarios) == 0 {
		t.Fatalf("at least one scenario is required")
	}
	for i, scenario := range artifact.Scenarios {
		if strings.TrimSpace(scenario.Name) == "" {
			t.Fatalf("scenario %d has empty name", i)
		}
		if strings.TrimSpace(scenario.ResourceType) == "" {
			t.Fatalf("scenario %s has empty resource_type", scenario.Name)
		}
		if len(scenario.OperationIDs) == 0 {
			t.Fatalf("scenario %s has no operation_ids", scenario.Name)
		}
		if len(scenario.ObservedFields) == 0 {
			t.Fatalf("scenario %s has no observed_fields", scenario.Name)
		}
		if len(scenario.ExpectedTransitions) == 0 {
			t.Fatalf("scenario %s has no expected_transitions", scenario.Name)
		}
		for _, path := range scenario.FixturePaths {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("scenario %s fixture %s is not readable: %v", scenario.Name, path, err)
			}
		}
		if artifact.Status == "planned" && len(scenario.ObservationArtifacts) != 0 {
			t.Fatalf("planned scenario %s must not claim observation_artifacts", scenario.Name)
		}
	}
}

func assertAzureParityRecordingArtifacts(t *testing.T, lane string, artifact azureParityArtifact) {
	t.Helper()
	if artifact.Status != "recorded" {
		return
	}
	for _, scenario := range artifact.Scenarios {
		if len(scenario.ObservationArtifacts) == 0 {
			t.Fatalf("recorded scenario %s must include observation_artifacts", scenario.Name)
		}
		for _, path := range scenario.ObservationArtifacts {
			recording := loadAzureParityLiveRecording(t, path)
			assertAzureParityLiveRecording(t, lane, artifact, scenario, recording)
		}
	}
}

func assertAzureParityLiveRecording(t *testing.T, lane string, artifact azureParityArtifact, scenario azureParityScenario, recording azureParityLiveRecording) {
	t.Helper()
	wantLane := strings.ToUpper(lane)
	if recording.Version != artifact.Version {
		t.Fatalf("%s recording version = %q, want %q", wantLane, recording.Version, artifact.Version)
	}
	if recording.Lane != wantLane {
		t.Fatalf("%s recording lane = %q, want %q", wantLane, recording.Lane, wantLane)
	}
	if recording.Scenario != scenario.Name {
		t.Fatalf("%s recording scenario = %q, want %q", wantLane, recording.Scenario, scenario.Name)
	}
	if strings.TrimSpace(recording.RecordedAt) == "" {
		t.Fatalf("%s recording must include recorded_at", wantLane)
	}
	if len(recording.Failures) != 0 {
		t.Fatalf("%s recording includes failures: %#v", wantLane, recording.Failures)
	}
	if len(recording.Observations) == 0 {
		t.Fatalf("%s recording must include observations", wantLane)
	}
	seen := map[string]bool{}
	for _, observation := range recording.Observations {
		if !slices.Contains(artifact.Runtimes, observation.Runtime) {
			t.Fatalf("%s recording runtime %q is not declared in artifact runtimes %#v", wantLane, observation.Runtime, artifact.Runtimes)
		}
		if seen[observation.Runtime] {
			t.Fatalf("%s recording repeats runtime %q", wantLane, observation.Runtime)
		}
		seen[observation.Runtime] = true
		if observation.DurationMS < 0 {
			t.Fatalf("%s recording runtime %q duration_ms = %d, want non-negative", wantLane, observation.Runtime, observation.DurationMS)
		}
		if !strings.HasPrefix(observation.Resource, artifact.Safety.ResourcePrefix) {
			t.Fatalf("%s recording resource %q does not use prefix %q", wantLane, observation.Resource, artifact.Safety.ResourcePrefix)
		}
		for key, value := range observation.Fields {
			if key == "after_apply.id" || key == "id" {
				t.Fatalf("%s recording must not persist raw resource id field %q", wantLane, key)
			}
			if valueString, ok := value.(string); ok && strings.Contains(strings.ToLower(valueString), "/subscriptions/") {
				t.Fatalf("%s recording field %s appears to contain a raw Azure resource id", wantLane, key)
			}
		}
	}
	for _, runtime := range []string{"opentofu", "ramen"} {
		if !seen[runtime] {
			t.Fatalf("%s recording missing required runtime %q", wantLane, runtime)
		}
	}
	switch lane {
	case "z01":
		want := compareAzureParityZ01Observations(recording.Observations)
		if !reflect.DeepEqual(recording.Comparison, want) {
			t.Fatalf("%s recording comparison = %#v, want %#v", wantLane, recording.Comparison, want)
		}
		if !recording.Comparison.Matched {
			t.Fatalf("%s recording comparison did not match", wantLane)
		}
	case "z02":
		want := compareAzureParityZ02Observations(recording.Observations)
		if !reflect.DeepEqual(recording.Comparison, want) {
			t.Fatalf("%s recording comparison = %#v, want %#v", wantLane, recording.Comparison, want)
		}
		if !recording.Comparison.Matched {
			t.Fatalf("%s recording comparison did not match", wantLane)
		}
	case "z04":
		want := compareAzureParityZ04Observations(recording.Observations)
		if !reflect.DeepEqual(recording.Comparison, want) {
			t.Fatalf("%s recording comparison = %#v, want %#v", wantLane, recording.Comparison, want)
		}
		if !recording.Comparison.Matched {
			t.Fatalf("%s recording comparison did not match", wantLane)
		}
	case "z05":
		want := compareAzureParityZ05Observations(recording.Observations)
		if !reflect.DeepEqual(recording.Comparison, want) {
			t.Fatalf("%s recording comparison = %#v, want %#v", wantLane, recording.Comparison, want)
		}
		if !recording.Comparison.Matched {
			t.Fatalf("%s recording comparison did not match", wantLane)
		}
	case "z08":
		want := compareAzureParityZ08Observations(recording.Observations)
		if !reflect.DeepEqual(recording.Comparison, want) {
			t.Fatalf("%s recording comparison = %#v, want %#v", wantLane, recording.Comparison, want)
		}
		if !recording.Comparison.Matched {
			t.Fatalf("%s recording comparison did not match", wantLane)
		}
	case "z09":
		want := compareAzureParityZ09Observations(recording.Observations)
		if !reflect.DeepEqual(recording.Comparison, want) {
			t.Fatalf("%s recording comparison = %#v, want %#v", wantLane, recording.Comparison, want)
		}
		if !recording.Comparison.Matched {
			t.Fatalf("%s recording comparison did not match", wantLane)
		}
	default:
		t.Fatalf("no recording assertions registered for Azure parity lane %s", lane)
	}
}

func assertAzureParitySafetyContract(t *testing.T, lane string, safety azureParitySafety) {
	t.Helper()
	for _, envName := range []string{"AZURE_SUBSCRIPTION_ID", "AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET"} {
		if !slices.Contains(safety.RequiredEnv, envName) {
			t.Fatalf("%s required_env = %#v, missing %s", strings.ToUpper(lane), safety.RequiredEnv, envName)
		}
	}
	for azureEnv, armEnv := range map[string]string{
		"AZURE_SUBSCRIPTION_ID": "ARM_SUBSCRIPTION_ID",
		"AZURE_TENANT_ID":       "ARM_TENANT_ID",
		"AZURE_CLIENT_ID":       "ARM_CLIENT_ID",
		"AZURE_CLIENT_SECRET":   "ARM_CLIENT_SECRET",
	} {
		if safety.TerraformEnvMap[azureEnv] != armEnv {
			t.Fatalf("%s terraform env map %s = %q, want %s", strings.ToUpper(lane), azureEnv, safety.TerraformEnvMap[azureEnv], armEnv)
		}
	}
	switch lane {
	case "z01":
		if !safety.LiveEnabled {
			t.Fatalf("Z01 must be marked live-enabled after guardrails are documented")
		}
		if safety.LiveWaiterMinAttempts < 30 {
			t.Fatalf("Z01 live waiter min attempts = %d, want at least 30", safety.LiveWaiterMinAttempts)
		}
		if !strings.Contains(safety.CleanupFallback, "az sql db delete") {
			t.Fatalf("Z01 cleanup fallback = %q, want az sql db delete", safety.CleanupFallback)
		}
	case "z02":
		if !safety.LiveEnabled {
			t.Fatalf("Z02 must be marked live-enabled after Cosmos cost/isolation/teardown guardrails are documented and approved")
		}
		if !strings.Contains(safety.CleanupFallback, "az cosmosdb delete") || !strings.Contains(safety.CleanupFallback, "az group delete") {
			t.Fatalf("Z02 cleanup fallback = %q, want az cosmosdb delete and az group delete", safety.CleanupFallback)
		}
		for _, guardrail := range []string{"isolated resource group", "approved Cosmos DB cost exposure", "teardown verification"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("Z02 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	case "z03":
		if safety.LiveEnabled {
			t.Fatalf("Z03 must remain live-disabled while read/import static evidence is planned")
		}
		for _, guardrail := range []string{"read/import first", "no child resources", "cleanup verification before recording"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("Z03 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	case "z04":
		if !safety.LiveEnabled {
			t.Fatalf("Z04 must be marked live-enabled after storage cost/naming/cleanup guardrails are approved")
		}
		if !strings.Contains(safety.CleanupFallback, "az storage account delete") || !strings.Contains(safety.CleanupFallback, "az group delete") {
			t.Fatalf("Z04 cleanup fallback = %q, want az storage account delete and az group delete", safety.CleanupFallback)
		}
		for _, guardrail := range []string{"static first", "one minimal storage account only if live-approved", "delete storage account and resource group before recording"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("Z04 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	case "z05":
		if !safety.LiveEnabled {
			t.Fatalf("Z05 must be marked live-enabled after SQL update cost/rollback/cleanup guardrails are approved")
		}
		if !strings.Contains(safety.CleanupFallback, "az sql db delete") {
			t.Fatalf("Z05 cleanup fallback = %q, want az sql db delete", safety.CleanupFallback)
		}
		for _, guardrail := range []string{"static update first", "one disposable database only if live-approved", "delete database before recording"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("Z05 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	case "z06":
		if safety.LiveEnabled {
			t.Fatalf("Z06 must remain live-disabled because the opt-in settle re-recording runs through Z02")
		}
		for _, guardrail := range []string{"explicit Cosmos DB cost approval", "reuse existing Z02 disposable account shape", "resource-group cleanup verification"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("Z06 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	case "z08":
		if !safety.LiveEnabled {
			t.Fatalf("Z08 must be marked live-enabled after existing Resource Group read guardrails are approved")
		}
		for _, guardrail := range []string{"existing resource group read only", "no live Azure mutation", "no raw resource group names in recordings"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("Z08 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	case "z09":
		if !safety.LiveEnabled {
			t.Fatalf("Z09 must be marked live-enabled after empty Resource Group lifecycle guardrails are approved")
		}
		if !strings.Contains(safety.CleanupFallback, "az group delete") {
			t.Fatalf("Z09 cleanup fallback = %q, want az group delete", safety.CleanupFallback)
		}
		for _, guardrail := range []string{"one empty disposable resource group per runtime", "no child resources", "delete resource group before recording"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("Z09 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	}
}

func compareAzureParityZ01Observations(observations []azureParityRuntimeObservation) azureParityObservationComparison {
	fields := []string{
		"after_apply.exists",
		"after_apply.id_present",
		"after_apply.location",
		"after_apply.sku.name",
		"after_apply.sku.tier",
		"after_destroy.exists",
	}
	if len(observations) == 0 {
		return azureParityObservationComparison{Matched: false, Fields: fields}
	}
	matched := true
	first := observations[0].Fields
	for _, observation := range observations {
		for _, field := range fields {
			if observation.Fields[field] != first[field] {
				matched = false
			}
		}
		if observation.Fields["after_apply.exists"] != true || observation.Fields["after_apply.id_present"] != true || observation.Fields["after_destroy.exists"] != false {
			matched = false
		}
	}
	return azureParityObservationComparison{Matched: matched, Fields: fields}
}

func compareAzureParityZ02Observations(observations []azureParityRuntimeObservation) azureParityObservationComparison {
	fields := []string{
		"after_apply.exists",
		"after_apply.id_present",
		"after_apply.location",
		"after_apply.kind",
		"after_apply.offer_type",
		"after_destroy.exists",
		"resource_group_after_cleanup.exists",
	}
	if len(observations) == 0 {
		return azureParityObservationComparison{Matched: false, Fields: fields}
	}
	matched := true
	first := observations[0].Fields
	for _, observation := range observations {
		for _, field := range fields {
			if observation.Fields[field] != first[field] {
				matched = false
			}
		}
		if observation.Fields["after_apply.exists"] != true || observation.Fields["after_apply.id_present"] != true || observation.Fields["after_destroy.exists"] != false {
			matched = false
		}
	}
	return azureParityObservationComparison{Matched: matched, Fields: fields}
}

func compareAzureParityZ04Observations(observations []azureParityRuntimeObservation) azureParityObservationComparison {
	fields := []string{
		"after_apply.exists",
		"after_apply.id_present",
		"after_apply.location",
		"after_apply.kind",
		"after_apply.sku.name",
		"after_destroy.exists",
		"resource_group_after_cleanup.exists",
	}
	return compareAzureParityObservationFields(observations, fields)
}

func compareAzureParityZ05Observations(observations []azureParityRuntimeObservation) azureParityObservationComparison {
	return compareAzureParityZ01Observations(observations)
}

func compareAzureParityZ08Observations(observations []azureParityRuntimeObservation) azureParityObservationComparison {
	fields := []string{
		"exists",
		"id_present",
		"location",
		"name_matches_input",
	}
	comparison := compareAzureParityObservationFields(observations, fields)
	if len(observations) == 0 {
		return comparison
	}
	for _, observation := range observations {
		if observation.Fields["exists"] != true || observation.Fields["id_present"] != true || observation.Fields["name_matches_input"] != true {
			comparison.Matched = false
		}
	}
	return comparison
}

func compareAzureParityZ09Observations(observations []azureParityRuntimeObservation) azureParityObservationComparison {
	fields := []string{
		"after_apply.exists",
		"after_apply.id_present",
		"after_apply.location",
		"after_destroy.exists",
	}
	comparison := compareAzureParityObservationFields(observations, fields)
	if len(observations) == 0 {
		return comparison
	}
	for _, observation := range observations {
		if observation.Fields["after_apply.exists"] != true || observation.Fields["after_apply.id_present"] != true || observation.Fields["after_destroy.exists"] != false {
			comparison.Matched = false
		}
	}
	return comparison
}

func compareAzureParityObservationFields(observations []azureParityRuntimeObservation, fields []string) azureParityObservationComparison {
	return compareAPIParityObservationFields(observations, fields)
}

func TestAzureProviderParityARMEnvMapping(t *testing.T) {
	t.Setenv("AZURE_SUBSCRIPTION_ID", "sub")
	t.Setenv("AZURE_TENANT_ID", "tenant")
	t.Setenv("AZURE_CLIENT_ID", "client")
	t.Setenv("AZURE_CLIENT_SECRET", "secret")
	env := azureParityARMEnvFromProfile()
	for _, want := range []string{
		"ARM_SUBSCRIPTION_ID=sub",
		"ARM_TENANT_ID=tenant",
		"ARM_CLIENT_ID=client",
		"ARM_CLIENT_SECRET=secret",
	} {
		if !slices.Contains(env, want) {
			t.Fatalf("ARM env mapping = %#v, missing %s", env, want)
		}
	}
}

func azureParityARMEnvFromProfile() []string {
	mapping := map[string]string{
		"AZURE_SUBSCRIPTION_ID": "ARM_SUBSCRIPTION_ID",
		"AZURE_TENANT_ID":       "ARM_TENANT_ID",
		"AZURE_CLIENT_ID":       "ARM_CLIENT_ID",
		"AZURE_CLIENT_SECRET":   "ARM_CLIENT_SECRET",
	}
	out := make([]string, 0, len(mapping))
	for azureEnv, armEnv := range mapping {
		if value := strings.TrimSpace(os.Getenv(azureEnv)); value != "" {
			out = append(out, armEnv+"="+value)
		}
	}
	slices.Sort(out)
	return out
}

func assertAzureParityStaticFixtures(t *testing.T, lane string, artifact azureParityArtifact) {
	t.Helper()
	if lane != "z06" {
		assertAzureParityHCLFixture(t, lane, artifact)
		assertAzureParityNativeProjectFixture(t, lane)
	}
	switch lane {
	case "z01":
		assertAzureParityPlanFixture(t, lane, "create", "Databases_CreateOrUpdate", "create")
		assertAzureParityPlanFixture(t, lane, "read", "Databases_Get", "read")
		assertAzureParityPlanFixture(t, lane, "delete", "Databases_Delete", "delete")
		assertAzureParityRequestBindings(t, lane, map[string][]string{
			"create": {"subscriptionId", "resourceGroupName", "serverName", "databaseName", "api-version", "location", "sku"},
			"read":   {"subscriptionId", "resourceGroupName", "serverName", "databaseName", "api-version"},
			"delete": {"subscriptionId", "resourceGroupName", "serverName", "databaseName", "api-version"},
		})
	case "z02":
		assertAzureParityPlanFixture(t, lane, "create", "DatabaseAccounts_CreateOrUpdate", "create")
		assertAzureParitySettleFixture(t, lane)
		assertAzureParityRequestBindings(t, lane, map[string][]string{
			"create": {"subscriptionId", "resourceGroupName", "accountName", "api-version", "location", "kind", "properties"},
			"read":   {"subscriptionId", "resourceGroupName", "accountName", "api-version"},
			"delete": {"subscriptionId", "resourceGroupName", "accountName", "api-version"},
		})
	case "z03", "z08":
		assertAzureParityPlanFixture(t, lane, "read", "ResourceGroups_Get", "read")
		assertAzureParityRequestBindings(t, lane, map[string][]string{
			"read": {"subscriptionId", "resourceGroupName", "api-version"},
		})
	case "z09":
		assertAzureParityPlanFixture(t, lane, "create", "ResourceGroups_CreateOrUpdate", "create")
		assertAzureParityPlanFixture(t, lane, "read", "ResourceGroups_Get", "read")
		assertAzureParityPlanFixture(t, lane, "delete", "ResourceGroups_Delete", "delete")
		assertAzureParityRequestBindings(t, lane, map[string][]string{
			"create": {"subscriptionId", "resourceGroupName", "api-version", "location"},
			"read":   {"subscriptionId", "resourceGroupName", "api-version"},
			"delete": {"subscriptionId", "resourceGroupName", "api-version"},
		})
	case "z04":
		assertAzureParityPlanFixture(t, lane, "create", "StorageAccounts_Create", "create")
		assertAzureParityPlanFixture(t, lane, "read", "StorageAccounts_GetProperties", "read")
		assertAzureParityPlanFixture(t, lane, "delete", "StorageAccounts_Delete", "delete")
		assertAzureParitySettleFixture(t, lane)
		assertAzureParityRequestBindings(t, lane, map[string][]string{
			"create": {"subscriptionId", "resourceGroupName", "accountName", "api-version", "location", "kind", "sku"},
			"read":   {"subscriptionId", "resourceGroupName", "accountName", "api-version"},
			"delete": {"subscriptionId", "resourceGroupName", "accountName", "api-version"},
		})
	case "z05":
		assertAzureParityPlanFixture(t, lane, "create", "Databases_CreateOrUpdate", "create")
		assertAzureParityPlanFixture(t, lane, "read", "Databases_Get", "read")
		assertAzureParityPlanFixture(t, lane, "delete", "Databases_Delete", "delete")
		assertAzureParitySettleFixture(t, lane)
		assertAzureParityRequestBindings(t, lane, map[string][]string{
			"create": {"subscriptionId", "resourceGroupName", "serverName", "databaseName", "api-version", "location", "sku"},
			"read":   {"subscriptionId", "resourceGroupName", "serverName", "databaseName", "api-version"},
			"delete": {"subscriptionId", "resourceGroupName", "serverName", "databaseName", "api-version"},
		})
	case "z06":
		assertAzureParityNativeProjectFixture(t, "z02")
		assertAzureParityPlanFixture(t, "z02", "delete", "DatabaseAccounts_Delete", "delete")
		assertAzureParitySettleFixture(t, "z02")
	default:
		t.Fatalf("no static assertions registered for Azure parity lane %s", lane)
	}
}

func assertAzureParitySettleFixture(t *testing.T, lane string) {
	t.Helper()
	result, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: filepath.Join(azureParityFixtureRoot, lane, "ramen"),
		StatePath:   filepath.Join(t.TempDir(), "state.db"),
		Action:      "delete",
	})
	if err != nil {
		t.Fatalf("build %s delete Ramen fixture plan: %v", strings.ToUpper(lane), err)
	}
	if result.Plan.Errored || len(result.Plan.Resources) != 1 {
		t.Fatalf("%s delete Ramen fixture plan unusable for settle assertion: %#v", strings.ToUpper(lane), result.Plan)
	}
	settle := result.Plan.Resources[0].RuntimeHints.Settle
	if settle["before"] != "delete" || strings.TrimSpace(fmt.Sprint(settle["duration"])) == "" || strings.TrimSpace(fmt.Sprint(settle["interval"])) == "" || settle["read_expect"] != "exists" {
		t.Fatalf("%s settle hints = %#v", strings.ToUpper(lane), settle)
	}
}

func assertAzureParityHCLFixture(t *testing.T, lane string, artifact azureParityArtifact) {
	t.Helper()
	doc, err := tfconfig.LoadDir(filepath.Join(azureParityFixtureRoot, lane, "hcl"))
	if err != nil {
		t.Fatalf("load %s HCL fixture: %v", strings.ToUpper(lane), err)
	}
	if len(doc.Diagnostics) != 0 {
		t.Fatalf("%s HCL fixture diagnostics: %#v", strings.ToUpper(lane), doc.Diagnostics)
	}
	if len(doc.Modules) != 1 {
		t.Fatalf("%s HCL fixture module count = %d, want 1", strings.ToUpper(lane), len(doc.Modules))
	}
	module := doc.Modules[0]
	if len(module.Resources) != 1 {
		t.Fatalf("%s HCL fixture resource count = %d, want 1", strings.ToUpper(lane), len(module.Resources))
	}
	wantType := artifact.Scenarios[0].TerraformResourceType
	if wantType == "" {
		wantType = artifact.Scenarios[0].ResourceType
	}
	if module.Resources[0].Type != wantType {
		t.Fatalf("%s HCL fixture resource type = %q, want %q", strings.ToUpper(lane), module.Resources[0].Type, wantType)
	}
	var foundProvider bool
	for _, req := range module.RequiredProviders {
		if req.LocalName == "azurerm" && req.Source == "hashicorp/azurerm" && slices.Contains(req.VersionConstraints, artifact.Provider.VersionConstraint) {
			foundProvider = true
		}
	}
	if !foundProvider {
		t.Fatalf("%s HCL fixture required providers = %#v, want hashicorp/azurerm %s", strings.ToUpper(lane), module.RequiredProviders, artifact.Provider.VersionConstraint)
	}
}

func assertAzureParityNativeProjectFixture(t *testing.T, lane string) {
	assertAPIParityNativeProjectFixture(t, "Azure", azureParityFixtureRoot, lane)
}

func assertAzureParityPlanFixture(t *testing.T, lane, action, operationID, summaryField string) {
	assertAPIParityPlanFixture(t, "Azure", azureParityFixtureRoot, lane, action, operationID, summaryField, nil)
}

func azureParitySummaryHasOne(summary tfplan.Summary, field string) bool {
	return apiParitySummaryHasOne(summary, field)
}

func assertAzureParityRequestBindings(t *testing.T, lane string, expected map[string][]string) {
	action := "create"
	if lane == "z03" || lane == "z08" {
		action = "read"
	}
	assertAPIParityRequestBindings(t, "Azure", azureParityFixtureRoot, lane, action, expected)
}

func azureParityFailure(runtime, class string, err error) azureParityRuntimeResult {
	return apiParityFailure("Azure", runtime, class, err)
}

func compareOrUpdateAzureParityRecording(t *testing.T, recording azureParityLiveRecording, path string) {
	compareOrUpdateAPIParityRecording(t, "Azure", azureParityEnv, azureParityRecordEnv, path, recording, false, normalizeAzureParityRecording)
}

func normalizeAzureParityRecording(t *testing.T, data []byte) azureParityLiveRecording {
	return normalizeAPIParityRecording(t, "Azure", data, nil)
}

func errAzureParityLiveRunnerParked(lane string) error {
	return fmt.Errorf("%s live runner is not implemented in this build; keep live Azure execution under %s=1 and %s=%s once the runner is completed", strings.ToUpper(lane), azureParityEnv, azureParityLaneEnv, lane)
}
