package corpus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tfplan "github.com/OpenUdon/ramen/plan"
)

const (
	kubernetesParityEnv          = "RAMEN_K8S_PARITY"
	kubernetesParityRecordEnv    = "RAMEN_K8S_PARITY_RECORD_UPDATE"
	kubernetesParityLaneEnv      = "RAMEN_K8S_PARITY_LANE"
	kubernetesParityTerraformEnv = "RAMEN_K8S_TERRAFORM"
	kubernetesParityTofuEnv      = "RAMEN_K8S_TOFU"
	kubernetesParityArtifactV1   = "ramen.kubernetes.provider-parity.v1"
	kubernetesParityFixtureRoot  = "testdata/parity/kubernetes"
	kubernetesProviderVersion    = "3.1.0"
)

var kubernetesParityLanes = []string{"k01", "k02", "k03", "k04", "k05"}

type kubernetesParityArtifact struct {
	Version          string                     `json:"version"`
	Lane             string                     `json:"lane"`
	Status           string                     `json:"status"`
	Provider         kubernetesParityProvider   `json:"provider"`
	OpenAPI          kubernetesParityOpenAPI    `json:"openapi"`
	Safety           kubernetesParitySafety     `json:"safety"`
	Runtimes         []string                   `json:"runtimes"`
	Scenarios        []kubernetesParityScenario `json:"scenarios"`
	RecordedAt       string                     `json:"recorded_at,omitempty"`
	RecordingsSource string                     `json:"recordings_source,omitempty"`
	Notes            []string                   `json:"notes,omitempty"`
}

type kubernetesParityProvider struct {
	Source         string `json:"source"`
	Version        string `json:"version"`
	Published      string `json:"published"`
	RegistryLatest bool   `json:"registry_latest"`
}

type kubernetesParityOpenAPI struct {
	SourcePath string `json:"source_path"`
	Fixture    string `json:"fixture"`
}

type kubernetesParitySafety struct {
	LiveEnv         string `json:"live_env"`
	RecordUpdateEnv string `json:"record_update_env"`
	ContextPrefix   string `json:"context_prefix"`
	ResourcePrefix  string `json:"resource_prefix"`
}

type kubernetesParityScenario struct {
	Name                 string   `json:"name"`
	ResourceType         string   `json:"resource_type"`
	FixturePaths         []string `json:"fixture_paths,omitempty"`
	ObservedFields       []string `json:"observed_fields"`
	ExpectedTransitions  []string `json:"expected_transitions"`
	ObservationArtifacts []string `json:"observation_artifacts"`
}

type kubernetesParityLiveRecording struct {
	Version      string                                `json:"version"`
	Lane         string                                `json:"lane"`
	Scenario     string                                `json:"scenario"`
	RecordedAt   string                                `json:"recorded_at"`
	Context      string                                `json:"context"`
	Observations []kubernetesParityRuntimeObservation  `json:"observations"`
	Comparison   kubernetesParityObservationComparison `json:"comparison"`
	Failures     []kubernetesParityRuntimeFailure      `json:"failures,omitempty"`
}

type kubernetesParityRuntimeObservation struct {
	Runtime                 string                                  `json:"runtime"`
	Namespace               string                                  `json:"namespace"`
	ConfigMapName           string                                  `json:"config_map_name,omitempty"`
	AfterApply              kubernetesParityObservation             `json:"after_apply"`
	NoOpPlan                *kubernetesParityNoOpObservation        `json:"no_op_plan,omitempty"`
	AfterDestroy            *kubernetesParityObservation            `json:"after_destroy,omitempty"`
	AfterOutOfBandDelete    *kubernetesParityObservation            `json:"after_out_of_band_delete,omitempty"`
	ReadMissing             *kubernetesParityReadMissingObservation `json:"read_missing,omitempty"`
	AfterReadMissingCleanup *kubernetesParityObservation            `json:"after_read_missing_cleanup,omitempty"`
	ConfigMapAfterApply     *kubernetesParityConfigMapObservation   `json:"config_map_after_apply,omitempty"`
	ConfigMapAfterUpdate    *kubernetesParityConfigMapObservation   `json:"config_map_after_update,omitempty"`
	ConfigMapAfterDestroy   *kubernetesParityConfigMapObservation   `json:"config_map_after_destroy,omitempty"`
}

type kubernetesParityObservation struct {
	Exists bool              `json:"exists"`
	Name   string            `json:"name,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
	Phase  string            `json:"phase,omitempty"`
}

type kubernetesParityConfigMapObservation struct {
	Exists     bool              `json:"exists"`
	Namespace  string            `json:"namespace,omitempty"`
	Name       string            `json:"name,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Data       map[string]string `json:"data,omitempty"`
	BinaryData map[string]string `json:"binaryData,omitempty"`
}

type kubernetesParityNoOpObservation struct {
	NoOp     bool   `json:"no_op"`
	ExitCode int    `json:"exit_code,omitempty"`
	Summary  string `json:"summary,omitempty"`
}

type kubernetesParityReadMissingObservation struct {
	Missing        bool   `json:"missing"`
	Classification string `json:"classification"`
	ExitCode       int    `json:"exit_code,omitempty"`
	Summary        string `json:"summary,omitempty"`
}

type kubernetesParityObservationComparison struct {
	Matched bool     `json:"matched"`
	Fields  []string `json:"fields"`
}

type kubernetesParityRuntimeFailure struct {
	Runtime string `json:"runtime"`
	Class   string `json:"class"`
	Message string `json:"message"`
}

type kubernetesParityLiveEnv struct {
	kubectl     string
	contextName string
	kubeconfig  string
}

type kubernetesParityRuntimeResult struct {
	Observation kubernetesParityRuntimeObservation
	Failure     *kubernetesParityRuntimeFailure
}

func TestKubernetesProviderParityReplayArtifacts(t *testing.T) {
	for _, lane := range kubernetesParityLanes {
		lane := lane
		t.Run(strings.ToUpper(lane), func(t *testing.T) {
			artifact := loadKubernetesParityArtifact(t, filepath.Join(kubernetesParityFixtureRoot, lane, "observations.json"))
			assertKubernetesParityArtifact(t, lane, artifact)
			assertKubernetesParityRecordingArtifacts(t, lane, artifact)
			if lane == "k01" {
				assertKubernetesK01PlanFixture(t)
			}
			if lane == "k02" {
				assertKubernetesK02PlanFixture(t)
			}
			if lane == "k03" {
				assertKubernetesK03PlanFixture(t)
			}
		})
	}
}

