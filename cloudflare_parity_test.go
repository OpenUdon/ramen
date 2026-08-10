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
	"github.com/OpenUdon/ramen/state"
	"github.com/OpenUdon/tfconfig"
)

const (
	cloudflareParityEnv          = "RAMEN_CLOUDFLARE_PARITY"
	cloudflareParityRecordEnv    = "RAMEN_CLOUDFLARE_PARITY_RECORD_UPDATE"
	cloudflareParityLaneEnv      = "RAMEN_CLOUDFLARE_PARITY_LANE"
	cloudflareParityTerraformEnv = "RAMEN_CLOUDFLARE_TERRAFORM"
	cloudflareParityTofuEnv      = "RAMEN_CLOUDFLARE_TOFU"
	cloudflareParityArtifactV1   = "ramen.cloudflare.provider-parity.v1"
	cloudflareParityFixtureRoot  = "testdata/parity/cloudflare"
)

var cloudflareParityLanes = []string{"c01", "c02", "c03", "c04", "c05", "c06"}
var cloudflareParityLiveRunnerLanes = []string{"c01", "c02", "c03", "c04", "c05", "c06"}

type cloudflareParityArtifact struct {
	Version          string                     `json:"version"`
	Lane             string                     `json:"lane"`
	Status           string                     `json:"status"`
	Provider         cloudflareParityProvider   `json:"provider"`
	OpenAPI          cloudflareParityOpenAPI    `json:"openapi"`
	Safety           cloudflareParitySafety     `json:"safety"`
	Recording        cloudflareParityRecording  `json:"recording,omitempty"`
	Promotion        cloudflareParityPromotion  `json:"promotion,omitempty"`
	Runtimes         []string                   `json:"runtimes"`
	Scenarios        []cloudflareParityScenario `json:"scenarios"`
	RecordedAt       string                     `json:"recorded_at,omitempty"`
	RecordingsSource string                     `json:"recordings_source,omitempty"`
	Notes            []string                   `json:"notes,omitempty"`
}

type cloudflareParityProvider struct {
	Source            string `json:"source"`
	VersionConstraint string `json:"version_constraint,omitempty"`
}

type cloudflareParityOpenAPI struct {
	SourcePath string `json:"source_path"`
	Fixture    string `json:"fixture"`
}

type cloudflareParitySafety struct {
	LiveEnv              string   `json:"live_env"`
	RecordUpdateEnv      string   `json:"record_update_env"`
	LaneEnv              string   `json:"lane_env"`
	OpenTofuEnv          string   `json:"opentofu_env,omitempty"`
	TerraformEnv         string   `json:"terraform_env,omitempty"`
	AccountEnv           string   `json:"account_env,omitempty"`
	CredentialBinding    string   `json:"credential_binding"`
	CredentialEnv        string   `json:"credential_env,omitempty"`
	ResourcePrefix       string   `json:"resource_prefix"`
	RequiresExplicitLane bool     `json:"requires_explicit_lane"`
	LiveEnabled          bool     `json:"live_enabled,omitempty"`
	CleanupFallback      string   `json:"cleanup_fallback,omitempty"`
	RequiredEnv          []string `json:"required_env,omitempty"`
	CostGuardrails       []string `json:"cost_guardrails,omitempty"`
	Promotion            string   `json:"promotion,omitempty"`
}

type cloudflareParityRecording struct {
	Status               string `json:"status,omitempty"`
	ArtifactPath         string `json:"artifact_path,omitempty"`
	UpdateEnv            string `json:"update_env,omitempty"`
	CompareWithoutUpdate bool   `json:"compare_without_update,omitempty"`
	Decision             string `json:"decision,omitempty"`
}

type cloudflareParityPromotion struct {
	Next          string   `json:"next,omitempty"`
	LiveCandidate bool     `json:"live_candidate,omitempty"`
	Preconditions []string `json:"preconditions,omitempty"`
	Blockers      []string `json:"blockers,omitempty"`
}

