package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/internal/asyncrecord"
	"github.com/OpenUdon/ramen/internal/browsercontract"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/state"
	"github.com/OpenUdon/ramen/tfmapping"
	"github.com/OpenUdon/uws/browserauthentication"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
	"github.com/OpenUdon/uws/validation"
)

func TestBuildBrowserActionDocumentSelectsMinimumUWSVersion(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		session string
		auth    *tfplan.AuthenticationProjection
		want    string
	}{
		{name: "browser 1.5", profile: "uws.browser.1.5", want: "1.5.0"},
		{name: "external session", profile: "uws.browser.1.5", session: "external", want: "1.7.0"},
		{name: "browser 1.6", profile: "uws.browser.1.6", want: "1.8.0"},
		{name: "browser 1.7", profile: "uws.browser.1.7", want: "1.9.0"},
		{name: "authentication 1.0", profile: "uws.browser.1.5", session: "member", auth: &tfplan.AuthenticationProjection{
			UWSOperationID: "authenticate", CallProfile: browserauthentication.CallProfileName,
			ProfileVersion: browserauthentication.ProfileName, ProfileRef: "authentication.yaml",
			Flow: "login", TimeoutSeconds: 120, Session: "member",
		}, want: "1.7.0"},
		{name: "authentication 1.1", profile: "uws.browser.1.5", session: "member", auth: &tfplan.AuthenticationProjection{
			UWSOperationID: "authenticate", CallProfile: browserauthentication.ContextCallProfileName,
			ProfileVersion: browserauthentication.ContextProfileName, ProfileRef: "authentication.yaml",
			Flow: "login", TimeoutSeconds: 120, Session: "member",
		}, want: "1.8.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := browserResourceForApplyTest(test.profile)
			resource.Mapping.Browser.Session = test.session
			resource.Mapping.Browser.ExternalSession = test.session != "" && test.auth == nil
			resource.Mapping.Browser.Authentication = test.auth
			doc, err := BuildActionDocument(resource, nil)
			if err != nil {
				t.Fatal(err)
			}
			if doc.UWS != test.want {
				t.Fatalf("UWS = %s, want %s", doc.UWS, test.want)
			}
		})
	}
}

