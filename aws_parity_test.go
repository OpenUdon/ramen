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
	awsParityEnv         = "RAMEN_AWS_PARITY"
	awsParityRecordEnv   = "RAMEN_AWS_PARITY_RECORD_UPDATE"
	awsParityLaneEnv     = "RAMEN_AWS_PARITY_LANE"
	awsParityTofuEnv     = "RAMEN_AWS_TOFU"
	awsParityArtifactV1  = "ramen.aws.provider-parity.v1"
	awsParityFixtureRoot = "testdata/parity/aws"
)

var awsParityLanes = []string{"w01", "w02", "w03", "w04"}
var awsParityLiveRunnerLanes = []string{"w01"}

type awsParityArtifact struct {
	Version   string              `json:"version"`
	Lane      string              `json:"lane"`
	Status    string              `json:"status"`
	Provider  awsParityProvider   `json:"provider"`
	Smithy    awsParitySmithy     `json:"smithy"`
	Safety    awsParitySafety     `json:"safety"`
	Recording awsParityRecording  `json:"recording,omitempty"`
	Promotion awsParityPromotion  `json:"promotion,omitempty"`
	Runtimes  []string            `json:"runtimes"`
	Scenarios []awsParityScenario `json:"scenarios"`
	Notes     []string            `json:"notes,omitempty"`
}

type awsParityProvider struct {
	Source            string `json:"source"`
	VersionConstraint string `json:"version_constraint,omitempty"`
}

type awsParitySmithy struct {
	SourcePath string `json:"source_path"`
	Fixture    string `json:"fixture"`
}

type awsParitySafety struct {
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
}

type awsParityRecording struct {
	Status               string `json:"status,omitempty"`
	ArtifactPath         string `json:"artifact_path,omitempty"`
	UpdateEnv            string `json:"update_env,omitempty"`
	CompareWithoutUpdate bool   `json:"compare_without_update,omitempty"`
	Decision             string `json:"decision,omitempty"`
}

type awsParityPromotion struct {
	Next          string   `json:"next,omitempty"`
	LiveCandidate bool     `json:"live_candidate,omitempty"`
	Preconditions []string `json:"preconditions,omitempty"`
	Blockers      []string `json:"blockers,omitempty"`
}

type awsParityScenario = apiParityScenario
type awsParityLiveRecording = apiParityLiveRecording
type awsParityRuntimeObservation = apiParityRuntimeObservation
type awsParityObservationComparison = apiParityObservationComparison
type awsParityRuntimeFailure = apiParityRuntimeFailure
type awsParityRuntimeResult = apiParityRuntimeResult

func TestAWSProviderParityReplayArtifacts(t *testing.T) {
	for _, lane := range awsParityLanes {
		lane := lane
		t.Run(strings.ToUpper(lane), func(t *testing.T) {
			artifact := loadAWSParityArtifact(t, filepath.Join(awsParityFixtureRoot, lane, "observations.json"))
			assertAWSParityArtifact(t, lane, artifact)
			assertAWSParityStaticFixtures(t, lane)
		})
	}
}

func loadAWSParityArtifact(t *testing.T, path string) awsParityArtifact {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var artifact awsParityArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return artifact
}

