package executor

import (
	"context"
	"strings"
	"testing"
)

func TestCapabilitiesAndIdempotency(t *testing.T) {
	action := Action{
		Address:     "example.one",
		Type:        "example",
		Action:      "read",
		DesiredHash: "sha256:test",
		Mapping:     ActionMapping{SourceKind: "openapi", SourceID: "api", OperationID: "readOne"},
	}
	req := Request{Action: action}
	req.Capabilities = RequirementsForAction(action)
	req.Idempotency = IdempotencyForAction(action)
	if req.Idempotency.Key == "" || req.Idempotency.Key != IdempotencyForAction(action).Key {
		t.Fatalf("idempotency key is not stable: %#v", req.Idempotency)
	}
	if err := EnsureSupported(&MockExecutor{}, req); err != nil {
		t.Fatalf("EnsureSupported returned error: %v", err)
	}
	req.Capabilities.Features = append(req.Capabilities.Features, "unsupported.feature")
	if err := EnsureSupported(&MockExecutor{}, req); err == nil || !strings.Contains(err.Error(), "unsupported.feature") {
		t.Fatalf("EnsureSupported error = %v, want unsupported feature", err)
	}
}

func TestRequirementsForRuntimeHints(t *testing.T) {
	action := Action{
		Address: "example.one",
		Type:    "example",
		Action:  "create",
		Mapping: ActionMapping{SourceKind: "openapi", SourceID: "api", OperationID: "createOne"},
	}
	reqs := RequirementsForRuntimeHints(RequirementsForAction(action), RuntimeHints{
		Retry:  map[string]any{"max_attempts": 3},
		Waiter: map[string]any{"until": "exists"},
	})
	for _, feature := range []string{FeatureRetry, FeatureWaiter} {
		if !contains(reqs.Features, feature) {
			t.Fatalf("features %v missing %s", reqs.Features, feature)
		}
	}
}

func TestRequirementsForBrowserAreExplicitAndMockSupported(t *testing.T) {
	action := Action{
		Address: "example.browser", Type: "example_browser", Action: "update",
		Mapping: ActionMapping{SourceKind: "browser-profile", SourceID: "browser", OperationID: "change_status"},
	}
	requirement := RequirementsForBrowser(RequirementsForAction(action), BrowserRequirements{
		Contexts: true, ScalarOutputs: true, NamedSession: true, ExternalSession: true,
		Authentication: true, MutationApproval: true, AuthenticationApproval: true,
	})
	for _, feature := range []string{
		FeatureBrowserContexts, FeatureBrowserScalarOutputs, FeatureBrowserNamedSession,
		FeatureBrowserExternalSession, FeatureBrowserAuthentication,
		FeatureBrowserMutationApproval, FeatureBrowserAuthenticationApproval,
	} {
		if !contains(requirement.Features, feature) {
			t.Fatalf("requirements %v missing %s", requirement.Features, feature)
		}
	}
	if err := EnsureSupported(&MockExecutor{}, Request{Action: action, Capabilities: requirement}); err != nil {
		t.Fatalf("mock browser support: %v", err)
	}
}

func TestRecordedExecutorReplayAndRecord(t *testing.T) {
	action := Action{
		Address: "example.one",
		Type:    "example",
		Action:  "create",
		Mapping: ActionMapping{SourceKind: "openapi", SourceID: "api", OperationID: "createOne"},
	}
	req := Request{Action: action}
	req.Capabilities = RequirementsForAction(action)
	req.Idempotency = IdempotencyForAction(action)
	result := Result{Success: true, Identity: map[string]any{"id": "one"}}
	replay := NewRecordedExecutor([]RecordedCall{{Request: req, Result: result}})
	got, err := replay.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("replay Execute returned error: %v", err)
	}
	if got.Identity["id"] != "one" {
		t.Fatalf("replay result = %#v", got)
	}

	recorder := &RecordedExecutor{Recorder: &MockExecutor{Results: map[string]Result{"example.one": {Identity: map[string]any{"id": "one", "secret_token": "do-not-record"}}}}}
	got, err = recorder.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("record Execute returned error: %v", err)
	}
	if !got.Success || len(recorder.Calls) != 1 || recorder.Calls[0].Key == "" {
		t.Fatalf("recorded call result=%#v calls=%#v", got, recorder.Calls)
	}
	if strings.Contains(recorder.Calls[0].Result.Identity["secret_token"].(string), "do-not-record") {
		t.Fatalf("recorded result was not redacted: %#v", recorder.Calls[0].Result)
	}
}

func TestMockExecutorEmitsProgressEvents(t *testing.T) {
	action := Action{Address: "example.one", Type: "example", Action: "delete", Mapping: ActionMapping{SourceKind: "openapi", OperationID: "deleteOne"}}
	req := Request{Action: action}
	req.Capabilities = RequirementsForAction(action)
	req.Idempotency = IdempotencyForAction(action)
	var events []Event
	req.Events = func(event Event) {
		events = append(events, event)
	}
	result, err := (&MockExecutor{}).Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(events) != 2 || len(result.Events) != 2 || events[0].Phase != "started" || events[1].Phase != "finished" {
		t.Fatalf("events=%#v result=%#v", events, result.Events)
	}
}