func TestBuildBrowserActionDocumentPreservesApprovedContract(t *testing.T) {
	resource := browserResourceForApplyTest("uws.browser.1.7")
	resource.Action = "update"
	resource.Mapping.Purpose = "update"
	resource.Mapping.OperationID = "change_status"
	resource.Mapping.RequestBindings = []project.RequestBinding{{OperationRole: "update", OperationID: "change_status", Path: "item", RequestPath: "item", Location: "body"}}
	browser := resource.Mapping.Browser
	browser.ActionID = "change_status"
	browser.UWSOperationID = "change_status_uws"
	browser.Request = map[string]any{"body": map[string]any{"item": "reviewed-default"}}
	browser.Session = "member_portal"
	browser.SideEffects = []string{"state_change"}
	browser.Confirmation = tfplan.ConfirmationProjection{Required: true, Prompt: "Approve change?"}
	browser.Outputs = []tfplan.OutputProjection{
		{Name: "enabled", Type: "boolean", Source: "a11y"},
		{Name: "status", Type: "string", Source: "a11y"},
	}
	browser.Contexts = []tfplan.ContextProjection{{ID: "detail", Kind: "frame", Parent: "main", Origin: "https://example.test"}}
	browser.Authentication = &tfplan.AuthenticationProjection{
		UWSOperationID: "authenticate", CallProfile: browserauthentication.ContextCallProfileName,
		ProfileVersion: browserauthentication.ContextProfileName, ProfileRef: "authentication.yaml",
		ProfilePath: "/reviewed/authentication.yaml", ProfileDigest: "sha256:auth",
		Flow: "login", TimeoutSeconds: 120, Session: "member_portal",
		CredentialBindings: []tfplan.CredentialBindingProjection{{Slot: "username", Binding: "member_username"}},
		Contexts:           []tfplan.ContextProjection{{ID: "idp_popup", Kind: "popup", Parent: "main", Origin: "https://login.example.test"}},
	}

	doc, err := BuildActionDocumentWithBindings(resource, nil, map[string]any{"item": "planned-item", "password": "do-not-pass"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc.UWS != "1.9.0" || len(doc.Operations) != 2 {
		t.Fatalf("document = %#v", doc)
	}
	authOperation, browserOperation := doc.Operations[0], doc.Operations[1]
	if authOperation.OperationID != "authenticate" || authOperation.ExtensionProfile() != browserauthentication.ContextCallProfileName || len(browserOperation.DependsOn) != 1 || browserOperation.DependsOn[0] != "authenticate" {
		t.Fatalf("authentication dependency = %#v browser=%#v", authOperation, browserOperation)
	}
	auth, ok, err := browserauthentication.ReadAuthenticationExtension(authOperation.Extensions)
	if err != nil || !ok || auth.Profile != "authentication.yaml" || auth.Flow != "login" || auth.CredentialBindings["username"] != "member_username" {
		t.Fatalf("authentication extension = %#v ok=%t err=%v", auth, ok, err)
	}
	session, ok, err := browserauthentication.ReadSessionExtension(browserOperation.Extensions)
	if err != nil || !ok || session.Session != "member_portal" {
		t.Fatalf("session extension = %#v ok=%t err=%v", session, ok, err)
	}
	if body := browserOperation.Request["body"].(map[string]any); body["item"] != "planned-item" {
		t.Fatalf("native binding did not overlay browser parameters: %#v", browserOperation.Request)
	}
	step := doc.Workflows[0].Steps[0]
	if step.Body["item"] != "planned-item" || step.Outputs["enabled"] != "$response.body.enabled" || browserOperation.Outputs["status"] != "$response.body.status" {
		t.Fatalf("browser step lowering = %#v operation outputs=%#v", step, browserOperation.Outputs)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "do-not-pass") || strings.Contains(string(data), "/reviewed/authentication.yaml") || strings.Contains(string(data), "/reviewed/browser.yaml") {
		t.Fatalf("runtime/private data leaked into generated document: %s", data)
	}
	encoded, err := uwsconvert.MarshalJSONIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	documentPath := filepath.Join(t.TempDir(), "browser-action.uws.json")
	writeApplyTestFile(t, documentPath, string(encoded))
	if _, err := validation.ValidateDocumentFile(documentPath); err != nil {
		t.Fatalf("generated browser UWS schema validation: %v", err)
	}

	action := executorAction(resource)
	requirements := RequirementsForResource(resource, action, executor.RuntimeHints{})
	for _, feature := range []string{
		executor.FeatureBrowserContexts, executor.FeatureBrowserScalarOutputs,
		executor.FeatureBrowserNamedSession, executor.FeatureBrowserAuthentication,
		executor.FeatureBrowserMutationApproval, executor.FeatureBrowserAuthenticationApproval,
	} {
		if !containsApplyTest(requirements.Features, feature) {
			t.Fatalf("requirements %v missing %s", requirements.Features, feature)
		}
	}
	if err := executor.EnsureSupported(&executor.MockExecutor{}, executor.Request{Action: action, Capabilities: requirements}); err != nil {
		t.Fatalf("mock browser support: %v", err)
	}
	limited := &limitedApplyExecutor{}
	if err := executor.EnsureSupported(limited, executor.Request{Action: action, Capabilities: requirements}); err == nil || len(limited.requests) != 0 {
		t.Fatalf("limited executor did not fail before execution: err=%v requests=%d", err, len(limited.requests))
	}
}

func TestBrowserArtifactIsRecheckedBeforeEveryExecutorHandoff(t *testing.T) {
	for _, test := range []struct {
		name                 string
		changeOnCapabilities bool
		changeOnFirstAttempt bool
		wantCalls            int
	}{
		{name: "before first handoff", changeOnCapabilities: true, wantCalls: 0},
		{name: "before retry handoff", changeOnFirstAttempt: true, wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			profilePath := filepath.Join(root, "browser.yaml")
			profileBytes, err := os.ReadFile(filepath.Join("..", "examples", "browser", "browser.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			writeApplyTestFile(t, profilePath, string(profileBytes))
			profile, err := browsercontract.LoadProfile(root, "browser.yaml")
			if err != nil {
				t.Fatal(err)
			}
			resource := browserResourceForApplyTest(profile.Version)
			resource.Mapping.Browser.ProfileRef = "browser.yaml"
			resource.Mapping.Browser.ProfilePath = profile.Path
			resource.Mapping.Browser.ProfileDigest = profile.Digest
			if test.changeOnFirstAttempt {
				resource.RuntimeHints = &project.RuntimeHints{Retry: map[string]any{"max_attempts": 2}}
			}

			store, err := state.Open(context.Background(), filepath.Join(root, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			runID, err := store.StartRun(context.Background(), "apply-browser-recheck-test")
			if err != nil {
				t.Fatal(err)
			}
			recorder := asyncrecord.New(store, runID)
			exec := &changingBrowserApplyExecutor{
				path:                 profilePath,
				data:                 []byte(strings.Replace(string(profileBytes), "Reviewed member status UI", "Changed member status UI", 1)),
				changeOnCapabilities: test.changeOnCapabilities,
				changeOnFirstAttempt: test.changeOnFirstAttempt,
			}
			_, err = executeReadPlanAction(context.Background(), exec, store, recorder, runID, resource, nil, nil, root, "")
			if exec.mutationErr != nil {
				t.Fatal(exec.mutationErr)
			}
			if err == nil || !strings.Contains(err.Error(), "apply.browser_artifact_changed") {
				t.Fatalf("handoff error = %v", err)
			}
			if exec.calls != test.wantCalls {
				t.Fatalf("executor received %d handoff(s), want %d", exec.calls, test.wantCalls)
			}
		})
	}
}

func browserResourceForApplyTest(profile string) tfplan.ResourcePlan {
	return tfplan.ResourcePlan{
		Address: "example.browser", Kind: "resource", Type: "example_browser", Action: "read", DesiredHash: "sha256:desired",
		Mapping: &tfplan.MappingPlan{
			Purpose: "read", SourceKind: "browser-profile", SourceID: "browser", OperationID: "read_status",
			Browser: &tfplan.BrowserPlan{
				UWSOperationID: "read_status_uws", ActionID: "read_status", ProfileVersion: profile,
				ProfileRef: "browser.yaml", ProfilePath: "/reviewed/browser.yaml", ProfileDigest: "sha256:browser",
				Outputs:     []tfplan.OutputProjection{{Name: "status", Type: "string", Source: "a11y"}},
				SideEffects: []string{"read_only"},
			},
		},
	}
}

func TestEncodeBindingValueBase64(t *testing.T) {
	got := encodeBindingValue(project.RequestBinding{Encoding: "base64"}, "ramen h03 create")
	if got != "cmFtZW4gaDAzIGNyZWF0ZQ==" {
		t.Fatalf("encoded binding value = %#v", got)
	}
}

func TestApplyAWSIAMRoleCreateThenNoOpWithMockExecutor(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, ".ramen", "state.db")
	writeApplyTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "apply-role"
  assume_role_policy = "{}"
}
`)
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	mock := &executor.MockExecutor{Results: map[string]executor.Result{
		"aws_iam_role.role": {
			Identity: map[string]any{"role_name": "apply-role", "secret_token": "should-not-persist"},
			Computed: map[string]any{"arn": "arn:aws:iam::123456789012:role/apply-role"},
		},
	}}

	result, err := Apply(context.Background(), Options{
		ConfigDir:   configDir,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		AutoApprove: true,
		OutDir:      filepath.Join(root, "out"),
		Executor:    mock,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Summary.Create != 1 || mock.RequestCount() != 1 {
		t.Fatalf("apply result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
	if result.Version != Version || len(result.Feedback) != 1 || result.Feedback[0].Version != executor.FeedbackVersion {
		t.Fatalf("apply feedback/version = version=%q feedback=%#v", result.Version, result.Feedback)
	}
	if result.Summary.Skipped != 0 {
		t.Fatalf("apply skipped=%d, want 0", result.Summary.Skipped)
	}
	if len(result.GeneratedDocuments) != 1 {
		t.Fatalf("generated docs = %#v", result.GeneratedDocuments)
	}
	docText := readApplyTestFile(t, result.GeneratedDocuments[0])
	for _, expected := range []string{"ramen_apply_action", "CreateRole", "aws-smithy", "x-ramen-apply", "x-ramen-executor", "idempotency", "outputs", "$response.body", "RoleName", "apply-role", "AssumeRolePolicyDocument", "Action", "CreateRole", "Version", "2010-05-08"} {
		if !strings.Contains(docText, expected) {
			t.Fatalf("generated UWS missing %q:\n%s", expected, docText)
		}
	}
	if strings.Contains(docText, "ramen-review-todo") || strings.Contains(docText, "x-ramen-terraform") {
		t.Fatalf("apply UWS leaked review scaffolding:\n%s", docText)
	}

	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	snap, err := store.CurrentResource(context.Background(), "aws_iam_role.role")
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close state: %v", err)
	}
	if snap == nil || snap.OperationID != "CreateRole" || !strings.Contains(snap.IdentityJSON, "role_name") {
		t.Fatalf("snapshot = %#v", snap)
	}
	if strings.Contains(snap.IdentityJSON, "should-not-persist") || !strings.Contains(snap.IdentityJSON, "${redacted}") {
		t.Fatalf("identity was not redacted: %s", snap.IdentityJSON)
	}
	store, err = state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("reopen state: %v", err)
	}
	events, err := store.ListRunEvents(context.Background(), 0)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	asyncRecords, err := store.ListAsyncEvidence(context.Background(), state.AsyncEvidenceFilter{RunID: result.RunID})
	if err != nil {
		t.Fatalf("list async evidence: %v", err)
	}
	_ = store.Close()
	if len(events) < 2 || events[0].ResourceAddress != "aws_iam_role.role" || events[0].Phase != "started" {
		t.Fatalf("run events = %#v", events)
	}
	if len(asyncRecords) < 4 {
		t.Fatalf("async evidence records = %#v", asyncRecords)
	}
	kinds := map[string]bool{}
	for _, record := range asyncRecords {
		kinds[record.RecordKind] = true
		if strings.Contains(record.RecordJSON, "should-not-persist") {
			t.Fatalf("async evidence leaked secret-like executor value: %s", record.RecordJSON)
		}
	}
	for _, kind := range []string{"execution_request", "execution_response", "status_observation"} {
		if !kinds[kind] {
			t.Fatalf("async evidence missing %s: %#v", kind, asyncRecords)
		}
	}

	result, err = Apply(context.Background(), Options{
		ConfigDir:   configDir,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		AutoApprove: true,
		Executor:    mock,
	})
	if err != nil {
		t.Fatalf("second Apply returned error: %v", err)
	}
	if result.Summary.NoOp != 1 || result.Summary.Skipped != 1 || mock.RequestCount() != 1 {
		t.Fatalf("second apply result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
}

func TestApplySkippedSummaryCountsNoOpsInMixedMutationPlan(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, ".ramen", "state.db")
	writeApplyTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "existing" {
  name = "existing-role"
  assume_role_policy = "{}"
}

resource "aws_iam_role" "next" {
  name = "next-role"
  assume_role_policy = "{}"
}
`)
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	planned, err := tfplan.Build(context.Background(), tfplan.Options{
		ConfigDir:  configDir,
		StatePath:  statePath,
		APISources: []tfplan.APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	var existing *tfplan.ResourcePlan
	for i := range planned.Plan.Resources {
		if planned.Plan.Resources[i].Address == "aws_iam_role.existing" {
			existing = &planned.Plan.Resources[i]
			break
		}
	}
	if existing == nil {
		t.Fatalf("planned resources missing existing role: %#v", planned.Plan.Resources)
	}
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{Address: existing.Address, Type: existing.Type, Provider: existing.Provider, DesiredHash: existing.DesiredHash, Status: "managed"}); err != nil {
		t.Fatalf("record existing: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close state: %v", err)
	}

	mock := &executor.MockExecutor{Results: map[string]executor.Result{
		"aws_iam_role.next": {Identity: map[string]any{"role_name": "next-role"}},
	}}
	result, err := Apply(context.Background(), Options{
		ConfigDir:   configDir,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		AutoApprove: true,
		Executor:    mock,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Summary.Create != 1 || result.Summary.NoOp != 1 || result.Summary.Skipped != 1 || mock.RequestCount() != 1 {
		t.Fatalf("apply result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
}

func TestApplyNativeReadBeforeWriteAndReadAfterWrite(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	statePath := filepath.Join(projectDir, "state.db")
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	projectPath := writeApplyProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources:  []project.Resource{applyProjectRole("aws_iam_role.role", "apply-read-role")},
	})
	var actions []string
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		actions = append(actions, req.Action.Action)
		switch strings.Join(actions, ",") {
		case "read":
			return executor.Result{Success: true, Missing: true}, nil
		case "read,create":
			return executor.Result{Success: true, Identity: map[string]any{"role_name": "mutation-result"}}, nil
		case "read,create,read":
			return executor.Result{
				Success: true,
				Computed: map[string]any{"Role": map[string]any{
					"RoleName": "apply-read-role",
					"Arn":      "arn:aws:iam::123456789012:role/apply-read-role",
				}},
			}, nil
		default:
			t.Fatalf("unexpected action sequence %v", actions)
			return executor.Result{}, nil
		}
	}}
	result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Summary.Create != 1 || strings.Join(actions, ",") != "read,create,read" {
		t.Fatalf("summary=%#v actions=%v", result.Summary, actions)
	}
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	snap, err := store.CurrentResource(context.Background(), "aws_iam_role.role")
	if err != nil {
		t.Fatalf("current resource: %v", err)
	}
	_ = store.Close()
	if snap == nil || !strings.Contains(snap.IdentityJSON, "apply-read-role") || strings.Contains(snap.IdentityJSON, "mutation-result") || !strings.Contains(snap.AttributesJSON, "arn:aws:iam") {
		t.Fatalf("snapshot = %#v", snap)
	}
}

func TestApplyNativeRuntimeHintsReachExecutorRequests(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	statePath := filepath.Join(projectDir, "state.db")
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	resource := applyProjectRole("aws_iam_role.role", "apply-hinted-role")
	resource.RuntimeHints = &project.RuntimeHints{
		Retry: map[string]any{
			"max_attempts": 3,
			"backoff":      "exponential",
		},
		Waiter: map[string]any{
			"until":        "exists",
			"max_attempts": 3,
		},
	}
	projectPath := writeApplyProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources:  []project.Resource{resource},
	})
	var requests []executor.Request
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		requests = append(requests, req)
		switch len(requests) {
		case 1:
			return executor.Result{Success: true, Missing: true}, nil
		case 2:
			return executor.Result{Success: true, Identity: map[string]any{"role_name": "mutation-result"}}, nil
		case 3:
			return executor.Result{Success: true, Missing: true}, nil
		case 4:
			return executor.Result{
				Success: true,
				Computed: map[string]any{"Role": map[string]any{
					"RoleName": "apply-hinted-role",
					"Arn":      "arn:aws:iam::123456789012:role/apply-hinted-role",
				}},
			}, nil
		default:
			t.Fatalf("unexpected request count %d", len(requests))
			return executor.Result{}, nil
		}
	}}
	result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Summary.Create != 1 || len(requests) != 4 {
		t.Fatalf("summary=%#v requests=%d", result.Summary, len(requests))
	}
	for i, req := range requests {
		if req.Runtime.Retry["backoff"] != "exponential" {
			t.Fatalf("request %d runtime hints = %#v", i, req.Runtime)
		}
		if !containsApplyTest(req.Capabilities.Features, executor.FeatureRetry) {
			t.Fatalf("request %d capabilities %v missing retry", i, req.Capabilities.Features)
		}
		wantWaiter := i >= 2
		if gotWaiter := req.Runtime.Waiter["until"] == "exists"; gotWaiter != wantWaiter {
			t.Fatalf("request %d waiter active=%t, want %t: %#v", i, gotWaiter, wantWaiter, req.Runtime)
		}
		if gotFeature := containsApplyTest(req.Capabilities.Features, executor.FeatureWaiter); gotFeature != wantWaiter {
			t.Fatalf("request %d waiter capability=%t, want %t: %#v", i, gotFeature, wantWaiter, req.Capabilities)
		}
	}
}

