package executor

import (
	"errors"
	"testing"
	"time"

	asyncevidence "github.com/OpenUdon/evidence/async"
)

func TestAsyncExecutionRequestEvidenceCarriesRuntimeMetadata(t *testing.T) {
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	action := Action{
		Address: "resource.example",
		Type:    "example_type",
		Action:  "put",
		Mapping: ActionMapping{Method: "put", SourceKind: "openapi", SourceID: "azure", SourcePath: "azure.json", OperationID: "Databases_CreateOrUpdate"},
	}
	req := Request{
		Action:      action,
		WorkingDir:  "/tmp/work",
		OutDir:      "/tmp/out",
		Idempotency: Idempotency{Key: "ramen-key", Scope: "resource-action", Supported: true},
		Runtime: RuntimeHints{
			Retry:  map[string]any{"max_attempts": 3},
			Waiter: map[string]any{"until": "exists"},
		},
	}
	record := AsyncExecutionRequestEvidence(req, "ev-1", "attempt-1", 7, now)
	if record.Version != asyncevidence.ExecutionRequestVersion {
		t.Fatalf("version = %q", record.Version)
	}
	if record.Attempt.EvidenceID != "ev-1" || record.Attempt.Sequence != 7 || !record.Attempt.RecordedAt.Equal(now) {
		t.Fatalf("attempt = %#v", record.Attempt)
	}
	if record.Operation.SubjectID != action.Address || record.Operation.Action != "put" || record.Operation.Method != "PUT" || record.Operation.OperationID != action.Mapping.OperationID {
		t.Fatalf("operation = %#v", record.Operation)
	}
	if record.Runtime.Retry["max_attempts"] != 3 || record.Runtime.Waiter["until"] != "exists" {
		t.Fatalf("runtime = %#v", record.Runtime)
	}
	if diagnostics := asyncevidence.ValidateExecutionRequest(record); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestAsyncExecutionResponseEvidenceDoesNotImplyConvergence(t *testing.T) {
	req := Request{Action: Action{Address: "resource.example", Type: "example_type", Action: "delete", Mapping: ActionMapping{SourceKind: "openapi", OperationID: "Databases_Delete"}}}
	record := AsyncExecutionResponseEvidence(req, Result{Success: true}, nil, "ev-response", "attempt-1", "ev-request", 2, time.Now())
	if record.Outcome != "accepted" {
		t.Fatalf("outcome = %q", record.Outcome)
	}
	if record.Operation.Action != "delete" {
		t.Fatalf("operation = %#v", record.Operation)
	}
	if diagnostics := asyncevidence.ValidateExecutionResponse(record); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestAsyncExecutionResponseEvidenceClassifiesExecutorFailure(t *testing.T) {
	req := Request{Action: Action{Address: "resource.example", Type: "example_type", Action: "put", Mapping: ActionMapping{SourceKind: "openapi", OperationID: "PutThing"}}}
	record := AsyncExecutionResponseEvidence(req, Result{}, errors.New("boom"), "ev-response", "attempt-1", "ev-request", 2, time.Now())
	if record.Outcome != "fatal_failure" || record.ErrorSummary != "boom" {
		t.Fatalf("record = %#v", record)
	}
}

func TestAsyncConfirmationReadObservationEvidenceClassifiesReadOutcome(t *testing.T) {
	req := Request{Action: Action{Address: "resource.example", Type: "example_type", Action: "read", Mapping: ActionMapping{SourceKind: "openapi", OperationID: "Databases_Get"}}}
	missing := AsyncConfirmationReadObservationEvidence(req, Result{Success: true, Missing: true}, nil, "ev-read", "attempt-1", "ev-request", 3, time.Now())
	if missing.Outcome != "missing" {
		t.Fatalf("missing outcome = %q", missing.Outcome)
	}
	exists := AsyncConfirmationReadObservationEvidence(req, Result{Success: true}, nil, "ev-read-2", "attempt-1", "ev-request", 4, time.Now())
	if exists.Outcome != "exists" {
		t.Fatalf("exists outcome = %q", exists.Outcome)
	}
	if diagnostics := asyncevidence.ValidateConfirmationReadObservation(missing); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestAsyncEvidenceAlignmentUsesNeutralValidators(t *testing.T) {
	now := time.Date(2026, 6, 3, 13, 0, 0, 0, time.UTC)
	req := Request{
		Action: Action{
			Address: "azurerm_sql_database.ramen",
			Type:    "azurerm_sql_database",
			Action:  "put",
			Mapping: ActionMapping{
				Method:      "put",
				SourceKind:  "openapi",
				SourceID:    "azure-sql",
				SourcePath:  "azure-sql.json",
				OperationID: "Databases_CreateOrUpdate",
			},
		},
		Idempotency: Idempotency{Key: "sha256:test", Scope: "resource-action"},
		Runtime: RuntimeHints{
			Retry:  map[string]any{"max_attempts": 2},
			Waiter: map[string]any{"until": "exists"},
		},
	}

	request := AsyncExecutionRequestEvidence(req, "ev-request", "attempt-1", 1, now)
	response := AsyncExecutionResponseEvidence(req, Result{Success: true, StartedAt: now, FinishedAt: now.Add(time.Second)}, nil, "ev-response", "attempt-1", request.Attempt.EvidenceID, 2, now.Add(time.Second))
	status := AsyncStatusObservationEvidence(req, Event{Phase: "waiting", Time: now.Add(2 * time.Second)}, "ev-status", "attempt-1", request.Attempt.EvidenceID, 3)
	read := AsyncConfirmationReadObservationEvidence(req, Result{Success: true}, nil, "ev-read", "attempt-1", request.Attempt.EvidenceID, 4, now.Add(3*time.Second))

	if diagnostics := asyncevidence.ValidateExecutionRequest(request); len(diagnostics) != 0 {
		t.Fatalf("request diagnostics = %#v", diagnostics)
	}
	if diagnostics := asyncevidence.ValidateExecutionResponse(response); len(diagnostics) != 0 {
		t.Fatalf("response diagnostics = %#v", diagnostics)
	}
	if diagnostics := asyncevidence.ValidateStatusObservation(status); len(diagnostics) != 0 {
		t.Fatalf("status diagnostics = %#v", diagnostics)
	}
	if diagnostics := asyncevidence.ValidateConfirmationReadObservation(read); len(diagnostics) != 0 {
		t.Fatalf("read diagnostics = %#v", diagnostics)
	}
	if response.Outcome != "accepted" || read.Outcome != "exists" {
		t.Fatalf("records should separate accepted execution from confirmation read: response=%#v read=%#v", response, read)
	}
	if request.Operation.SubjectKind != "azurerm_sql_database" || request.Operation.SubjectID != "azurerm_sql_database.ramen" {
		t.Fatalf("operation should remain product-neutral resource identity: %#v", request.Operation)
	}
}
