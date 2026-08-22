package run

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/state"
	"github.com/OpenUdon/uws/browserauthentication"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestRunBrowserArtifactsAreValidatedApprovedAndCapabilityGated(t *testing.T) {
	root := t.TempDir()
	docPath := writeRunBrowserTestUWS(t, root)
	statePath := filepath.Join(root, "state.db")
	preview, err := Execute(context.Background(), Options{DocumentPath: docPath, StatePath: statePath, Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.BrowserArtifacts) != 2 || preview.ApprovalDigest == "" {
		t.Fatalf("browser preview = %#v", preview)
	}
	for _, artifact := range preview.BrowserArtifacts {
		if artifact.Digest == "" || artifact.Profile == "" || artifact.Reference == "" {
			t.Fatalf("artifact = %#v", artifact)
		}
	}
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		if req.Capabilities.Protocol != "uws" {
			t.Fatalf("protocol = %q", req.Capabilities.Protocol)
		}
		for _, feature := range []string{
			executor.FeatureBrowserContexts, executor.FeatureBrowserScalarOutputs,
			executor.FeatureBrowserNamedSession, executor.FeatureBrowserAuthentication,
			executor.FeatureBrowserMutationApproval, executor.FeatureBrowserAuthenticationApproval,
		} {
			if !hasRunFeature(req.Capabilities.Features, feature) {
				t.Fatalf("requirements %v missing %s", req.Capabilities.Features, feature)
			}
		}
		return executor.Result{Success: true}, nil
	}}
	result, err := Execute(context.Background(), Options{
		DocumentPath: docPath, StatePath: statePath, ApprovalDigest: preview.ApprovalDigest, Executor: mock,
	})
	if err != nil || result.Summary.Executed != 1 || mock.RequestCount() != 1 {
		t.Fatalf("browser run result=%#v requests=%d err=%v", result, mock.RequestCount(), err)
	}
	encoded, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(encoded), `"browser_artifacts"`) {
		t.Fatalf("browser artifacts missing from result JSON: %s err=%v", encoded, err)
	}
}

func TestRunBrowserArtifactChangesInvalidateApprovalAndPreHandoffRecheck(t *testing.T) {
	root := t.TempDir()
	docPath := writeRunBrowserTestUWS(t, root)
	preview, err := Execute(context.Background(), Options{DocumentPath: docPath, StatePath: filepath.Join(root, "state.db"), Check: true})
	if err != nil {
		t.Fatal(err)
	}
	browserPath := filepath.Join(root, "browser.yaml")
	browser := readRunTestFile(t, browserPath)
	writeRunTestFile(t, browserPath, strings.Replace(browser, "Reviewed status", "Changed status", 1))
	mock := &executor.MockExecutor{}
	_, err = Execute(context.Background(), Options{
		DocumentPath: docPath, StatePath: filepath.Join(root, "state.db"), ApprovalDigest: preview.ApprovalDigest, Executor: mock,
	})
	if err == nil || !strings.Contains(err.Error(), "run.approval_mismatch") || mock.RequestCount() != 0 {
		t.Fatalf("changed browser artifact retained approval: err=%v requests=%d", err, mock.RequestCount())
	}

	doc, _, err := loadDocument(docPath)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := loadBrowserRunContract(docPath, doc)
	if err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(root, "authentication.yaml")
	auth := readRunTestFile(t, authPath)
	writeRunTestFile(t, authPath, strings.Replace(auth, "Reviewed login", "Changed login", 1))
	if err := recheckBrowserRunContract(docPath, doc, contract); err == nil || !strings.Contains(err.Error(), "run.browser_artifact_changed") {
		t.Fatalf("pre-handoff recheck error = %v", err)
	}
}