func TestApplyNativeWaiterCapabilityOnlyRequiredForWaiterReads(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	statePath := filepath.Join(projectDir, "state.db")
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	resource := applyProjectRole("aws_iam_role.role", "apply-waiter-scope-role")
	resource.RuntimeHints = &project.RuntimeHints{
		Retry: map[string]any{
			"max_attempts": 2,
		},
		Waiter: map[string]any{
			"until":        "exists",
			"max_attempts": 2,
		},
	}
	projectPath := writeApplyProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources:  []project.Resource{resource},
	})
	limited := &limitedApplyExecutor{executeFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		switch req.Action.Action {
		case "read":
			return executor.Result{Success: true, Missing: true}, nil
		case "create":
			return executor.Result{Success: true, Identity: map[string]any{"role_name": "apply-waiter-scope-role"}}, nil
		default:
			t.Fatalf("unexpected action %q", req.Action.Action)
			return executor.Result{}, nil
		}
	}}
	result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: limited})
	if err == nil || !strings.Contains(err.Error(), "apply.failed") {
		t.Fatalf("Apply error = %v, want failed convergence when waiter capability is required", err)
	}
	if result.Summary.Failed != 1 || len(limited.requests) != 2 {
		t.Fatalf("summary=%#v requests=%d", result.Summary, len(limited.requests))
	}
	if strings.Join([]string{limited.requests[0].Action.Action, limited.requests[1].Action.Action}, ",") != "read,create" {
		t.Fatalf("actions = %#v", limited.requests)
	}
	for i, req := range limited.requests {
		if len(req.Runtime.Waiter) != 0 || containsApplyTest(req.Capabilities.Features, executor.FeatureWaiter) {
			t.Fatalf("request %d should not require waiter: runtime=%#v capabilities=%#v", i, req.Runtime, req.Capabilities)
		}
		if !containsApplyTest(req.Capabilities.Features, executor.FeatureRetry) {
			t.Fatalf("request %d should still require retry: %#v", i, req.Capabilities)
		}
	}
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "unsupported feature \"waiter\"") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestApplyNativeWaiterTimeoutFailsConvergence(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	statePath := filepath.Join(projectDir, "state.db")
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	resource := applyProjectRole("aws_iam_role.role", "apply-wait-timeout-role")
	resource.RuntimeHints = &project.RuntimeHints{
		Waiter: map[string]any{
			"until":        "exists",
			"max_attempts": 2,
		},
	}
	projectPath := writeApplyProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources:  []project.Resource{resource},
	})
	var requests []executor.Request
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		requests = append(requests, req)
		switch len(requests) {
		case 1:
			return executor.Result{Success: true, Missing: true}, nil
		case 2:
			return executor.Result{Success: true, Identity: map[string]any{"role_name": "mutation-result"}}, nil
		default:
			return executor.Result{Success: true, Missing: true}, nil
		}
	}}
	result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err == nil || !strings.Contains(err.Error(), "apply failed") {
		t.Fatalf("Apply error = %v, want failed apply", err)
	}
	if result.Summary.Failed != 1 || len(requests) != 4 {
		t.Fatalf("summary=%#v requests=%d", result.Summary, len(requests))
	}
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "apply.waiter_timeout") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestApplyNativeAPIDeleteOperationExecutesWithoutDestroyCommand(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "api.yaml")
	statePath := filepath.Join(projectDir, "state.db")
	writeApplyTestFile(t, sourcePath, `openapi: 3.0.0
info:
  title: Delete API
  version: v1
paths:
  /widgets/{name}:
    get:
      operationId: getWidget
      parameters:
        - name: name
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
    delete:
      operationId: deleteWidget
      parameters:
        - name: name
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
`)
	projectPath := writeApplyProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: "api.yaml"}},
		Resources: []project.Resource{{
			Address:    "resource.widget",
			Kind:       "resource",
			Type:       "widget",
			Name:       "widget",
			Provider:   "openapi",
			Attributes: map[string]any{"name": "ramen"},
			Operations: map[string]project.OperationRole{
				"read":   {Purpose: "read", Method: "GET", SourceKind: "openapi", SourceID: "api", SourcePath: "api.yaml", OperationID: "getWidget"},
				"delete": {Purpose: "delete", Method: "DELETE", SourceKind: "openapi", SourceID: "api", SourcePath: "api.yaml", OperationID: "deleteWidget"},
			},
			IdentityAttributes: []project.IdentityAttribute{{Name: "name", Path: "name", Required: true}},
			Schema:             []project.SchemaPath{{Path: "name", Type: "string", Required: true, Identity: true}},
			RequestBindings: []project.RequestBinding{
				{OperationRole: "read", OperationID: "getWidget", Path: "name", RequestPath: "name", Location: "path", Required: true, Identity: true},
				{OperationRole: "delete", OperationID: "deleteWidget", Path: "name", RequestPath: "name", Location: "path", Required: true, Identity: true},
			},
			RuntimeHints:       &project.RuntimeHints{Waiter: map[string]any{"until": "missing"}},
			RequiredOperations: []string{"delete"},
		}},
	})
	var actions []string
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		actions = append(actions, req.Action.Action)
		switch strings.Join(actions, ",") {
		case "read":
			if req.Action.Mapping.OperationID != "getWidget" {
				t.Fatalf("baseline mapping operation = %q", req.Action.Mapping.OperationID)
			}
			return executor.Result{Success: true, Identity: map[string]any{"name": "ramen"}}, nil
		case "read,delete":
			if req.Action.Mapping.Method != "DELETE" {
				t.Fatalf("executor mapping method = %q", req.Action.Mapping.Method)
			}
			return executor.Result{Success: true}, nil
		case "read,delete,read":
			if req.Action.Mapping.OperationID != "getWidget" || req.Runtime.Waiter["until"] != "missing" {
				t.Fatalf("confirmation request = action=%#v runtime=%#v", req.Action, req.Runtime)
			}
			return executor.Result{Success: true, Missing: true}, nil
		default:
			t.Fatalf("unexpected actions %v", actions)
			return executor.Result{}, nil
		}
	}}
	result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Summary.Delete != 1 || strings.Join(actions, ",") != "read,delete,read" {
		t.Fatalf("summary=%#v actions=%v", result.Summary, actions)
	}
}

func TestApplyNativeAzureSQLPutUsesBodyBindings(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	statePath := filepath.Join(projectDir, "state.db")
	sourcePath, err := filepath.Abs(filepath.Join("..", "testdata", "api-sources", "azure-sql-database-openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	projectPath := writeApplyProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "azure-sql", Path: sourcePath}},
		Resources: []project.Resource{{
			Address:  "resource.sql_database_ramen_m28",
			Kind:     "resource",
			Type:     "azure_sql_database",
			Name:     "ramen-m28",
			Provider: "openapi",
			Attributes: map[string]any{
				"api-version":       "2023-08-01",
				"databaseName":      "ramen-m28",
				"location":          "eastus",
				"resourceGroupName": "SQL",
				"serverName":        "greetingland-sql-server",
				"sku":               map[string]any{"name": "Basic", "tier": "Basic"},
				"subscriptionId":    "00000000-0000-0000-0000-000000000000",
			},
			Operations: map[string]project.OperationRole{
				"put": {Purpose: "put", Method: "PUT", SourceKind: "openapi", SourceID: "azure-sql", SourcePath: sourcePath, OperationID: "Databases_CreateOrUpdate"},
			},
			IdentityAttributes: []project.IdentityAttribute{{Name: "databaseName", Path: "databaseName", RequestKeys: []string{"databaseName"}, Required: true}},
			Schema: []project.SchemaPath{
				{Path: "subscriptionId", Type: "string", Required: true},
				{Path: "resourceGroupName", Type: "string", Required: true},
				{Path: "serverName", Type: "string", Required: true},
				{Path: "databaseName", Type: "string", Required: true, Identity: true},
				{Path: "api-version", Type: "string", Required: true},
				{Path: "location", Type: "string", Required: true},
				{Path: "sku", Type: "object", Required: true},
			},
			RequestBindings: []project.RequestBinding{
				{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "subscriptionId", RequestPath: "subscriptionId", Location: "path", Required: true},
				{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "resourceGroupName", RequestPath: "resourceGroupName", Location: "path", Required: true},
				{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "serverName", RequestPath: "serverName", Location: "path", Required: true},
				{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "databaseName", RequestPath: "databaseName", Location: "path", Required: true, Identity: true},
				{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "api-version", RequestPath: "api-version", Location: "query", Required: true},
				{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "location", RequestPath: "location", Location: "body", Required: true},
				{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "sku", RequestPath: "sku", Location: "body", Required: true},
			},
			RequiredOperations: []string{"put"},
		}},
	})
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		if req.Action.Action != "put" || req.Action.Mapping.OperationID != "Databases_CreateOrUpdate" {
			t.Fatalf("action = %#v", req.Action)
		}
		op := req.Document.Operations[0]
		body, ok := op.Request["body"].(map[string]any)
		if !ok {
			t.Fatalf("body request missing: %#v", op.Request)
		}
		if body["location"] != "eastus" {
			t.Fatalf("body location = %#v", body["location"])
		}
		sku, ok := body["sku"].(map[string]any)
		if !ok || sku["name"] != "Basic" || sku["tier"] != "Basic" {
			t.Fatalf("body sku = %#v", body["sku"])
		}
		query, ok := op.Request["query"].(map[string]any)
		if !ok || query["api-version"] != "2023-08-01" {
			t.Fatalf("query = %#v", op.Request["query"])
		}
		return executor.Result{Success: true, Identity: map[string]any{"databaseName": "ramen-m28"}}, nil
	}}
	result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Summary.Put != 1 || mock.RequestCount() != 1 {
		t.Fatalf("summary=%#v requests=%d", result.Summary, mock.RequestCount())
	}
}