func assertAWSParityArtifact(t *testing.T, lane string, artifact awsParityArtifact) {
	t.Helper()
	wantLane := strings.ToUpper(lane)
	if artifact.Version != awsParityArtifactV1 {
		t.Fatalf("artifact version = %q, want %q", artifact.Version, awsParityArtifactV1)
	}
	if artifact.Lane != wantLane {
		t.Fatalf("artifact lane = %q, want %q", artifact.Lane, wantLane)
	}
	if artifact.Status != "planned" && artifact.Status != "recorded" {
		t.Fatalf("artifact status = %q, want planned or recorded", artifact.Status)
	}
	if artifact.Provider.Source != "hashicorp/aws" {
		t.Fatalf("provider source = %q, want hashicorp/aws", artifact.Provider.Source)
	}
	if strings.TrimSpace(artifact.Provider.VersionConstraint) == "" {
		t.Fatalf("provider version_constraint is required")
	}
	if _, err := os.Stat(artifact.Smithy.SourcePath); err != nil {
		t.Fatalf("smithy source path %s is not readable: %v", artifact.Smithy.SourcePath, err)
	}
	if artifact.Safety.LiveEnv != awsParityEnv || artifact.Safety.RecordUpdateEnv != awsParityRecordEnv || artifact.Safety.LaneEnv != awsParityLaneEnv {
		t.Fatalf("unexpected AWS live env gates: %#v", artifact.Safety)
	}
	if artifact.Safety.OpenTofuEnv != awsParityTofuEnv {
		t.Fatalf("OpenTofu env = %q, want %q", artifact.Safety.OpenTofuEnv, awsParityTofuEnv)
	}
	if !artifact.Safety.RequiresExplicitLane {
		t.Fatalf("%s must require explicit lane selection", wantLane)
	}
	if artifact.Safety.CredentialBinding != "aws_hmac" {
		t.Fatalf("credential binding = %q, want aws_hmac", artifact.Safety.CredentialBinding)
	}
	if !strings.HasPrefix(artifact.Safety.ResourcePrefix, "ramen-parity-"+lane+"-") {
		t.Fatalf("resource prefix = %q, want ramen-parity-%s-*", artifact.Safety.ResourcePrefix, lane)
	}
	for _, runtime := range []string{"opentofu", "ramen"} {
		if !slices.Contains(artifact.Runtimes, runtime) {
			t.Fatalf("artifact runtimes %v missing %s", artifact.Runtimes, runtime)
		}
	}
	assertAWSParitySafetyContract(t, lane, artifact.Safety)
	if artifact.Safety.LiveEnabled && !slices.Contains(awsParityLiveRunnerLanes, lane) {
		t.Fatalf("%s is marked live-enabled but has no registered AWS live runner", wantLane)
	}
	assertAWSParityRecordingPolicy(t, lane, artifact.Recording)
	assertAWSParityPromotionPolicy(t, lane, artifact.Promotion)
	for _, scenario := range artifact.Scenarios {
		if len(scenario.OperationIDs) == 0 || len(scenario.ObservedFields) == 0 || len(scenario.ExpectedTransitions) == 0 {
			t.Fatalf("scenario %s is incomplete: %#v", scenario.Name, scenario)
		}
		for _, path := range scenario.FixturePaths {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("scenario %s fixture %s is not readable: %v", scenario.Name, path, err)
			}
		}
	}
}

func assertAWSParityRecordingPolicy(t *testing.T, lane string, recording awsParityRecording) {
	t.Helper()
	if strings.TrimSpace(recording.Status) == "" {
		t.Fatalf("%s recording policy status is required", strings.ToUpper(lane))
	}
	if recording.UpdateEnv != "" && recording.UpdateEnv != awsParityRecordEnv {
		t.Fatalf("%s recording update env = %q, want %q", strings.ToUpper(lane), recording.UpdateEnv, awsParityRecordEnv)
	}
	if lane == "w01" {
		if recording.Status != "deferred" {
			t.Fatalf("W01 recording status = %q, want deferred until an explicit operator recording run", recording.Status)
		}
		wantPath := filepath.Join(awsParityFixtureRoot, lane, "live.observations.json")
		if recording.ArtifactPath != wantPath {
			t.Fatalf("W01 recording artifact path = %q, want %q", recording.ArtifactPath, wantPath)
		}
		if !recording.CompareWithoutUpdate || strings.TrimSpace(recording.Decision) == "" {
			t.Fatalf("W01 recording policy is incomplete: %#v", recording)
		}
		if _, err := os.Stat(wantPath); !os.IsNotExist(err) {
			t.Fatalf("W01 must not commit %s until an explicit recording update is approved", wantPath)
		}
		return
	}
	if recording.Status != "static-only" {
		t.Fatalf("%s recording status = %q, want static-only", strings.ToUpper(lane), recording.Status)
	}
	if strings.TrimSpace(recording.ArtifactPath) == "" {
		t.Fatalf("%s static-only recording artifact path is required", strings.ToUpper(lane))
	}
	if _, err := os.Stat(recording.ArtifactPath); !os.IsNotExist(err) {
		t.Fatalf("%s must not commit %s while the lane is static-only", strings.ToUpper(lane), recording.ArtifactPath)
	}
}

