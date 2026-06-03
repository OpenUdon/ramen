package executor

import (
	"strings"
	"time"

	asyncevidence "github.com/OpenUdon/evidence/async"
)

func AsyncExecutionRequestEvidence(req Request, evidenceID, attemptID string, sequence int64, recordedAt time.Time) asyncevidence.ExecutionRequest {
	return asyncevidence.NormalizeExecutionRequest(asyncevidence.ExecutionRequest{
		Version: asyncevidence.ExecutionRequestVersion,
		Attempt: asyncevidence.AttemptMetadata{
			EvidenceID: evidenceID,
			AttemptID:  attemptID,
			Sequence:   sequence,
			Source:     "ramen.executor",
			RecordedAt: recordedAt,
		},
		RequestID: req.Idempotency.Key,
		Operation: asyncOperationRef(req.Action),
		Runtime: asyncevidence.RuntimeHints{
			Retry:      cloneAnyMap(req.Runtime.Retry),
			Waiter:     cloneAnyMap(req.Runtime.Waiter),
			Pagination: cloneAnyMap(req.Runtime.Pagination),
		},
		Transport: map[string]string{
			"working_dir": strings.TrimSpace(req.WorkingDir),
			"out_dir":     strings.TrimSpace(req.OutDir),
		},
		Metadata: map[string]string{
			"idempotency_scope": strings.TrimSpace(req.Idempotency.Scope),
		},
	})
}

func AsyncExecutionResponseEvidence(req Request, result Result, execErr error, evidenceID, attemptID, requestEvidenceID string, sequence int64, recordedAt time.Time) asyncevidence.ExecutionResponse {
	outcome := "accepted"
	errorSummary := ""
	if execErr != nil {
		outcome = "fatal_failure"
		errorSummary = execErr.Error()
	} else if !result.Success {
		outcome = "rejected"
		errorSummary = strings.Join(result.Messages, "; ")
	}
	return asyncevidence.NormalizeExecutionResponse(asyncevidence.ExecutionResponse{
		Version: asyncevidence.ExecutionResponseVersion,
		Attempt: asyncevidence.AttemptMetadata{
			EvidenceID: evidenceID,
			AttemptID:  attemptID,
			Sequence:   sequence,
			Source:     "ramen.executor",
			RecordedAt: recordedAt,
		},
		RequestEvidenceID: strings.TrimSpace(requestEvidenceID),
		Operation:         asyncOperationRef(req.Action),
		Outcome:           outcome,
		ErrorSummary:      errorSummary,
		StartedAt:         result.StartedAt,
		FinishedAt:        result.FinishedAt,
	})
}

func AsyncStatusObservationEvidence(req Request, event Event, evidenceID, attemptID, requestEvidenceID string, sequence int64) asyncevidence.StatusObservation {
	return asyncevidence.NormalizeStatusObservation(asyncevidence.StatusObservation{
		Version: asyncevidence.StatusObservationVersion,
		Attempt: asyncevidence.AttemptMetadata{
			EvidenceID: evidenceID,
			AttemptID:  attemptID,
			Sequence:   sequence,
			Source:     "ramen.executor",
			RecordedAt: event.Time,
		},
		RequestEvidenceID: strings.TrimSpace(requestEvidenceID),
		Operation:         asyncOperationRef(req.Action),
		Status:            event.Phase,
		TerminalityHint:   terminalityHint(event.Phase),
		ObservedAt:        event.Time,
	})
}

func AsyncConfirmationReadObservationEvidence(req Request, result Result, execErr error, evidenceID, attemptID, requestEvidenceID string, sequence int64, observedAt time.Time) asyncevidence.ConfirmationReadObservation {
	outcome := "unknown"
	if execErr != nil {
		outcome = "failed"
	} else if result.Missing {
		outcome = "missing"
	} else if result.Success {
		outcome = "exists"
	} else {
		outcome = "failed"
	}
	return asyncevidence.NormalizeConfirmationReadObservation(asyncevidence.ConfirmationReadObservation{
		Version: asyncevidence.ConfirmationReadObservationVersion,
		Attempt: asyncevidence.AttemptMetadata{
			EvidenceID: evidenceID,
			AttemptID:  attemptID,
			Sequence:   sequence,
			Source:     "ramen.executor",
			RecordedAt: observedAt,
		},
		RequestEvidenceID: strings.TrimSpace(requestEvidenceID),
		Operation:         asyncOperationRef(req.Action),
		Outcome:           outcome,
		ObservedAt:        observedAt,
	})
}

func asyncOperationRef(action Action) asyncevidence.OperationRef {
	return asyncevidence.NormalizeOperation(asyncevidence.OperationRef{
		SubjectKind: action.Type,
		SubjectID:   action.Address,
		Action:      action.Action,
		Method:      action.Mapping.Method,
		SourceKind:  action.Mapping.SourceKind,
		SourceID:    action.Mapping.SourceID,
		SourcePath:  action.Mapping.SourcePath,
		OperationID: action.Mapping.OperationID,
	})
}

func terminalityHint(phase string) string {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "finished", "confirmed", "missing", "exists", "succeeded", "success":
		return "success"
	case "failed", "error":
		return "failure"
	case "started", "polling", "waiting", "retrying":
		return "in_progress"
	default:
		return ""
	}
}

func cloneAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		if strings.TrimSpace(key) != "" {
			out[key] = value
		}
	}
	return out
}