func TestBuildActionDocumentNativeBindingsDoNotDuplicateOpenAPIPathValuesInBody(t *testing.T) {
	resource := tfplan.ResourcePlan{
		Address:  "azurerm_cosmosdb_account.test",
		Kind:     "resource",
		Type:     "azurerm_cosmosdb_account",
		Provider: "provider.azurerm",
		Action:   "read",
		Mapping: &tfplan.MappingPlan{
			Purpose:     "read",
			SourceKind:  "openapi",
			SourceID:    "azure-cosmos-db-resource-manager-openapi",
			SourcePath:  "openapi/azure_cosmos_db_resource_manager_openapi.json",
			OperationID: "DatabaseAccounts_Get",
			IdentityAttributes: []tfmapping.IdentityAttribute{
				{Name: "account_name", TerraformPath: "accountName", RequestKeys: []string{"accountName"}, Required: true},
				{Name: "resource_group_name", TerraformPath: "resourceGroupName", RequestKeys: []string{"resourceGroupName"}, Required: true},
			},
			RequestBindings: []project.RequestBinding{
				{OperationRole: "read", OperationID: "DatabaseAccounts_Get", Path: "subscriptionId", RequestPath: "subscriptionId", Location: "path", Required: true},
				{OperationRole: "read", OperationID: "DatabaseAccounts_Get", Path: "resourceGroupName", RequestPath: "resourceGroupName", Location: "path", Required: true},
				{OperationRole: "read", OperationID: "DatabaseAccounts_Get", Path: "accountName", RequestPath: "accountName", Location: "path", Required: true, Identity: true},
				{OperationRole: "read", OperationID: "DatabaseAccounts_Get", Path: "api-version", RequestPath: "api-version", Location: "query", Required: true},
			},
		},
	}
	attrs := map[string]any{
		"subscriptionId":    "00000000-0000-0000-0000-000000000000",
		"resourceGroupName": "SQL",
		"accountName":       "ramen-cosmos-live-1780474014",
		"api-version":       "2024-11-15",
	}
	identity := map[string]any{
		"account_name":        "ramen-cosmos-live-1780474014",
		"resource_group_name": "SQL",
	}

	doc, err := BuildActionDocumentWithBindings(resource, nil, attrs, identity)
	if err != nil {
		t.Fatalf("BuildActionDocumentWithBindings returned error: %v", err)
	}
	request := doc.Operations[0].Request
	path, ok := request["path"].(map[string]any)
	if !ok || path["accountName"] != "ramen-cosmos-live-1780474014" || path["resourceGroupName"] != "SQL" {
		t.Fatalf("path request = %#v", request["path"])
	}
	if body, ok := request["body"].(map[string]any); ok {
		if _, exists := body["accountName"]; exists {
			t.Fatalf("accountName duplicated into body: %#v", body)
		}
		if _, exists := body["resourceGroupName"]; exists {
			t.Fatalf("resourceGroupName duplicated into body: %#v", body)
		}
	}
}

func TestBuildActionDocumentNativeBindingsReadDottedAttributePaths(t *testing.T) {
	resource := tfplan.ResourcePlan{
		Address:  "google_storage_bucket.bucket",
		Kind:     "resource",
		Type:     "google_storage_bucket",
		Provider: "google",
		Action:   "update",
		Mapping: &tfplan.MappingPlan{
			Purpose:     "update",
			SourceKind:  "google-discovery",
			SourceID:    "storage",
			SourcePath:  "google-cloud-storage-discovery-v1.json",
			OperationID: "storage.buckets.patch",
			RequestBindings: []project.RequestBinding{
				{OperationRole: "update", OperationID: "storage.buckets.patch", Path: "name", RequestPath: "bucket", Location: "path", Required: true, Identity: true},
				{OperationRole: "update", OperationID: "storage.buckets.patch", Path: "labels.ramen_parity_phase", RequestPath: "labels.ramen_parity_phase", Location: "body"},
			},
		},
	}
	attrs := map[string]any{
		"name": "ramen-parity-y03-ramen-test",
		"labels": map[string]any{
			"ramen_parity_phase": "update",
		},
	}
	doc, err := BuildActionDocumentWithBindings(resource, nil, attrs, nil)
	if err != nil {
		t.Fatalf("BuildActionDocumentWithBindings returned error: %v", err)
	}
	request := doc.Operations[0].Request
	path, ok := request["path"].(map[string]any)
	if !ok || path["bucket"] != "ramen-parity-y03-ramen-test" {
		t.Fatalf("path request = %#v", request["path"])
	}
	body, ok := request["body"].(map[string]any)
	if !ok {
		t.Fatalf("body request missing: %#v", request)
	}
	labels, ok := body["labels"].(map[string]any)
	if !ok || labels["ramen_parity_phase"] != "update" {
		t.Fatalf("body labels = %#v", body["labels"])
	}
}

func TestApplyNativePutWithAlternateResponseShapeAndConvergenceWaiter(t *testing.T) {
	tests := []struct {
		name     string
		mutation executor.Result
		reads    []executor.Result
	}{
		{
			name: "Put response does not match base schema but waits until read convergence",
			mutation: executor.Result{Success: true, Computed: map[string]any{
				"operation": "create",
				"status":    "Accepted",
				"id":        "op-1",
			}},
			reads: []executor.Result{
				{Success: true, Missing: true},
				{Success: true, Missing: false, Computed: map[string]any{
					"name":    "ramen-m28",
					"status":  "Online",
					"id":      "db-1",
					"ignored": "payload",
				}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			projectDir := filepath.Join(root, "project")
			statePath := filepath.Join(projectDir, "state.db")
			sourcePath, err := filepath.Abs(filepath.Join("..", "testdata", "api-sources", "azure-sql-database-openapi.json"))
			if err != nil {
				t.Fatal(err)
			}
			projectPath := writeApplyProjectForTest(t, projectDir, project.Profile{
				Version:    project.Version,
				APISources: []project.APISource{{Kind: "openapi", ID: "azure-sql", Path: sourcePath}},
				Resources: []project.Resource{{
					Address:  "resource.sql_database_ramen_m28",
					Kind:     "resource",
					Type:     "azure_sql_database",
					Name:     "ramen-m28",
					Provider: "openapi",
					Attributes: map[string]any{
						"api-version":       "2023-08-01",
						"databaseName":      "ramen-m28",
						"location":          "eastus",
						"resourceGroupName": "SQL",
						"serverName":        "greetingland-sql-server",
						"sku":               map[string]any{"name": "Basic", "tier": "Basic"},
						"subscriptionId":    "00000000-0000-0000-0000-000000000000",
					},
					Operations: map[string]project.OperationRole{
						"put":  {Purpose: "put", Method: "PUT", SourceKind: "openapi", SourceID: "azure-sql", SourcePath: sourcePath, OperationID: "Databases_CreateOrUpdate"},
						"read": {Purpose: "read", Method: "GET", SourceKind: "openapi", SourceID: "azure-sql", SourcePath: sourcePath, OperationID: "Databases_Get"},
					},
					IdentityAttributes: []project.IdentityAttribute{{Name: "databaseName", Path: "databaseName", RequestKeys: []string{"databaseName"}, Required: true}},
					Schema: []project.SchemaPath{
						{Path: "subscriptionId", Type: "string", Required: true},
						{Path: "resourceGroupName", Type: "string", Required: true},
						{Path: "serverName", Type: "string", Required: true},
						{Path: "databaseName", Type: "string", Required: true, Identity: true},
						{Path: "api-version", Type: "string", Required: true},
						{Path: "location", Type: "string", Required: true},
						{Path: "sku", Type: "object", Required: true},
					},
					RequestBindings: []project.RequestBinding{
						{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "subscriptionId", RequestPath: "subscriptionId", Location: "path", Required: true},
						{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "resourceGroupName", RequestPath: "resourceGroupName", Location: "path", Required: true},
						{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "serverName", RequestPath: "serverName", Location: "path", Required: true},
						{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "databaseName", RequestPath: "databaseName", Location: "path", Required: true, Identity: true},
						{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "api-version", RequestPath: "api-version", Location: "query", Required: true},
						{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "location", RequestPath: "location", Location: "body", Required: true},
						{OperationRole: "put", OperationID: "Databases_CreateOrUpdate", Path: "sku", RequestPath: "sku", Location: "body", Required: true},
						{OperationRole: "read", OperationID: "Databases_Get", Path: "subscriptionId", RequestPath: "subscriptionId", Location: "path", Required: true},
						{OperationRole: "read", OperationID: "Databases_Get", Path: "resourceGroupName", RequestPath: "resourceGroupName", Location: "path", Required: true},
						{OperationRole: "read", OperationID: "Databases_Get", Path: "serverName", RequestPath: "serverName", Location: "path", Required: true},
						{OperationRole: "read", OperationID: "Databases_Get", Path: "databaseName", RequestPath: "databaseName", Location: "path", Required: true, Identity: true},
						{OperationRole: "read", OperationID: "Databases_Get", Path: "api-version", RequestPath: "api-version", Location: "query", Required: true},
					},
					RequiredOperations: []string{"put", "read"},
					ResponseBindings: []project.ResponseBinding{{
						OperationRole: "read",
						ResponsePath:  "databaseName",
						StatePath:     "databaseName",
						Identity:      true,
						Computed:      true,
					}},
					RuntimeHints: &project.RuntimeHints{Waiter: map[string]any{"until": "exists", "max_attempts": 2}},
				}},
			})
			var actions []string
			readCount := 0
			mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
				actions = append(actions, req.Action.Action)
				switch req.Action.Action {
				case "put":
					return tc.mutation, nil
				case "read":
					if readCount >= len(tc.reads) {
						return tc.reads[len(tc.reads)-1], nil
					}
					result := tc.reads[readCount]
					readCount++
					return result, nil
				default:
					t.Fatalf("unexpected action %q", req.Action.Action)
					return executor.Result{}, nil
				}
			}}
			result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
			if err != nil {
				t.Fatalf("Apply returned error: %v", err)
			}
			if result.Summary.Put != 1 || strings.Join(actions, ",") != "read,put,read" {
				t.Fatalf("summary=%#v actions=%v", result.Summary, actions)
			}
		})
	}
}

