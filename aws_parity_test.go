package corpus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tfplan "github.com/OpenUdon/ramen/plan"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
)

const (
	awsParityArtifactV1  = "ramen.aws.provider-parity.v1"
	awsParityFixtureRoot = "testdata/parity/aws"
)

var awsParityLanes = []string{"w01"}

type awsParityArtifact struct {
	Version   string              `json:"version"`
	Lane      string              `json:"lane"`
	Status    string              `json:"status"`
	Provider  awsParityProvider   `json:"provider"`
	Smithy    awsParitySmithy     `json:"smithy"`
	Safety    awsParitySafety     `json:"safety"`
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
	CredentialBinding    string   `json:"credential_binding"`
	ResourcePrefix       string   `json:"resource_prefix"`
	RequiresExplicitLane bool     `json:"requires_explicit_lane"`
	LiveEnabled          bool     `json:"live_enabled,omitempty"`
	CostGuardrails       []string `json:"cost_guardrails,omitempty"`
}

type awsParityScenario struct {
	Name                  string   `json:"name"`
	ResourceType          string   `json:"resource_type"`
	TerraformResourceType string   `json:"terraform_resource_type,omitempty"`
	FixturePaths          []string `json:"fixture_paths,omitempty"`
	OperationIDs          []string `json:"operation_ids"`
	ObservedFields        []string `json:"observed_fields"`
	ExpectedTransitions   []string `json:"expected_transitions"`
}

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
	if artifact.Status != "planned" {
		t.Fatalf("artifact status = %q, want planned", artifact.Status)
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
	if artifact.Safety.LiveEnv != "RAMEN_AWS_PARITY" || artifact.Safety.RecordUpdateEnv != "RAMEN_AWS_PARITY_RECORD_UPDATE" || artifact.Safety.LaneEnv != "RAMEN_AWS_PARITY_LANE" {
		t.Fatalf("unexpected AWS live env gates: %#v", artifact.Safety)
	}
	if artifact.Safety.LiveEnabled {
		t.Fatalf("%s must remain live-disabled until explicit AWS mutation approval", wantLane)
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
	for _, guardrail := range []string{"static first", "opentofu baseline only for future live runs", "delete IAM user before recording"} {
		if !slices.Contains(artifact.Safety.CostGuardrails, guardrail) {
			t.Fatalf("cost guardrails = %#v, missing %q", artifact.Safety.CostGuardrails, guardrail)
		}
	}
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

func assertAWSParityStaticFixtures(t *testing.T, lane string) {
	t.Helper()
	projectPath := filepath.Join(awsParityFixtureRoot, lane, "ramen")
	result, err := ramenvalidate.Run(context.Background(), ramenvalidate.Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatalf("validate %s native Ramen project fixture: %v", strings.ToUpper(lane), err)
	}
	if !result.Valid || result.Summary.Diagnostics != 0 {
		t.Fatalf("%s native Ramen project fixture diagnostics: valid=%t summary=%#v diagnostics=%#v", strings.ToUpper(lane), result.Valid, result.Summary, result.Diagnostics)
	}
	for _, tc := range []struct {
		action    string
		operation string
	}{
		{action: "create", operation: "CreateUser"},
		{action: "read", operation: "GetUser"},
	} {
		planResult, err := tfplan.Build(context.Background(), tfplan.Options{
			ProjectPath: projectPath,
			StatePath:   filepath.Join(t.TempDir(), tc.action+".db"),
			Action:      tc.action,
		})
		if err != nil {
			t.Fatalf("build %s %s plan: %v", strings.ToUpper(lane), tc.action, err)
		}
		if planResult.Plan.Errored || len(planResult.Plan.Resources) != 1 {
			t.Fatalf("%s %s plan unusable: %#v", strings.ToUpper(lane), tc.action, planResult.Plan)
		}
		resource := planResult.Plan.Resources[0]
		if resource.Mapping == nil || resource.Mapping.OperationID != tc.operation {
			t.Fatalf("%s %s operation = %#v, want %s", strings.ToUpper(lane), tc.action, resource.Mapping, tc.operation)
		}
	}
}
