package corpus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/state"
	"github.com/OpenUdon/tfconfig"
)

const (
	githubParityEnv         = "RAMEN_GITHUB_PARITY"
	githubParityRecordEnv   = "RAMEN_GITHUB_PARITY_RECORD_UPDATE"
	githubParityLaneEnv     = "RAMEN_GITHUB_PARITY_LANE"
	githubParityTofuEnv     = "RAMEN_GITHUB_TOFU"
	githubParityArtifactV1  = "ramen.github.provider-parity.v1"
	githubParityFixtureRoot = "testdata/parity/github"
)

var githubParityLanes = []string{"h01", "h02", "h03"}
var githubParityLiveRunnerLanes = []string{"h01", "h02", "h03"}

type githubParityArtifact struct {
	Version   string                 `json:"version"`
	Lane      string                 `json:"lane"`
	Status    string                 `json:"status"`
	Provider  githubParityProvider   `json:"provider"`
	OpenAPI   githubParityOpenAPI    `json:"openapi"`
	Safety    githubParitySafety     `json:"safety"`
	Recording githubParityRecording  `json:"recording,omitempty"`
	Promotion githubParityPromotion  `json:"promotion,omitempty"`
	Runtimes  []string               `json:"runtimes"`
	Scenarios []githubParityScenario `json:"scenarios"`
	Notes     []string               `json:"notes,omitempty"`
}

type githubParityProvider struct {
	Source            string `json:"source"`
	VersionConstraint string `json:"version_constraint,omitempty"`
}

type githubParityOpenAPI struct {
	SourcePath string `json:"source_path"`
	Fixture    string `json:"fixture"`
}