func TestApplyNativeCreateUsesMutationIdentityForResponseDerivedConvergenceRead(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	statePath := filepath.Join(projectDir, "state.db")
	sourcePath := filepath.Join(projectDir, "api.yaml")
	writeApplyTestFile(t, sourcePath, `openapi: 3.0.0
info:
  title: D1 API
  version: v1
paths:
  /databases:
    post:
      operationId: createDatabase
      responses:
        "200":
          description: created
  /databases/{database_id}:
    get:
      operationId: readDatabase
      parameters:
        - name: database_id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: read
    delete:
      operationId: deleteDatabase
      parameters:
        - name: database_id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: deleted
`)
	projectPath := writeApplyProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "d1", Path: sourcePath}},
		Resources: []project.Resource{{
			Address:    "cloudflare_d1_database.database",
			Kind:       "resource",
			Type:       "cloudflare_d1_database",
			Provider:   "openapi",
			Attributes: map[string]any{"account_id": "account", "name": "ramen-d1"},
			Operations: map[string]project.OperationRole{
				"create": {Purpose: "create", Method: "POST", SourceKind: "openapi", SourceID: "d1", SourcePath: sourcePath, OperationID: "createDatabase"},
				"read":   {Purpose: "read", Method: "GET", SourceKind: "openapi", SourceID: "d1", SourcePath: sourcePath, OperationID: "readDatabase"},
				"delete": {Purpose: "delete", Method: "DELETE", SourceKind: "openapi", SourceID: "d1", SourcePath: sourcePath, OperationID: "deleteDatabase"},
			},
			RequestBindings: []project.RequestBinding{
				{OperationRole: "create", OperationID: "createDatabase", Path: "name", RequestPath: "name", Location: "body", Required: true, Identity: true},
				{OperationRole: "read", OperationID: "readDatabase", Path: "database_id", RequestPath: "database_id", Location: "path", Required: true, Identity: true},
				{OperationRole: "delete", OperationID: "deleteDatabase", Path: "database_id", RequestPath: "database_id", Location: "path", Required: true, Identity: true},
			},
			ResponseBindings: []project.ResponseBinding{
				{OperationRole: "read", OperationID: "readDatabase", ResponsePath: "result.name", StatePath: "name", Observed: true},
				{OperationRole: "read", OperationID: "readDatabase", ResponsePath: "result.uuid", StatePath: "database_id", Identity: true, ResponseDerivedIdentity: true},
			},
			RequiredOperations: []string{"create", "read", "delete"},
		}},
	})

	var actions []string
	var readPathValues []any
	var deletePathValues []any
	deleted := false
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		actions = append(actions, req.Action.Action)
		switch req.Action.Action {
		case "create":
			return executor.Result{Success: true, Identity: map[string]any{"database_id": "db-123"}}, nil
		case "read":
			path, _ := req.Document.Operations[0].Request["path"].(map[string]any)
			readPathValues = append(readPathValues, path["database_id"])
			if deleted {
				return executor.Result{Success: true, Missing: true}, nil
			}
			return executor.Result{Success: true, Computed: map[string]any{
				"result": map[string]any{"name": "ramen-d1", "uuid": "db-123"},
			}}, nil
		case "delete":
			path, _ := req.Document.Operations[0].Request["path"].(map[string]any)
			deletePathValues = append(deletePathValues, path["database_id"])
			deleted = true
			return executor.Result{Success: true}, nil
		default:
			t.Fatalf("unexpected action %q", req.Action.Action)
			return executor.Result{}, nil
		}
	}}
	result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Summary.Create != 1 || strings.Join(actions, ",") != "create,read" {
		t.Fatalf("summary=%#v actions=%v", result.Summary, actions)
	}
	if len(readPathValues) != 1 || readPathValues[0] != "db-123" {
		t.Fatalf("read database_id values = %#v, want db-123", readPathValues)
	}
	deletePlanPath := filepath.Join(projectDir, "delete-plan.json")
	if _, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: projectPath,
		StatePath:   statePath,
		Action:      "delete",
		OutPath:     deletePlanPath,
	}); err != nil {
		t.Fatalf("build delete plan: %v", err)
	}
	deleteResult, err := Apply(context.Background(), Options{PlanPath: deletePlanPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err != nil {
		t.Fatalf("delete Apply returned error: %v", err)
	}
	if deleteResult.Summary.Delete != 1 || strings.Join(actions, ",") != "create,read,read,delete,read" {
		t.Fatalf("delete summary=%#v actions=%v", deleteResult.Summary, actions)
	}
	if len(deletePathValues) != 1 || deletePathValues[0] != "db-123" {
		t.Fatalf("delete database_id values = %#v, want db-123", deletePathValues)
	}
}

func TestApplyNativeDeleteWithoutReadRoleShouldNotRecordSuccess(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "api.yaml")
	statePath := filepath.Join(projectDir, "state.db")
	writeApplyTestFile(t, sourcePath, `openapi: 3.0.0
info:
  title: Delete API
  version: v1
paths:
  /widgets/{name}:
    delete:
      operationId: deleteWidget
      parameters:
        - name: name
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
`)
	projectPath := writeApplyProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: "api.yaml"}},
		Resources: []project.Resource{{
			Address:    "resource.widget",
			Kind:       "resource",
			Type:       "widget",
			Name:       "widget",
			Provider:   "openapi",
			Attributes: map[string]any{"name": "ramen"},
			Operations: map[string]project.OperationRole{
				"delete": {Purpose: "delete", Method: "DELETE", SourceKind: "openapi", SourceID: "api", SourcePath: "api.yaml", OperationID: "deleteWidget"},
			},
			IdentityAttributes: []project.IdentityAttribute{{Name: "name", Path: "name", RequestKeys: []string{"name"}, Required: true}},
			Schema:             []project.SchemaPath{{Path: "name", Type: "string", Required: true, Identity: true}},
			RequestBindings: []project.RequestBinding{
				{OperationRole: "delete", OperationID: "deleteWidget", Path: "name", RequestPath: "name", Location: "path", Required: true, Identity: true},
			},
			RuntimeHints:       &project.RuntimeHints{Waiter: map[string]any{"until": "missing"}},
			RequiredOperations: []string{"delete"},
		}},
	})
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		if req.Action.Action != "delete" {
			t.Fatalf("unexpected action %q", req.Action.Action)
		}
		return executor.Result{Success: true}, nil
	}}
	result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err == nil || !strings.Contains(err.Error(), "apply.failed") {
		t.Fatalf("expected apply failure, got %v", err)
	}
	if result.Summary.Failed != 1 || result.Summary.Delete != 0 || mock.RequestCount() != 0 {
		t.Fatalf("summary=%#v requests=%d", result.Summary, mock.RequestCount())
	}
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "apply.delete_confirmation_missing") {
		t.Fatalf("errors=%#v", result.Errors)
	}
}

func TestApplyNativeDeleteConfirmsMissingWithReadRole(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "api.yaml")
	statePath := filepath.Join(projectDir, "state.db")
	writeApplyDeleteWidgetSource(t, sourcePath)
	projectPath := writeApplyDeleteWidgetProject(t, projectDir, &project.RuntimeHints{Waiter: map[string]any{"max_attempts": 2}})
	var actions []string
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		actions = append(actions, req.Action.Action)
		switch strings.Join(actions, ",") {
		case "read":
			if req.Runtime.Waiter["until"] != nil {
				t.Fatalf("baseline read should not force waiter: %#v", req.Runtime)
			}
			return executor.Result{Success: true, Identity: map[string]any{"name": "ramen"}}, nil
		case "read,delete":
			return executor.Result{Success: true}, nil
		case "read,delete,read":
			if req.Runtime.Waiter["until"] != "missing" {
				t.Fatalf("delete confirmation waiter = %#v", req.Runtime.Waiter)
			}
			return executor.Result{Success: true, Missing: true}, nil
		default:
			t.Fatalf("unexpected actions %v", actions)
			return executor.Result{}, nil
		}
	}}
	result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Summary.Delete != 1 || strings.Join(actions, ",") != "read,delete,read" {
		t.Fatalf("summary=%#v actions=%v", result.Summary, actions)
	}
}

