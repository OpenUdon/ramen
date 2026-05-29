//go:build udon

package corpus

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/ramen/apply"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/executor/udon"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/reconcile"
)

func runKubernetesParityRamenRuntime(ctx context.Context, t *testing.T, env kubernetesParityLiveEnv, _ string) kubernetesParityRuntimeResult {
	t.Helper()
	runtimeName := "ramen"
	namespace := "ramen-parity-k01-" + runtimeName
	workDir := filepath.Join(t.TempDir(), runtimeName)
	projectPath := filepath.Join(workDir, "ramen", "project.uws.yaml")
	openAPIPath := filepath.Join(workDir, "ramen", "openapi", "core.json")
	serverURL, stopProxy, err := startKubernetesParityProxy(ctx, t, env)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "fixture", err)
	}
	defer stopProxy()
	if err := renderKubernetesParityOpenAPI(filepath.Join(kubernetesParityFixtureRoot, "k01", "openapi", "core.json"), openAPIPath, serverURL); err != nil {
		return kubernetesParityFailure(runtimeName, "fixture", err)
	}
	if err := renderKubernetesParityProject(filepath.Join(kubernetesParityFixtureRoot, "k01", "ramen", "project.uws.yaml"), projectPath, namespace, "openapi/core.json"); err != nil {
		return kubernetesParityFailure(runtimeName, "fixture", err)
	}
	statePath := filepath.Join(workDir, "state.db")
	udonExecutor := udon.Executor{
		OutputDir: filepath.Join(workDir, "udon"),
		OutputProjector: func(projectorCtx context.Context, req executor.Request, _ string) (executor.Result, error) {
			result := executor.Result{
				Address:   req.Action.Address,
				Operation: req.Action.Mapping.OperationID,
				Success:   true,
			}
			if req.Action.Action == "delete" {
				return result, nil
			}
			observed, err := observeKubernetesParityNamespace(projectorCtx, env, namespace)
			if err != nil {
				if isKubernetesParityNotFound(err) {
					result.Missing = true
					return result, nil
				}
				return executor.Result{}, err
			}
			if !observed.Exists {
				result.Missing = true
				return result, nil
			}
			result.Identity = map[string]any{"namespace_name": observed.Name}
			result.Computed = map[string]any{
				"metadata.name":   observed.Name,
				"metadata.labels": observed.Labels,
				"status.phase":    observed.Phase,
			}
			return result, nil
		},
	}
	applyResult, err := apply.Apply(ctx, apply.Options{
		ProjectPath: projectPath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    udonExecutor,
	})
	if err != nil {
		return kubernetesParityFailure(runtimeName, "apply", fmt.Errorf("%w; errors=%v feedback=%v", err, applyResultErrors(applyResult), applyResultFeedbackMessages(applyResult)))
	}
	if applyResult.Summary.Create != 1 || applyResult.Summary.Failed != 0 {
		return kubernetesParityFailure(runtimeName, "apply", errUnexpectedKubernetesParitySummary("apply", applyResult.Summary))
	}
	afterApply, err := observeKubernetesParityNamespace(ctx, env, namespace)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "observe", err)
	}
	planResult, err := tfplan.Build(ctx, tfplan.Options{ProjectPath: projectPath, StatePath: statePath})
	if err != nil {
		return kubernetesParityFailure(runtimeName, "plan", err)
	}
	noOp := !planResult.Plan.Errored && planResult.Plan.Summary.NoOp == 1 && len(planResult.Plan.Resources) == 1
	refreshResult, err := reconcile.Refresh(ctx, reconcile.Options{
		ProjectPath: projectPath,
		StatePath:   statePath,
		Executor:    udonExecutor,
	})
	if err != nil {
		return kubernetesParityFailure(runtimeName, "refresh", fmt.Errorf("%w; feedback=%v", err, reconcileFeedbackMessages(refreshResult)))
	}
	if refreshResult.Summary.Read != 1 || refreshResult.Summary.Unchanged != 1 || refreshResult.Summary.Failed != 0 {
		return kubernetesParityFailure(runtimeName, "refresh", errUnexpectedKubernetesParitySummary("refresh", refreshResult.Summary))
	}
	destroyResult, err := reconcile.Destroy(ctx, reconcile.Options{
		ProjectPath: projectPath,
		StatePath:   statePath,
		AutoApprove: true,
		Executor:    udonExecutor,
	})
	if err != nil {
		return kubernetesParityFailure(runtimeName, "destroy", fmt.Errorf("%w; feedback=%v", err, reconcileFeedbackMessages(destroyResult)))
	}
	if destroyResult.Summary.Delete != 1 || destroyResult.Summary.Failed != 0 {
		return kubernetesParityFailure(runtimeName, "destroy", errUnexpectedKubernetesParitySummary("destroy", destroyResult.Summary))
	}
	afterDestroy, err := observeKubernetesParityNamespaceAbsent(ctx, env, namespace)
	if err != nil {
		return kubernetesParityFailure(runtimeName, "observe", err)
	}
	return kubernetesParityRuntimeResult{Observation: kubernetesParityRuntimeObservation{
		Runtime:      runtimeName,
		Namespace:    namespace,
		AfterApply:   afterApply,
		NoOpPlan:     kubernetesParityNoOpObservation{NoOp: noOp, Summary: fmt.Sprintf("%+v", planResult.Plan.Summary)},
		AfterDestroy: afterDestroy,
	}}
}