type githubParitySafety struct {
	LiveEnv              string   `json:"live_env"`
	RecordUpdateEnv      string   `json:"record_update_env"`
	LaneEnv              string   `json:"lane_env"`
	OpenTofuEnv          string   `json:"opentofu_env,omitempty"`
	TerraformEnv         string   `json:"terraform_env,omitempty"`
	OwnerEnv             string   `json:"owner_env,omitempty"`
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

type githubParityRecording struct {
	Status               string `json:"status,omitempty"`
	ArtifactPath         string `json:"artifact_path,omitempty"`
	UpdateEnv            string `json:"update_env,omitempty"`
	CompareWithoutUpdate bool   `json:"compare_without_update,omitempty"`
	Decision             string `json:"decision,omitempty"`
}

type githubParityPromotion struct {
	Next          string   `json:"next,omitempty"`
	LiveCandidate bool     `json:"live_candidate,omitempty"`
	Preconditions []string `json:"preconditions,omitempty"`
}

type githubParityScenario = apiParityScenario
type githubParityLiveRecording = apiParityLiveRecording
type githubParityRuntimeObservation = apiParityRuntimeObservation
type githubParityObservationComparison = apiParityObservationComparison
type githubParityRuntimeFailure = apiParityRuntimeFailure
type githubParityRuntimeResult = apiParityRuntimeResult

func TestGitHubProviderParityReplayArtifacts(t *testing.T) {
	for _, lane := range githubParityLanes {
		lane := lane
		t.Run(strings.ToUpper(lane), func(t *testing.T) {
			artifact := loadGitHubParityArtifact(t, filepath.Join(githubParityFixtureRoot, lane, "observations.json"))
			assertGitHubParityArtifact(t, lane, artifact)
			assertGitHubParityStaticFixtures(t, lane, artifact)
		})
	}
}

func loadGitHubParityArtifact(t *testing.T, path string) githubParityArtifact {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var artifact githubParityArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return artifact
}

func assertGitHubParityArtifact(t *testing.T, lane string, artifact githubParityArtifact) {
	t.Helper()
	wantLane := strings.ToUpper(lane)
	if artifact.Version != githubParityArtifactV1 {
		t.Fatalf("artifact version = %q, want %q", artifact.Version, githubParityArtifactV1)
	}
	if artifact.Lane != wantLane {
		t.Fatalf("artifact lane = %s, want %s", artifact.Lane, wantLane)
	}
	if artifact.Status != "planned" && artifact.Status != "recorded" {
		t.Fatalf("artifact status = %s, want planned or recorded", artifact.Status)
	}
	if artifact.Provider.Source != "integrations/github" || strings.TrimSpace(artifact.Provider.VersionConstraint) == "" {
		t.Fatalf("%s provider metadata invalid: %#v", wantLane, artifact.Provider)
	}
	if _, err := os.Stat(artifact.OpenAPI.SourcePath); err != nil {
		t.Fatalf("%s OpenAPI source path %s is not readable: %v", wantLane, artifact.OpenAPI.SourcePath, err)
	}
	if artifact.Safety.LiveEnv != githubParityEnv || artifact.Safety.RecordUpdateEnv != githubParityRecordEnv || artifact.Safety.LaneEnv != githubParityLaneEnv {
		t.Fatalf("%s live gates invalid: %#v", wantLane, artifact.Safety)
	}
	if artifact.Safety.OpenTofuEnv != githubParityTofuEnv || artifact.Safety.OwnerEnv != "GITHUB_OWNER" {
		t.Fatalf("%s runtime env metadata invalid: %#v", wantLane, artifact.Safety)
	}
	if artifact.Safety.TerraformEnv != "" {
		t.Fatalf("%s GitHub live parity uses OpenTofu plus Ramen only; terraform_env must be empty: %#v", wantLane, artifact.Safety)
	}
	if artifact.Safety.CredentialBinding != "github_token" || artifact.Safety.CredentialEnv != "UDON_CREDENTIAL_GITHUB_TOKEN" {
		t.Fatalf("%s credential metadata invalid: %#v", wantLane, artifact.Safety)
	}
	if !strings.HasPrefix(artifact.Safety.ResourcePrefix, "ramen-parity-"+lane+"-") || !artifact.Safety.RequiresExplicitLane {
		t.Fatalf("%s resource prefix/lane guard invalid: %#v", wantLane, artifact.Safety)
	}
	if artifact.Safety.LiveEnabled && !slices.Contains(githubParityLiveRunnerLanes, lane) {
		t.Fatalf("%s is live-enabled but has no registered GitHub live runner", wantLane)
	}
	if !artifact.Safety.LiveEnabled && slices.Contains(githubParityLiveRunnerLanes, lane) {
		t.Fatalf("%s has a registered GitHub live runner but is not live-enabled", wantLane)
	}
	for _, envName := range []string{githubParityTofuEnv, "GITHUB_OWNER", "GITHUB_TOKEN", "UDON_CREDENTIAL_GITHUB_TOKEN"} {
		if !slices.Contains(artifact.Safety.RequiredEnv, envName) {
			t.Fatalf("%s required_env = %#v, missing %s", wantLane, artifact.Safety.RequiredEnv, envName)
		}
	}
	if slices.Contains(artifact.Safety.RequiredEnv, "RAMEN_GITHUB_TERRAFORM") {
		t.Fatalf("%s required_env must not require Terraform for GitHub parity: %#v", wantLane, artifact.Safety.RequiredEnv)
	}
	for _, runtime := range []string{"opentofu", "ramen"} {
		if !slices.Contains(artifact.Runtimes, runtime) {
			t.Fatalf("%s runtimes %v missing %s", wantLane, artifact.Runtimes, runtime)
		}
	}
	if slices.Contains(artifact.Runtimes, "terraform") {
		t.Fatalf("%s runtimes must not include Terraform for GitHub parity: %#v", wantLane, artifact.Runtimes)
	}
	if artifact.Recording.Status != "deferred" && artifact.Recording.Status != "recorded" {
		t.Fatalf("%s recording status invalid: %#v", wantLane, artifact.Recording)
	}
	if artifact.Recording.ArtifactPath != filepath.Join(githubParityFixtureRoot, lane, "live.observations.json") {
		t.Fatalf("%s recording artifact path invalid: %#v", wantLane, artifact.Recording)
	}
	if artifact.Recording.Status == "deferred" && !artifact.Recording.CompareWithoutUpdate {
		t.Fatalf("%s recording policy invalid: %#v", wantLane, artifact.Recording)
	}
	if artifact.Promotion.LiveCandidate != artifact.Safety.LiveEnabled {
		t.Fatalf("%s live candidate metadata must match live_enabled: %#v", wantLane, artifact.Promotion)
	}
	if artifact.Promotion.LiveCandidate && !slices.Contains(artifact.Promotion.Preconditions, "scoped GitHub organization token") {
		t.Fatalf("%s promotion policy invalid: %#v", wantLane, artifact.Promotion)
	}
	if len(artifact.Scenarios) != 1 {
		t.Fatalf("%s scenarios = %#v, want one", wantLane, artifact.Scenarios)
	}
	scenario := artifact.Scenarios[0]
	if artifact.Status == "planned" && len(scenario.ObservationArtifacts) != 0 {
		t.Fatalf("%s planned scenario must not claim observation_artifacts: %#v", wantLane, scenario)
	}
	if artifact.Status == "recorded" {
		if artifact.Recording.Status != "recorded" {
			t.Fatalf("%s recorded artifact has recording status %#v", wantLane, artifact.Recording)
		}
		if !slices.Contains(scenario.ObservationArtifacts, artifact.Recording.ArtifactPath) {
			t.Fatalf("%s recorded scenario does not reference %s: %#v", wantLane, artifact.Recording.ArtifactPath, scenario)
		}
		assertGitHubParityRecordedLiveArtifact(t, lane, artifact, scenario)
	}
	for _, path := range scenario.FixturePaths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s fixture %s is not readable: %v", wantLane, path, err)
		}
	}
}