func assertAWSParityPromotionPolicy(t *testing.T, lane string, promotion awsParityPromotion) {
	t.Helper()
	if strings.TrimSpace(promotion.Next) == "" {
		t.Fatalf("%s promotion next step is required", strings.ToUpper(lane))
	}
	switch lane {
	case "w01":
		if !promotion.LiveCandidate || !slices.Contains(promotion.Preconditions, "explicit operator recording run") {
			t.Fatalf("W01 promotion policy = %#v, want live recording candidate with explicit operator run", promotion)
		}
	case "w02":
		if !promotion.LiveCandidate || !slices.Contains(promotion.Preconditions, "no attached policies instance profiles permissions boundaries or access keys") {
			t.Fatalf("W02 promotion policy = %#v, want IAM Role live candidate guardrails", promotion)
		}
	case "w03", "w04":
		if promotion.LiveCandidate {
			t.Fatalf("%s promotion policy must remain live-disabled: %#v", strings.ToUpper(lane), promotion)
		}
		if len(promotion.Blockers) == 0 || !strings.Contains(strings.Join(promotion.Blockers, " "), "global bucket") {
			t.Fatalf("%s promotion blockers = %#v, want global bucket naming blocker", strings.ToUpper(lane), promotion.Blockers)
		}
	default:
		t.Fatalf("no promotion assertions registered for AWS parity lane %s", lane)
	}
}

func assertAWSParitySafetyContract(t *testing.T, lane string, safety awsParitySafety) {
	t.Helper()
	for _, envName := range []string{awsParityTofuEnv, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"} {
		if !slices.Contains(safety.RequiredEnv, envName) {
			t.Fatalf("%s required_env = %#v, missing %s", strings.ToUpper(lane), safety.RequiredEnv, envName)
		}
	}
	if !strings.Contains(safety.CleanupFallback, "aws ") {
		t.Fatalf("%s cleanup fallback = %q, want AWS CLI command", strings.ToUpper(lane), safety.CleanupFallback)
	}
	switch lane {
	case "w01":
		if !safety.LiveEnabled {
			t.Fatalf("W01 must be marked live-enabled after IAM user guardrails are documented")
		}
		for _, guardrail := range []string{"one disposable IAM user at a time", "no access keys policies groups or login profile", "delete IAM user before recording"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("W01 cost guardrails = %#v, missing %q", safety.CostGuardrails, guardrail)
			}
		}
	case "w02", "w03", "w04":
		if safety.LiveEnabled {
			t.Fatalf("%s must remain live-disabled for static parity evidence", strings.ToUpper(lane))
		}
		for _, guardrail := range []string{"static first", "no live AWS mutation", "cleanup verification before recording"} {
			if !slices.Contains(safety.CostGuardrails, guardrail) {
				t.Fatalf("%s cost guardrails = %#v, missing %q", strings.ToUpper(lane), safety.CostGuardrails, guardrail)
			}
		}
	default:
		t.Fatalf("no safety assertions registered for AWS parity lane %s", lane)
	}
}

