package corpus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/ramen/apply"
	"github.com/OpenUdon/ramen/executor"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/reconcile"
	"github.com/OpenUdon/ramen/state"
)

const (
	kubernetesLiveEnv         = "RAMEN_K8S_LIVE"
	kubernetesRecordUpdateEnv = "RAMEN_K8S_RECORD_UPDATE"

	kubernetesNamespaceEquivalenceRoot  = "testdata/equivalence/kubernetes/namespace_v1"
	kubernetesNamespacedEquivalenceRoot = "testdata/equivalence/kubernetes/namespaced_v1"
	kubernetesM22Namespace              = "ramen-live-m22"
)

type kubernetesNamespaceScenario struct {
	name          string
	projectPath   string
	recordingPath string
	address       string
	namespace     string
}

var (
	kubernetesDestroyScenario = kubernetesNamespaceScenario{
		name:          "destroy",
		projectPath:   filepath.ToSlash(filepath.Join(kubernetesNamespaceEquivalenceRoot, "destroy", "project.uws.yaml")),
		recordingPath: filepath.ToSlash(filepath.Join(kubernetesNamespaceEquivalenceRoot, "destroy.recording.json")),
		address:       "kubernetes_namespace_v1.m21_destroy",
		namespace:     "ramen-live-m21-destroy",
	}
	kubernetesMissingScenario = kubernetesNamespaceScenario{
		name:          "missing",
		projectPath:   filepath.ToSlash(filepath.Join(kubernetesNamespaceEquivalenceRoot, "missing", "project.uws.yaml")),
		recordingPath: filepath.ToSlash(filepath.Join(kubernetesNamespaceEquivalenceRoot, "missing.recording.json")),
		address:       "kubernetes_namespace_v1.m21_missing",
		namespace:     "ramen-live-m21-missing",
	}
)

type kubernetesNamespacedScenario struct {
	name          string
	projectPath   string
	updatePath    string
	recordingPath string
	address       string
	resourceType  string
	resourceName  string
	namespace     string
}

var (
	kubernetesConfigMapScenario = kubernetesNamespacedScenario{
		name:          "config_map",
		projectPath:   filepath.ToSlash(filepath.Join(kubernetesNamespacedEquivalenceRoot, "config_map", "create", "project.uws.yaml")),
		updatePath:    filepath.ToSlash(filepath.Join(kubernetesNamespacedEquivalenceRoot, "config_map", "update", "project.uws.yaml")),
		recordingPath: filepath.ToSlash(filepath.Join(kubernetesNamespacedEquivalenceRoot, "config_map.recording.json")),
		address:       "kubernetes_config_map_v1.m22_config_map",
		resourceType:  "configmap",
		resourceName:  "ramen-live-m22-config-map",
		namespace:     kubernetesM22Namespace,
	}
	kubernetesServiceAccountScenario = kubernetesNamespacedScenario{
		name:          "service_account",
		projectPath:   filepath.ToSlash(filepath.Join(kubernetesNamespacedEquivalenceRoot, "service_account", "project.uws.yaml")),
		recordingPath: filepath.ToSlash(filepath.Join(kubernetesNamespacedEquivalenceRoot, "service_account.recording.json")),
		address:       "kubernetes_service_account_v1.m22_service_account",
		resourceType:  "serviceaccount",
		resourceName:  "ramen-live-m22-service-account",
		namespace:     kubernetesM22Namespace,
	}
	kubernetesRoleScenario = kubernetesNamespacedScenario{
		name:          "role",
		projectPath:   filepath.ToSlash(filepath.Join(kubernetesNamespacedEquivalenceRoot, "role", "project.uws.yaml")),
		recordingPath: filepath.ToSlash(filepath.Join(kubernetesNamespacedEquivalenceRoot, "role.recording.json")),
		address:       "kubernetes_role_v1.m22_role",
		resourceType:  "role",
		resourceName:  "ramen-live-m22-role",
		namespace:     kubernetesM22Namespace,
	}
)

func TestKubernetesNamespaceReplayEquivalence(t *testing.T) {
	t.Run(kubernetesDestroyScenario.name, func(t *testing.T) {
		recorder := loadKubernetesRecording(t, kubernetesDestroyScenario.recordingPath)
		runKubernetesDestroyScenario(t, context.Background(), kubernetesDestroyScenario, recorder, nil)
	})
	t.Run(kubernetesMissingScenario.name, func(t *testing.T) {
		recorder := loadKubernetesRecording(t, kubernetesMissingScenario.recordingPath)
		runKubernetesMissingScenario(t, context.Background(), kubernetesMissingScenario, recorder, nil)
	})
}