func assertGitHubParityRecordedLiveArtifact(t *testing.T, lane string, artifact githubParityArtifact, scenario githubParityScenario) {
	t.Helper()
	data, err := os.ReadFile(artifact.Recording.ArtifactPath)
	if err != nil {
		t.Fatalf("%s read live recording %s: %v", strings.ToUpper(lane), artifact.Recording.ArtifactPath, err)
	}
	assertGitHubParityRecordingSanitizedBytes(t, strings.ToUpper(lane), data)
	var recording githubParityLiveRecording
	if err := json.Unmarshal(data, &recording); err != nil {
		t.Fatalf("%s decode live recording %s: %v", strings.ToUpper(lane), artifact.Recording.ArtifactPath, err)
	}
	if recording.Version != githubParityArtifactV1 || recording.Lane != strings.ToUpper(lane) || recording.Scenario != scenario.Name {
		t.Fatalf("%s live recording metadata invalid: %#v", strings.ToUpper(lane), recording)
	}
	if !recording.Comparison.Matched {
		t.Fatalf("%s live recording comparison did not match: %#v", strings.ToUpper(lane), recording.Comparison)
	}
	for _, field := range recording.Comparison.Fields {
		if !slices.Contains(scenario.ObservedFields, field) {
			t.Fatalf("%s live recording comparison field %q missing from scenario observed fields %#v", strings.ToUpper(lane), field, scenario.ObservedFields)
		}
	}
	if len(recording.Observations) != len(artifact.Runtimes) {
		t.Fatalf("%s live observations = %d, want %d runtimes", strings.ToUpper(lane), len(recording.Observations), len(artifact.Runtimes))
	}
	seen := map[string]bool{}
	for _, observation := range recording.Observations {
		if !slices.Contains(artifact.Runtimes, observation.Runtime) {
			t.Fatalf("%s live recording runtime %q not declared in %#v", strings.ToUpper(lane), observation.Runtime, artifact.Runtimes)
		}
		if seen[observation.Runtime] {
			t.Fatalf("%s live recording has duplicate runtime %q", strings.ToUpper(lane), observation.Runtime)
		}
		seen[observation.Runtime] = true
		assertGitHubParityLiveObservationSemantics(t, lane, observation)
		assertGitHubParityObservationSanitized(t, strings.ToUpper(lane), observation)
	}
}

func assertGitHubParityRecordingSanitizedBytes(t *testing.T, lane string, data []byte) {
	t.Helper()
	normalized := strings.ToLower(string(data))
	for _, bad := range githubParityForbiddenRecordingSubstrings() {
		if strings.Contains(normalized, bad) {
			t.Fatalf("%s recording contains forbidden material %q", lane, bad)
		}
	}
}

func compareGitHubParityObservations(observations []githubParityRuntimeObservation, fields []string) githubParityObservationComparison {
	return compareAPIParityObservationFields(observations, fields)
}