type cloudflareParityScenario = apiParityScenario
type cloudflareParityLiveRecording = apiParityLiveRecording
type cloudflareParityRuntimeObservation = apiParityRuntimeObservation
type cloudflareParityObservationComparison = apiParityObservationComparison
type cloudflareParityRuntimeFailure = apiParityRuntimeFailure
type cloudflareParityRuntimeResult = apiParityRuntimeResult

func TestCloudflareProviderParityReplayArtifacts(t *testing.T) {
	for _, lane := range cloudflareParityLanes {
		lane := lane
		t.Run(strings.ToUpper(lane), func(t *testing.T) {
			artifact := loadCloudflareParityArtifact(t, filepath.Join(cloudflareParityFixtureRoot, lane, "observations.json"))
			assertCloudflareParityArtifact(t, lane, artifact)
			assertCloudflareParityRecordingArtifacts(t, lane, artifact)
			assertCloudflareParityStaticFixtures(t, lane, artifact)
		})
	}
}

func TestCloudflareC05D1UpdateRemainsUnclaimedWithoutSourceOperation(t *testing.T) {
	artifact := loadCloudflareParityArtifact(t, filepath.Join(cloudflareParityFixtureRoot, "c05", "observations.json"))
	var d1Scenario *cloudflareParityScenario
	for i := range artifact.Scenarios {
		if artifact.Scenarios[i].ResourceType == "cloudflare_d1_database" {
			d1Scenario = &artifact.Scenarios[i]
			break
		}
	}
	if d1Scenario == nil {
		t.Fatal("C05 artifact missing cloudflare_d1_database scenario")
	}
	for _, operationID := range d1Scenario.OperationIDs {
		if strings.Contains(strings.ToLower(operationID), "update") {
			t.Fatalf("C05 D1 scenario unexpectedly advertises update operation %q", operationID)
		}
	}
	notes := strings.ToLower(strings.Join(artifact.Notes, "\n"))
	if !strings.Contains(notes, "does not claim d1 update") {
		t.Fatalf("C05 notes must keep D1 update exclusion explicit: %#v", artifact.Notes)
	}
	source, err := os.ReadFile(artifact.OpenAPI.SourcePath)
	if err != nil {
		t.Fatalf("read Cloudflare OpenAPI source: %v", err)
	}
	if strings.Contains(strings.ToLower(string(source)), "d1-update") {
		t.Fatalf("focused Cloudflare source unexpectedly contains d1-update operation")
	}
}

func loadCloudflareParityArtifact(t *testing.T, path string) cloudflareParityArtifact {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var artifact cloudflareParityArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return artifact
}

func loadCloudflareParityLiveRecording(t *testing.T, path string) cloudflareParityLiveRecording {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var recording cloudflareParityLiveRecording
	if err := json.Unmarshal(data, &recording); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return recording
}