func TestApplyNativeDeleteRunsSettleBeforeMutation(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "api.yaml")
	statePath := filepath.Join(projectDir, "state.db")
	writeApplyDeleteWidgetSource(t, sourcePath)
	projectPath := writeApplyDeleteWidgetProject(t, projectDir, &project.RuntimeHints{
		Retry:  map[string]any{"max_attempts": 2},
		Waiter: map[string]any{"max_attempts": 2},
		Settle: map[string]any{"before": "delete", "duration": "1ms", "interval": "1ms", "read_expect": "exists"},
	})
	var actions []string
	var settleReads int
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		actions = append(actions, req.Action.Action)
		if req.Action.Action == "read" && strings.Contains(req.Idempotency.Key, "-settle") {
			settleReads++
			if req.Runtime.Waiter["until"] != nil || containsApplyTest(req.Capabilities.Features, executor.FeatureWaiter) {
				t.Fatalf("settle read should not require waiter: runtime=%#v capabilities=%#v", req.Runtime, req.Capabilities)
			}
			if req.Runtime.Retry["max_attempts"] == nil || !containsApplyTest(req.Capabilities.Features, executor.FeatureRetry) {
				t.Fatalf("settle read should retain retry hints: runtime=%#v capabilities=%#v", req.Runtime, req.Capabilities)
			}
			return executor.Result{Success: true, Identity: map[string]any{"name": "ramen"}}, nil
		}
		switch req.Action.Action {
		case "read":
			if len(actions) == 1 {
				return executor.Result{Success: true, Identity: map[string]any{"name": "ramen"}}, nil
			}
			if req.Runtime.Waiter["until"] != "missing" {
				t.Fatalf("delete confirmation waiter = %#v", req.Runtime.Waiter)
			}
			return executor.Result{Success: true, Missing: true}, nil
		case "delete":
			return executor.Result{Success: true}, nil
		default:
			t.Fatalf("unexpected action %q", req.Action.Action)
			return executor.Result{}, nil
		}
	}}
	result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	joined := strings.Join(actions, ",")
	if result.Summary.Delete != 1 || settleReads < 1 || !strings.HasPrefix(joined, "read,read") || !strings.HasSuffix(joined, "delete,read") {
		t.Fatalf("summary=%#v actions=%v settleReads=%d", result.Summary, actions, settleReads)
	}
}

func TestApplyNativeDeleteSettleMissingFailsBeforeDelete(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "api.yaml")
	statePath := filepath.Join(projectDir, "state.db")
	writeApplyDeleteWidgetSource(t, sourcePath)
	projectPath := writeApplyDeleteWidgetProject(t, projectDir, &project.RuntimeHints{
		Settle: map[string]any{"before": "delete", "duration": "1ms", "interval": "1ms", "read_expect": "exists"},
	})
	var actions []string
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		actions = append(actions, req.Action.Action)
		if req.Action.Action != "read" {
			t.Fatalf("delete should not run after failed settle: %#v", req.Action)
		}
		if strings.Contains(req.Idempotency.Key, "-settle") {
			return executor.Result{Success: true, Missing: true}, nil
		}
		return executor.Result{Success: true, Identity: map[string]any{"name": "ramen"}}, nil
	}}
	result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err == nil || !strings.Contains(err.Error(), "apply.failed") {
		t.Fatalf("Apply error = %v, want failed apply", err)
	}
	if result.Summary.Failed != 1 || strings.Join(actions, ",") != "read,read" || len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "apply.settle_failed") {
		t.Fatalf("summary=%#v actions=%v errors=%v", result.Summary, actions, result.Errors)
	}
}

func TestApplyNativeDeleteSettleMalformedFailsBeforeDelete(t *testing.T) {
	cases := []struct {
		name       string
		settle     map[string]any
		wantError  string
		wantReads  int
		wantAction string
	}{
		{
			name:       "unsupported before",
			settle:     map[string]any{"before": "create", "duration": "1ms", "interval": "1ms", "read_expect": "exists"},
			wantError:  `settle.before "create" is not supported`,
			wantReads:  1,
			wantAction: "read",
		},
		{
			name:       "invalid duration",
			settle:     map[string]any{"before": "delete", "duration": "soon", "interval": "1ms", "read_expect": "exists"},
			wantError:  "settle.duration must be a positive duration",
			wantReads:  1,
			wantAction: "read",
		},
		{
			name:       "missing interval",
			settle:     map[string]any{"before": "delete", "duration": "1ms", "read_expect": "exists"},
			wantError:  "settle.interval must be a positive duration",
			wantReads:  1,
			wantAction: "read",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			projectDir := filepath.Join(root, "project")
			sourcePath := filepath.Join(projectDir, "api.yaml")
			statePath := filepath.Join(projectDir, "state.db")
			writeApplyDeleteWidgetSource(t, sourcePath)
			projectPath := writeApplyDeleteWidgetProject(t, projectDir, &project.RuntimeHints{Settle: tc.settle})
			var actions []string
			mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
				actions = append(actions, req.Action.Action)
				if req.Action.Action != "read" {
					t.Fatalf("delete should not run with malformed settle hints: %#v", req.Action)
				}
				return executor.Result{Success: true, Identity: map[string]any{"name": "ramen"}}, nil
			}}
			result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
			if err == nil || !strings.Contains(err.Error(), "apply.failed") {
				t.Fatalf("Apply error = %v, want failed apply", err)
			}
			if result.Summary.Failed != 1 || strings.Join(actions, ",") != tc.wantAction || len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "apply.settle_failed") || !strings.Contains(result.Errors[0], tc.wantError) {
				t.Fatalf("summary=%#v actions=%v errors=%v", result.Summary, actions, result.Errors)
			}
			if mock.RequestCount() != tc.wantReads {
				t.Fatalf("requests=%d, want %d", mock.RequestCount(), tc.wantReads)
			}
		})
	}
}

func writeApplyDeleteWidgetSource(t *testing.T, sourcePath string) {
	t.Helper()
	writeApplyTestFile(t, sourcePath, `openapi: 3.0.0
info:
  title: Delete API
  version: v1
paths:
  /widgets/{name}:
    get:
      operationId: getWidget
      parameters:
        - name: name
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
    delete:
      operationId: deleteWidget
      parameters:
        - name: name
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
`)
}

func writeApplyDeleteWidgetProject(t *testing.T, projectDir string, runtimeHints *project.RuntimeHints) string {
	t.Helper()
	return writeApplyProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: "api.yaml"}},
		Resources: []project.Resource{{
			Address:    "resource.widget",
			Kind:       "resource",
			Type:       "widget",
			Name:       "widget",
			Provider:   "openapi",
			Attributes: map[string]any{"name": "ramen"},
			Operations: map[string]project.OperationRole{
				"read":   {Purpose: "read", Method: "GET", SourceKind: "openapi", SourceID: "api", SourcePath: "api.yaml", OperationID: "getWidget"},
				"delete": {Purpose: "delete", Method: "DELETE", SourceKind: "openapi", SourceID: "api", SourcePath: "api.yaml", OperationID: "deleteWidget"},
			},
			IdentityAttributes: []project.IdentityAttribute{{Name: "name", Path: "name", RequestKeys: []string{"name"}, Required: true}},
			Schema:             []project.SchemaPath{{Path: "name", Type: "string", Required: true, Identity: true}},
			RequestBindings: []project.RequestBinding{
				{OperationRole: "read", OperationID: "getWidget", Path: "name", RequestPath: "name", Location: "path", Required: true, Identity: true},
				{OperationRole: "delete", OperationID: "deleteWidget", Path: "name", RequestPath: "name", Location: "path", Required: true, Identity: true},
			},
			RequiredOperations: []string{"delete"},
			RuntimeHints:       runtimeHints,
		}},
	})
}

func TestApplyNativeReadBeforeWriteRejectsExistingCreate(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	statePath := filepath.Join(projectDir, "state.db")
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	projectPath := writeApplyProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources:  []project.Resource{applyProjectRole("aws_iam_role.role", "existing-role")},
	})
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		if req.Action.Action != "read" {
			t.Fatalf("unexpected mutation after existing baseline: %#v", req.Action)
		}
		return executor.Result{Success: true, Identity: map[string]any{"role_name": "existing-role"}}, nil
	}}
	result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err == nil || !strings.Contains(err.Error(), "apply.failed") {
		t.Fatalf("expected apply failure, got %v", err)
	}
	if result.Summary.Failed != 1 || mock.RequestCount() != 1 || len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "apply.baseline_exists") {
		t.Fatalf("result=%#v errors=%v requests=%d", result.Summary, result.Errors, mock.RequestCount())
	}
}