func TestKubernetesNamespaceLiveEquivalence(t *testing.T) {
	if os.Getenv(kubernetesLiveEnv) != "1" {
		t.Skipf("set %s=1 to run the local kind-backed Kubernetes live equivalence lane", kubernetesLiveEnv)
	}
	kubectl, contextName := requireKindKubectl(t)
	live := &kubectlKubernetesExecutor{kubectl: kubectl, contextName: contextName}
	for _, scenario := range []kubernetesNamespaceScenario{kubernetesDestroyScenario, kubernetesMissingScenario} {
		if err := validateSafeNamespace(scenario.namespace); err != nil {
			t.Fatalf("unsafe scenario namespace %q: %v", scenario.namespace, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run(kubernetesDestroyScenario.name, func(t *testing.T) {
		if err := live.deleteNamespaceIfExists(ctx, kubernetesDestroyScenario.namespace); err != nil {
			t.Fatalf("pre-cleanup %s: %v", kubernetesDestroyScenario.namespace, err)
		}
		defer func() {
			if err := live.deleteNamespaceIfExists(context.Background(), kubernetesDestroyScenario.namespace); err != nil {
				t.Logf("cleanup %s: %v", kubernetesDestroyScenario.namespace, err)
			}
		}()
		recorder := &executor.RecordedExecutor{Recorder: live}
		runKubernetesDestroyScenario(t, ctx, kubernetesDestroyScenario, recorder, nil)
		compareOrUpdateKubernetesRecording(t, recorder, kubernetesDestroyScenario.recordingPath)
	})

	t.Run(kubernetesMissingScenario.name, func(t *testing.T) {
		if err := live.deleteNamespaceIfExists(ctx, kubernetesMissingScenario.namespace); err != nil {
			t.Fatalf("pre-cleanup %s: %v", kubernetesMissingScenario.namespace, err)
		}
		defer func() {
			if err := live.deleteNamespaceIfExists(context.Background(), kubernetesMissingScenario.namespace); err != nil {
				t.Logf("cleanup %s: %v", kubernetesMissingScenario.namespace, err)
			}
		}()
		recorder := &executor.RecordedExecutor{Recorder: live}
		runKubernetesMissingScenario(t, ctx, kubernetesMissingScenario, recorder, live.deleteNamespaceIfExists)
		compareOrUpdateKubernetesRecording(t, recorder, kubernetesMissingScenario.recordingPath)
	})
}

func TestKubernetesNamespacedResourceReplayEquivalence(t *testing.T) {
	t.Run(kubernetesConfigMapScenario.name, func(t *testing.T) {
		recorder := loadKubernetesRecording(t, kubernetesConfigMapScenario.recordingPath)
		runKubernetesConfigMapScenario(t, context.Background(), kubernetesConfigMapScenario, recorder)
	})
	t.Run(kubernetesServiceAccountScenario.name, func(t *testing.T) {
		recorder := loadKubernetesRecording(t, kubernetesServiceAccountScenario.recordingPath)
		runKubernetesNamespacedMissingScenario(t, context.Background(), kubernetesServiceAccountScenario, recorder, nil)
	})
	t.Run(kubernetesRoleScenario.name, func(t *testing.T) {
		recorder := loadKubernetesRecording(t, kubernetesRoleScenario.recordingPath)
		runKubernetesNamespacedDestroyScenario(t, context.Background(), kubernetesRoleScenario, recorder)
	})
}

func TestKubernetesNamespacedResourceLiveEquivalence(t *testing.T) {
	if os.Getenv(kubernetesLiveEnv) != "1" {
		t.Skipf("set %s=1 to run the local kind-backed Kubernetes live equivalence lane", kubernetesLiveEnv)
	}
	kubectl, contextName := requireKindKubectl(t)
	live := &kubectlKubernetesExecutor{kubectl: kubectl, contextName: contextName}
	for _, scenario := range []kubernetesNamespacedScenario{kubernetesConfigMapScenario, kubernetesServiceAccountScenario, kubernetesRoleScenario} {
		if err := validateSafeNamespacedScenario(scenario); err != nil {
			t.Fatalf("unsafe scenario %s: %v", scenario.name, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := live.recreateM22Namespace(ctx); err != nil {
		t.Fatalf("setup namespace %s: %v", kubernetesM22Namespace, err)
	}
	defer func() {
		if err := live.deleteNamespaceIfExists(context.Background(), kubernetesM22Namespace); err != nil {
			t.Logf("cleanup namespace %s: %v", kubernetesM22Namespace, err)
		}
	}()

	t.Run(kubernetesConfigMapScenario.name, func(t *testing.T) {
		recorder := &executor.RecordedExecutor{Recorder: live}
		runKubernetesConfigMapScenario(t, ctx, kubernetesConfigMapScenario, recorder)
		compareOrUpdateKubernetesRecording(t, recorder, kubernetesConfigMapScenario.recordingPath)
	})
	t.Run(kubernetesServiceAccountScenario.name, func(t *testing.T) {
		recorder := &executor.RecordedExecutor{Recorder: live}
		runKubernetesNamespacedMissingScenario(t, ctx, kubernetesServiceAccountScenario, recorder, live.deleteNamespacedResourceIfExists)
		compareOrUpdateKubernetesRecording(t, recorder, kubernetesServiceAccountScenario.recordingPath)
	})
	t.Run(kubernetesRoleScenario.name, func(t *testing.T) {
		recorder := &executor.RecordedExecutor{Recorder: live}
		runKubernetesNamespacedDestroyScenario(t, ctx, kubernetesRoleScenario, recorder)
		compareOrUpdateKubernetesRecording(t, recorder, kubernetesRoleScenario.recordingPath)
	})
}

func runKubernetesDestroyScenario(t *testing.T, ctx context.Context, scenario kubernetesNamespaceScenario, exec executor.Executor, _ func(context.Context, string) error) {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "state.db")
	applyResult, err := apply.Apply(ctx, apply.Options{
		ProjectPath: scenario.projectPath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("apply returned error: %v", err)
	}
	if applyResult.Summary.Create != 1 || applyResult.Summary.Failed != 0 {
		t.Fatalf("apply summary = %#v", applyResult.Summary)
	}

	refreshResult, err := reconcile.Refresh(ctx, reconcile.Options{
		ProjectPath: scenario.projectPath,
		StatePath:   statePath,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("refresh returned error: %v", err)
	}
	if refreshResult.Summary.Read != 1 || refreshResult.Summary.Unchanged != 1 || refreshResult.Summary.Failed != 0 {
		t.Fatalf("refresh summary = %#v", refreshResult.Summary)
	}
	assertKubernetesNoOpPlan(t, ctx, scenario, statePath)

	destroyResult, err := reconcile.Destroy(ctx, reconcile.Options{
		ProjectPath: scenario.projectPath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("destroy returned error: %v", err)
	}
	if destroyResult.Summary.Delete != 1 || destroyResult.Summary.Failed != 0 {
		t.Fatalf("destroy summary = %#v", destroyResult.Summary)
	}
	assertKubernetesCurrentResource(t, ctx, statePath, scenario.address, false)
}

func runKubernetesMissingScenario(t *testing.T, ctx context.Context, scenario kubernetesNamespaceScenario, exec executor.Executor, deleteOutOfBand func(context.Context, string) error) {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "state.db")
	applyResult, err := apply.Apply(ctx, apply.Options{
		ProjectPath: scenario.projectPath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("apply returned error: %v", err)
	}
	if applyResult.Summary.Create != 1 || applyResult.Summary.Failed != 0 {
		t.Fatalf("apply summary = %#v", applyResult.Summary)
	}
	assertKubernetesNoOpPlan(t, ctx, scenario, statePath)
	if deleteOutOfBand != nil {
		if err := deleteOutOfBand(ctx, scenario.namespace); err != nil {
			t.Fatalf("out-of-band delete %s: %v", scenario.namespace, err)
		}
	}

	refreshResult, err := reconcile.Refresh(ctx, reconcile.Options{
		ProjectPath: scenario.projectPath,
		StatePath:   statePath,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("refresh returned error: %v", err)
	}
	if refreshResult.Summary.Read != 1 || refreshResult.Summary.Missing != 1 || refreshResult.Summary.Failed != 0 {
		t.Fatalf("refresh summary = %#v", refreshResult.Summary)
	}
	assertKubernetesCurrentResource(t, ctx, statePath, scenario.address, true)
	assertKubernetesRevision(t, ctx, statePath, scenario.address, "refresh_missing")
}

func runKubernetesConfigMapScenario(t *testing.T, ctx context.Context, scenario kubernetesNamespacedScenario, exec executor.Executor) {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "state.db")
	applyResult, err := apply.Apply(ctx, apply.Options{
		ProjectPath: scenario.projectPath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("apply create returned error: %v", err)
	}
	if applyResult.Summary.Create != 1 || applyResult.Summary.Failed != 0 {
		t.Fatalf("apply create summary = %#v", applyResult.Summary)
	}
	refreshResult, err := reconcile.Refresh(ctx, reconcile.Options{
		ProjectPath: scenario.projectPath,
		StatePath:   statePath,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("refresh create returned error: %v", err)
	}
	if refreshResult.Summary.Read != 1 || refreshResult.Summary.Unchanged != 1 || refreshResult.Summary.Failed != 0 {
		t.Fatalf("refresh create summary = %#v", refreshResult.Summary)
	}
	assertKubernetesNoOpProjectPlan(t, ctx, scenario.projectPath, statePath)

	updateResult, err := apply.Apply(ctx, apply.Options{
		ProjectPath: scenario.updatePath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("apply update returned error: %v", err)
	}
	if updateResult.Summary.Update != 1 || updateResult.Summary.Failed != 0 {
		t.Fatalf("apply update summary = %#v", updateResult.Summary)
	}
	refreshResult, err = reconcile.Refresh(ctx, reconcile.Options{
		ProjectPath: scenario.updatePath,
		StatePath:   statePath,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("refresh update returned error: %v", err)
	}
	if refreshResult.Summary.Read != 1 || refreshResult.Summary.Unchanged != 1 || refreshResult.Summary.Failed != 0 {
		t.Fatalf("refresh update summary = %#v", refreshResult.Summary)
	}
	assertKubernetesNoOpProjectPlan(t, ctx, scenario.updatePath, statePath)

	destroyResult, err := reconcile.Destroy(ctx, reconcile.Options{
		ProjectPath: scenario.updatePath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("destroy returned error: %v", err)
	}
	if destroyResult.Summary.Delete != 1 || destroyResult.Summary.Failed != 0 {
		t.Fatalf("destroy summary = %#v", destroyResult.Summary)
	}
	assertKubernetesCurrentResource(t, ctx, statePath, scenario.address, false)
}

func runKubernetesNamespacedDestroyScenario(t *testing.T, ctx context.Context, scenario kubernetesNamespacedScenario, exec executor.Executor) {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "state.db")
	applyResult, err := apply.Apply(ctx, apply.Options{
		ProjectPath: scenario.projectPath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("apply returned error: %v", err)
	}
	if applyResult.Summary.Create != 1 || applyResult.Summary.Failed != 0 {
		t.Fatalf("apply summary = %#v", applyResult.Summary)
	}
	refreshResult, err := reconcile.Refresh(ctx, reconcile.Options{
		ProjectPath: scenario.projectPath,
		StatePath:   statePath,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("refresh returned error: %v", err)
	}
	if refreshResult.Summary.Read != 1 || refreshResult.Summary.Unchanged != 1 || refreshResult.Summary.Failed != 0 {
		t.Fatalf("refresh summary = %#v", refreshResult.Summary)
	}
	assertKubernetesNoOpProjectPlan(t, ctx, scenario.projectPath, statePath)
	destroyResult, err := reconcile.Destroy(ctx, reconcile.Options{
		ProjectPath: scenario.projectPath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("destroy returned error: %v", err)
	}
	if destroyResult.Summary.Delete != 1 || destroyResult.Summary.Failed != 0 {
		t.Fatalf("destroy summary = %#v", destroyResult.Summary)
	}
	assertKubernetesCurrentResource(t, ctx, statePath, scenario.address, false)
}

func runKubernetesNamespacedMissingScenario(t *testing.T, ctx context.Context, scenario kubernetesNamespacedScenario, exec executor.Executor, deleteOutOfBand func(context.Context, kubernetesNamespacedScenario) error) {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "state.db")
	applyResult, err := apply.Apply(ctx, apply.Options{
		ProjectPath: scenario.projectPath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("apply returned error: %v", err)
	}
	if applyResult.Summary.Create != 1 || applyResult.Summary.Failed != 0 {
		t.Fatalf("apply summary = %#v", applyResult.Summary)
	}
	assertKubernetesNoOpProjectPlan(t, ctx, scenario.projectPath, statePath)
	if deleteOutOfBand != nil {
		if err := deleteOutOfBand(ctx, scenario); err != nil {
			t.Fatalf("out-of-band delete %s/%s: %v", scenario.namespace, scenario.resourceName, err)
		}
	}
	refreshResult, err := reconcile.Refresh(ctx, reconcile.Options{
		ProjectPath: scenario.projectPath,
		StatePath:   statePath,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("refresh missing returned error: %v", err)
	}
	if refreshResult.Summary.Read != 1 || refreshResult.Summary.Missing != 1 || refreshResult.Summary.Failed != 0 {
		t.Fatalf("refresh missing summary = %#v", refreshResult.Summary)
	}
	assertKubernetesCurrentResource(t, ctx, statePath, scenario.address, true)
	assertKubernetesRevision(t, ctx, statePath, scenario.address, "refresh_missing")
	destroyResult, err := reconcile.Destroy(ctx, reconcile.Options{
		ProjectPath: scenario.projectPath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    exec,
	})
	if err != nil {
		t.Fatalf("destroy missing returned error: %v", err)
	}
	if destroyResult.Summary.Delete != 1 || destroyResult.Summary.Failed != 0 {
		t.Fatalf("destroy missing summary = %#v", destroyResult.Summary)
	}
	assertKubernetesCurrentResource(t, ctx, statePath, scenario.address, false)
}

func assertKubernetesNoOpPlan(t *testing.T, ctx context.Context, scenario kubernetesNamespaceScenario, statePath string) {
	t.Helper()
	assertKubernetesNoOpProjectPlan(t, ctx, scenario.projectPath, statePath)
}

func assertKubernetesNoOpProjectPlan(t *testing.T, ctx context.Context, projectPath, statePath string) {
	t.Helper()
	planResult, err := tfplan.Build(ctx, tfplan.Options{ProjectPath: projectPath, StatePath: statePath})
	if err != nil {
		t.Fatalf("plan returned error: %v", err)
	}
	if planResult.Plan.Errored || planResult.Plan.Summary.NoOp != 1 || len(planResult.Plan.Resources) != 1 {
		t.Fatalf("plan = errored:%v summary:%#v resources:%d diagnostics:%#v", planResult.Plan.Errored, planResult.Plan.Summary, len(planResult.Plan.Resources), planResult.Plan.Diagnostics)
	}
}

func assertKubernetesCurrentResource(t *testing.T, ctx context.Context, statePath, address string, wantPresent bool) {
	t.Helper()
	store, err := state.Open(ctx, statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	defer store.Close()
	snap, err := store.CurrentResource(ctx, address)
	if err != nil {
		t.Fatalf("current resource %s: %v", address, err)
	}
	if (snap != nil) != wantPresent {
		t.Fatalf("current resource present=%v, want %v: %#v", snap != nil, wantPresent, snap)
	}
}

func assertKubernetesRevision(t *testing.T, ctx context.Context, statePath, address, action string) {
	t.Helper()
	store, err := state.Open(ctx, statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	defer store.Close()
	revisions, err := store.ListRevisions(ctx, address)
	if err != nil {
		t.Fatalf("list revisions %s: %v", address, err)
	}
	for _, revision := range revisions {
		if revision.Action == action {
			return
		}
	}
	t.Fatalf("revision %s not found in %#v", action, revisions)
}

func loadKubernetesRecording(t *testing.T, path string) *executor.RecordedExecutor {
	t.Helper()
	recorder, err := executor.LoadRecording(path)
	if err != nil {
		t.Fatalf("load recording %s: %v", path, err)
	}
	return recorder
}

func compareOrUpdateKubernetesRecording(t *testing.T, recorder *executor.RecordedExecutor, path string) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "recording.json")
	if err := recorder.Save(tmp); err != nil {
		t.Fatalf("save recording: %v", err)
	}
	got, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read saved recording: %v", err)
	}
	if os.Getenv(kubernetesRecordUpdateEnv) == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("update recording %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed recording %s: %v; rerun with %s=1 %s=1 after reviewing a local kind cluster", path, err, kubernetesLiveEnv, kubernetesRecordUpdateEnv)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("live recording for %s differs from committed fixture; rerun with %s=1 %s=1 to refresh after reviewing the diff", path, kubernetesLiveEnv, kubernetesRecordUpdateEnv)
	}
}

type kubectlKubernetesExecutor struct {
	kubectl     string
	contextName string
}

func (k *kubectlKubernetesExecutor) Capabilities() executor.CapabilityDescriptor {
	return executor.CapabilityDescriptor{
		Protocols:   []string{"openapi"},
		AuthSchemes: []string{"kubeconfig"},
		Features: []string{
			executor.FeatureOutputIdentity,
			executor.FeatureOutputComputed,
			executor.FeatureMissingEvidence,
			executor.FeatureProgressEvents,
			executor.FeatureIdempotency,
		},
	}
}

func (k *kubectlKubernetesExecutor) Execute(ctx context.Context, req executor.Request) (executor.Result, error) {
	if err := executor.EnsureSupported(k, req); err != nil {
		return executor.Result{}, err
	}
	executor.Emit(req, "started", "kubectl Kubernetes executor started", nil)
	identity, err := kubernetesIdentityFromRequest(req)
	if err != nil {
		return executor.Result{}, err
	}
	var result executor.Result
	switch req.Action.Mapping.OperationID {
	case "createCoreV1Namespace":
		name := identity.name
		if err := validateSafeNamespace(name); err != nil {
			return executor.Result{}, err
		}
		if _, err := k.run(ctx, "create", "namespace", name, "-o", "json"); err != nil {
			return executor.Result{}, err
		}
		result = namespaceExecutorResult(req, name, false)
	case "readCoreV1Namespace":
		name := identity.name
		if err := validateSafeNamespace(name); err != nil {
			return executor.Result{}, err
		}
		out, err := k.run(ctx, "get", "namespace", name, "-o", "json")
		if err != nil {
			if isKubectlNotFound(err) {
				result = namespaceExecutorResult(req, name, true)
				break
			}
			return executor.Result{}, err
		}
		result = namespaceExecutorResult(req, name, false)
		if phase := namespacePhase(out); phase != "" {
			result.Computed["status.phase"] = phase
		}
	case "deleteCoreV1Namespace":
		name := identity.name
		if err := validateSafeNamespace(name); err != nil {
			return executor.Result{}, err
		}
		if _, err := k.run(ctx, "delete", "namespace", name, "--wait=true", "--timeout=60s"); err != nil {
			if !isKubectlNotFound(err) {
				return executor.Result{}, err
			}
		}
		if err := k.waitNamespaceAbsent(ctx, name); err != nil {
			return executor.Result{}, err
		}
		result = namespaceExecutorResult(req, name, false)
	case "createCoreV1NamespacedConfigMap", "replaceCoreV1NamespacedConfigMap",
		"createCoreV1NamespacedServiceAccount", "replaceCoreV1NamespacedServiceAccount",
		"createRbacAuthorizationV1NamespacedRole", "replaceRbacAuthorizationV1NamespacedRole":
		if err := validateSafeNamespacedIdentity(identity); err != nil {
			return executor.Result{}, err
		}
		manifest, err := manifestFromRequest(req, identity)
		if err != nil {
			return executor.Result{}, err
		}
		out, err := k.applyManifest(ctx, manifest)
		if err != nil {
			return executor.Result{}, err
		}
		result = namespacedExecutorResult(req, identity, out, false)
	case "readCoreV1NamespacedConfigMap", "readCoreV1NamespacedServiceAccount", "readRbacAuthorizationV1NamespacedRole":
		if err := validateSafeNamespacedIdentity(identity); err != nil {
			return executor.Result{}, err
		}
		out, err := k.getNamespacedResource(ctx, identity)
		if err != nil {
			if isKubectlNotFound(err) {
				result = namespacedExecutorResult(req, identity, nil, true)
				break
			}
			return executor.Result{}, err
		}
		result = namespacedExecutorResult(req, identity, out, false)
	case "deleteCoreV1NamespacedConfigMap", "deleteCoreV1NamespacedServiceAccount", "deleteRbacAuthorizationV1NamespacedRole":
		if err := validateSafeNamespacedIdentity(identity); err != nil {
			return executor.Result{}, err
		}
		if err := k.deleteNamespacedResourceIfExists(ctx, kubernetesNamespacedScenario{resourceType: identity.resourceType, resourceName: identity.name, namespace: identity.namespace}); err != nil {
			return executor.Result{}, err
		}
		result = namespacedExecutorResult(req, identity, nil, false)
	default:
		return executor.Result{}, fmt.Errorf("unsupported Kubernetes operation %q", req.Action.Mapping.OperationID)
	}
	executor.Emit(req, "finished", "kubectl Kubernetes executor finished", nil)
	return result, nil
}

func (k *kubectlKubernetesExecutor) deleteNamespaceIfExists(ctx context.Context, name string) error {
	if err := validateSafeNamespace(name); err != nil {
		return err
	}
	if _, err := k.run(ctx, "delete", "namespace", name, "--ignore-not-found=true", "--wait=true", "--timeout=60s"); err != nil {
		return err
	}
	return k.waitNamespaceAbsent(ctx, name)
}

func (k *kubectlKubernetesExecutor) recreateM22Namespace(ctx context.Context) error {
	if err := k.deleteNamespaceIfExists(ctx, kubernetesM22Namespace); err != nil {
		return err
	}
	if _, err := k.run(ctx, "create", "namespace", kubernetesM22Namespace); err != nil {
		return err
	}
	return nil
}

func (k *kubectlKubernetesExecutor) deleteNamespacedResourceIfExists(ctx context.Context, scenario kubernetesNamespacedScenario) error {
	if err := validateSafeNamespacedScenario(scenario); err != nil {
		return err
	}
	_, err := k.run(ctx, "delete", scenario.resourceType, scenario.resourceName, "-n", scenario.namespace, "--ignore-not-found=true", "--wait=true", "--timeout=60s")
	return err
}

func (k *kubectlKubernetesExecutor) getNamespacedResource(ctx context.Context, identity kubernetesResourceIdentity) ([]byte, error) {
	return k.run(ctx, "get", identity.resourceType, identity.name, "-n", identity.namespace, "-o", "json")
}

func (k *kubectlKubernetesExecutor) applyManifest(ctx context.Context, manifest map[string]any) ([]byte, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	kubectl := strings.TrimSpace(k.kubectl)
	if kubectl == "" {
		kubectl = "kubectl"
	}
	fullArgs := []string{}
	if strings.TrimSpace(k.contextName) != "" {
		fullArgs = append(fullArgs, "--context", k.contextName)
	}
	fullArgs = append(fullArgs, "apply", "-f", "-", "-o", "json")
	cmd := osexec.CommandContext(ctx, kubectl, fullArgs...)
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("kubectl %s: %w: %s", strings.Join(fullArgs, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func (k *kubectlKubernetesExecutor) waitNamespaceAbsent(ctx context.Context, name string) error {
	deadline := time.Now().Add(60 * time.Second)
	var lastErr error
	for {
		_, err := k.run(ctx, "get", "namespace", name, "-o", "json")
		if err != nil {
			if isKubectlNotFound(err) {
				return nil
			}
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return lastErr
			}
			return fmt.Errorf("namespace %s still exists after timeout", name)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

func (k *kubectlKubernetesExecutor) run(ctx context.Context, args ...string) ([]byte, error) {
	kubectl := strings.TrimSpace(k.kubectl)
	if kubectl == "" {
		kubectl = "kubectl"
	}
	fullArgs := []string{}
	if strings.TrimSpace(k.contextName) != "" {
		fullArgs = append(fullArgs, "--context", k.contextName)
	}
	fullArgs = append(fullArgs, args...)
	cmd := osexec.CommandContext(ctx, kubectl, fullArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("kubectl %s: %w: %s", strings.Join(fullArgs, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func namespaceNameFromRequest(req executor.Request) (string, error) {
	if req.Document == nil || len(req.Document.Operations) == 0 || req.Document.Operations[0] == nil {
		return "", fmt.Errorf("executor request has no UWS operation")
	}
	body, _ := req.Document.Operations[0].Request["body"].(map[string]any)
	if name := stringFromAny(body["name"]); name != "" {
		return name, nil
	}
	metadata, _ := body["metadata"].(map[string]any)
	if name := stringFromAny(metadata["name"]); name != "" {
		return name, nil
	}
	return "", fmt.Errorf("executor request for %s has no namespace name", req.Action.Address)
}

type kubernetesResourceIdentity struct {
	resourceType string
	apiVersion   string
	kind         string
	name         string
	namespace    string
}

func kubernetesIdentityFromRequest(req executor.Request) (kubernetesResourceIdentity, error) {
	resourceType, apiVersion, kind, err := kubernetesOperationResource(req.Action.Mapping.OperationID)
	if err != nil {
		return kubernetesResourceIdentity{}, err
	}
	name, err := namespaceNameFromRequest(req)
	if err != nil {
		return kubernetesResourceIdentity{}, err
	}
	identity := kubernetesResourceIdentity{resourceType: resourceType, apiVersion: apiVersion, kind: kind, name: name}
	if resourceType == "namespace" {
		return identity, nil
	}
	body, _ := req.Document.Operations[0].Request["body"].(map[string]any)
	if namespace := stringFromAny(body["namespace"]); namespace != "" {
		identity.namespace = namespace
	}
	metadata, _ := body["metadata"].(map[string]any)
	if namespace := stringFromAny(metadata["namespace"]); namespace != "" {
		identity.namespace = namespace
	}
	if identity.namespace == "" {
		return kubernetesResourceIdentity{}, fmt.Errorf("executor request for %s has no namespace", req.Action.Address)
	}
	return identity, nil
}

func kubernetesOperationResource(operationID string) (string, string, string, error) {
	switch operationID {
	case "createCoreV1Namespace", "readCoreV1Namespace", "deleteCoreV1Namespace":
		return "namespace", "v1", "Namespace", nil
	case "createCoreV1NamespacedConfigMap", "readCoreV1NamespacedConfigMap", "replaceCoreV1NamespacedConfigMap", "deleteCoreV1NamespacedConfigMap":
		return "configmap", "v1", "ConfigMap", nil
	case "createCoreV1NamespacedServiceAccount", "readCoreV1NamespacedServiceAccount", "replaceCoreV1NamespacedServiceAccount", "deleteCoreV1NamespacedServiceAccount":
		return "serviceaccount", "v1", "ServiceAccount", nil
	case "createRbacAuthorizationV1NamespacedRole", "readRbacAuthorizationV1NamespacedRole", "replaceRbacAuthorizationV1NamespacedRole", "deleteRbacAuthorizationV1NamespacedRole":
		return "role", "rbac.authorization.k8s.io/v1", "Role", nil
	default:
		return "", "", "", fmt.Errorf("unsupported Kubernetes operation %q", operationID)
	}
}

func namespaceExecutorResult(req executor.Request, name string, missing bool) executor.Result {
	result := executor.Result{
		Address:   req.Action.Address,
		Operation: req.Action.Mapping.OperationID,
		Success:   true,
		Missing:   missing,
	}
	if !missing {
		result.Identity = map[string]any{"namespace_name": name}
		result.Computed = map[string]any{"metadata.name": name, "status.phase": "Active"}
	}
	return result
}

func namespacedExecutorResult(req executor.Request, identity kubernetesResourceIdentity, data []byte, missing bool) executor.Result {
	result := executor.Result{
		Address:   req.Action.Address,
		Operation: req.Action.Mapping.OperationID,
		Success:   true,
		Missing:   missing,
	}
	if missing {
		return result
	}
	result.Identity = map[string]any{"name": identity.name, "namespace": identity.namespace}
	result.Computed = map[string]any{
		"metadata.name":      identity.name,
		"metadata.namespace": identity.namespace,
	}
	var doc map[string]any
	if len(data) > 0 && json.Unmarshal(data, &doc) == nil {
		if metadata, _ := doc["metadata"].(map[string]any); len(metadata) > 0 {
			if name := stringFromAny(metadata["name"]); name != "" {
				result.Identity["name"] = name
				result.Computed["metadata.name"] = name
			}
			if namespace := stringFromAny(metadata["namespace"]); namespace != "" {
				result.Identity["namespace"] = namespace
				result.Computed["metadata.namespace"] = namespace
			}
			if labels, ok := metadata["labels"]; ok {
				result.Computed["metadata.labels"] = labels
			}
			if annotations, ok := metadata["annotations"]; ok {
				result.Computed["metadata.annotations"] = annotations
			}
		}
		for _, key := range []string{"data", "binaryData", "automountServiceAccountToken", "rules"} {
			if value, ok := doc[key]; ok {
				result.Computed[key] = value
			}
		}
	}
	return result
}

func manifestFromRequest(req executor.Request, identity kubernetesResourceIdentity) (map[string]any, error) {
	if req.Document == nil || len(req.Document.Operations) == 0 || req.Document.Operations[0] == nil {
		return nil, fmt.Errorf("executor request has no UWS operation")
	}
	body, _ := req.Document.Operations[0].Request["body"].(map[string]any)
	manifest := cloneAnyMap(body)
	delete(manifest, "name")
	delete(manifest, "namespace")
	manifest["apiVersion"] = identity.apiVersion
	manifest["kind"] = identity.kind
	metadata, _ := manifest["metadata"].(map[string]any)
	if metadata == nil {
		metadata = map[string]any{}
		manifest["metadata"] = metadata
	}
	metadata["name"] = identity.name
	metadata["namespace"] = identity.namespace
	return manifest, nil
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	data, err := json.Marshal(in)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func namespacePhase(data []byte) string {
	var doc struct {
		Status struct {
			Phase string `json:"phase"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return ""
	}
	return strings.TrimSpace(doc.Status.Phase)
}

func validateSafeNamespace(name string) error {
	if !strings.HasPrefix(name, "ramen-live-m21-") && name != kubernetesM22Namespace {
		return fmt.Errorf("namespace must use ramen-live-m21- prefix or equal %s", kubernetesM22Namespace)
	}
	if len(name) > 63 {
		return fmt.Errorf("namespace is too long")
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Errorf("namespace contains invalid character %q at offset %d", r, i)
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return fmt.Errorf("namespace must not start or end with '-'")
	}
	return nil
}

func validateSafeNamespacedScenario(scenario kubernetesNamespacedScenario) error {
	identity := kubernetesResourceIdentity{
		resourceType: scenario.resourceType,
		name:         scenario.resourceName,
		namespace:    scenario.namespace,
	}
	return validateSafeNamespacedIdentity(identity)
}

func validateSafeNamespacedIdentity(identity kubernetesResourceIdentity) error {
	if identity.namespace != kubernetesM22Namespace {
		return fmt.Errorf("namespaced resource namespace must equal %s", kubernetesM22Namespace)
	}
	if err := validateSafeNamespace(identity.namespace); err != nil {
		return err
	}
	switch identity.resourceType {
	case "configmap", "serviceaccount", "role":
	default:
		return fmt.Errorf("unsupported Kubernetes resource type %q", identity.resourceType)
	}
	if !strings.HasPrefix(identity.name, "ramen-live-m22-") {
		return fmt.Errorf("resource name must use ramen-live-m22- prefix")
	}
	if len(identity.name) > 63 {
		return fmt.Errorf("resource name is too long")
	}
	for i, r := range identity.name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Errorf("resource name contains invalid character %q at offset %d", r, i)
	}
	if strings.HasPrefix(identity.name, "-") || strings.HasSuffix(identity.name, "-") {
		return fmt.Errorf("resource name must not start or end with '-'")
	}
	return nil
}

func requireKindKubectl(t *testing.T) (string, string) {
	t.Helper()
	kubectl, err := osexec.LookPath("kubectl")
	if err != nil {
		t.Fatalf("kubectl is required when %s=1: %v", kubernetesLiveEnv, err)
	}
	out, err := osexec.Command(kubectl, "config", "current-context").CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl current context failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	contextName := strings.TrimSpace(string(out))
	if !strings.HasPrefix(contextName, "kind-") {
		t.Fatalf("refusing Kubernetes live test against non-kind context %q", contextName)
	}
	exec := &kubectlKubernetesExecutor{kubectl: kubectl, contextName: contextName}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := exec.run(ctx, "get", "namespace", "default", "-o", "name"); err != nil {
		t.Fatalf("kind context %q is not ready: %v", contextName, err)
	}
	if contextName != "kind-ramen-m21" && contextName != "kind-ramen-m22" {
		t.Logf("using kind context %q; documented defaults are kind-ramen-m21 and kind-ramen-m22", contextName)
	}
	return kubectl, contextName
}

func isKubectlNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "notfound") || strings.Contains(msg, "not found")
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}
