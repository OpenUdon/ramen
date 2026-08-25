package corpus

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	tfplan "github.com/OpenUdon/ramen/plan"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
)

type apiParityScenario struct {
	Name                  string   `json:"name"`
	ResourceType          string   `json:"resource_type"`
	TerraformResourceType string   `json:"terraform_resource_type,omitempty"`
	FixturePaths          []string `json:"fixture_paths,omitempty"`
	OperationIDs          []string `json:"operation_ids"`
	ObservedFields        []string `json:"observed_fields"`
	ExpectedTransitions   []string `json:"expected_transitions"`
	ObservationArtifacts  []string `json:"observation_artifacts,omitempty"`
}

type apiParityLiveRecording struct {
	Version      string                         `json:"version"`
	Lane         string                         `json:"lane"`
	Scenario     string                         `json:"scenario"`
	RecordedAt   string                         `json:"recorded_at"`
	DurationMS   int64                          `json:"duration_ms,omitempty"`
	Observations []apiParityRuntimeObservation  `json:"observations"`
	Comparison   apiParityObservationComparison `json:"comparison"`
	Failures     []apiParityRuntimeFailure      `json:"failures,omitempty"`
}

type apiParityRuntimeObservation struct {
	Runtime    string         `json:"runtime"`
	Resource   string         `json:"resource"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	Fields     map[string]any `json:"fields,omitempty"`
}

type apiParityObservationComparison struct {
	Matched bool     `json:"matched"`
	Fields  []string `json:"fields"`
}

type apiParityRuntimeFailure struct {
	Runtime string `json:"runtime"`
	Class   string `json:"class"`
	Message string `json:"message"`
}

type apiParityRuntimeResult struct {
	Observation apiParityRuntimeObservation
	Failure     *apiParityRuntimeFailure
}

func assertAPIParityNativeProjectFixture(t *testing.T, provider, fixtureRoot, lane string) {
	t.Helper()
	result, err := ramenvalidate.Run(context.Background(), ramenvalidate.Options{
		ProjectPath: filepath.Join(fixtureRoot, lane, "ramen"),
	})
	if err != nil {
		t.Fatalf("validate %s native Ramen project fixture for %s: %v", strings.ToUpper(lane), provider, err)
	}
	if !result.Valid || result.Summary.Diagnostics != 0 {
		t.Fatalf("%s native Ramen project fixture diagnostics for %s: valid=%t summary=%#v diagnostics=%#v", strings.ToUpper(lane), provider, result.Valid, result.Summary, result.Diagnostics)
	}
}

func assertAPIParityPlanFixture(t *testing.T, provider, fixtureRoot, lane, action, operationID, summaryField string, seedState func(*testing.T, string, string)) {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "state.db")
	if seedState != nil {
		seedState(t, lane, statePath)
	}
	result := buildAPIParityFixturePlan(t, provider, fixtureRoot, lane, action, statePath, action+" plan")
	resource := result.Plan.Resources[0]
	if resource.Mapping == nil || resource.Mapping.OperationID != operationID {
		t.Fatalf("%s %s operation = %#v, want %s", strings.ToUpper(lane), action, resource.Mapping, operationID)
	}
	if !apiParitySummaryHasOne(result.Plan.Summary, summaryField) {
		t.Fatalf("%s %s plan summary = %#v, want one %s action", strings.ToUpper(lane), action, result.Plan.Summary, summaryField)
	}
}

func apiParitySummaryHasOne(summary tfplan.Summary, field string) bool {
	switch field {
	case "create":
		return summary.Create == 1
	case "read":
		return summary.Read == 1
	case "update":
		return summary.Update == 1
	case "delete":
		return summary.Delete == 1
	case "post":
		return summary.Post == 1
	case "put":
		return summary.Put == 1
	case "patch":
		return summary.Patch == 1
	case "replace":
		return summary.Replace == 1
	case "no-op":
		return summary.NoOp == 1
	default:
		return false
	}
}

func assertAPIParityRequestBindings(t *testing.T, provider, fixtureRoot, lane, action string, expected map[string][]string) {
	t.Helper()
	result := buildAPIParityFixturePlan(t, provider, fixtureRoot, lane, action, filepath.Join(t.TempDir(), "state.db"), "request bindings")
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

func assertAPIParityResponseBindings(t *testing.T, provider, fixtureRoot, lane string, expected []string) {
	t.Helper()
	result := buildAPIParityFixturePlan(t, provider, fixtureRoot, lane, "read", filepath.Join(t.TempDir(), "state.db"), "response bindings")
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

func buildAPIParityFixturePlan(t *testing.T, provider, fixtureRoot, lane, action, statePath, purpose string) *tfplan.Result {
	t.Helper()
	result, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: filepath.Join(fixtureRoot, lane, "ramen"),
		StatePath:   statePath,
		Action:      action,
	})
	if err != nil {
		t.Fatalf("build %s Ramen fixture %s for %s: %v", strings.ToUpper(lane), purpose, provider, err)
	}
	if result.Plan.Errored || len(result.Plan.Resources) != 1 || result.Plan.Resources[0].Mapping == nil {
		t.Fatalf("%s Ramen fixture plan unusable for %s: %#v", strings.ToUpper(lane), purpose, result.Plan)
	}
	return result
}

func compareAPIParityObservationFields(observations []apiParityRuntimeObservation, fields []string) apiParityObservationComparison {
	if len(observations) == 0 {
		return apiParityObservationComparison{Matched: false, Fields: fields}
	}
	matched := true
	first := observations[0].Fields
	for _, observation := range observations {
		for _, field := range fields {
			if !reflect.DeepEqual(observation.Fields[field], first[field]) {
				matched = false
			}
		}
	}
	return apiParityObservationComparison{Matched: matched, Fields: fields}
}

func apiParityFailure(provider, runtime, class string, err error) apiParityRuntimeResult {
	if err == nil {
		err = errors.New("unknown " + provider + " parity failure")
	}
	return apiParityRuntimeResult{Failure: &apiParityRuntimeFailure{
		Runtime: runtime,
		Class:   class,
		Message: err.Error(),
	}}
}

func compareOrUpdateAPIParityRecording(t *testing.T, provider, liveEnv, recordEnv, path string, recording apiParityLiveRecording, allowMissing bool, normalize func(*testing.T, []byte) apiParityLiveRecording) {
	t.Helper()
	data, err := json.MarshalIndent(recording, "", "  ")
	if err != nil {
		t.Fatalf("encode %s parity recording: %v", provider, err)
	}
	data = append(data, '\n')
	if os.Getenv(recordEnv) == "1" {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write %s parity recording %s: %v", provider, path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if os.IsNotExist(err) && allowMissing {
		t.Logf("no committed %s parity recording at %s; live run was not recorded because %s is not set", provider, path, recordEnv)
		return
	}
	if err != nil {
		t.Fatalf("read committed %s parity recording %s: %v; rerun with %s=1 %s=1 only after reviewing sanitized live output", provider, path, err, liveEnv, recordEnv)
	}
	if !reflect.DeepEqual(normalize(t, want), normalize(t, data)) {
		t.Fatalf("live %s parity recording differs from %s; rerun with %s=1 %s=1 only after reviewing sanitized live output", provider, path, liveEnv, recordEnv)
	}
}

func normalizeAPIParityRecording(t *testing.T, provider string, data []byte, normalizeObservation func(string, *apiParityRuntimeObservation)) apiParityLiveRecording {
	t.Helper()
	var recording apiParityLiveRecording
	if err := json.Unmarshal(data, &recording); err != nil {
		t.Fatalf("decode %s parity recording: %v", provider, err)
	}
	recording.RecordedAt = ""
	recording.DurationMS = 0
	for i := range recording.Observations {
		recording.Observations[i].DurationMS = 0
		if normalizeObservation != nil {
			normalizeObservation(recording.Lane, &recording.Observations[i])
		}
	}
	return recording
}