func TestApplyNativeReadBeforeWriteRejectsUpdateDrift(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	statePath := filepath.Join(projectDir, "state.db")
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	projectPath := writeApplyProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources:  []project.Resource{applyProjectRole("aws_iam_role.role", "drift-role")},
	})
	planned, err := tfplan.Build(context.Background(), tfplan.Options{ProjectPath: projectPath, StatePath: statePath})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{
		Address:        "aws_iam_role.role",
		Type:           "aws_iam_role",
		Provider:       "provider.aws",
		DesiredHash:    "old-hash",
		IdentityJSON:   `{"role_name":"drift-role"}`,
		AttributesJSON: `{"arn":"old"}`,
		Status:         "managed",
	}); err != nil {
		t.Fatalf("record state: %v", err)
	}
	_ = store.Close()
	if planned.Plan.Resources[0].DesiredHash == "old-hash" {
		t.Fatal("test fixture did not create an update hash")
	}
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		if req.Action.Action != "read" {
			t.Fatalf("unexpected mutation after drift baseline: %#v", req.Action)
		}
		return executor.Result{
			Success: true,
			Computed: map[string]any{"Role": map[string]any{
				"RoleName": "drift-role",
				"Arn":      "remote-drift",
			}},
		}, nil
	}}
	result, err := Apply(context.Background(), Options{ProjectPath: projectPath, StatePath: statePath, AutoApprove: true, Executor: mock})
	if err == nil || !strings.Contains(err.Error(), "apply.failed") {
		t.Fatalf("expected apply failure, got %v", err)
	}
	if result.Summary.Failed != 1 || mock.RequestCount() != 1 || len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "apply.baseline_drift") {
		t.Fatalf("result=%#v errors=%v requests=%d", result.Summary, result.Errors, mock.RequestCount())
	}
}

func TestApplyRecordsFailureAndBlocksDependentResources(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, ".ramen", "state.db")
	writeApplyTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "failed-role"
  assume_role_policy = "{}"
}

resource "aws_iam_role_policy" "policy" {
  name   = "policy"
  role   = aws_iam_role.role.name
  policy = "{}"
}
`)
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyFailureTest())
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		if req.Action.Address == "aws_iam_role.role" {
			return executor.Result{}, fmt.Errorf("token ABCDEFG should redact")
		}
		return executor.Result{Success: true}, nil
	}}
	result, err := Apply(context.Background(), Options{
		ConfigDir:   configDir,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		AutoApprove: true,
		Executor:    mock,
	})
	if err == nil {
		t.Fatalf("expected apply failure")
	}
	if result.Summary.Failed != 1 || result.Summary.Blocked != 1 || mock.RequestCount() != 1 {
		t.Fatalf("result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	revs, err := store.ListRevisions(context.Background(), "")
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revs) != 2 || revs[0].Action != "failed" || revs[1].Action != "blocked" {
		t.Fatalf("revisions = %#v", revs)
	}
	if strings.Contains(revs[0].DiffJSON, "ABCDEFG") || !strings.Contains(revs[0].DiffJSON, "${redacted}") {
		t.Fatalf("failure revision was not redacted: %s", revs[0].DiffJSON)
	}
	_ = store.Close()
}

func TestApplyGoogleStorageBucketCreateThenNoOpWithMockExecutor(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "storage.json")
	statePath := filepath.Join(root, ".ramen", "state.db")
	writeApplyTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "google_storage_bucket" "bucket" {
  name     = "apply-bucket"
  location = "US"
  project  = "review-project"
}
`)
	writeApplyTestFile(t, sourcePath, minimalStorageDiscoveryForApplyTest())
	mock := &executor.MockExecutor{Results: map[string]executor.Result{
		"google_storage_bucket.bucket": {
			Identity: map[string]any{"bucket_name": "apply-bucket"},
			Computed: map[string]any{"id": "apply-bucket"},
		},
	}}
	result, err := Apply(context.Background(), Options{
		ConfigDir:   configDir,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "google-discovery", ID: "storage", Path: sourcePath}},
		AutoApprove: true,
		Executor:    mock,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Summary.Create != 1 || mock.RequestCount() != 1 {
		t.Fatalf("apply result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
	result, err = Apply(context.Background(), Options{
		ConfigDir:   configDir,
		StatePath:   statePath,
		APISources:  []APISourceInput{{Kind: "google-discovery", ID: "storage", Path: sourcePath}},
		AutoApprove: true,
		Executor:    mock,
	})
	if err != nil {
		t.Fatalf("second Apply returned error: %v", err)
	}
	if result.Summary.NoOp != 1 || mock.RequestCount() != 1 {
		t.Fatalf("second apply result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
}

func TestApplyRequiresApprovalForMutations(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	writeApplyTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "approval-role"
  assume_role_policy = "{}"
}
`)
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	mock := &executor.MockExecutor{}
	_, err := Apply(context.Background(), Options{
		ConfigDir:  configDir,
		StatePath:  filepath.Join(root, ".ramen", "state.db"),
		APISources: []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		Executor:   mock,
	})
	if err == nil || !strings.Contains(err.Error(), "apply.approval_required") || !strings.Contains(err.Error(), "--auto-approve") {
		t.Fatalf("expected approval error, got %v", err)
	}
	if mock.RequestCount() != 0 {
		t.Fatalf("executor was called before approval")
	}
}

func TestApplyRequiresExplicitExecutorBeforeCredentialedWork(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	writeApplyTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "executor-role"
  assume_role_policy = "{}"
}
`)
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	result, err := Apply(context.Background(), Options{
		ConfigDir:   configDir,
		StatePath:   filepath.Join(root, ".ramen", "state.db"),
		APISources:  []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		AutoApprove: true,
	})
	if err == nil || !strings.Contains(err.Error(), "apply.executor_required") {
		t.Fatalf("expected executor error, got %v", err)
	}
	if result == nil || result.Summary.Create != 0 || len(result.Executed) != 0 {
		t.Fatalf("apply should not execute without an executor: %#v", result)
	}
}

func TestApplyExecutesVerifiedPlanArtifact(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, "state.db")
	planPath := filepath.Join(root, "plan.json")
	writeApplyTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "artifact-role"
  assume_role_policy = "{}"
}
`)
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	if _, err := tfplan.Build(context.Background(), tfplan.Options{
		ConfigDir:  configDir,
		StatePath:  statePath,
		APISources: []tfplan.APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		OutPath:    planPath,
	}); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	mock := &executor.MockExecutor{Results: map[string]executor.Result{
		"aws_iam_role.role": {Identity: map[string]any{"role_name": "artifact-role"}},
	}}
	result, err := Apply(context.Background(), Options{PlanPath: planPath, AutoApprove: true, Executor: mock})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Summary.Create != 1 || mock.RequestCount() != 1 {
		t.Fatalf("result=%#v requests=%d", result.Summary, mock.RequestCount())
	}
}

func TestApplyExecutesReadPlanArtifact(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourcePath := filepath.Join(projectDir, "aws-smithy", "iam.json")
	statePath := filepath.Join(projectDir, "state.db")
	planPath := filepath.Join(root, "read-plan.json")
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	projectPath := writeApplyProjectForTest(t, projectDir, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "aws-smithy", ID: "iam", Path: "aws-smithy/iam.json"}},
		Resources:  []project.Resource{applyProjectRole("aws_iam_role.role", "read-plan-role")},
	})
	if _, err := tfplan.Build(context.Background(), tfplan.Options{
		ProjectPath: projectPath,
		StatePath:   statePath,
		Action:      "read",
		OutPath:     planPath,
	}); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	var actions []string
	mock := &executor.MockExecutor{ExecuteFn: func(_ context.Context, req executor.Request) (executor.Result, error) {
		actions = append(actions, req.Action.Action)
		return executor.Result{
			Success: true,
			Computed: map[string]any{"Role": map[string]any{
				"RoleName": "read-plan-role",
				"Arn":      "arn:aws:iam::123456789012:role/read-plan-role",
			}},
		}, nil
	}}
	outDir := filepath.Join(root, "out")
	result, err := Apply(context.Background(), Options{PlanPath: planPath, AutoApprove: true, OutDir: outDir, Executor: mock})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Summary.Read != 1 || result.Summary.Skipped != 0 || len(result.Executed) != 1 || strings.Join(actions, ",") != "read" {
		t.Fatalf("summary=%#v executed=%#v actions=%v", result.Summary, result.Executed, actions)
	}
	if len(result.GeneratedDocuments) != 1 || !strings.Contains(readApplyTestFile(t, result.GeneratedDocuments[0]), "GetRole") {
		t.Fatalf("generated read documents = %#v", result.GeneratedDocuments)
	}
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	snap, err := store.CurrentResource(context.Background(), "aws_iam_role.role")
	if err != nil {
		t.Fatalf("current resource: %v", err)
	}
	revs, err := store.ListRevisions(context.Background(), "aws_iam_role.role")
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	_ = store.Close()
	if snap == nil || snap.OperationID != "GetRole" || !strings.Contains(snap.IdentityJSON, "read-plan-role") || !strings.Contains(snap.AttributesJSON, "arn:aws:iam") {
		t.Fatalf("snapshot = %#v", snap)
	}
	if len(revs) != 1 || revs[0].Action != "read" {
		t.Fatalf("revisions = %#v", revs)
	}
}

func TestApplyRejectsStaleOrTamperedPlanArtifactBeforeExecutor(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, "state.db")
	planPath := filepath.Join(root, "plan.json")
	writeApplyTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "artifact-role"
  assume_role_policy = "{}"
}
`)
	writeApplyTestFile(t, sourcePath, minimalIAMSmithyForApplyTest())
	if _, err := tfplan.Build(context.Background(), tfplan.Options{
		ConfigDir:  configDir,
		StatePath:  statePath,
		APISources: []tfplan.APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: sourcePath}},
		OutPath:    planPath,
	}); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	var planned tfplan.Document
	if err := json.Unmarshal([]byte(readApplyTestFile(t, planPath)), &planned); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{Address: "aws_iam_role.role", Type: "aws_iam_role", DesiredHash: planned.Resources[0].DesiredHash, Status: "managed"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	_ = store.Close()
	mock := &executor.MockExecutor{}
	_, err = Apply(context.Background(), Options{PlanPath: planPath, AutoApprove: true, Executor: mock})
	if err == nil || !strings.Contains(err.Error(), "apply.approval_mismatch") {
		t.Fatalf("stale plan error = %v", err)
	}
	if mock.RequestCount() != 0 {
		t.Fatalf("executor was called for stale plan")
	}

	doc := planned
	doc.Resources[0].DesiredHash = "tampered"
	tamperedPath := filepath.Join(root, "tampered.json")
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal tampered plan: %v", err)
	}
	writeApplyTestFile(t, tamperedPath, string(append(data, '\n')))
	_, err = Apply(context.Background(), Options{PlanPath: tamperedPath, AutoApprove: true, Executor: mock})
	if err == nil || !strings.Contains(err.Error(), "apply.approval_invalid") {
		t.Fatalf("tampered plan error = %v", err)
	}
	if mock.RequestCount() != 0 {
		t.Fatalf("executor was called for tampered plan")
	}
}