func assertGitHubParityLiveObservationSemantics(t *testing.T, lane string, observation githubParityRuntimeObservation) {
	t.Helper()
	wantFields := map[string]any{}
	switch strings.ToLower(strings.TrimSpace(lane)) {
	case "h01":
		wantFields = map[string]any{
			"after_create.exists":      true,
			"after_create.visibility":  "private",
			"after_update.description": "Ramen GitHub H01 parity fixture updated",
			"no_op":                    true,
			"after_delete.exists":      false,
		}
	case "h02":
		wantFields = map[string]any{
			"after_create.exists":      true,
			"after_create.color":       "0e8a16",
			"after_update.description": "Ramen GitHub H02 parity fixture updated",
			"no_op":                    true,
			"after_delete.exists":      false,
			"after_cleanup.exists":     false,
		}
	case "h03":
		wantFields = map[string]any{
			"after_create.exists":      true,
			"after_create.path":        "ramen-parity-h03.txt",
			"after_update.exists":      true,
			"after_update.sha_changed": true,
			"no_op":                    true,
			"after_delete.exists":      false,
			"after_cleanup.exists":     false,
		}
	default:
		t.Fatalf("no GitHub live semantic assertions registered for lane %s", lane)
	}
	for field, want := range wantFields {
		if got := observation.Fields[field]; got != want {
			t.Fatalf("%s %s live field %s = %#v, want %#v", strings.ToUpper(lane), observation.Runtime, field, got, want)
		}
	}
}

func assertGitHubParityObservationSanitized(t *testing.T, lane string, observation githubParityRuntimeObservation) {
	t.Helper()
	values := []string{observation.Resource}
	for key, value := range observation.Fields {
		values = append(values, key)
		if valueString, ok := value.(string); ok {
			values = append(values, valueString)
		}
	}
	for _, value := range values {
		normalized := strings.ToLower(value)
		for _, bad := range githubParityForbiddenRecordingSubstrings() {
			if strings.Contains(normalized, bad) {
				t.Fatalf("%s recording contains forbidden material %q in %q", lane, bad, value)
			}
		}
	}
}

func githubParityForbiddenRecordingSubstrings() []string {
	return []string{"authorization", "bearer", "credential", "raw_response", "secret", "token", "ramen h03 create", "ramen h03 update"}
}

func githubParityFailure(runtime, class string, err error) githubParityRuntimeResult {
	return apiParityFailure("GitHub", runtime, class, err)
}

func compareOrUpdateGitHubParityRecording(t *testing.T, recording githubParityLiveRecording, path string) {
	compareOrUpdateAPIParityRecording(t, "GitHub", githubParityEnv, githubParityRecordEnv, path, recording, true, normalizeGitHubParityRecording)
}

func normalizeGitHubParityRecording(t *testing.T, data []byte) githubParityLiveRecording {
	return normalizeAPIParityRecording(t, "GitHub", data, func(lane string, observation *apiParityRuntimeObservation) {
		observation.Resource = normalizeGitHubParityGeneratedResource(lane, observation.Runtime, observation.Resource)
		for key, value := range observation.Fields {
			if valueString, ok := value.(string); ok {
				observation.Fields[key] = normalizeGitHubParityGeneratedResource(lane, observation.Runtime, valueString)
			}
		}
	})
}