func TestRunBrowserRequirementsFailBeforeExecutorInvocation(t *testing.T) {
	root := t.TempDir()
	docPath := writeRunBrowserTestUWS(t, root)
	exec := &runWithoutBrowserExecutor{}
	result, err := Execute(context.Background(), Options{
		DocumentPath: docPath, StatePath: filepath.Join(root, "state.db"), AutoApprove: true, Executor: exec,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Failed != 1 || exec.called != 0 || len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "browser.") {
		t.Fatalf("result=%#v executor calls=%d", result, exec.called)
	}
}

type runWithoutBrowserExecutor struct{ called int }

func (e *runWithoutBrowserExecutor) Capabilities() executor.CapabilityDescriptor {
	return executor.CapabilityDescriptor{Protocols: []string{"uws"}, Features: []string{executor.FeatureIdempotency, executor.FeatureProgressEvents}}
}

func (e *runWithoutBrowserExecutor) Execute(context.Context, executor.Request) (executor.Result, error) {
	e.called++
	return executor.Result{Success: true}, nil
}

func TestRunCheckModeDoesNotCallExecutorOrWriteState(t *testing.T) {
	root := t.TempDir()
	docPath := writeRunTestUWS(t, root)
	mock := &executor.MockExecutor{}
	result, err := Execute(context.Background(), Options{DocumentPath: docPath, StatePath: filepath.Join(root, "state.db"), Targets: []string{"a", "b"}, Check: true, Executor: mock})
	if err != nil {
		t.Fatalf("check run: %v", err)
	}
	if !result.Check || result.Summary.Skipped != 2 || result.ApprovalDigest == "" || mock.RequestCount() != 0 {
		t.Fatalf("result=%#v requests=%d", result, mock.RequestCount())
	}
	if store, err := state.OpenReadOnly(context.Background(), filepath.Join(root, "state.db")); err != nil || store != nil {
		t.Fatalf("state should not exist after check mode: store=%#v err=%v", store, err)
	}
}

func TestRunRequiresApprovalAndRecordsHistory(t *testing.T) {
	root := t.TempDir()
	docPath := writeRunTestUWS(t, root)
	statePath := filepath.Join(root, "state.db")
	preview, err := Execute(context.Background(), Options{DocumentPath: docPath, StatePath: statePath, Targets: []string{"b", "a"}, Check: true})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	_, err = Execute(context.Background(), Options{DocumentPath: docPath, StatePath: statePath, Targets: []string{"a"}, Executor: &executor.MockExecutor{}})
	if err == nil || !strings.Contains(err.Error(), "run approval required") {
		t.Fatalf("approval error = %v", err)
	}
	result, err := Execute(context.Background(), Options{DocumentPath: docPath, StatePath: statePath, Targets: []string{"b", "a"}, ApprovalDigest: preview.ApprovalDigest, Executor: &executor.MockExecutor{}})
	if err != nil {
		t.Fatalf("approved run: %v", err)
	}
	if result.Summary.Executed != 2 || result.RunID == 0 || len(result.Executed) != 2 {
		t.Fatalf("result = %#v", result)
	}
	store, err := state.OpenReadOnly(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	runs, err := store.ListRuns(context.Background(), "")
	if err != nil || len(runs) != 1 || runs[0].Command != "run" || runs[0].Status != "completed" {
		t.Fatalf("runs=%#v err=%v", runs, err)
	}
	events, err := store.ListRunEvents(context.Background(), result.RunID)
	if err != nil || len(events) == 0 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	asyncRecords, err := store.ListAsyncEvidence(context.Background(), state.AsyncEvidenceFilter{RunID: result.RunID})
	if err != nil {
		t.Fatalf("async evidence: %v", err)
	}
	if len(asyncRecords) < 6 {
		t.Fatalf("async evidence records = %#v", asyncRecords)
	}
	kinds := map[string]int{}
	for _, record := range asyncRecords {
		kinds[record.RecordKind]++
	}
	if kinds["execution_request"] != 2 || kinds["execution_response"] != 2 || kinds["status_observation"] < 2 {
		t.Fatalf("async evidence kinds = %#v records=%#v", kinds, asyncRecords)
	}
	_ = store.Close()
}

func TestRunLoadsDocumentThroughUWSSchemaValidation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "run.uws.json")
	if err := os.WriteFile(path, []byte(`{
  "uws": "1.4.0",
  "info": {"title": "schema_invalid", "version": "1.0.0"},
  "operations": [
    {"operationId": "do", "x-uws-operation-profile": "ramen-run-test"}
  ],
  "workflows": [
    {"workflowId": "main", "steps": [{"stepId": "do", "operationRef": "do"}]}
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Execute(context.Background(), Options{DocumentPath: path, StatePath: filepath.Join(root, "state.db"), Check: true})
	if err == nil || !strings.Contains(err.Error(), "run.document_invalid") || !strings.Contains(err.Error(), "jsonschema validation failed") {
		t.Fatalf("expected UWS schema validation failure, got %v", err)
	}
}

func writeRunTestUWS(t *testing.T, dir string) string {
	t.Helper()
	doc := &uws1.Document{
		UWS:  "1.4.0",
		Info: &uws1.Info{Title: "run_fixture", Version: "1.0.0"},
		Operations: []*uws1.Operation{{
			OperationID: "do",
			Request:     map[string]any{"x-test": true},
			Extensions:  map[string]any{uws1.ExtensionOperationProfile: "ramen-run-test"},
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID: "main",
			Type:       uws1.WorkflowTypeSequence,
			Steps: []*uws1.Step{{
				StepID:       "do",
				OperationRef: "do",
			}},
		}},
	}
	data, err := uwsconvert.MarshalJSONIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "run.uws.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeRunBrowserTestUWS(t *testing.T, dir string) string {
	t.Helper()
	writeRunTestFile(t, filepath.Join(dir, "browser.yaml"), `profile: uws.browser.1.7
info: {title: Reviewed status, origin: https://example.test, loginStateRequired: true}
observationKind: accessibility_snapshot
evidence: {learnedAt: "2026-08-20T00:00:00Z", source: reviewed_synthetic_fixture}
confidence: high
expiresAfter: P30D
verification: {lastVerifiedAt: "2026-08-20T00:00:00Z", successfulRuns: 1}
contexts:
  detail_frame: {kind: frame, parent: main, origin: https://example.test, path: /detail, name: Detail}
actions:
  change_status:
    parameters:
      type: object
      required: [item]
      properties: {item: {type: string}}
    sequence:
      - navigate: /status
      - wait_for: {locator: {role: status, name: Ready}, context: detail_frame}
    outputs:
      enabled: {type: boolean, source: a11y, locator: {role: status, name: Enabled}}
    sideEffects: [state_change]
    confirmationPolicy: {required: true, prompt: "Approve changing {{item}}?"}
`)
	writeRunTestFile(t, filepath.Join(dir, "authentication.yaml"), `profile: uws.browser-authentication.1.1
info:
  title: Reviewed login
  applicationOrigins: [https://example.test]
  authenticationOrigins: [https://login.example.test]
observationKind: accessibility_snapshot
evidence: {learnedAt: "2026-08-20T00:00:00Z", source: reviewed_synthetic_fixture}
confidence: high
expiresAfter: P30D
verification: {lastVerifiedAt: "2026-08-20T00:00:00Z", successfulRuns: 1}
contexts:
  idp_popup: {kind: popup, parent: main, origin: https://login.example.test}
credentialSlots:
  username: {kind: identifier}
flows:
  login:
    sequence:
      - click: {locator: {role: button, name: Continue}, opensContext: idp_popup}
      - type_credential: {locator: {role: textbox, name: Username}, slot: username, context: idp_popup}
      - wait_for: {locator: {role: heading, name: Dashboard}}
    effects: [establishes_session]
    success: {origin: https://example.test, locator: {role: heading, name: Dashboard}}
`)
	timeout := 120.0
	authOperation := &uws1.Operation{
		OperationID:              "authenticate",
		OperationExecutionFields: uws1.OperationExecutionFields{Timeout: &timeout},
		Extensions:               map[string]any{uws1.ExtensionOperationProfile: browserauthentication.ContextCallProfileName},
	}
	if err := browserauthentication.SetAuthenticationExtension(&authOperation.Extensions, &browserauthentication.OperationAuthentication{
		Profile: "authentication.yaml", Flow: "login", Session: "member_portal",
		CredentialBindings: map[string]string{"username": "member_username"},
	}); err != nil {
		t.Fatal(err)
	}
	browserOperation := &uws1.Operation{
		OperationID: "change_status_uws", SourceDescription: "browser", SourceOperationID: "change_status",
		Request:                  map[string]any{"body": map[string]any{"item": "reviewed-item"}},
		OperationExecutionFields: uws1.OperationExecutionFields{DependsOn: []string{"authenticate"}},
	}
	if err := browserauthentication.SetSessionExtension(&browserOperation.Extensions, &browserauthentication.OperationSession{Session: "member_portal"}); err != nil {
		t.Fatal(err)
	}
	doc := &uws1.Document{
		UWS: "1.9.0", Info: &uws1.Info{Title: "run_browser_fixture", Version: "1.0.0"},
		SourceDescriptions: []*uws1.SourceDescription{{Name: "browser", URL: "browser.yaml", Type: uws1.SourceDescriptionTypeBrowserProfile}},
		Operations:         []*uws1.Operation{authOperation, browserOperation},
		Workflows:          []*uws1.Workflow{{WorkflowID: "main", Type: uws1.WorkflowTypeSequence, Steps: []*uws1.Step{{StepID: "change", OperationRef: "change_status_uws"}}}},
	}
	data, err := uwsconvert.MarshalJSONIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "browser-run.uws.json")
	writeRunTestFile(t, path, string(data))
	return path
}

func writeRunTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readRunTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func hasRunFeature(features []string, want string) bool {
	for _, feature := range features {
		if feature == want {
			return true
		}
	}
	return false
}