func TestKubernetesProviderParity(t *testing.T) {
	if os.Getenv(kubernetesParityEnv) != "1" {
		t.Skipf("set %s=1 to run the opt-in Kubernetes provider parity harness", kubernetesParityEnv)
	}
	terraform := requireKubernetesParityTool(t, kubernetesParityTerraformEnv, "terraform")
	tofu := requireKubernetesParityTool(t, kubernetesParityTofuEnv, "tofu")
	kubectl := requireKubernetesParityTool(t, "", "kubectl")
	kind := requireKubernetesParityTool(t, "", "kind")
	env := prepareKubernetesParityLiveEnv(t, kubectl)
	if out, err := osexec.Command(kind, "get", "clusters").CombinedOutput(); err != nil {
		t.Fatalf("kind is required when %s=1: %v: %s", kubernetesParityEnv, err, strings.TrimSpace(string(out)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	liveRuns := []struct {
		lane string
		run  func(*testing.T, context.Context, kubernetesParityLiveEnv, string, string) kubernetesParityLiveRecording
	}{
		{lane: "k01", run: runKubernetesK01LiveParity},
		{lane: "k02", run: runKubernetesK02LiveParity},
		{lane: "k03", run: runKubernetesK03LiveParity},
	}
	selectedLane := strings.ToLower(strings.TrimSpace(os.Getenv(kubernetesParityLaneEnv)))
	if selectedLane != "" && !slices.Contains(kubernetesParityLanes, selectedLane) {
		t.Fatalf("%s=%s is not a known Kubernetes parity lane", kubernetesParityLaneEnv, selectedLane)
	}
	var ran int
	for _, liveRun := range liveRuns {
		if selectedLane != "" && selectedLane != liveRun.lane {
			continue
		}
		ran++
		artifact := loadKubernetesParityArtifact(t, filepath.Join(kubernetesParityFixtureRoot, liveRun.lane, "observations.json"))
		assertKubernetesParityArtifact(t, liveRun.lane, artifact)
		recording := liveRun.run(t, ctx, env, terraform, tofu)
		compareOrUpdateKubernetesParityRecording(t, recording, filepath.Join(kubernetesParityFixtureRoot, liveRun.lane, "live.observations.json"))
	}
	if ran == 0 {
		t.Fatalf("no Kubernetes parity live lanes were selected")
	}
}

func loadKubernetesParityArtifact(t *testing.T, path string) kubernetesParityArtifact {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var artifact kubernetesParityArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return artifact
}

func loadKubernetesParityLiveRecording(t *testing.T, path string) kubernetesParityLiveRecording {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var recording kubernetesParityLiveRecording
	if err := json.Unmarshal(data, &recording); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return recording
}

func assertKubernetesParityArtifact(t *testing.T, lane string, artifact kubernetesParityArtifact) {
	t.Helper()
	wantLane := strings.ToUpper(lane)
	if artifact.Version != kubernetesParityArtifactV1 {
		t.Fatalf("artifact version = %q, want %q", artifact.Version, kubernetesParityArtifactV1)
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
	if artifact.Provider.Source != "hashicorp/kubernetes" {
		t.Fatalf("provider source = %q, want hashicorp/kubernetes", artifact.Provider.Source)
	}
	if artifact.Provider.Version != kubernetesProviderVersion {
		t.Fatalf("provider version = %q, want %q", artifact.Provider.Version, kubernetesProviderVersion)
	}
	if artifact.Provider.Published != "2026-04-16" {
		t.Fatalf("provider published = %q, want 2026-04-16", artifact.Provider.Published)
	}
	if !artifact.Provider.RegistryLatest {
		t.Fatalf("provider registry_latest must be true for the pinned planning baseline")
	}
	if artifact.OpenAPI.SourcePath != "../apitools/catalog-openapi-cache/openapi/kubernetes-v1-19-2-swagger.json" {
		t.Fatalf("openapi source path = %q", artifact.OpenAPI.SourcePath)
	}
	if artifact.OpenAPI.Fixture == "" {
		t.Fatalf("openapi fixture path is required")
	}
	if artifact.Safety.LiveEnv != kubernetesParityEnv {
		t.Fatalf("live env = %q, want %q", artifact.Safety.LiveEnv, kubernetesParityEnv)
	}
	if artifact.Safety.RecordUpdateEnv != kubernetesParityRecordEnv {
		t.Fatalf("record update env = %q, want %q", artifact.Safety.RecordUpdateEnv, kubernetesParityRecordEnv)
	}
	if artifact.Safety.ContextPrefix != "kind-" {
		t.Fatalf("context prefix = %q, want kind-", artifact.Safety.ContextPrefix)
	}
	if !strings.HasPrefix(artifact.Safety.ResourcePrefix, "ramen-parity-"+lane+"-") {
		t.Fatalf("resource prefix = %q, want ramen-parity-%s-*", artifact.Safety.ResourcePrefix, lane)
	}
	wantRuntimes := []string{"opentofu", "terraform", "ramen"}
	for _, runtime := range wantRuntimes {
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
		if artifact.Status == "recorded" && len(scenario.ObservationArtifacts) == 0 {
			t.Fatalf("recorded scenario %s has no observation_artifacts", scenario.Name)
		}
		if artifact.Status == "planned" && len(scenario.ObservationArtifacts) != 0 {
			t.Fatalf("planned scenario %s must not claim observation_artifacts", scenario.Name)
		}
	}
}

func assertKubernetesParityRecordingArtifacts(t *testing.T, lane string, artifact kubernetesParityArtifact) {
	t.Helper()
	if artifact.Status != "recorded" {
		return
	}
	for _, scenario := range artifact.Scenarios {
		for _, path := range scenario.ObservationArtifacts {
			recording := loadKubernetesParityLiveRecording(t, path)
			assertKubernetesParityLiveRecording(t, lane, scenario.Name, artifact.Runtimes, recording)
		}
	}
}

func assertKubernetesParityLiveRecording(t *testing.T, lane, scenario string, runtimes []string, recording kubernetesParityLiveRecording) {
	t.Helper()
	wantLane := strings.ToUpper(lane)
	if recording.Version != kubernetesParityArtifactV1 {
		t.Fatalf("%s live recording version = %q, want %q", wantLane, recording.Version, kubernetesParityArtifactV1)
	}
	if recording.Lane != wantLane {
		t.Fatalf("%s live recording lane = %q, want %q", wantLane, recording.Lane, wantLane)
	}
	if recording.Scenario != scenario {
		t.Fatalf("%s live recording scenario = %q, want %q", wantLane, recording.Scenario, scenario)
	}
	if !recording.Comparison.Matched {
		t.Fatalf("%s live recording comparison did not match", wantLane)
	}
	if len(recording.Failures) != 0 {
		t.Fatalf("%s live recording has failures: %#v", wantLane, recording.Failures)
	}
	assertKubernetesParityRuntimeSet(t, wantLane, runtimes, recording.Observations)
	var comparison kubernetesParityObservationComparison
	switch lane {
	case "k01":
		comparison = compareKubernetesParityObservations(t, recording.Observations)
	case "k02":
		comparison = compareKubernetesReadMissingParityObservations(t, recording.Observations)
	case "k03":
		comparison = compareKubernetesConfigMapParityObservations(t, recording.Observations)
	default:
		t.Fatalf("%s is recorded but has no semantic replay assertions", wantLane)
	}
	if !reflect.DeepEqual(recording.Comparison.Fields, comparison.Fields) {
		t.Fatalf("%s comparison fields = %#v, want %#v", wantLane, recording.Comparison.Fields, comparison.Fields)
	}
}

func assertKubernetesParityRuntimeSet(t *testing.T, label string, runtimes []string, observations []kubernetesParityRuntimeObservation) {
	t.Helper()
	if len(observations) != len(runtimes) {
		t.Fatalf("%s live recording observation count = %d, want %d", label, len(observations), len(runtimes))
	}
	seen := map[string]bool{}
	for _, observation := range observations {
		if seen[observation.Runtime] {
			t.Fatalf("%s live recording has duplicate runtime %q", label, observation.Runtime)
		}
		seen[observation.Runtime] = true
	}
	for _, runtime := range runtimes {
		if !seen[runtime] {
			t.Fatalf("%s live recording missing runtime %q", label, runtime)
		}
	}
}

func runKubernetesK01LiveParity(t *testing.T, ctx context.Context, env kubernetesParityLiveEnv, terraformPath, tofuPath string) kubernetesParityLiveRecording {
	t.Helper()
	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, kubernetesParityLiveEnv, string) kubernetesParityRuntimeResult
		tool    string
	}{
		{runtime: "opentofu", run: runKubernetesParityHCLRuntime, tool: tofuPath},
		{runtime: "terraform", run: runKubernetesParityHCLRuntime, tool: terraformPath},
		{runtime: "ramen", run: runKubernetesParityRamenRuntime, tool: ""},
	}
	var observations []kubernetesParityRuntimeObservation
	var failures []kubernetesParityRuntimeFailure
	for _, run := range runs {
		namespace := "ramen-parity-k01-" + run.runtime
		if err := validateKubernetesParityNamespace(namespace, "k01"); err != nil {
			t.Fatalf("unsafe namespace %s: %v", namespace, err)
		}
		if err := deleteKubernetesParityNamespaceIfExists(ctx, env, namespace); err != nil {
			t.Fatalf("pre-cleanup %s: %v", namespace, err)
		}
		t.Cleanup(func() {
			if err := deleteKubernetesParityNamespaceIfExists(context.Background(), env, namespace); err != nil {
				t.Logf("cleanup namespace %s: %v", namespace, err)
			}
		})
		result := run.run(ctx, t, env, run.tool)
		if result.Failure != nil {
			failures = append(failures, *result.Failure)
			continue
		}
		observations = append(observations, result.Observation)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Logf("%s parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("K01 provider parity did not complete for all runtimes")
	}
	comparison := compareKubernetesParityObservations(t, observations)
	return kubernetesParityLiveRecording{
		Version:      kubernetesParityArtifactV1,
		Lane:         "K01",
		Scenario:     "namespace_lifecycle",
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		Context:      env.contextName,
		Observations: observations,
		Comparison:   comparison,
	}
}

func runKubernetesK02LiveParity(t *testing.T, ctx context.Context, env kubernetesParityLiveEnv, terraformPath, tofuPath string) kubernetesParityLiveRecording {
	t.Helper()
	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, kubernetesParityLiveEnv, string) kubernetesParityRuntimeResult
		tool    string
	}{
		{runtime: "opentofu", run: runKubernetesParityHCLReadMissingRuntime, tool: tofuPath},
		{runtime: "terraform", run: runKubernetesParityHCLReadMissingRuntime, tool: terraformPath},
		{runtime: "ramen", run: runKubernetesParityRamenReadMissingRuntime, tool: ""},
	}
	var observations []kubernetesParityRuntimeObservation
	var failures []kubernetesParityRuntimeFailure
	for _, run := range runs {
		namespace := "ramen-parity-k02-" + run.runtime
		if err := validateKubernetesParityNamespace(namespace, "k02"); err != nil {
			t.Fatalf("unsafe namespace %s: %v", namespace, err)
		}
		if err := deleteKubernetesParityNamespaceIfExists(ctx, env, namespace); err != nil {
			t.Fatalf("pre-cleanup %s: %v", namespace, err)
		}
		t.Cleanup(func() {
			if err := deleteKubernetesParityNamespaceIfExists(context.Background(), env, namespace); err != nil {
				t.Logf("cleanup namespace %s: %v", namespace, err)
			}
		})
		result := run.run(ctx, t, env, run.tool)
		if result.Failure != nil {
			failures = append(failures, *result.Failure)
			continue
		}
		observations = append(observations, result.Observation)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Logf("%s parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("K02 provider parity did not complete for all runtimes")
	}
	comparison := compareKubernetesReadMissingParityObservations(t, observations)
	return kubernetesParityLiveRecording{
		Version:      kubernetesParityArtifactV1,
		Lane:         "K02",
		Scenario:     "namespace_read_missing",
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		Context:      env.contextName,
		Observations: observations,
		Comparison:   comparison,
	}
}

func runKubernetesK03LiveParity(t *testing.T, ctx context.Context, env kubernetesParityLiveEnv, terraformPath, tofuPath string) kubernetesParityLiveRecording {
	t.Helper()
	runs := []struct {
		runtime string
		run     func(context.Context, *testing.T, kubernetesParityLiveEnv, string) kubernetesParityRuntimeResult
		tool    string
	}{
		{runtime: "opentofu", run: runKubernetesParityHCLConfigMapRuntime, tool: tofuPath},
		{runtime: "terraform", run: runKubernetesParityHCLConfigMapRuntime, tool: terraformPath},
		{runtime: "ramen", run: runKubernetesParityRamenConfigMapRuntime, tool: ""},
	}
	var observations []kubernetesParityRuntimeObservation
	var failures []kubernetesParityRuntimeFailure
	for _, run := range runs {
		namespace := "ramen-parity-k03-" + run.runtime
		if err := validateKubernetesParityNamespace(namespace, "k03"); err != nil {
			t.Fatalf("unsafe namespace %s: %v", namespace, err)
		}
		if err := deleteKubernetesParityNamespaceIfExists(ctx, env, namespace); err != nil {
			t.Fatalf("pre-cleanup %s: %v", namespace, err)
		}
		t.Cleanup(func() {
			if err := deleteKubernetesParityNamespaceIfExists(context.Background(), env, namespace); err != nil {
				t.Logf("cleanup namespace %s: %v", namespace, err)
			}
		})
		result := run.run(ctx, t, env, run.tool)
		if result.Failure != nil {
			failures = append(failures, *result.Failure)
			continue
		}
		observations = append(observations, result.Observation)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Logf("%s parity failure [%s]: %s", failure.Runtime, failure.Class, failure.Message)
		}
		t.Fatalf("K03 provider parity did not complete for all runtimes")
	}
	comparison := compareKubernetesConfigMapParityObservations(t, observations)
	return kubernetesParityLiveRecording{
		Version:      kubernetesParityArtifactV1,
		Lane:         "K03",
		Scenario:     "config_map_lifecycle",
		RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		Context:      env.contextName,
		Observations: observations,
		Comparison:   comparison,
	}
}

func runKubernetesParityHCLRuntime(ctx context.Context, t *testing.T, env kubernetesParityLiveEnv, tool string) kubernetesParityRuntimeResult {
	t.Helper()
	runtimeName := "terraform"
	if strings.HasSuffix(filepath.Base(tool), "tofu") {
		runtimeName = "opentofu"
	}
	namespace := "ramen-parity-k01-" + runtimeName
	workDir := filepath.Join(t.TempDir(), runtimeName)
	if err := copyFixtureFile(filepath.Join(kubernetesParityFixtureRoot, "k01", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return kubernetesParityFailure(runtimeName, "fixture", err)
	}
	tfvars := map[string]string{
		"kubeconfig_path": env.kubeconfig,
		"kube_context":    env.contextName,
		"namespace_name":  namespace,
	}
	if err := writeJSONFile(filepath.Join(workDir, "terraform.tfvars.json"), tfvars); err != nil {
		return kubernetesParityFailure(runtimeName, "fixture", err)
	}
	if err := runKubernetesParityCommand(ctx, workDir, tool, "init", "-input=false", "-no-color"); err != nil {
		return kubernetesParityFailure(runtimeName, "init", err)
	}
	if err := runKubernetesParityCommand(ctx, workDir, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return kubernetesParityFailure(runtimeName, "apply", err)
	}
	afterApply, err := observeKubernetesParityNamespace(ctx, env, namespace)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "observe", err)
	}
	planExit, planSummary, err := runKubernetesParityPlan(ctx, workDir, tool)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "plan", err)
	}
	if err := runKubernetesParityCommand(ctx, workDir, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return kubernetesParityFailure(runtimeName, "destroy", err)
	}
	afterDestroy, err := observeKubernetesParityNamespaceAbsent(ctx, env, namespace)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "observe", err)
	}
	return kubernetesParityRuntimeResult{Observation: kubernetesParityRuntimeObservation{
		Runtime:    runtimeName,
		Namespace:  namespace,
		AfterApply: afterApply,
		NoOpPlan: &kubernetesParityNoOpObservation{
			NoOp:     planExit == 0,
			ExitCode: planExit,
			Summary:  planSummary,
		},
		AfterDestroy: &afterDestroy,
	}}
}