func assertAWSParityStaticFixtures(t *testing.T, lane string) {
	t.Helper()
	artifact := loadAWSParityArtifact(t, filepath.Join(awsParityFixtureRoot, lane, "observations.json"))
	assertAWSParityHCLFixture(t, lane, artifact)
	assertAWSParityNativeProjectFixture(t, lane)
	switch lane {
	case "w01":
		assertAWSParityPlanFixture(t, lane, "create", "CreateUser", "create", false)
		assertAWSParityPlanFixture(t, lane, "read", "GetUser", "read", false)
		assertAWSParityPlanFixture(t, lane, "delete", "DeleteUser", "delete", true)
		assertAWSParitySettleFixture(t, lane)
		assertAWSParityRequestBindings(t, lane, map[string][]string{
			"create": {"UserName"},
			"read":   {"UserName"},
			"delete": {"UserName"},
		})
		assertAWSParityResponseBindings(t, lane, []string{"User.UserName", "User.Arn", "User.UserId"})
		assertAWSParityUdonMetadata(t, lane, "iam")
	case "w02":
		assertAWSParityPlanFixture(t, lane, "create", "CreateRole", "create", false)
		assertAWSParityPlanFixture(t, lane, "read", "GetRole", "read", false)
		assertAWSParityPlanFixture(t, lane, "create", "UpdateRole", "update", true)
		assertAWSParityPlanFixture(t, lane, "delete", "DeleteRole", "delete", true)
		assertAWSParityRequestBindings(t, lane, map[string][]string{
			"create": {"RoleName", "AssumeRolePolicyDocument", "Description", "MaxSessionDuration"},
			"read":   {"RoleName"},
			"update": {"RoleName", "Description", "MaxSessionDuration"},
			"delete": {"RoleName"},
		})
		assertAWSParityResponseBindings(t, lane, []string{"Role.RoleName", "Role.Arn", "Role.RoleId", "Role.Description", "Role.MaxSessionDuration"})
		assertAWSParityUdonMetadata(t, lane, "iam")
	case "w03":
		assertAWSParityPlanFixture(t, lane, "create", "PutPublicAccessBlock", "create", false)
		assertAWSParityPlanFixture(t, lane, "read", "GetPublicAccessBlock", "read", false)
		assertAWSParityPlanFixture(t, lane, "put", "PutPublicAccessBlock", "put", false)
		assertAWSParityPlanFixture(t, lane, "delete", "DeletePublicAccessBlock", "delete", true)
		assertAWSParityRequestBindings(t, lane, map[string][]string{
			"create": {"Bucket", "PublicAccessBlockConfiguration"},
			"read":   {"Bucket"},
			"put":    {"Bucket", "PublicAccessBlockConfiguration"},
			"delete": {"Bucket"},
		})
		assertAWSParityResponseBindings(t, lane, []string{"PublicAccessBlockConfiguration.BlockPublicAcls", "PublicAccessBlockConfiguration.BlockPublicPolicy", "PublicAccessBlockConfiguration.IgnorePublicAcls", "PublicAccessBlockConfiguration.RestrictPublicBuckets"})
		assertAWSParityUdonMetadata(t, lane, "s3")
	case "w04":
		assertAWSParityPlanFixture(t, lane, "create", "PutBucketVersioning", "create", false)
		assertAWSParityPlanFixture(t, lane, "read", "GetBucketVersioning", "read", false)
		assertAWSParityPlanFixture(t, lane, "put", "PutBucketVersioning", "put", false)
		assertAWSParityRequestBindings(t, lane, map[string][]string{
			"create": {"Bucket", "VersioningConfiguration"},
			"read":   {"Bucket"},
			"put":    {"Bucket", "VersioningConfiguration"},
		})
		assertAWSParityResponseBindings(t, lane, []string{"Status"})
		assertAWSParityUdonMetadata(t, lane, "s3")
	default:
		t.Fatalf("no static assertions registered for AWS parity lane %s", lane)
	}
}

func assertAWSParityHCLFixture(t *testing.T, lane string, artifact awsParityArtifact) {
	t.Helper()
	doc, err := tfconfig.LoadDir(filepath.Join(awsParityFixtureRoot, lane, "hcl"))
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
		if req.LocalName == "aws" && req.Source == "hashicorp/aws" && slices.Contains(req.VersionConstraints, artifact.Provider.VersionConstraint) {
			foundProvider = true
		}
	}
	if !foundProvider {
		t.Fatalf("%s HCL fixture required providers = %#v, want hashicorp/aws %s", strings.ToUpper(lane), module.RequiredProviders, artifact.Provider.VersionConstraint)
	}
}

func assertAWSParityNativeProjectFixture(t *testing.T, lane string) {
	assertAPIParityNativeProjectFixture(t, "AWS", awsParityFixtureRoot, lane)
}

func assertAWSParityPlanFixture(t *testing.T, lane, action, operationID, summaryField string, seedState bool) {
	var seed func(*testing.T, string, string)
	if seedState {
		seed = seedAWSParityState
	}
	assertAPIParityPlanFixture(t, "AWS", awsParityFixtureRoot, lane, action, operationID, summaryField, seed)
}