func normalizeGitHubParityGeneratedResource(lane, runtime, value string) string {
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

func assertGitHubParityStaticFixtures(t *testing.T, lane string, artifact githubParityArtifact) {
	t.Helper()
	assertGitHubParityHCLFixture(t, lane, artifact)
	assertGitHubParityNativeProjectFixture(t, lane)
	switch lane {
	case "h01":
		assertGitHubParityPlanFixture(t, lane, "create", "repos/create-in-org", "create", false)
		assertGitHubParityPlanFixture(t, lane, "read", "repos/get", "read", false)
		assertGitHubParityPlanFixture(t, lane, "update", "repos/update", "update", true)
		assertGitHubParityPlanFixture(t, lane, "delete", "repos/delete", "delete", true)
		assertGitHubParityRequestBindings(t, lane, map[string][]string{
			"create": {"org", "name", "description", "visibility", "has_issues", "auto_init"},
			"read":   {"owner", "repo"},
			"update": {"owner", "repo", "description", "visibility", "has_issues"},
			"delete": {"owner", "repo"},
		})
		assertGitHubParityResponseBindings(t, lane, []string{"name", "full_name", "visibility", "default_branch"})
	case "h02":
		assertGitHubParityPlanFixture(t, lane, "create", "issues/create-label", "create", false)
		assertGitHubParityPlanFixture(t, lane, "read", "issues/get-label", "read", false)
		assertGitHubParityPlanFixture(t, lane, "update", "issues/update-label", "update", true)
		assertGitHubParityPlanFixture(t, lane, "delete", "issues/delete-label", "delete", true)
		assertGitHubParityRequestBindings(t, lane, map[string][]string{
			"create": {"owner", "repo", "name", "color", "description"},
			"read":   {"owner", "repo", "name"},
			"update": {"owner", "repo", "name", "color", "description"},
			"delete": {"owner", "repo", "name"},
		})
		assertGitHubParityResponseBindings(t, lane, []string{"name", "color", "description"})
	case "h03":
		assertGitHubParityPlanFixture(t, lane, "create", "repos/create-or-update-file-contents", "create", false)
		assertGitHubParityPlanFixture(t, lane, "read", "repos/get-content", "read", false)
		assertGitHubParityPlanFixture(t, lane, "update", "repos/create-or-update-file-contents", "update", true)
		assertGitHubParityPlanFixture(t, lane, "delete", "repos/delete-file", "delete", true)
		assertGitHubParityRequestBindings(t, lane, map[string][]string{
			"create": {"owner", "repo", "path", "message", "content", "branch"},
			"read":   {"owner", "repo", "path"},
			"update": {"owner", "repo", "path", "message", "content", "branch", "sha"},
			"delete": {"owner", "repo", "path", "message", "branch", "sha"},
		})
		assertGitHubParityResponseBindings(t, lane, []string{"path", "sha"})
		assertGitHubParityBase64Binding(t, lane)
	default:
		t.Fatalf("no static assertions registered for GitHub parity lane %s", lane)
	}
	assertGitHubParityUdonMetadata(t, lane)
}

func assertGitHubParityHCLFixture(t *testing.T, lane string, artifact githubParityArtifact) {
	t.Helper()
	doc, err := tfconfig.LoadDir(filepath.Join(githubParityFixtureRoot, lane, "hcl"))
	if err != nil {
		t.Fatalf("load %s HCL fixture: %v", strings.ToUpper(lane), err)
	}
	if len(doc.Diagnostics) != 0 {
		t.Fatalf("%s HCL fixture diagnostics: %#v", strings.ToUpper(lane), doc.Diagnostics)
	}
	if len(doc.Modules) != 1 || len(doc.Modules[0].Resources) != 1 {
		t.Fatalf("%s HCL fixture shape: modules=%d resources=%d", strings.ToUpper(lane), len(doc.Modules), len(doc.Modules[0].Resources))
	}
	if got, want := doc.Modules[0].Resources[0].Type, artifact.Scenarios[0].TerraformResourceType; got != want {
		t.Fatalf("%s HCL resource type = %q, want %q", strings.ToUpper(lane), got, want)
	}
	var foundProvider bool
	for _, req := range doc.Modules[0].RequiredProviders {
		if req.LocalName == "github" && req.Source == "integrations/github" && slices.Contains(req.VersionConstraints, artifact.Provider.VersionConstraint) {
			foundProvider = true
		}
	}
	if !foundProvider {
		t.Fatalf("%s HCL required providers = %#v, want integrations/github %s", strings.ToUpper(lane), doc.Modules[0].RequiredProviders, artifact.Provider.VersionConstraint)
	}
}

func assertGitHubParityNativeProjectFixture(t *testing.T, lane string) {
	assertAPIParityNativeProjectFixture(t, "GitHub", githubParityFixtureRoot, lane)
}

func assertGitHubParityPlanFixture(t *testing.T, lane, action, operationID, summaryField string, seedState bool) {
	var seed func(*testing.T, string, string)
	if seedState {
		seed = seedGitHubParityState
	}
	assertAPIParityPlanFixture(t, "GitHub", githubParityFixtureRoot, lane, action, operationID, summaryField, seed)
}

func seedGitHubParityState(t *testing.T, lane, statePath string) {
	t.Helper()
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open %s seed state: %v", strings.ToUpper(lane), err)
	}
	defer store.Close()
	snapshot, err := githubParitySeedSnapshot(lane)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordResource(context.Background(), snapshot); err != nil {
		t.Fatalf("record %s seed state: %v", strings.ToUpper(lane), err)
	}
}