func runKubernetesParityHCLReadMissingRuntime(ctx context.Context, t *testing.T, env kubernetesParityLiveEnv, tool string) kubernetesParityRuntimeResult {
	t.Helper()
	runtimeName := "terraform"
	if strings.HasSuffix(filepath.Base(tool), "tofu") {
		runtimeName = "opentofu"
	}
	namespace := "ramen-parity-k02-" + runtimeName
	workDir := filepath.Join(t.TempDir(), runtimeName)
	if err := copyFixtureFile(filepath.Join(kubernetesParityFixtureRoot, "k02", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return kubernetesParityFailure(runtimeName, "fixture", err)
	}
	tfvars := map[string]string{
		"kubeconfig_path": env.kubeconfig,
		"kube_context":    env.contextName,
		"namespace_name":  namespace,
	}
	if err := writeJSONFile(filepath.Join(workDir, "terraform.tfvars.json"), tfvars); err != nil {
		return kubernetesParityFailure(runtimeName, "fixture", err)
	}
	if err := runKubernetesParityCommand(ctx, workDir, tool, "init", "-input=false", "-no-color"); err != nil {
		return kubernetesParityFailure(runtimeName, "init", err)
	}
	if err := runKubernetesParityCommand(ctx, workDir, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return kubernetesParityFailure(runtimeName, "apply", err)
	}
	afterApply, err := observeKubernetesParityNamespace(ctx, env, namespace)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "observe", err)
	}
	if err := deleteKubernetesParityNamespaceIfExists(ctx, env, namespace); err != nil {
		return kubernetesParityFailure(runtimeName, "out_of_band_delete", err)
	}
	afterDelete, err := observeKubernetesParityNamespace(ctx, env, namespace)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "observe", err)
	}
	refreshExit, refreshSummary, err := runKubernetesParityPlanArgs(ctx, workDir, tool, "plan", "-refresh-only", "-input=false", "-no-color", "-detailed-exitcode")
	if err != nil {
		return kubernetesParityFailure(runtimeName, "read_missing", err)
	}
	if err := runKubernetesParityCommand(ctx, workDir, tool, "apply", "-refresh-only", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return kubernetesParityFailure(runtimeName, "cleanup", err)
	}
	afterCleanup, err := observeKubernetesParityNamespace(ctx, env, namespace)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "observe", err)
	}
	return kubernetesParityRuntimeResult{Observation: kubernetesParityRuntimeObservation{
		Runtime:              runtimeName,
		Namespace:            namespace,
		AfterApply:           afterApply,
		AfterOutOfBandDelete: &afterDelete,
		ReadMissing: &kubernetesParityReadMissingObservation{
			Missing:        refreshExit == 2,
			Classification: "missing",
			ExitCode:       refreshExit,
			Summary:        refreshSummary,
		},
		AfterReadMissingCleanup: &afterCleanup,
	}}
}