func seedAWSParityState(t *testing.T, lane, statePath string) {
	t.Helper()
	spec := map[string]struct {
		address    string
		resource   string
		identity   string
		attributes string
	}{
		"w01": {address: "aws_iam_user.test", resource: "aws_iam_user", identity: `{"user_name":"ramen-parity-w01-static"}`, attributes: `{"name":"ramen-parity-w01-static"}`},
		"w02": {address: "aws_iam_role.test", resource: "aws_iam_role", identity: `{"role_name":"ramen-parity-w02-static"}`, attributes: `{"name":"ramen-parity-w02-static","description":"old"}`},
		"w03": {address: "aws_s3_bucket_public_access_block.test", resource: "aws_s3_bucket_public_access_block", identity: `{"bucket":"ramen-parity-w03-fixture-bucket"}`, attributes: `{"bucket":"ramen-parity-w03-fixture-bucket"}`},
		"w04": {address: "aws_s3_bucket_versioning.test", resource: "aws_s3_bucket_versioning", identity: `{"bucket":"ramen-parity-w04-fixture-bucket"}`, attributes: `{"bucket":"ramen-parity-w04-fixture-bucket"}`},
	}
	item, ok := spec[lane]
	if !ok {
		t.Fatalf("no seed state registered for AWS parity lane %s", lane)
	}
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open %s seed state: %v", strings.ToUpper(lane), err)
	}
	defer store.Close()
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{
		Address:        item.address,
		Type:           item.resource,
		Provider:       "provider.aws",
		DesiredHash:    "sha256:previous",
		IdentityJSON:   item.identity,
		AttributesJSON: item.attributes,
		Status:         "managed",
	}); err != nil {
		t.Fatalf("record %s seed state: %v", strings.ToUpper(lane), err)
	}
}

func awsParitySummaryHasOne(summary tfplan.Summary, field string) bool {
	return apiParitySummaryHasOne(summary, field)
}

func assertAWSParitySettleFixture(t *testing.T, lane string) {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "state.db")
	seedAWSParityState(t, lane, statePath)
	result, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: filepath.Join(awsParityFixtureRoot, lane, "ramen"),
		StatePath:   statePath,
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

func assertAWSParityRequestBindings(t *testing.T, lane string, expected map[string][]string) {
	assertAPIParityRequestBindings(t, "AWS", awsParityFixtureRoot, lane, firstAWSParityActionForBindings(lane), expected)
}

func firstAWSParityActionForBindings(lane string) string {
	if lane == "w04" {
		return "put"
	}
	return "create"
}

func assertAWSParityResponseBindings(t *testing.T, lane string, expected []string) {
	assertAPIParityResponseBindings(t, "AWS", awsParityFixtureRoot, lane, expected)
}

func assertAWSParityUdonMetadata(t *testing.T, lane, service string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(awsParityFixtureRoot, lane, "ramen", "project.uws.yaml"))
	if err != nil {
		t.Fatalf("read %s native fixture: %v", strings.ToUpper(lane), err)
	}
	text := string(data)
	for _, want := range []string{"x-udon-config", "awsSigV4", "binding: aws_hmac", "aws_signing_name: " + service, "region: us-east-1", "service: " + service} {
		if !strings.Contains(text, want) {
			t.Fatalf("%s native fixture missing %q", strings.ToUpper(lane), want)
		}
	}
}

func compareAWSParityW01Observations(observations []awsParityRuntimeObservation) awsParityObservationComparison {
	fields := []string{
		"after_apply.exists",
		"after_apply.arn_present",
		"after_apply.user_id_present",
		"no_op",
		"after_destroy.exists",
	}
	comparison := compareAPIParityObservationFields(observations, fields)
	for _, observation := range observations {
		if observation.Fields["after_apply.exists"] != true || observation.Fields["after_apply.arn_present"] != true || observation.Fields["after_apply.user_id_present"] != true || observation.Fields["after_destroy.exists"] != false {
			comparison.Matched = false
		}
	}
	return comparison
}

func awsParityFailure(runtime, class string, err error) awsParityRuntimeResult {
	return apiParityFailure("AWS", runtime, class, err)
}

func compareOrUpdateAWSParityRecording(t *testing.T, recording awsParityLiveRecording, path string) {
	compareOrUpdateAPIParityRecording(t, "AWS", awsParityEnv, awsParityRecordEnv, path, recording, true, normalizeAWSParityRecording)
}

func normalizeAWSParityRecording(t *testing.T, data []byte) awsParityLiveRecording {
	return normalizeAPIParityRecording(t, "AWS", data, nil)
}