func githubParitySeedSnapshot(lane string) (state.ResourceSnapshot, error) {
	switch lane {
	case "h01":
		return state.ResourceSnapshot{
			Address:        "github_repository.repo",
			Type:           "github_repository",
			Provider:       "provider.github",
			DesiredHash:    "sha256:previous",
			IdentityJSON:   `{"owner":"github-owner-placeholder","name":"ramen-parity-h01-static"}`,
			AttributesJSON: `{"owner":"github-owner-placeholder","name":"ramen-parity-h01-static","description":"previous","visibility":"private","has_issues":true,"auto_init":true}`,
			Status:         "managed",
		}, nil
	case "h02":
		return state.ResourceSnapshot{
			Address:        "github_issue_label.label",
			Type:           "github_issue_label",
			Provider:       "provider.github",
			DesiredHash:    "sha256:previous",
			IdentityJSON:   `{"owner":"github-owner-placeholder","repository":"ramen-parity-h02-static","name":"ramen-parity-h02"}`,
			AttributesJSON: `{"owner":"github-owner-placeholder","repository":"ramen-parity-h02-static","name":"ramen-parity-h02","color":"b60205","description":"previous"}`,
			Status:         "managed",
		}, nil
	case "h03":
		return state.ResourceSnapshot{
			Address:        "github_repository_file.file",
			Type:           "github_repository_file",
			Provider:       "provider.github",
			DesiredHash:    "sha256:previous",
			IdentityJSON:   `{"owner":"github-owner-placeholder","repository":"ramen-parity-h03-static","file":"ramen-parity-h03.txt","sha":"0000000000000000000000000000000000000000"}`,
			AttributesJSON: `{"owner":"github-owner-placeholder","repository":"ramen-parity-h03-static","file":"ramen-parity-h03.txt","content":"previous","branch":"main","commit_message":"Previous","sha":"0000000000000000000000000000000000000000"}`,
			Status:         "managed",
		}, nil
	default:
		return state.ResourceSnapshot{}, fmt.Errorf("no seed state registered for GitHub parity lane %s", lane)
	}
}

func githubParitySummaryHasOne(summary tfplan.Summary, field string) bool {
	return apiParitySummaryHasOne(summary, field)
}

func assertGitHubParityRequestBindings(t *testing.T, lane string, expected map[string][]string) {
	assertAPIParityRequestBindings(t, "GitHub", githubParityFixtureRoot, lane, "create", expected)
}

func assertGitHubParityResponseBindings(t *testing.T, lane string, expected []string) {
	assertAPIParityResponseBindings(t, "GitHub", githubParityFixtureRoot, lane, expected)
}

func assertGitHubParityBase64Binding(t *testing.T, lane string) {
	t.Helper()
	result, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: filepath.Join(githubParityFixtureRoot, lane, "ramen"),
		StatePath:   filepath.Join(t.TempDir(), "state.db"),
		Action:      "create",
	})
	if err != nil {
		t.Fatalf("build %s Ramen fixture plan for base64 binding: %v", strings.ToUpper(lane), err)
	}
	for _, binding := range result.Plan.Resources[0].Mapping.RequestBindings {
		if binding.Path == "content" && binding.RequestPath == "content" && binding.Encoding == "base64" {
			return
		}
	}
	t.Fatalf("%s missing base64 content binding: %#v", strings.ToUpper(lane), result.Plan.Resources[0].Mapping.RequestBindings)
}

func assertGitHubParityUdonMetadata(t *testing.T, lane string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(githubParityFixtureRoot, lane, "ramen", "project.uws.yaml"))
	if err != nil {
		t.Fatalf("read %s native fixture: %v", strings.ToUpper(lane), err)
	}
	text := string(data)
	for _, want := range []string{"x-udon-config", "serverUrl: https://api.github.com", "binding: github_token", "scheme: bearer"} {
		if !strings.Contains(text, want) {
			t.Fatalf("%s native fixture missing %q", strings.ToUpper(lane), want)
		}
	}
}