func runKubernetesParityHCLConfigMapRuntime(ctx context.Context, t *testing.T, env kubernetesParityLiveEnv, tool string) kubernetesParityRuntimeResult {
	t.Helper()
	runtimeName := "terraform"
	if strings.HasSuffix(filepath.Base(tool), "tofu") {
		runtimeName = "opentofu"
	}
	namespace := "ramen-parity-k03-" + runtimeName
	configMapName := "ramen-parity-k03-config-map"
	if err := validateKubernetesParityConfigMapName(configMapName, "k03"); err != nil {
		return kubernetesParityFailure(runtimeName, "fixture", err)
	}
	if err := createKubernetesParityNamespace(ctx, env, namespace); err != nil {
		return kubernetesParityFailure(runtimeName, "namespace", err)
	}
	workDir := filepath.Join(t.TempDir(), runtimeName)
	if err := copyFixtureFile(filepath.Join(kubernetesParityFixtureRoot, "k03", "hcl", "main.tf"), filepath.Join(workDir, "main.tf")); err != nil {
		return kubernetesParityFailure(runtimeName, "fixture", err)
	}
	createVars := map[string]string{
		"kubeconfig_path": env.kubeconfig,
		"kube_context":    env.contextName,
		"namespace_name":  namespace,
		"config_map_name": configMapName,
		"mode":            "create",
		"payload":         "cGFyaXR5LWNyZWF0ZQ==",
	}
	if err := writeJSONFile(filepath.Join(workDir, "terraform.tfvars.json"), createVars); err != nil {
		return kubernetesParityFailure(runtimeName, "fixture", err)
	}
	if err := runKubernetesParityCommand(ctx, workDir, tool, "init", "-input=false", "-no-color"); err != nil {
		return kubernetesParityFailure(runtimeName, "init", err)
	}
	if err := runKubernetesParityCommand(ctx, workDir, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return kubernetesParityFailure(runtimeName, "apply", err)
	}
	namespaceAfterApply, err := observeKubernetesParityNamespace(ctx, env, namespace)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "observe", err)
	}
	configMapAfterApply, err := observeKubernetesParityConfigMap(ctx, env, namespace, configMapName)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "observe", err)
	}
	updateVars := map[string]string{
		"kubeconfig_path": env.kubeconfig,
		"kube_context":    env.contextName,
		"namespace_name":  namespace,
		"config_map_name": configMapName,
		"mode":            "update",
		"payload":         "cGFyaXR5LXVwZGF0ZQ==",
	}
	if err := writeJSONFile(filepath.Join(workDir, "terraform.tfvars.json"), updateVars); err != nil {
		return kubernetesParityFailure(runtimeName, "fixture", err)
	}
	if err := runKubernetesParityCommand(ctx, workDir, tool, "apply", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return kubernetesParityFailure(runtimeName, "update", err)
	}
	configMapAfterUpdate, err := observeKubernetesParityConfigMap(ctx, env, namespace, configMapName)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "observe", err)
	}
	planExit, planSummary, err := runKubernetesParityPlan(ctx, workDir, tool)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "plan", err)
	}
	if err := runKubernetesParityCommand(ctx, workDir, tool, "destroy", "-input=false", "-no-color", "-auto-approve"); err != nil {
		return kubernetesParityFailure(runtimeName, "destroy", err)
	}
	configMapAfterDestroy, err := observeKubernetesParityConfigMapAbsent(ctx, env, namespace, configMapName)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "observe", err)
	}
	return kubernetesParityRuntimeResult{Observation: kubernetesParityRuntimeObservation{
		Runtime:               runtimeName,
		Namespace:             namespace,
		ConfigMapName:         configMapName,
		AfterApply:            namespaceAfterApply,
		ConfigMapAfterApply:   &configMapAfterApply,
		ConfigMapAfterUpdate:  &configMapAfterUpdate,
		ConfigMapAfterDestroy: &configMapAfterDestroy,
		NoOpPlan: &kubernetesParityNoOpObservation{
			NoOp:     planExit == 0,
			ExitCode: planExit,
			Summary:  planSummary,
		},
	}}
}