func TestApplyRejectsErroredPlanBeforeExecutor(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_iam_role" "role" {
  name = "role"
  assume_role_policy = "{}"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mock := &executor.MockExecutor{}
	result, err := Apply(context.Background(), Options{
		ConfigDir:   configDir,
		StatePath:   filepath.Join(root, "state.db"),
		APISources:  []APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: filepath.Join(root, "missing.json")}},
		AutoApprove: true,
		Executor:    mock,
	})
	if err == nil {
		t.Fatal("Apply succeeded unexpectedly")
	}
	if result == nil || !result.Plan.Errored {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(err.Error(), "errored") {
		t.Fatalf("error = %v", err)
	}
	if mock.RequestCount() != 0 {
		t.Fatalf("executor was called %d time(s)", mock.RequestCount())
	}
}

func TestApplyRejectsErroredPlanArtifactBeforeExecutor(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	planPath := filepath.Join(root, "errored.json")
	writeApplyTestFile(t, filepath.Join(configDir, "main.tf"), `
resource "aws_iam_role" "role" {
  name = "role"
  assume_role_policy = "{}"
}
`)
	if _, err := tfplan.Build(context.Background(), tfplan.Options{
		ConfigDir:  configDir,
		StatePath:  filepath.Join(root, "state.db"),
		APISources: []tfplan.APISourceInput{{Kind: "aws-smithy", ID: "iam", Path: filepath.Join(root, "missing.json")}},
		OutPath:    planPath,
	}); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	mock := &executor.MockExecutor{}
	result, err := Apply(context.Background(), Options{PlanPath: planPath, AutoApprove: true, Executor: mock})
	if err == nil || !strings.Contains(err.Error(), "apply.approval_invalid") {
		t.Fatalf("errored artifact error = %v", err)
	}
	if result == nil || !result.Plan.Errored {
		t.Fatalf("result = %#v", result)
	}
	if mock.RequestCount() != 0 {
		t.Fatalf("executor was called for errored artifact")
	}
}

type limitedApplyExecutor struct {
	requests  []executor.Request
	executeFn func(context.Context, executor.Request) (executor.Result, error)
}

type changingBrowserApplyExecutor struct {
	path                 string
	data                 []byte
	changeOnCapabilities bool
	changeOnFirstAttempt bool
	mutated              bool
	mutationErr          error
	calls                int
}

func (e *changingBrowserApplyExecutor) Capabilities() executor.CapabilityDescriptor {
	if e.changeOnCapabilities {
		e.mutate()
	}
	return (&executor.MockExecutor{}).Capabilities()
}

func (e *changingBrowserApplyExecutor) Execute(context.Context, executor.Request) (executor.Result, error) {
	e.calls++
	if e.changeOnFirstAttempt && e.calls == 1 {
		e.mutate()
		return executor.Result{}, fmt.Errorf("transient executor error")
	}
	return executor.Result{Success: true}, nil
}

func (e *changingBrowserApplyExecutor) mutate() {
	if !e.mutated {
		e.mutated = true
		e.mutationErr = os.WriteFile(e.path, e.data, 0o644)
	}
}

func (e *limitedApplyExecutor) Capabilities() executor.CapabilityDescriptor {
	return executor.CapabilityDescriptor{
		Protocols:   []string{"aws-smithy", "openapi", "google-discovery", "unknown"},
		AuthSchemes: []string{"test"},
		Features: []string{
			executor.FeatureOutputIdentity,
			executor.FeatureOutputComputed,
			executor.FeatureMissingEvidence,
			executor.FeatureProgressEvents,
			executor.FeatureIdempotency,
			executor.FeatureRetry,
		},
	}
}

func (e *limitedApplyExecutor) Execute(ctx context.Context, req executor.Request) (executor.Result, error) {
	e.requests = append(e.requests, req)
	if e.executeFn != nil {
		return e.executeFn(ctx, req)
	}
	return executor.Result{Success: true}, nil
}

func writeApplyProjectForTest(t *testing.T, dir string, profile project.Profile) string {
	t.Helper()
	doc := &uws1.Document{
		UWS: "1.4.0",
		Info: &uws1.Info{
			Title:   "apply_project_fixture",
			Version: "1.0.0",
		},
		Operations: []*uws1.Operation{{
			OperationID: "review",
			Request:     map[string]any{"x-test": true},
			Extensions:  map[string]any{uws1.ExtensionOperationProfile: "ramen-apply-test"},
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID: "main",
			Type:       uws1.WorkflowTypeSequence,
			Steps: []*uws1.Step{{
				StepID:       "review",
				OperationRef: "review",
			}},
		}},
		Extensions: map[string]any{
			project.ExtensionKey: profile,
		},
	}
	data, err := uwsconvert.MarshalJSONIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := filepath.Join(dir, project.DefaultJSON)
	writeApplyTestFile(t, path, string(data))
	return path
}

func applyProjectRole(address, name string) project.Resource {
	return project.Resource{
		Address:    address,
		Kind:       "resource",
		Type:       "aws_iam_role",
		Name:       strings.TrimPrefix(address, "aws_iam_role."),
		Provider:   "provider.aws",
		Attributes: map[string]any{"name": name, "assume_role_policy": "{}"},
		Operations: map[string]project.OperationRole{
			"create": {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "CreateRole"},
			"update": {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "CreateRole"},
			"read":   {SourceKind: "aws-smithy", SourceID: "iam", SourcePath: "aws-smithy/iam.json", OperationID: "GetRole"},
		},
		IdentityAttributes: []project.IdentityAttribute{{
			Name:        "role_name",
			Path:        "name",
			RequestKeys: []string{"RoleName"},
			Required:    true,
		}},
		ResponseBindings: []project.ResponseBinding{
			{OperationRole: "read", ResponsePath: "Role.RoleName", StatePath: "role_name", Identity: true},
			{OperationRole: "read", ResponsePath: "Role.Arn", StatePath: "arn", Computed: true},
		},
	}
}

func writeApplyTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readApplyTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func containsApplyTest(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func minimalIAMSmithyForApplyTest() string {
	return `{
  "smithy": "2.0",
  "shapes": {
    "com.amazonaws.iam#IAM": {
      "type": "service",
      "version": "2010-05-08",
      "operations": [
        {"target": "com.amazonaws.iam#CreateRole"},
        {"target": "com.amazonaws.iam#GetRole"}
      ],
      "traits": {
        "aws.api#service": {"sdkId": "IAM", "endpointPrefix": "iam"},
        "aws.auth#sigv4": {"name": "iam"},
        "aws.protocols#awsQuery": {}
      }
    },
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#GetRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#GetRoleRequest"}, "output": {"target": "com.amazonaws.iam#GetRoleResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#GetRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#GetRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`
}

func minimalStorageDiscoveryForApplyTest() string {
	return `{
  "discoveryVersion": "v1",
  "name": "storage",
  "version": "v1",
  "rootUrl": "https://storage.googleapis.com/",
  "servicePath": "storage/v1/",
  "schemas": {
    "Bucket": {
      "id": "Bucket",
      "type": "object",
      "properties": {"name": {"type": "string"}, "location": {"type": "string"}}
    }
  },
  "resources": {
    "buckets": {
      "methods": {
        "insert": {
          "id": "storage.buckets.insert",
          "path": "b",
          "httpMethod": "POST",
          "parameters": {"project": {"type": "string", "required": true, "location": "query"}},
          "request": {"$ref": "Bucket"},
          "response": {"$ref": "Bucket"}
        },
        "get": {
          "id": "storage.buckets.get",
          "path": "b/{bucket}",
          "httpMethod": "GET",
          "parameters": {"bucket": {"type": "string", "required": true, "location": "path"}},
          "response": {"$ref": "Bucket"}
        }
      }
    }
  }
}`
}

func minimalIAMSmithyForApplyFailureTest() string {
	return `{
  "smithy": "2.0",
  "shapes": {
    "com.amazonaws.iam#IAM": {
      "type": "service",
      "version": "2010-05-08",
      "operations": [
        {"target": "com.amazonaws.iam#CreateRole"},
        {"target": "com.amazonaws.iam#PutRolePolicy"}
      ],
      "traits": {
        "aws.api#service": {"sdkId": "IAM", "endpointPrefix": "iam"},
        "aws.auth#sigv4": {"name": "iam"},
        "aws.protocols#awsQuery": {}
      }
    },
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#PutRolePolicy": {"type": "operation", "input": {"target": "com.amazonaws.iam#PutRolePolicyRequest"}, "output": {"target": "com.amazonaws.iam#PutRolePolicyResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#PutRolePolicyRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "PolicyName": {"target": "com.amazonaws.iam#policyNameType"}, "PolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#PutRolePolicyResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`
}