func assertCloudflareParityArtifact(t *testing.T, lane string, artifact cloudflareParityArtifact) {
	t.Helper()
	wantLane := strings.ToUpper(lane)
	if artifact.Version != cloudflareParityArtifactV1 {
		t.Fatalf("artifact version = %q, want %q", artifact.Version, cloudflareParityArtifactV1)
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
	if artifact.Provider.Source != "cloudflare/cloudflare" {
		t.Fatalf("provider source = %q, want cloudflare/cloudflare", artifact.Provider.Source)
	}
	if strings.TrimSpace(artifact.Provider.VersionConstraint) == "" {
		t.Fatalf("provider version_constraint is required")
	}
	if _, err := os.Stat(artifact.OpenAPI.SourcePath); err != nil {
		t.Fatalf("Cloudflare OpenAPI source path %s is not readable: %v", artifact.OpenAPI.SourcePath, err)
	}
	if artifact.Safety.LiveEnv != cloudflareParityEnv || artifact.Safety.RecordUpdateEnv != cloudflareParityRecordEnv || artifact.Safety.LaneEnv != cloudflareParityLaneEnv {
		t.Fatalf("unexpected Cloudflare live env gates: %#v", artifact.Safety)
	}
	if artifact.Safety.OpenTofuEnv != cloudflareParityTofuEnv {
		t.Fatalf("OpenTofu env = %q, want %q", artifact.Safety.OpenTofuEnv, cloudflareParityTofuEnv)
	}
	if artifact.Safety.TerraformEnv != cloudflareParityTerraformEnv {
		t.Fatalf("Terraform env = %q, want %q", artifact.Safety.TerraformEnv, cloudflareParityTerraformEnv)
	}
	if artifact.Safety.AccountEnv != "CLOUDFLARE_ACCOUNT_ID" {
		t.Fatalf("account env = %q, want CLOUDFLARE_ACCOUNT_ID", artifact.Safety.AccountEnv)
	}
	if artifact.Safety.CredentialBinding != "cloudflare_api_token" {
		t.Fatalf("credential binding = %q, want cloudflare_api_token", artifact.Safety.CredentialBinding)
	}
	if artifact.Safety.CredentialEnv != "UDON_CREDENTIAL_CLOUDFLARE_API_TOKEN" {
		t.Fatalf("credential env = %q, want UDON_CREDENTIAL_CLOUDFLARE_API_TOKEN", artifact.Safety.CredentialEnv)
	}
	if !strings.HasPrefix(artifact.Safety.ResourcePrefix, "ramen-parity-"+lane+"-") {
		t.Fatalf("resource prefix = %q, want ramen-parity-%s-*", artifact.Safety.ResourcePrefix, lane)
	}
	if !artifact.Safety.RequiresExplicitLane {
		t.Fatalf("%s must require explicit lane selection", wantLane)
	}
	for _, runtime := range []string{"opentofu", "terraform", "ramen"} {
		if !slices.Contains(artifact.Runtimes, runtime) {
			t.Fatalf("artifact runtimes %v missing %s", artifact.Runtimes, runtime)
		}
	}
	assertCloudflareParitySafetyContract(t, lane, artifact.Safety)
	if artifact.Safety.LiveEnabled && !slices.Contains(cloudflareParityLiveRunnerLanes, lane) {
		t.Fatalf("%s is marked live-enabled but has no registered Cloudflare live runner", wantLane)
	}
	assertCloudflareParityRecordingPolicy(t, lane, artifact.Recording)
	assertCloudflareParityPromotionPolicy(t, lane, artifact.Promotion)
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

func assertCloudflareParityRecordingArtifacts(t *testing.T, lane string, artifact cloudflareParityArtifact) {
	t.Helper()
	if artifact.Status != "recorded" {
		return
	}
	for _, scenario := range artifact.Scenarios {
		if len(scenario.ObservationArtifacts) == 0 {
			t.Fatalf("recorded scenario %s must include observation_artifacts", scenario.Name)
		}
		for _, path := range scenario.ObservationArtifacts {
			recording := loadCloudflareParityLiveRecording(t, path)
			assertCloudflareParityLiveRecording(t, lane, artifact, scenario, recording)
		}
	}
}

// cloudflareParityRequiredFieldValues pins exact observed field values that
// cross-runtime equality cannot catch on its own. A lane whose point is that an
// update actually changes a field (e.g. C06 toggling D1 read replication to
// "auto") would otherwise pass on a uniform no-op across all runtimes. Add an
// entry whenever a lane asserts a specific post-mutation value.
var cloudflareParityRequiredFieldValues = map[string]map[string]any{
	"C06": {"after_update.read_replication_mode": "auto"},
}

func assertCloudflareParityLiveRecording(t *testing.T, lane string, artifact cloudflareParityArtifact, scenario cloudflareParityScenario, recording cloudflareParityLiveRecording) {
	t.Helper()
	wantLane := strings.ToUpper(lane)
	if recording.Version != artifact.Version || recording.Lane != wantLane || recording.Scenario != scenario.Name {
		t.Fatalf("%s recording header = %#v, want version=%s lane=%s scenario=%s", wantLane, recording, artifact.Version, wantLane, scenario.Name)
	}
	if strings.TrimSpace(recording.RecordedAt) == "" || recording.DurationMS < 0 {
		t.Fatalf("%s recording timestamp/duration invalid: %#v", wantLane, recording)
	}
	if len(recording.Failures) != 0 || len(recording.Observations) == 0 {
		t.Fatalf("%s recording failures/observations invalid: %#v", wantLane, recording)
	}
	seen := map[string]bool{}
	for _, observation := range recording.Observations {
		if !slices.Contains(artifact.Runtimes, observation.Runtime) {
			t.Fatalf("%s recording runtime %q is not declared in %#v", wantLane, observation.Runtime, artifact.Runtimes)
		}
		if seen[observation.Runtime] {
			t.Fatalf("%s recording repeats runtime %q", wantLane, observation.Runtime)
		}
		seen[observation.Runtime] = true
		if !strings.HasPrefix(observation.Resource, artifact.Safety.ResourcePrefix) {
			t.Fatalf("%s recording resource %q does not use prefix %q", wantLane, observation.Resource, artifact.Safety.ResourcePrefix)
		}
		assertCloudflareParityRecordingSanitized(t, wantLane, observation)
	}
	want := compareCloudflareParityObservations(recording.Observations, recording.Comparison.Fields)
	if !reflect.DeepEqual(recording.Comparison, want) {
		t.Fatalf("%s recording comparison = %#v, want %#v", wantLane, recording.Comparison, want)
	}
	if !recording.Comparison.Matched {
		t.Fatalf("%s recording comparison did not match", wantLane)
	}
	for field, wantValue := range cloudflareParityRequiredFieldValues[wantLane] {
		for _, observation := range recording.Observations {
			if observation.Fields[field] != wantValue {
				t.Fatalf("%s recording runtime %q field %q = %#v, want %#v", wantLane, observation.Runtime, field, observation.Fields[field], wantValue)
			}
		}
	}
}

func assertCloudflareParityRecordingSanitized(t *testing.T, lane string, observation cloudflareParityRuntimeObservation) {
	t.Helper()
	forbidden := []string{"api_token", "authorization", "bearer", "credential", "raw_response", "secret", "token"}
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

func assertCloudflareParitySafetyContract(t *testing.T, lane string, safety cloudflareParitySafety) {
	t.Helper()
	for _, envName := range []string{cloudflareParityTofuEnv, cloudflareParityTerraformEnv, "CLOUDFLARE_ACCOUNT_ID", "CLOUDFLARE_API_TOKEN", "UDON_CREDENTIAL_CLOUDFLARE_API_TOKEN"} {
		if !slices.Contains(safety.RequiredEnv, envName) {
			t.Fatalf("%s required_env = %#v, missing %s", strings.ToUpper(lane), safety.RequiredEnv, envName)
		}
	}
	if !strings.Contains(safety.CleanupFallback, "Cloudflare API") {
		t.Fatalf("%s cleanup fallback = %q, want Cloudflare API command", strings.ToUpper(lane), safety.CleanupFallback)
	}
	switch lane {
	case "c01", "c03":
		if !safety.LiveEnabled {
			t.Fatalf("%s must be marked live-enabled for opt-in R2 parity", strings.ToUpper(lane))
		}
		for _, guardrail := range []string{"one disposable R2 bucket per runtime", "no objects public access custom domains or workers", "delete bucket before recording"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("%s cost guardrails = %#v, missing %q", strings.ToUpper(lane), safety.CostGuardrails, guardrail)
			}
		}
	case "c02":
		if !safety.LiveEnabled {
			t.Fatalf("C02 must be marked live-enabled for opt-in R2 read-missing parity")
		}
		for _, guardrail := range []string{"one disposable R2 bucket per runtime", "out-of-band delete before read-missing", "cleanup absence before recording"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("C02 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	case "c04", "c05", "c06":
		if !safety.LiveEnabled {
			t.Fatalf("%s must be marked live-enabled for opt-in D1 parity", strings.ToUpper(lane))
		}
		for _, guardrail := range []string{"one disposable D1 database per runtime", "delete database before recording", "cleanup absence before recording"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("%s cost guardrails = %#v, missing %q", strings.ToUpper(lane), safety.CostGuardrails, guardrail)
			}
		}
		if lane == "c06" && !slices.Contains(safety.CostGuardrails, "read-replication mode update only") {
			t.Fatalf("C06 cost guardrails = %#v, missing read-replication update guardrail", safety.CostGuardrails)
		}
	default:
		t.Fatalf("no safety assertions registered for Cloudflare parity lane %s", lane)
	}
}

func assertCloudflareParityRecordingPolicy(t *testing.T, lane string, recording cloudflareParityRecording) {
	t.Helper()
	if strings.TrimSpace(recording.Status) == "" {
		t.Fatalf("%s recording policy status is required", strings.ToUpper(lane))
	}
	if recording.UpdateEnv != "" && recording.UpdateEnv != cloudflareParityRecordEnv {
		t.Fatalf("%s recording update env = %q, want %q", strings.ToUpper(lane), recording.UpdateEnv, cloudflareParityRecordEnv)
	}
	wantPath := filepath.Join(cloudflareParityFixtureRoot, lane, "live.observations.json")
	if recording.ArtifactPath != wantPath {
		t.Fatalf("%s recording artifact path = %q, want %q", strings.ToUpper(lane), recording.ArtifactPath, wantPath)
	}
	_, statErr := os.Stat(recording.ArtifactPath)
	hasRecording := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		t.Fatalf("%s recording artifact stat failed for %s: %v", strings.ToUpper(lane), recording.ArtifactPath, statErr)
	}
	switch lane {
	case "c01", "c02", "c03", "c04", "c05", "c06":
		if hasRecording {
			if recording.Status != "recorded" || recording.CompareWithoutUpdate || strings.TrimSpace(recording.Decision) == "" {
				t.Fatalf("%s recording policy = %#v, want recorded policy for committed live observations", strings.ToUpper(lane), recording)
			}
			return
		}
		if recording.Status != "deferred" && recording.Status != "pending-live" {
			t.Fatalf("%s recording policy = %#v, want deferred or pending-live policy", strings.ToUpper(lane), recording)
		}
		if strings.TrimSpace(recording.Decision) == "" {
			t.Fatalf("%s recording policy decision is required: %#v", strings.ToUpper(lane), recording)
		}
	default:
		t.Fatalf("no recording policy assertions registered for Cloudflare parity lane %s", lane)
	}
}

func assertCloudflareParityPromotionPolicy(t *testing.T, lane string, promotion cloudflareParityPromotion) {
	t.Helper()
	if strings.TrimSpace(promotion.Next) == "" {
		t.Fatalf("%s promotion next step is required", strings.ToUpper(lane))
	}
	switch lane {
	case "c01", "c02", "c03", "c04", "c05", "c06":
		if !promotion.LiveCandidate || !slices.Contains(promotion.Preconditions, "scoped Cloudflare account token") {
			t.Fatalf("%s promotion policy = %#v, want live candidate with scoped account token", strings.ToUpper(lane), promotion)
		}
	default:
		t.Fatalf("no promotion assertions registered for Cloudflare parity lane %s", lane)
	}
}

func assertCloudflareParityStaticFixtures(t *testing.T, lane string, artifact cloudflareParityArtifact) {
	t.Helper()
	assertCloudflareParityHCLFixture(t, lane, artifact)
	assertCloudflareParityNativeProjectFixture(t, lane)
	switch lane {
	case "c01", "c03":
		assertCloudflareParityPlanFixture(t, lane, "create", "r2-create-bucket", "create", false)
		assertCloudflareParityPlanFixture(t, lane, "read", "r2-get-bucket", "read", false)
		assertCloudflareParityPlanFixture(t, lane, "update", "r2-patch-bucket", "update", true)
		assertCloudflareParityPlanFixture(t, lane, "delete", "r2-delete-bucket", "delete", true)
		assertCloudflareParityRequestBindings(t, lane, map[string][]string{
			"create": {"account_id", "name", "locationHint", "storageClass"},
			"read":   {"account_id", "bucket_name"},
			"update": {"account_id", "bucket_name", "cf-r2-storage-class"},
			"delete": {"account_id", "bucket_name"},
		})
		assertCloudflareParityResponseBindings(t, lane, []string{"result.name", "result.location", "result.storageClass", "result.jurisdiction"})
	case "c02":
		assertCloudflareParityPlanFixture(t, lane, "create", "r2-create-bucket", "create", false)
		assertCloudflareParityPlanFixture(t, lane, "read", "r2-get-bucket", "read", false)
		assertCloudflareParityPlanFixture(t, lane, "delete", "r2-delete-bucket", "delete", true)
		assertCloudflareParityRequestBindings(t, lane, map[string][]string{
			"create": {"account_id", "name"},
			"read":   {"account_id", "bucket_name"},
			"delete": {"account_id", "bucket_name"},
		})
		assertCloudflareParityResponseBindings(t, lane, []string{"result.name"})
	case "c04":
		assertCloudflareParityPlanFixture(t, lane, "create", "d1-create-database", "create", false)
		assertCloudflareParityPlanFixture(t, lane, "read", "d1-get-database", "read", false)
		assertCloudflareParityRequestBindings(t, lane, map[string][]string{
			"create": {"account_id", "name"},
			"read":   {"account_id", "database_id"},
		})
		assertCloudflareParityResponseBindings(t, lane, []string{"result.name", "result.uuid"})
		assertCloudflareParityD1UpdateNotAdvertised(t, lane, artifact)
	case "c05":
		assertCloudflareParityPlanFixture(t, lane, "create", "d1-create-database", "create", false)
		assertCloudflareParityPlanFixture(t, lane, "read", "d1-get-database", "read", false)
		assertCloudflareParityPlanFixture(t, lane, "delete", "d1-delete-database", "delete", true)
		assertCloudflareParityRequestBindings(t, lane, map[string][]string{
			"create": {"account_id", "name"},
			"read":   {"account_id", "database_id"},
			"delete": {"account_id", "database_id"},
		})
		assertCloudflareParityResponseBindings(t, lane, []string{"result.name", "result.uuid"})
		assertCloudflareParityD1UpdateNotAdvertised(t, lane, artifact)
	case "c06":
		assertCloudflareParityPlanFixture(t, lane, "create", "d1-create-database", "create", false)
		assertCloudflareParityPlanFixture(t, lane, "read", "d1-get-database", "read", false)
		assertCloudflareParityPlanFixture(t, lane, "update", "d1-update-database", "update", true)
		assertCloudflareParityPlanFixture(t, lane, "delete", "d1-delete-database", "delete", true)
		assertCloudflareParityRequestBindings(t, lane, map[string][]string{
			"create": {"account_id", "name", "read_replication"},
			"read":   {"account_id", "database_id"},
			"update": {"account_id", "database_id", "read_replication"},
			"delete": {"account_id", "database_id"},
		})
		assertCloudflareParityResponseBindings(t, lane, []string{"result.name", "result.uuid", "result.read_replication.mode"})
	default:
		t.Fatalf("no static assertions registered for Cloudflare parity lane %s", lane)
	}
	assertCloudflareParityUdonMetadata(t, lane)
}

func assertCloudflareParityD1UpdateNotAdvertised(t *testing.T, lane string, artifact cloudflareParityArtifact) {
	t.Helper()
	data, err := os.ReadFile(artifact.OpenAPI.SourcePath)
	if err != nil {
		t.Fatalf("read %s OpenAPI source %s: %v", strings.ToUpper(lane), artifact.OpenAPI.SourcePath, err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode %s OpenAPI source %s: %v", strings.ToUpper(lane), artifact.OpenAPI.SourcePath, err)
	}
	for path, methods := range doc.Paths {
		if !strings.Contains(strings.ToLower(path), "/d1/database") {
			continue
		}
		for method, operation := range methods {
			method = strings.ToUpper(method)
			if method == "PATCH" || method == "PUT" || strings.Contains(strings.ToLower(operation.OperationID), "update") {
				t.Fatalf("%s D1 update remains unclaimed, but %s %s advertises operation %q", strings.ToUpper(lane), method, path, operation.OperationID)
			}
		}
	}
}

func assertCloudflareParityHCLFixture(t *testing.T, lane string, artifact cloudflareParityArtifact) {
	t.Helper()
	doc, err := tfconfig.LoadDir(filepath.Join(cloudflareParityFixtureRoot, lane, "hcl"))
	if err != nil {
		t.Fatalf("load %s HCL fixture: %v", strings.ToUpper(lane), err)
	}
	if len(doc.Diagnostics) != 0 {
		t.Fatalf("%s HCL fixture diagnostics: %#v", strings.ToUpper(lane), doc.Diagnostics)
	}
	if len(doc.Modules) != 1 || len(doc.Modules[0].Resources) != 1 {
		t.Fatalf("%s HCL fixture shape: modules=%d resources=%d", strings.ToUpper(lane), len(doc.Modules), len(doc.Modules[0].Resources))
	}
	wantType := artifact.Scenarios[0].TerraformResourceType
	if doc.Modules[0].Resources[0].Type != wantType {
		t.Fatalf("%s HCL resource type = %q, want %q", strings.ToUpper(lane), doc.Modules[0].Resources[0].Type, wantType)
	}
	var foundProvider bool
	for _, req := range doc.Modules[0].RequiredProviders {
		if req.LocalName == "cloudflare" && req.Source == "cloudflare/cloudflare" && slices.Contains(req.VersionConstraints, artifact.Provider.VersionConstraint) {
			foundProvider = true
		}
	}
	if !foundProvider {
		t.Fatalf("%s HCL required providers = %#v, want cloudflare/cloudflare %s", strings.ToUpper(lane), doc.Modules[0].RequiredProviders, artifact.Provider.VersionConstraint)
	}
}

func assertCloudflareParityNativeProjectFixture(t *testing.T, lane string) {
	assertAPIParityNativeProjectFixture(t, "Cloudflare", cloudflareParityFixtureRoot, lane)
}

func assertCloudflareParityPlanFixture(t *testing.T, lane, action, operationID, summaryField string, seedState bool) {
	var seed func(*testing.T, string, string)
	if seedState {
		seed = seedCloudflareParityState
	}
	assertAPIParityPlanFixture(t, "Cloudflare", cloudflareParityFixtureRoot, lane, action, operationID, summaryField, seed)
}

func seedCloudflareParityState(t *testing.T, lane, statePath string) {
	t.Helper()
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open %s seed state: %v", strings.ToUpper(lane), err)
	}
	defer store.Close()
	snapshot, err := cloudflareParitySeedSnapshot(lane)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordResource(context.Background(), snapshot); err != nil {
		t.Fatalf("record %s seed state: %v", strings.ToUpper(lane), err)
	}
}

func cloudflareParitySeedSnapshot(lane string) (state.ResourceSnapshot, error) {
	switch lane {
	case "c01", "c02", "c03":
		return state.ResourceSnapshot{
			Address:        "cloudflare_r2_bucket.bucket",
			Type:           "cloudflare_r2_bucket",
			Provider:       "provider.cloudflare",
			DesiredHash:    "sha256:previous",
			IdentityJSON:   `{"account_id":"cloudflare-account-placeholder","bucket_name":"ramen-parity-` + lane + `-static"}`,
			AttributesJSON: `{"account_id":"cloudflare-account-placeholder","name":"ramen-parity-` + lane + `-static","location":"ENAM","storage_class":"Standard","jurisdiction":"default"}`,
			Status:         "managed",
		}, nil
	case "c04", "c05":
		return state.ResourceSnapshot{
			Address:        "cloudflare_d1_database.database",
			Type:           "cloudflare_d1_database",
			Provider:       "provider.cloudflare",
			DesiredHash:    "sha256:previous",
			IdentityJSON:   `{"account_id":"cloudflare-account-placeholder","database_id":"00000000-0000-0000-0000-000000000000"}`,
			AttributesJSON: `{"account_id":"cloudflare-account-placeholder","name":"ramen-parity-` + lane + `-static","database_id":"00000000-0000-0000-0000-000000000000"}`,
			Status:         "managed",
		}, nil
	case "c06":
		return state.ResourceSnapshot{
			Address:        "cloudflare_d1_database.database",
			Type:           "cloudflare_d1_database",
			Provider:       "provider.cloudflare",
			DesiredHash:    "sha256:previous",
			IdentityJSON:   `{"account_id":"cloudflare-account-placeholder","database_id":"00000000-0000-0000-0000-000000000000"}`,
			AttributesJSON: `{"account_id":"cloudflare-account-placeholder","name":"ramen-parity-c06-static","database_id":"00000000-0000-0000-0000-000000000000","read_replication":{"mode":"disabled"}}`,
			Status:         "managed",
		}, nil
	default:
		return state.ResourceSnapshot{}, fmt.Errorf("no seed state registered for Cloudflare parity lane %s", lane)
	}
}

func cloudflareParitySummaryHasOne(summary tfplan.Summary, field string) bool {
	return apiParitySummaryHasOne(summary, field)
}

func assertCloudflareParityRequestBindings(t *testing.T, lane string, expected map[string][]string) {
	assertAPIParityRequestBindings(t, "Cloudflare", cloudflareParityFixtureRoot, lane, "create", expected)
}

func assertCloudflareParityResponseBindings(t *testing.T, lane string, expected []string) {
	assertAPIParityResponseBindings(t, "Cloudflare", cloudflareParityFixtureRoot, lane, expected)
}

func assertCloudflareParityUdonMetadata(t *testing.T, lane string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(cloudflareParityFixtureRoot, lane, "ramen", "project.uws.yaml"))
	if err != nil {
		t.Fatalf("read %s native fixture: %v", strings.ToUpper(lane), err)
	}
	text := string(data)
	for _, want := range []string{"x-udon-config", "serverUrl: https://api.cloudflare.com/client/v4", "binding: cloudflare_api_token", "scheme: bearer"} {
		if !strings.Contains(text, want) {
			t.Fatalf("%s native fixture missing %q", strings.ToUpper(lane), want)
		}
	}
}

func compareCloudflareParityObservations(observations []cloudflareParityRuntimeObservation, fields []string) cloudflareParityObservationComparison {
	comparison := compareAPIParityObservationFields(observations, fields)
	for _, observation := range observations {
		// Mirror compareGoogleParityObservations: cross-runtime agreement alone
		// is satisfied by a uniformly-wrong recording (e.g. every runtime failed
		// to create), so require the resource to actually exist after create.
		if observation.Fields["after_create.exists"] != true {
			comparison.Matched = false
		}
	}
	return comparison
}

func cloudflareParityFailure(runtime, class string, err error) cloudflareParityRuntimeResult {
	return apiParityFailure("Cloudflare", runtime, class, err)
}

func compareOrUpdateCloudflareParityRecording(t *testing.T, recording cloudflareParityLiveRecording, path string) {
	compareOrUpdateAPIParityRecording(t, "Cloudflare", cloudflareParityEnv, cloudflareParityRecordEnv, path, recording, true, normalizeCloudflareParityRecording)
}

func normalizeCloudflareParityRecording(t *testing.T, data []byte) cloudflareParityLiveRecording {
	return normalizeAPIParityRecording(t, "Cloudflare", data, func(lane string, observation *apiParityRuntimeObservation) {
		observation.Resource = normalizeCloudflareParityGeneratedResource(lane, observation.Runtime, observation.Resource)
		for key, value := range observation.Fields {
			if valueString, ok := value.(string); ok {
				observation.Fields[key] = normalizeCloudflareParityGeneratedResource(lane, observation.Runtime, valueString)
			}
		}
	})
}

func normalizeCloudflareParityGeneratedResource(lane, runtime, value string) string {
	lane = strings.ToLower(strings.TrimSpace(lane))
	runtime = strings.ToLower(strings.TrimSpace(runtime))
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