func runKubernetesParityPlan(ctx context.Context, dir, tool string) (int, string, error) {
	return runKubernetesParityPlanArgs(ctx, dir, tool, "plan", "-input=false", "-no-color", "-detailed-exitcode")
}

func runKubernetesParityPlanArgs(ctx context.Context, dir, tool string, args ...string) (int, string, error) {
	cmd := osexec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir
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
		return exitErr.ExitCode(), summary, fmt.Errorf("%s %s failed: %w: %s", filepath.Base(tool), strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return -1, summary, fmt.Errorf("%s %s failed: %w: %s", filepath.Base(tool), strings.Join(args, " "), err, strings.TrimSpace(string(out)))
}

func runKubernetesParityCommand(ctx context.Context, dir, tool string, args ...string) error {
	cmd := osexec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", filepath.Base(tool), strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func observeKubernetesParityNamespace(ctx context.Context, env kubernetesParityLiveEnv, namespace string) (kubernetesParityObservation, error) {
	if err := validateKubernetesParityNamespaceForLane(namespace); err != nil {
		return kubernetesParityObservation{}, err
	}
	out, err := runKubernetesParityKubectl(ctx, env, "get", "namespace", namespace, "-o", "json")
	if err != nil {
		if isKubernetesParityNotFound(err) {
			return kubernetesParityObservation{Exists: false}, nil
		}
		return kubernetesParityObservation{}, err
	}
	var doc struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Status struct {
			Phase string `json:"phase"`
		} `json:"status"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return kubernetesParityObservation{}, err
	}
	labels := map[string]string{}
	for _, key := range []string{"app.kubernetes.io/managed-by", "ramen.openudon.dev/lane"} {
		if value := doc.Metadata.Labels[key]; value != "" {
			labels[key] = value
		}
	}
	return kubernetesParityObservation{
		Exists: true,
		Name:   doc.Metadata.Name,
		Labels: labels,
		Phase:  strings.TrimSpace(doc.Status.Phase),
	}, nil
}

func observeKubernetesParityNamespaceAbsent(ctx context.Context, env kubernetesParityLiveEnv, namespace string) (kubernetesParityObservation, error) {
	var last kubernetesParityObservation
	deadline := time.Now().Add(45 * time.Second)
	for {
		observed, err := observeKubernetesParityNamespace(ctx, env, namespace)
		if err != nil {
			return kubernetesParityObservation{}, err
		}
		last = observed
		if !observed.Exists {
			return observed, nil
		}
		if time.Now().After(deadline) {
			return last, nil
		}
		select {
		case <-ctx.Done():
			return kubernetesParityObservation{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func observeKubernetesParityConfigMap(ctx context.Context, env kubernetesParityLiveEnv, namespace, name string) (kubernetesParityConfigMapObservation, error) {
	if err := validateKubernetesParityNamespaceForLane(namespace); err != nil {
		return kubernetesParityConfigMapObservation{}, err
	}
	lane, err := kubernetesParityLaneFromNamespace(namespace)
	if err != nil {
		return kubernetesParityConfigMapObservation{}, err
	}
	if err := validateKubernetesParityConfigMapName(name, lane); err != nil {
		return kubernetesParityConfigMapObservation{}, err
	}
	out, err := runKubernetesParityKubectl(ctx, env, "get", "configmap", name, "-n", namespace, "-o", "json")
	if err != nil {
		if isKubernetesParityNotFound(err) {
			return kubernetesParityConfigMapObservation{Exists: false}, nil
		}
		return kubernetesParityConfigMapObservation{}, err
	}
	var doc struct {
		Metadata struct {
			Name      string            `json:"name"`
			Namespace string            `json:"namespace"`
			Labels    map[string]string `json:"labels"`
		} `json:"metadata"`
		Data       map[string]string `json:"data"`
		BinaryData map[string]string `json:"binaryData"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return kubernetesParityConfigMapObservation{}, err
	}
	labels := map[string]string{}
	for _, key := range []string{"app.kubernetes.io/managed-by", "ramen.openudon.dev/lane"} {
		if value := doc.Metadata.Labels[key]; value != "" {
			labels[key] = value
		}
	}
	return kubernetesParityConfigMapObservation{
		Exists:     true,
		Namespace:  doc.Metadata.Namespace,
		Name:       doc.Metadata.Name,
		Labels:     labels,
		Data:       cloneStringMap(doc.Data),
		BinaryData: cloneStringMap(doc.BinaryData),
	}, nil
}