func startKubernetesParityProxy(ctx context.Context, t *testing.T, env kubernetesParityLiveEnv) (string, func(), error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return "", nil, err
	}
	proxyCtx, cancel := context.WithCancel(ctx)
	logPath := filepath.Join(t.TempDir(), "kubectl-proxy.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		cancel()
		return "", nil, err
	}
	args := []string{"--context", env.contextName}
	if strings.TrimSpace(env.kubeconfig) != "" {
		args = append(args, "--kubeconfig", env.kubeconfig)
	}
	args = append(args,
		"proxy",
		"--address=127.0.0.1",
		fmt.Sprintf("--port=%d", port),
		"--accept-hosts=^127\\.0\\.0\\.1$,^localhost$",
	)
	cmd := osexec.CommandContext(proxyCtx, env.kubectl, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		cancel()
		return "", nil, err
	}
	waitCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		_ = logFile.Close()
		waitCh <- err
	}()
	stop := func() {
		cancel()
		_ = cmd.Process.Kill()
		<-waitCh
	}
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case err := <-waitCh:
			cancel()
			logData, _ := os.ReadFile(logPath)
			return "", nil, fmt.Errorf("kubectl proxy exited before readiness: %v: %s", err, strings.TrimSpace(string(logData)))
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/version", nil)
		if err == nil {
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 500 {
					return serverURL, stop, nil
				}
			}
		}
		if time.Now().After(deadline) {
			stop()
			logData, _ := os.ReadFile(logPath)
			return "", nil, fmt.Errorf("kubectl proxy did not become ready: %s", strings.TrimSpace(string(logData)))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func renderKubernetesParityOpenAPI(srcPath, dstPath, serverURL string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return err
	}
	doc["host"] = parsed.Host
	doc["schemes"] = []any{parsed.Scheme}
	if path := strings.TrimRight(parsed.EscapedPath(), "/"); path != "" {
		doc["basePath"] = path
	} else {
		delete(doc, "basePath")
	}
	rendered, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dstPath, append(rendered, '\n'), 0o644)
}

func errUnexpectedKubernetesParitySummary(phase string, summary any) error {
	return &kubernetesParitySummaryError{phase: phase, summary: summary}
}

func applyResultErrors(result *apply.Result) []string {
	if result == nil {
		return nil
	}
	return result.Errors
}

func applyResultFeedbackMessages(result *apply.Result) []string {
	if result == nil {
		return nil
	}
	var messages []string
	for _, feedback := range result.Feedback {
		messages = append(messages, feedback.Messages...)
		if feedback.ErrorClass != "" {
			messages = append(messages, feedback.ErrorClass)
		}
	}
	return messages
}

func reconcileFeedbackMessages(result *reconcile.Result) []string {
	if result == nil {
		return nil
	}
	var messages []string
	for _, feedback := range result.Feedback {
		messages = append(messages, feedback.Messages...)
		if feedback.ErrorClass != "" {
			messages = append(messages, feedback.ErrorClass)
		}
	}
	return messages
}

type kubernetesParitySummaryError struct {
	phase   string
	summary any
}

func (e *kubernetesParitySummaryError) Error() string {
	return fmt.Sprintf("%s summary did not match K01 parity expectations: %#v", e.phase, e.summary)
}
