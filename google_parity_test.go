package corpus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/state"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
	"github.com/OpenUdon/tfconfig"
)

const (
	googleParityEnv         = "RAMEN_GOOGLE_PARITY"
	googleParityRecordEnv   = "RAMEN_GOOGLE_PARITY_RECORD_UPDATE"
	googleParityLaneEnv     = "RAMEN_GOOGLE_PARITY_LANE"
	googleParityTofuEnv     = "RAMEN_GOOGLE_TOFU"
	googleParityArtifactV1  = "ramen.google.provider-parity.v1"
	googleParityFixtureRoot = "testdata/parity/google"
)

var googleParityLanes = []string{"y01", "y02", "y03", "y04", "y05", "y06"}

type googleParityArtifact struct {
	Version          string                 `json:"version"`
	Lane             string                 `json:"lane"`
	Status           string                 `json:"status"`
	Provider         googleParityProvider   `json:"provider"`
	Discovery        googleParityDiscovery  `json:"discovery"`
	Safety           googleParitySafety     `json:"safety"`
	Runtimes         []string               `json:"runtimes"`
	Scenarios        []googleParityScenario `json:"scenarios"`
	RecordedAt       string                 `json:"recorded_at,omitempty"`
	RecordingsSource string                 `json:"recordings_source,omitempty"`
	Notes            []string               `json:"notes,omitempty"`
}

type googleParityProvider struct {
	Source            string `json:"source"`
	VersionConstraint string `json:"version_constraint,omitempty"`
}

type googleParityDiscovery struct {
	SourcePath string `json:"source_path"`
	Fixture    string `json:"fixture"`
}

type googleParitySafety struct {
	LiveEnv              string   `json:"live_env"`
	RecordUpdateEnv      string   `json:"record_update_env"`
	LaneEnv              string   `json:"lane_env"`
	OpenTofuEnv          string   `json:"opentofu_env,omitempty"`
	CredentialBinding    string   `json:"credential_binding"`
	ResourcePrefix       string   `json:"resource_prefix"`
	RequiresExplicitLane bool     `json:"requires_explicit_lane"`
	LiveEnabled          bool     `json:"live_enabled,omitempty"`
	CleanupFallback      string   `json:"cleanup_fallback,omitempty"`
	RequiredEnv          []string `json:"required_env,omitempty"`
	CostGuardrails       []string `json:"cost_guardrails,omitempty"`
	Promotion            string   `json:"promotion,omitempty"`
}

type googleParityScenario struct {
	Name                  string   `json:"name"`
	ResourceType          string   `json:"resource_type"`
	TerraformResourceType string   `json:"terraform_resource_type,omitempty"`
	FixturePaths          []string `json:"fixture_paths,omitempty"`
	OperationIDs          []string `json:"operation_ids"`
	ObservedFields        []string `json:"observed_fields"`
	ExpectedTransitions   []string `json:"expected_transitions"`
	ObservationArtifacts  []string `json:"observation_artifacts,omitempty"`
}

type googleParityLiveRecording struct {
	Version      string                            `json:"version"`
	Lane         string                            `json:"lane"`
	Scenario     string                            `json:"scenario"`
	RecordedAt   string                            `json:"recorded_at"`
	DurationMS   int64                             `json:"duration_ms,omitempty"`
	Observations []googleParityRuntimeObservation  `json:"observations"`
	Comparison   googleParityObservationComparison `json:"comparison"`
	Failures     []googleParityRuntimeFailure      `json:"failures,omitempty"`
}