func observeKubernetesParityConfigMapAbsent(ctx context.Context, env kubernetesParityLiveEnv, namespace, name string) (kubernetesParityConfigMapObservation, error) {
	var last kubernetesParityConfigMapObservation
	deadline := time.Now().Add(45 * time.Second)
	for {
		observed, err := observeKubernetesParityConfigMap(ctx, env, namespace, name)
		if err != nil {
			return kubernetesParityConfigMapObservation{}, err
		}
		last = observed
		if !observed.Exists {
			return observed, nil
		}
		if time.Now().After(deadline) {
			return last, nil
		}
		select {
		case <-ctx.Done():
			return kubernetesParityConfigMapObservation{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func compareKubernetesParityObservations(t *testing.T, observations []kubernetesParityRuntimeObservation) kubernetesParityObservationComparison {
	t.Helper()
	if len(observations) != 3 {
		t.Fatalf("expected 3 runtime observations, got %d", len(observations))
	}
	expectedLabels := map[string]string{
		"app.kubernetes.io/managed-by": "ramen-parity",
		"ramen.openudon.dev/lane":      "k01",
	}
	for _, observation := range observations {
		if !observation.AfterApply.Exists {
			t.Fatalf("%s after_apply does not exist", observation.Runtime)
		}
		if !strings.HasPrefix(observation.AfterApply.Name, "ramen-parity-k01-") {
			t.Fatalf("%s after_apply name = %q", observation.Runtime, observation.AfterApply.Name)
		}
		if !reflect.DeepEqual(observation.AfterApply.Labels, expectedLabels) {
			t.Fatalf("%s after_apply labels = %#v, want %#v", observation.Runtime, observation.AfterApply.Labels, expectedLabels)
		}
		if observation.AfterApply.Phase != "Active" {
			t.Fatalf("%s after_apply phase = %q, want Active", observation.Runtime, observation.AfterApply.Phase)
		}
		if observation.NoOpPlan == nil || !observation.NoOpPlan.NoOp {
			t.Fatalf("%s no-op plan = %#v", observation.Runtime, observation.NoOpPlan)
		}
		if observation.AfterDestroy == nil || observation.AfterDestroy.Exists {
			t.Fatalf("%s after_destroy still exists: %#v", observation.Runtime, observation.AfterDestroy)
		}
	}
	return kubernetesParityObservationComparison{
		Matched: true,
		Fields:  []string{"metadata.labels", "status.phase", "no-op", "destroy.absent"},
	}
}

func compareKubernetesReadMissingParityObservations(t *testing.T, observations []kubernetesParityRuntimeObservation) kubernetesParityObservationComparison {
	t.Helper()
	if len(observations) != 3 {
		t.Fatalf("expected 3 runtime observations, got %d", len(observations))
	}
	expectedLabels := map[string]string{
		"app.kubernetes.io/managed-by": "ramen-parity",
		"ramen.openudon.dev/lane":      "k02",
	}
	for _, observation := range observations {
		if !observation.AfterApply.Exists {
			t.Fatalf("%s after_apply does not exist", observation.Runtime)
		}
		if !strings.HasPrefix(observation.AfterApply.Name, "ramen-parity-k02-") {
			t.Fatalf("%s after_apply name = %q", observation.Runtime, observation.AfterApply.Name)
		}
		if !reflect.DeepEqual(observation.AfterApply.Labels, expectedLabels) {
			t.Fatalf("%s after_apply labels = %#v, want %#v", observation.Runtime, observation.AfterApply.Labels, expectedLabels)
		}
		if observation.AfterApply.Phase != "Active" {
			t.Fatalf("%s after_apply phase = %q, want Active", observation.Runtime, observation.AfterApply.Phase)
		}
		if observation.AfterOutOfBandDelete == nil || observation.AfterOutOfBandDelete.Exists {
			t.Fatalf("%s after_out_of_band_delete = %#v, want absent", observation.Runtime, observation.AfterOutOfBandDelete)
		}
		if observation.ReadMissing == nil || !observation.ReadMissing.Missing || observation.ReadMissing.Classification != "missing" {
			t.Fatalf("%s read_missing = %#v, want missing classification", observation.Runtime, observation.ReadMissing)
		}
		if observation.AfterReadMissingCleanup == nil || observation.AfterReadMissingCleanup.Exists {
			t.Fatalf("%s after_read_missing_cleanup = %#v, want absent", observation.Runtime, observation.AfterReadMissingCleanup)
		}
	}
	return kubernetesParityObservationComparison{
		Matched: true,
		Fields:  []string{"metadata.labels", "out-of-band-delete.absent", "read-missing.classification", "cleanup.absent"},
	}
}

func compareKubernetesConfigMapParityObservations(t *testing.T, observations []kubernetesParityRuntimeObservation) kubernetesParityObservationComparison {
	t.Helper()
	if len(observations) != 3 {
		t.Fatalf("expected 3 runtime observations, got %d", len(observations))
	}
	expectedLabels := map[string]string{
		"app.kubernetes.io/managed-by": "ramen-parity",
		"ramen.openudon.dev/lane":      "k03",
	}
	expectedCreateData := map[string]string{"mode": "create", "owner": "ramen"}
	expectedCreateBinaryData := map[string]string{"payload": "cGFyaXR5LWNyZWF0ZQ=="}
	expectedUpdateData := map[string]string{"mode": "update", "owner": "ramen"}
	expectedUpdateBinaryData := map[string]string{"payload": "cGFyaXR5LXVwZGF0ZQ=="}
	for _, observation := range observations {
		if !observation.AfterApply.Exists {
			t.Fatalf("%s namespace after_apply does not exist", observation.Runtime)
		}
		if !strings.HasPrefix(observation.Namespace, "ramen-parity-k03-") {
			t.Fatalf("%s namespace = %q", observation.Runtime, observation.Namespace)
		}
		if observation.ConfigMapName != "ramen-parity-k03-config-map" {
			t.Fatalf("%s config_map_name = %q", observation.Runtime, observation.ConfigMapName)
		}
		assertKubernetesConfigMapObservation(t, observation.Runtime+" config_map_after_apply", observation.ConfigMapAfterApply, observation.Namespace, observation.ConfigMapName, expectedLabels, expectedCreateData, expectedCreateBinaryData)
		assertKubernetesConfigMapObservation(t, observation.Runtime+" config_map_after_update", observation.ConfigMapAfterUpdate, observation.Namespace, observation.ConfigMapName, expectedLabels, expectedUpdateData, expectedUpdateBinaryData)
		if observation.NoOpPlan == nil || !observation.NoOpPlan.NoOp {
			t.Fatalf("%s no-op plan = %#v", observation.Runtime, observation.NoOpPlan)
		}
		if observation.ConfigMapAfterDestroy == nil || observation.ConfigMapAfterDestroy.Exists {
			t.Fatalf("%s config_map_after_destroy still exists: %#v", observation.Runtime, observation.ConfigMapAfterDestroy)
		}
	}
	return kubernetesParityObservationComparison{
		Matched: true,
		Fields:  []string{"metadata.name", "metadata.namespace", "metadata.labels", "data", "binaryData", "update", "no-op", "destroy.absent"},
	}
}

func assertKubernetesConfigMapObservation(t *testing.T, label string, observation *kubernetesParityConfigMapObservation, namespace, name string, labels, data, binaryData map[string]string) {
	t.Helper()
	if observation == nil || !observation.Exists {
		t.Fatalf("%s = %#v, want existing ConfigMap", label, observation)
	}
	if observation.Namespace != namespace {
		t.Fatalf("%s namespace = %q, want %q", label, observation.Namespace, namespace)
	}
	if observation.Name != name {
		t.Fatalf("%s name = %q, want %q", label, observation.Name, name)
	}
	if !reflect.DeepEqual(observation.Labels, labels) {
		t.Fatalf("%s labels = %#v, want %#v", label, observation.Labels, labels)
	}
	if !reflect.DeepEqual(observation.Data, data) {
		t.Fatalf("%s data = %#v, want %#v", label, observation.Data, data)
	}
	if !reflect.DeepEqual(observation.BinaryData, binaryData) {
		t.Fatalf("%s binaryData = %#v, want %#v", label, observation.BinaryData, binaryData)
	}
}

func compareOrUpdateKubernetesParityRecording(t *testing.T, recording kubernetesParityLiveRecording, path string) {
	t.Helper()
	data, err := json.MarshalIndent(recording, "", "  ")
	if err != nil {
		t.Fatalf("marshal live recording: %v", err)
	}
	data = append(data, '\n')
	if os.Getenv(kubernetesParityRecordEnv) == "1" {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("update %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed parity recording %s: %v; rerun with %s=1 %s=1 after reviewing the local kind run", path, err, kubernetesParityEnv, kubernetesParityRecordEnv)
	}
	if !reflect.DeepEqual(normalizeKubernetesParityRecording(t, want), normalizeKubernetesParityRecording(t, data)) {
		t.Fatalf("live parity recording differs from %s; rerun with %s=1 %s=1 after reviewing the diff", path, kubernetesParityEnv, kubernetesParityRecordEnv)
	}
}

func normalizeKubernetesParityRecording(t *testing.T, data []byte) kubernetesParityLiveRecording {
	t.Helper()
	var recording kubernetesParityLiveRecording
	if err := json.Unmarshal(data, &recording); err != nil {
		t.Fatalf("decode parity recording: %v", err)
	}
	recording.RecordedAt = ""
	recording.Context = ""
	for i := range recording.Observations {
		if recording.Observations[i].NoOpPlan != nil {
			recording.Observations[i].NoOpPlan.ExitCode = 0
			recording.Observations[i].NoOpPlan.Summary = ""
		}
		if recording.Observations[i].ReadMissing != nil {
			recording.Observations[i].ReadMissing.ExitCode = 0
			recording.Observations[i].ReadMissing.Summary = ""
		}
	}
	return recording
}

func createKubernetesParityNamespace(ctx context.Context, env kubernetesParityLiveEnv, namespace string) error {
	lane, err := kubernetesParityLaneFromNamespace(namespace)
	if err != nil {
		return err
	}
	if err := validateKubernetesParityNamespace(namespace, lane); err != nil {
		return err
	}
	if _, err := runKubernetesParityKubectl(ctx, env, "create", "namespace", namespace); err != nil && !strings.Contains(strings.ToLower(err.Error()), "alreadyexists") && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		return err
	}
	_, err = runKubernetesParityKubectl(ctx, env,
		"label", "namespace", namespace,
		"app.kubernetes.io/managed-by=ramen-parity",
		"ramen.openudon.dev/lane="+lane,
		"--overwrite",
	)
	return err
}

func deleteKubernetesParityNamespaceIfExists(ctx context.Context, env kubernetesParityLiveEnv, namespace string) error {
	if err := validateKubernetesParityNamespaceForLane(namespace); err != nil {
		return err
	}
	if _, err := runKubernetesParityKubectl(ctx, env, "delete", "namespace", namespace, "--ignore-not-found=true", "--wait=true", "--timeout=60s"); err != nil {
		return err
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		_, err := runKubernetesParityKubectl(ctx, env, "get", "namespace", namespace, "-o", "json")
		if err != nil && isKubernetesParityNotFound(err) {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return err
			}
			return fmt.Errorf("namespace %s still exists after timeout", namespace)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

func runKubernetesParityKubectl(ctx context.Context, env kubernetesParityLiveEnv, args ...string) ([]byte, error) {
	fullArgs := []string{"--kubeconfig", env.kubeconfig, "--context", env.contextName}
	fullArgs = append(fullArgs, args...)
	cmd := osexec.CommandContext(ctx, env.kubectl, fullArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("kubectl %s: %w: %s", strings.Join(fullArgs, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func validateKubernetesParityNamespace(namespace, lane string) error {
	prefix := "ramen-parity-" + lane + "-"
	if !strings.HasPrefix(namespace, prefix) {
		return fmt.Errorf("namespace must use %s prefix", prefix)
	}
	if len(namespace) > 63 {
		return fmt.Errorf("namespace is too long")
	}
	for i, r := range namespace {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Errorf("namespace contains invalid character %q at offset %d", r, i)
	}
	if strings.HasPrefix(namespace, "-") || strings.HasSuffix(namespace, "-") {
		return fmt.Errorf("namespace must not start or end with '-'")
	}
	return nil
}

func validateKubernetesParityConfigMapName(name, lane string) error {
	prefix := "ramen-parity-" + lane + "-"
	if !strings.HasPrefix(name, prefix) {
		return fmt.Errorf("ConfigMap name must use %s prefix", prefix)
	}
	if len(name) > 253 {
		return fmt.Errorf("ConfigMap name is too long")
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			continue
		}
		return fmt.Errorf("ConfigMap name contains invalid character %q at offset %d", r, i)
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return fmt.Errorf("ConfigMap name must not start or end with '-' or '.'")
	}
	return nil
}

func validateKubernetesParityNamespaceForLane(namespace string) error {
	lane, err := kubernetesParityLaneFromNamespace(namespace)
	if err != nil {
		return err
	}
	return validateKubernetesParityNamespace(namespace, lane)
}

func kubernetesParityLaneFromNamespace(namespace string) (string, error) {
	trimmed := strings.TrimSpace(namespace)
	const prefix = "ramen-parity-"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", fmt.Errorf("namespace must use %s prefix", prefix)
	}
	rest := strings.TrimPrefix(trimmed, prefix)
	lane, _, ok := strings.Cut(rest, "-")
	if !ok || !slices.Contains(kubernetesParityLanes, lane) {
		return "", fmt.Errorf("namespace has unsupported Kubernetes parity lane")
	}
	return lane, nil
}

func prepareKubernetesParityLiveEnv(t *testing.T, kubectl string) kubernetesParityLiveEnv {
	t.Helper()
	kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
	out, err := osexec.Command(kubectl, "config", "view", "--raw", "--flatten").CombinedOutput()
	if err != nil {
		t.Fatalf("flatten kubeconfig for %s=1: %v: %s", kubernetesParityEnv, err, strings.TrimSpace(string(out)))
	}
	if len(out) == 0 {
		t.Fatalf("flatten kubeconfig for %s=1 produced no output", kubernetesParityEnv)
	}
	if err := os.WriteFile(kubeconfig, out, 0o600); err != nil {
		t.Fatalf("write flattened kubeconfig: %v", err)
	}
	contextOut, err := osexec.Command(kubectl, "--kubeconfig", kubeconfig, "config", "current-context").CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl current-context from flattened kubeconfig failed: %v: %s", err, strings.TrimSpace(string(contextOut)))
	}
	contextName := strings.TrimSpace(string(contextOut))
	if !strings.HasPrefix(contextName, "kind-") {
		t.Fatalf("refusing Kubernetes provider parity against non-kind context %q", contextName)
	}
	return kubernetesParityLiveEnv{kubectl: kubectl, contextName: contextName, kubeconfig: kubeconfig}
}

func requireKubernetesParityTool(t *testing.T, envName, defaultName string) string {
	t.Helper()
	if envName != "" {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			if _, err := os.Stat(value); err != nil {
				t.Skipf("%s=%s is not usable when %s=1: %v", envName, value, kubernetesParityEnv, err)
			}
			return value
		}
	}
	path, err := osexec.LookPath(defaultName)
	if err != nil {
		if envName != "" {
			t.Skipf("%s is required when %s=1; set %s to an executable path: %v", defaultName, kubernetesParityEnv, envName, err)
		}
		t.Skipf("%s is required when %s=1: %v", defaultName, kubernetesParityEnv, err)
	}
	return path
}

func kubernetesParityFailure(runtime, class string, err error) kubernetesParityRuntimeResult {
	return kubernetesParityRuntimeResult{Failure: &kubernetesParityRuntimeFailure{Runtime: runtime, Class: class, Message: err.Error()}}
}

func isKubernetesParityNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "notfound") || strings.Contains(msg, "not found")
}

func copyFixtureFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func renderKubernetesParityProject(src, dst, namespace, sourcePath string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	lane, err := kubernetesParityLaneFromNamespace(namespace)
	if err != nil {
		return err
	}
	if err := validateKubernetesParityNamespace(namespace, lane); err != nil {
		return err
	}
	rendered := strings.ReplaceAll(string(data), "ramen-parity-"+lane+"-namespace", namespace)
	if strings.TrimSpace(sourcePath) != "" {
		rendered = strings.ReplaceAll(rendered, "../openapi/core.json", filepath.ToSlash(sourcePath))
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(rendered), 0o644)
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func lastNonEmptyLine(value string) string {
	lines := strings.Split(value, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func assertKubernetesK01PlanFixture(t *testing.T) {
	t.Helper()
	assertKubernetesRamenPlanFixture(t, "k01")
}

func assertKubernetesK02PlanFixture(t *testing.T) {
	t.Helper()
	assertKubernetesRamenPlanFixture(t, "k02")
}

func assertKubernetesK03PlanFixture(t *testing.T) {
	t.Helper()
	assertKubernetesRamenPlanFixture(t, "k03")
	assertKubernetesRamenPlanFixturePath(t, "K03 update", filepath.Join(kubernetesParityFixtureRoot, "k03", "ramen", "project.update.uws.yaml"))
}

func assertKubernetesRamenPlanFixture(t *testing.T, lane string) {
	t.Helper()
	assertKubernetesRamenPlanFixturePath(t, strings.ToUpper(lane), filepath.Join(kubernetesParityFixtureRoot, lane, "ramen", "project.uws.yaml"))
}

func assertKubernetesRamenPlanFixturePath(t *testing.T, label, projectPath string) {
	t.Helper()
	result, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: projectPath,
		StatePath:   filepath.Join(t.TempDir(), "state.db"),
	})
	if err != nil {
		t.Fatalf("build %s Ramen fixture plan: %v", label, err)
	}
	if result.Plan.Errored {
		t.Fatalf("%s Ramen fixture plan errored: %#v", label, result.Plan.Diagnostics)
	}
	if result.Plan.Summary.Create != 1 || len(result.Plan.Resources) != 1 {
		t.Fatalf("%s Ramen fixture plan summary = %#v resources=%d", label, result.Plan.Summary, len(result.Plan.Resources))
	}
}