type googleParityRuntimeObservation struct {
	Runtime    string         `json:"runtime"`
	Resource   string         `json:"resource"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	Fields     map[string]any `json:"fields,omitempty"`
}

type googleParityObservationComparison struct {
	Matched bool     `json:"matched"`
	Fields  []string `json:"fields"`
}

type googleParityRuntimeFailure struct {
	Runtime string `json:"runtime"`
	Class   string `json:"class"`
	Message string `json:"message"`
}

type googleParityRuntimeResult struct {
	Observation googleParityRuntimeObservation
	Failure     *googleParityRuntimeFailure
}

func TestGoogleProviderParityReplayArtifacts(t *testing.T) {
	for _, lane := range googleParityLanes {
		lane := lane
		t.Run(strings.ToUpper(lane), func(t *testing.T) {
			artifact := loadGoogleParityArtifact(t, filepath.Join(googleParityFixtureRoot, lane, "observations.json"))
			assertGoogleParityArtifact(t, lane, artifact)
			assertGoogleParityRecordingArtifacts(t, lane, artifact)
			assertGoogleParityStaticFixtures(t, lane, artifact)
		})
	}
}

func loadGoogleParityArtifact(t *testing.T, path string) googleParityArtifact {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var artifact googleParityArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return artifact
}

func loadGoogleParityLiveRecording(t *testing.T, path string) googleParityLiveRecording {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var recording googleParityLiveRecording
	if err := json.Unmarshal(data, &recording); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return recording
}

func assertGoogleParityArtifact(t *testing.T, lane string, artifact googleParityArtifact) {
	t.Helper()
	wantLane := strings.ToUpper(lane)
	if artifact.Version != googleParityArtifactV1 {
		t.Fatalf("artifact version = %q, want %q", artifact.Version, googleParityArtifactV1)
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
	if artifact.Provider.Source != "hashicorp/google" {
		t.Fatalf("provider source = %q, want hashicorp/google", artifact.Provider.Source)
	}
	if strings.TrimSpace(artifact.Provider.VersionConstraint) == "" {
		t.Fatalf("provider version_constraint is required")
	}
	if _, err := os.Stat(artifact.Discovery.SourcePath); err != nil {
		t.Fatalf("Google Discovery source path %s is not readable: %v", artifact.Discovery.SourcePath, err)
	}
	if artifact.Safety.LiveEnv != googleParityEnv || artifact.Safety.RecordUpdateEnv != googleParityRecordEnv || artifact.Safety.LaneEnv != googleParityLaneEnv {
		t.Fatalf("unexpected Google live env gates: %#v", artifact.Safety)
	}
	if artifact.Safety.OpenTofuEnv != googleParityTofuEnv {
		t.Fatalf("OpenTofu env = %q, want %q", artifact.Safety.OpenTofuEnv, googleParityTofuEnv)
	}
	if artifact.Safety.CredentialBinding != "google_oauth2" {
		t.Fatalf("credential binding = %q, want google_oauth2", artifact.Safety.CredentialBinding)
	}
	if !strings.HasPrefix(artifact.Safety.ResourcePrefix, "ramen-parity-"+lane+"-") {
		t.Fatalf("resource prefix = %q, want ramen-parity-%s-*", artifact.Safety.ResourcePrefix, lane)
	}
	if !artifact.Safety.RequiresExplicitLane {
		t.Fatalf("%s must require explicit lane: %#v", wantLane, artifact.Safety)
	}
	for _, runtime := range []string{"opentofu", "ramen"} {
		if !slices.Contains(artifact.Runtimes, runtime) {
			t.Fatalf("artifact runtimes %v missing %s", artifact.Runtimes, runtime)
		}
	}
	assertGoogleParitySafetyContract(t, lane, artifact.Safety)
	for _, scenario := range artifact.Scenarios {
		if len(scenario.OperationIDs) == 0 || len(scenario.ObservedFields) == 0 || len(scenario.ExpectedTransitions) == 0 {
			t.Fatalf("scenario %s is incomplete: %#v", scenario.Name, scenario)
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

func assertGoogleParityRecordingArtifacts(t *testing.T, lane string, artifact googleParityArtifact) {
	t.Helper()
	if artifact.Status != "recorded" {
		return
	}
	for _, scenario := range artifact.Scenarios {
		if len(scenario.ObservationArtifacts) == 0 {
			t.Fatalf("recorded scenario %s must include observation_artifacts", scenario.Name)
		}
		for _, path := range scenario.ObservationArtifacts {
			recording := loadGoogleParityLiveRecording(t, path)
			assertGoogleParityLiveRecording(t, lane, artifact, scenario, recording)
		}
	}
}

func assertGoogleParityLiveRecording(t *testing.T, lane string, artifact googleParityArtifact, scenario googleParityScenario, recording googleParityLiveRecording) {
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
	if recording.DurationMS < 0 {
		t.Fatalf("%s recording duration_ms = %d, want non-negative", wantLane, recording.DurationMS)
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
		assertGoogleParityRecordingSanitized(t, wantLane, observation)
	}
	for _, runtime := range []string{"opentofu", "ramen"} {
		if !seen[runtime] {
			t.Fatalf("%s recording missing required runtime %q", wantLane, runtime)
		}
	}
	switch lane {
	case "y03":
		want := compareGoogleParityObservations(recording.Observations, recording.Comparison.Fields)
		if !reflect.DeepEqual(recording.Comparison, want) {
			t.Fatalf("%s recording comparison = %#v, want %#v", wantLane, recording.Comparison, want)
		}
		if !recording.Comparison.Matched {
			t.Fatalf("%s recording comparison did not match", wantLane)
		}
	default:
		t.Fatalf("no recording assertions registered for Google parity lane %s", lane)
	}
}

func assertGoogleParityRecordingSanitized(t *testing.T, lane string, observation googleParityRuntimeObservation) {
	t.Helper()
	forbidden := []string{
		"access_token",
		"application_default_credentials",
		"authorization",
		"client_secret",
		"credential",
		"google_application_credentials",
		"oauth",
		"private_key",
		"raw_response",
		"refresh_token",
	}
	values := []string{observation.Resource}
	for key, value := range observation.Fields {
		values = append(values, key)
		if valueString, ok := value.(string); ok {
			values = append(values, valueString)
		}
	}
	for _, value := range values {
		normalized := strings.ToLower(value)
		for _, bad := range forbidden {
			if strings.Contains(normalized, bad) {
				t.Fatalf("%s recording contains forbidden material %q in %q", lane, bad, value)
			}
		}
	}
}

func assertGoogleParitySafetyContract(t *testing.T, lane string, safety googleParitySafety) {
	t.Helper()
	for _, envName := range []string{googleParityTofuEnv, "GOOGLE_APPLICATION_CREDENTIALS", "RAMEN_GOOGLE_PROJECT"} {
		if !slices.Contains(safety.RequiredEnv, envName) {
			t.Fatalf("%s required_env = %#v, missing %s", strings.ToUpper(lane), safety.RequiredEnv, envName)
		}
	}
	if !strings.Contains(safety.CleanupFallback, "gcloud ") {
		t.Fatalf("%s cleanup fallback = %q, want gcloud command", strings.ToUpper(lane), safety.CleanupFallback)
	}
	switch lane {
	case "y01":
		if safety.LiveEnabled {
			t.Fatalf("Y01 must remain live-disabled as the static baseline")
		}
		for _, guardrail := range []string{"static first", "no live GCP mutation", "global bucket cleanup review before recording"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("Y01 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
		if !strings.Contains(safety.Promotion, "parked") {
			t.Fatalf("Y01 promotion = %q, want parked live promotion", safety.Promotion)
		}
	case "y02":
		if !safety.LiveEnabled {
			t.Fatalf("Y02 must be live-enabled for opt-in read-only observation")
		}
		if !slices.Contains(safety.RequiredEnv, "RAMEN_GOOGLE_EXISTING_BUCKET") {
			t.Fatalf("Y02 required_env = %#v, missing RAMEN_GOOGLE_EXISTING_BUCKET", safety.RequiredEnv)
		}
		for _, guardrail := range []string{"read-only live observation", "no live GCP mutation", "operator-provided existing bucket"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("Y02 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	case "y03":
		if !safety.LiveEnabled {
			t.Fatalf("Y03 must be live-enabled for opt-in disposable bucket mutation")
		}
		for _, guardrail := range []string{"one disposable bucket at a time", "empty bucket only", "delete bucket before recording"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("Y03 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	case "y04":
		if !safety.LiveEnabled {
			t.Fatalf("Y04 must be live-enabled for opt-in read-missing mutation")
		}
		for _, guardrail := range []string{"one disposable bucket at a time", "empty bucket only", "out-of-band delete before read-missing"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("Y04 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	case "y05":
		if !safety.LiveEnabled {
			t.Fatalf("Y05 must be live-enabled for opt-in object metadata upload mutation")
		}
		for _, guardrail := range []string{"one disposable support bucket per runtime", "tiny non-secret object content", "no object content in observations"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("Y05 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	case "y06":
		if !safety.LiveEnabled {
			t.Fatalf("Y06 must be live-enabled for opt-in managed folder mutation")
		}
		for _, guardrail := range []string{"one disposable HNS bucket per runtime", "no IAM mutation", "delete managed folder and bucket before recording"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("Y06 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	default:
		t.Fatalf("no safety assertions registered for Google parity lane %s", lane)
	}
}

func assertGoogleParityStaticFixtures(t *testing.T, lane string, artifact googleParityArtifact) {
	t.Helper()
	assertGoogleParityHCLFixture(t, lane, artifact)
	assertGoogleParityNativeProjectFixture(t, lane)
	switch lane {
	case "y01", "y03", "y04":
		assertGoogleParityPlanFixture(t, lane, "create", "storage.buckets.insert", "create", false)
		assertGoogleParityPlanFixture(t, lane, "read", "storage.buckets.get", "read", false)
		if lane != "y04" {
			assertGoogleParityPlanFixture(t, lane, "create", "storage.buckets.patch", "update", true)
		}
		assertGoogleParityPlanFixture(t, lane, "delete", "storage.buckets.delete", "delete", true)
		if lane == "y03" {
			assertGoogleParityRequestBindings(t, lane, map[string][]string{
				"create": {"name", "location", "iamConfiguration.uniformBucketLevelAccess.enabled", "project", "labels"},
				"read":   {"bucket"},
				"update": {"bucket", "labels.ramen_parity_phase"},
				"delete": {"bucket"},
			})
		} else if lane == "y04" {
			assertGoogleParityRequestBindings(t, lane, map[string][]string{
				"create": {"name", "location", "iamConfiguration.uniformBucketLevelAccess.enabled", "project"},
				"read":   {"bucket"},
				"delete": {"bucket"},
			})
		} else {
			assertGoogleParityRequestBindings(t, lane, map[string][]string{
				"create": {"name", "location", "iamConfiguration.uniformBucketLevelAccess.enabled"},
				"read":   {"bucket"},
				"update": {"bucket", "location", "iamConfiguration.uniformBucketLevelAccess.enabled"},
				"delete": {"bucket"},
			})
		}
		assertGoogleParityResponseBindings(t, lane, []string{"name", "id", "location", "iamConfiguration.uniformBucketLevelAccess.enabled"})
	case "y02":
		assertGoogleParityPlanFixture(t, lane, "read", "storage.buckets.get", "read", false)
		assertGoogleParityRequestBindings(t, lane, map[string][]string{
			"read": {"bucket"},
		})
		assertGoogleParityResponseBindings(t, lane, []string{"name", "id", "location", "iamConfiguration.uniformBucketLevelAccess.enabled"})
	case "y05":
		assertGoogleParityPlanFixture(t, lane, "create", "storage.objects.insert", "create", false)
		assertGoogleParityPlanFixture(t, lane, "read", "storage.objects.get", "read", false)
		assertGoogleParityPlanFixture(t, lane, "create", "storage.objects.patch", "update", true)
		assertGoogleParityPlanFixture(t, lane, "delete", "storage.objects.delete", "delete", true)
		assertGoogleParityRequestBindings(t, lane, map[string][]string{
			"create": {"bucket", "name", "uploadType", "metadata.contentType", "metadata.metadata", "content"},
			"read":   {"bucket", "object"},
			"update": {"bucket", "object", "metadata.ramen_parity_phase"},
			"delete": {"bucket", "object"},
		})
		assertGoogleParityResponseBindings(t, lane, []string{"name", "bucket", "id", "generation", "size", "metadata.ramen_parity_phase"})
	case "y06":
		assertGoogleParityPlanFixture(t, lane, "create", "storage.managedFolders.insert", "create", false)
		assertGoogleParityPlanFixture(t, lane, "read", "storage.managedFolders.get", "read", false)
		assertGoogleParityPlanFixture(t, lane, "delete", "storage.managedFolders.delete", "delete", true)
		assertGoogleParityRequestBindings(t, lane, map[string][]string{
			"create": {"bucket", "name"},
			"read":   {"bucket", "managedFolder"},
			"delete": {"bucket", "managedFolder"},
		})
		assertGoogleParityResponseBindings(t, lane, []string{"name", "bucket", "id", "metageneration"})
	default:
		t.Fatalf("no static assertions registered for Google parity lane %s", lane)
	}
	assertGoogleParityRequestBindingLocations(t, lane)
	assertGoogleParityUdonMetadata(t, lane)
}

func assertGoogleParityHCLFixture(t *testing.T, lane string, artifact googleParityArtifact) {
	t.Helper()
	doc, err := tfconfig.LoadDir(filepath.Join(googleParityFixtureRoot, lane, "hcl"))
	if err != nil {
		t.Fatalf("load %s HCL fixture: %v", strings.ToUpper(lane), err)
	}
	if len(doc.Diagnostics) != 0 {
		t.Fatalf("%s HCL fixture diagnostics: %#v", strings.ToUpper(lane), doc.Diagnostics)
	}
	if len(doc.Modules) != 1 {
		t.Fatalf("%s HCL fixture module count = %d, want 1", strings.ToUpper(lane), len(doc.Modules))
	}
	wantType := artifact.Scenarios[0].TerraformResourceType
	var gotType string
	switch lane {
	case "y02":
		if len(doc.Modules[0].Resources) != 0 || len(doc.Modules[0].DataSources) != 1 {
			t.Fatalf("%s HCL fixture shape: resources=%d data_sources=%d", strings.ToUpper(lane), len(doc.Modules[0].Resources), len(doc.Modules[0].DataSources))
		}
		gotType = doc.Modules[0].DataSources[0].Type
	default:
		if len(doc.Modules[0].Resources) != 1 || len(doc.Modules[0].DataSources) != 0 {
			t.Fatalf("%s HCL fixture shape: resources=%d data_sources=%d", strings.ToUpper(lane), len(doc.Modules[0].Resources), len(doc.Modules[0].DataSources))
		}
		gotType = doc.Modules[0].Resources[0].Type
	}
	if gotType != wantType {
		t.Fatalf("%s HCL object type = %q, want %q", strings.ToUpper(lane), gotType, wantType)
	}
	var foundProvider bool
	for _, req := range doc.Modules[0].RequiredProviders {
		if req.LocalName == "google" && req.Source == "hashicorp/google" && slices.Contains(req.VersionConstraints, artifact.Provider.VersionConstraint) {
			foundProvider = true
		}
	}
	if !foundProvider {
		t.Fatalf("%s HCL required providers = %#v, want hashicorp/google %s", strings.ToUpper(lane), doc.Modules[0].RequiredProviders, artifact.Provider.VersionConstraint)
	}
}

func assertGoogleParityNativeProjectFixture(t *testing.T, lane string) {
	t.Helper()
	projectPath := filepath.Join(googleParityFixtureRoot, lane, "ramen")
	result, err := ramenvalidate.Run(context.Background(), ramenvalidate.Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatalf("validate %s native Ramen project fixture: %v", strings.ToUpper(lane), err)
	}
	if !result.Valid || result.Summary.Diagnostics != 0 {
		t.Fatalf("%s native Ramen project fixture diagnostics: valid=%t summary=%#v diagnostics=%#v", strings.ToUpper(lane), result.Valid, result.Summary, result.Diagnostics)
	}
}

func assertGoogleParityPlanFixture(t *testing.T, lane, action, operationID, summaryField string, seedState bool) {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "state.db")
	if seedState {
		seedGoogleParityState(t, lane, statePath)
	}
	result, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: filepath.Join(googleParityFixtureRoot, lane, "ramen"),
		StatePath:   statePath,
		Action:      action,
	})
	if err != nil {
		t.Fatalf("build %s %s Ramen fixture plan: %v", strings.ToUpper(lane), action, err)
	}
	if result.Plan.Errored || len(result.Plan.Resources) != 1 {
		t.Fatalf("%s %s Ramen fixture plan unusable: %#v", strings.ToUpper(lane), action, result.Plan)
	}
	resource := result.Plan.Resources[0]
	if resource.Mapping == nil || resource.Mapping.OperationID != operationID {
		t.Fatalf("%s %s operation = %#v, want %s", strings.ToUpper(lane), action, resource.Mapping, operationID)
	}
	if !googleParitySummaryHasOne(result.Plan.Summary, summaryField) {
		t.Fatalf("%s %s plan summary = %#v, want one %s action", strings.ToUpper(lane), action, result.Plan.Summary, summaryField)
	}
}

func seedGoogleParityState(t *testing.T, lane, statePath string) {
	t.Helper()
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open %s seed state: %v", strings.ToUpper(lane), err)
	}
	defer store.Close()
	snapshot, err := googleParitySeedSnapshot(lane)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordResource(context.Background(), snapshot); err != nil {
		t.Fatalf("record %s seed state: %v", strings.ToUpper(lane), err)
	}
}

func googleParitySeedSnapshot(lane string) (state.ResourceSnapshot, error) {
	switch lane {
	case "y01", "y03", "y04":
		return state.ResourceSnapshot{
			Address:        "google_storage_bucket.bucket",
			Type:           "google_storage_bucket",
			Provider:       "provider.google",
			DesiredHash:    "sha256:previous",
			IdentityJSON:   `{"bucket_name":"ramen-parity-` + lane + `-static"}`,
			AttributesJSON: `{"name":"ramen-parity-` + lane + `-static","location":"EU","uniform_bucket_level_access":false}`,
			Status:         "managed",
		}, nil
	case "y05":
		return state.ResourceSnapshot{
			Address:        "google_storage_bucket_object.object",
			Type:           "google_storage_bucket_object",
			Provider:       "provider.google",
			DesiredHash:    "sha256:previous",
			IdentityJSON:   `{"bucket_name":"ramen-parity-y05-static","object_name":"ramen-parity-y05-object.txt"}`,
			AttributesJSON: `{"bucket":"ramen-parity-y05-static","name":"ramen-parity-y05-object.txt","upload_type":"multipart","content":"ramen-y05-static-fixture","content_type":"text/plain","metadata":{"ramen_parity_phase":"create"}}`,
			Status:         "managed",
		}, nil
	case "y06":
		return state.ResourceSnapshot{
			Address:        "google_storage_managed_folder.folder",
			Type:           "google_storage_managed_folder",
			Provider:       "provider.google",
			DesiredHash:    "sha256:previous",
			IdentityJSON:   `{"bucket_name":"ramen-parity-y06-static","folder_name":"managed/y06/"}`,
			AttributesJSON: `{"bucket":"ramen-parity-y06-static","name":"managed/y06/"}`,
			Status:         "managed",
		}, nil
	default:
		return state.ResourceSnapshot{}, fmt.Errorf("no seed state registered for Google parity lane %s", lane)
	}
}

func googleParitySummaryHasOne(summary tfplan.Summary, field string) bool {
	switch field {
	case "create":
		return summary.Create == 1
	case "read":
		return summary.Read == 1
	case "update":
		return summary.Update == 1
	case "delete":
		return summary.Delete == 1
	default:
		return false
	}
}

func compareGoogleParityObservations(observations []googleParityRuntimeObservation, fields []string) googleParityObservationComparison {
	if len(observations) == 0 {
		return googleParityObservationComparison{Matched: false, Fields: fields}
	}
	matched := true
	first := observations[0].Fields
	for _, observation := range observations {
		for _, field := range fields {
			if observation.Fields[field] != first[field] {
				matched = false
			}
		}
		if observation.Fields["exists"] != true {
			matched = false
		}
	}
	return googleParityObservationComparison{Matched: matched, Fields: fields}
}

func googleParityFailure(runtime, class string, err error) googleParityRuntimeResult {
	if err == nil {
		err = errors.New("unknown Google parity failure")
	}
	return googleParityRuntimeResult{Failure: &googleParityRuntimeFailure{
		Runtime: runtime,
		Class:   class,
		Message: err.Error(),
	}}
}

func compareOrUpdateGoogleParityRecording(t *testing.T, recording googleParityLiveRecording, path string) {
	t.Helper()
	data, err := json.MarshalIndent(recording, "", "  ")
	if err != nil {
		t.Fatalf("encode Google parity recording: %v", err)
	}
	data = append(data, '\n')
	if os.Getenv(googleParityRecordEnv) == "1" {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write Google parity recording %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Logf("no committed Google parity recording at %s; live run was not recorded because %s is not set", path, googleParityRecordEnv)
		return
	}
	if err != nil {
		t.Fatalf("read committed Google parity recording %s: %v", path, err)
	}
	if !reflect.DeepEqual(normalizeGoogleParityRecording(t, want), normalizeGoogleParityRecording(t, data)) {
		t.Fatalf("live Google parity recording differs from %s; rerun with %s=1 %s=1 only after reviewing sanitized live output", path, googleParityEnv, googleParityRecordEnv)
	}
}

func normalizeGoogleParityRecording(t *testing.T, data []byte) googleParityLiveRecording {
	t.Helper()
	var recording googleParityLiveRecording
	if err := json.Unmarshal(data, &recording); err != nil {
		t.Fatalf("decode Google parity recording: %v", err)
	}
	recording.RecordedAt = ""
	recording.DurationMS = 0
	for i := range recording.Observations {
		recording.Observations[i].DurationMS = 0
		recording.Observations[i].Resource = normalizeGoogleParityGeneratedResource(recording.Lane, recording.Observations[i].Runtime, recording.Observations[i].Resource)
		for key, value := range recording.Observations[i].Fields {
			if valueString, ok := value.(string); ok {
				recording.Observations[i].Fields[key] = normalizeGoogleParityGeneratedResource(recording.Lane, recording.Observations[i].Runtime, valueString)
			}
		}
	}
	return recording
}

func normalizeGoogleParityGeneratedResource(lane, runtime, value string) string {
	lane = strings.ToLower(strings.TrimSpace(lane))
	runtime = strings.ToLower(strings.TrimSpace(runtime))
	if lane == "" || runtime == "" {
		return value
	}
	prefix := "ramen-parity-" + lane + "-" + runtime + "-"
	idx := strings.Index(value, prefix)
	if idx < 0 {
		return value
	}
	end := idx + len(prefix)
	for end < len(value) {
		c := value[end]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			end++
			continue
		}
		break
	}
	if end == idx+len(prefix) {
		return value
	}
	return value[:idx] + prefix + "<run>" + value[end:]
}

func assertGoogleParityRequestBindings(t *testing.T, lane string, expected map[string][]string) {
	t.Helper()
	result, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: filepath.Join(googleParityFixtureRoot, lane, "ramen"),
		StatePath:   filepath.Join(t.TempDir(), "state.db"),
		Action:      googleParityActionForBindings(lane),
	})
	if err != nil {
		t.Fatalf("build %s Ramen fixture plan for request bindings: %v", strings.ToUpper(lane), err)
	}
	if result.Plan.Errored || len(result.Plan.Resources) != 1 || result.Plan.Resources[0].Mapping == nil {
		t.Fatalf("%s Ramen fixture plan unusable for request bindings: %#v", strings.ToUpper(lane), result.Plan)
	}
	bindings := map[string][]string{}
	for _, binding := range result.Plan.Resources[0].Mapping.RequestBindings {
		bindings[binding.OperationRole] = append(bindings[binding.OperationRole], binding.RequestPath)
	}
	for role, wantPaths := range expected {
		for _, wantPath := range wantPaths {
			if !slices.Contains(bindings[role], wantPath) {
				t.Fatalf("%s request bindings for role %s = %#v, want %s", strings.ToUpper(lane), role, bindings[role], wantPath)
			}
		}
	}
}

func googleParityActionForBindings(lane string) string {
	if lane == "y02" {
		return "read"
	}
	return "create"
}

func assertGoogleParityRequestBindingLocations(t *testing.T, lane string) {
	t.Helper()
	result, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: filepath.Join(googleParityFixtureRoot, lane, "ramen"),
		StatePath:   filepath.Join(t.TempDir(), "state.db"),
		Action:      googleParityActionForBindings(lane),
	})
	if err != nil {
		t.Fatalf("build %s Ramen fixture plan for request binding locations: %v", strings.ToUpper(lane), err)
	}
	if result.Plan.Errored || len(result.Plan.Resources) != 1 || result.Plan.Resources[0].Mapping == nil {
		t.Fatalf("%s Ramen fixture plan unusable for request binding locations: %#v", strings.ToUpper(lane), result.Plan)
	}
	for _, binding := range result.Plan.Resources[0].Mapping.RequestBindings {
		switch binding.RequestPath {
		case "bucket":
			if binding.Location != "path" {
				t.Fatalf("%s bucket request binding for role %s location = %q, want path", strings.ToUpper(lane), binding.OperationRole, binding.Location)
			}
		case "project":
			if (lane == "y03" || lane == "y04") && binding.Location != "query" {
				t.Fatalf("%s project request binding location = %q, want query", strings.ToUpper(lane), binding.Location)
			}
		case "object", "managedFolder":
			if binding.Location != "path" {
				t.Fatalf("%s %s request binding for role %s location = %q, want path", strings.ToUpper(lane), binding.RequestPath, binding.OperationRole, binding.Location)
			}
		}
	}
}

func assertGoogleParityResponseBindings(t *testing.T, lane string, expected []string) {
	t.Helper()
	result, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: filepath.Join(googleParityFixtureRoot, lane, "ramen"),
		StatePath:   filepath.Join(t.TempDir(), "state.db"),
		Action:      "read",
	})
	if err != nil {
		t.Fatalf("build %s Ramen fixture plan for response bindings: %v", strings.ToUpper(lane), err)
	}
	if result.Plan.Errored || len(result.Plan.Resources) != 1 || result.Plan.Resources[0].Mapping == nil {
		t.Fatalf("%s Ramen fixture plan unusable for response bindings: %#v", strings.ToUpper(lane), result.Plan)
	}
	var got []string
	for _, binding := range result.Plan.Resources[0].Mapping.ResponseBindings {
		got = append(got, binding.ResponsePath)
	}
	for _, want := range expected {
		if !slices.Contains(got, want) {
			t.Fatalf("%s response bindings = %#v, want %s", strings.ToUpper(lane), got, want)
		}
	}
}

func assertGoogleParityUdonMetadata(t *testing.T, lane string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(googleParityFixtureRoot, lane, "ramen", "project.uws.yaml"))
	if err != nil {
		t.Fatalf("read %s native fixture: %v", strings.ToUpper(lane), err)
	}
	text := string(data)
	for _, want := range []string{"x-udon-config", "google_oauth2", "provider", "storage.googleapis.com"} {
		if !strings.Contains(text, want) {
			t.Fatalf("%s native fixture missing %q", strings.ToUpper(lane), want)
		}
	}
}
