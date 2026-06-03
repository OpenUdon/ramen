package asyncrecord

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	asyncevidence "github.com/OpenUdon/evidence/async"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/state"
)

type Recorder struct {
	Store    *state.Store
	RunID    int64
	mu       sync.Mutex
	sequence int64
}

func New(store *state.Store, runID int64) *Recorder {
	return &Recorder{Store: store, RunID: runID}
}

func (r *Recorder) RecordRequest(ctx context.Context, req executor.Request) (string, error) {
	if r == nil || r.Store == nil {
		return "", nil
	}
	sequence := r.next()
	record := executor.AsyncExecutionRequestEvidence(req, r.evidenceID(req, "execution-request", sequence), r.attemptID(req), sequence, time.Now().UTC())
	if diagnostics := asyncevidence.ValidateExecutionRequest(record); len(diagnostics) != 0 {
		return "", fmt.Errorf("async execution request evidence invalid: %#v", diagnostics)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	return record.Attempt.EvidenceID, r.Store.RecordAsyncEvidence(ctx, state.AsyncEvidenceRecord{
		RunID:           r.RunID,
		ResourceAddress: req.Action.Address,
		Action:          req.Action.Action,
		OperationID:     req.Action.Mapping.OperationID,
		RecordKind:      "execution_request",
		Phase:           "submitted",
		EvidenceID:      record.Attempt.EvidenceID,
		AttemptID:       record.Attempt.AttemptID,
		Sequence:        record.Attempt.Sequence,
		RecordJSON:      string(data),
		CreatedAt:       record.Attempt.RecordedAt,
	})
}

func (r *Recorder) RecordResponse(ctx context.Context, req executor.Request, result executor.Result, execErr error, requestEvidenceID string) error {
	if r == nil || r.Store == nil {
		return nil
	}
	sequence := r.next()
	record := executor.AsyncExecutionResponseEvidence(req, result, execErr, r.evidenceID(req, "execution-response", sequence), r.attemptID(req), requestEvidenceID, sequence, time.Now().UTC())
	if diagnostics := asyncevidence.ValidateExecutionResponse(record); len(diagnostics) != 0 {
		return fmt.Errorf("async execution response evidence invalid: %#v", diagnostics)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return r.Store.RecordAsyncEvidence(ctx, state.AsyncEvidenceRecord{
		RunID:             r.RunID,
		ResourceAddress:   req.Action.Address,
		Action:            req.Action.Action,
		OperationID:       req.Action.Mapping.OperationID,
		RecordKind:        "execution_response",
		Phase:             record.Outcome,
		EvidenceID:        record.Attempt.EvidenceID,
		AttemptID:         record.Attempt.AttemptID,
		RequestEvidenceID: record.RequestEvidenceID,
		Sequence:          record.Attempt.Sequence,
		RecordJSON:        string(data),
		CreatedAt:         record.Attempt.RecordedAt,
	})
}

func (r *Recorder) RecordStatus(ctx context.Context, req executor.Request, event executor.Event, requestEvidenceID string) error {
	if r == nil || r.Store == nil {
		return nil
	}
	sequence := r.next()
	record := executor.AsyncStatusObservationEvidence(req, event, r.evidenceID(req, "status-observation", sequence), r.attemptID(req), requestEvidenceID, sequence)
	if diagnostics := asyncevidence.ValidateStatusObservation(record); len(diagnostics) != 0 {
		return fmt.Errorf("async status evidence invalid: %#v", diagnostics)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return r.Store.RecordAsyncEvidence(ctx, state.AsyncEvidenceRecord{
		RunID:             r.RunID,
		ResourceAddress:   req.Action.Address,
		Action:            req.Action.Action,
		OperationID:       req.Action.Mapping.OperationID,
		RecordKind:        "status_observation",
		Phase:             record.Status,
		EvidenceID:        record.Attempt.EvidenceID,
		AttemptID:         record.Attempt.AttemptID,
		RequestEvidenceID: record.RequestEvidenceID,
		Sequence:          record.Attempt.Sequence,
		RecordJSON:        string(data),
		CreatedAt:         record.Attempt.RecordedAt,
	})
}

func (r *Recorder) RecordConfirmationRead(ctx context.Context, req executor.Request, result executor.Result, execErr error, requestEvidenceID string) error {
	if r == nil || r.Store == nil {
		return nil
	}
	sequence := r.next()
	record := executor.AsyncConfirmationReadObservationEvidence(req, result, execErr, r.evidenceID(req, "confirmation-read", sequence), r.attemptID(req), requestEvidenceID, sequence, time.Now().UTC())
	if diagnostics := asyncevidence.ValidateConfirmationReadObservation(record); len(diagnostics) != 0 {
		return fmt.Errorf("async confirmation read evidence invalid: %#v", diagnostics)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return r.Store.RecordAsyncEvidence(ctx, state.AsyncEvidenceRecord{
		RunID:             r.RunID,
		ResourceAddress:   req.Action.Address,
		Action:            req.Action.Action,
		OperationID:       req.Action.Mapping.OperationID,
		RecordKind:        "confirmation_read",
		Phase:             record.Outcome,
		EvidenceID:        record.Attempt.EvidenceID,
		AttemptID:         record.Attempt.AttemptID,
		RequestEvidenceID: record.RequestEvidenceID,
		Sequence:          record.Attempt.Sequence,
		RecordJSON:        string(data),
		CreatedAt:         record.Attempt.RecordedAt,
	})
}

func (r *Recorder) EventSink(ctx context.Context, req executor.Request, requestEvidenceID string, next executor.EventSink) executor.EventSink {
	return func(event executor.Event) {
		if next != nil {
			next(event)
		}
		_ = r.RecordStatus(ctx, req, event, requestEvidenceID)
	}
}

func (r *Recorder) next() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sequence++
	return r.sequence
}

func (r *Recorder) evidenceID(req executor.Request, kind string, sequence int64) string {
	return fmt.Sprintf("ramen-run-%d-%s-%s-%d", r.RunID, kind, stablePart(req), sequence)
}

func (r *Recorder) attemptID(req executor.Request) string {
	return fmt.Sprintf("ramen-run-%d-%s", r.RunID, stablePart(req))
}

var unsafeIDPart = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func stablePart(req executor.Request) string {
	parts := []string{req.Action.Address, req.Action.Action, req.Action.Mapping.OperationID, req.Idempotency.Key}
	joined := strings.Trim(unsafeIDPart.ReplaceAllString(strings.Join(parts, "-"), "-"), "-")
	if joined == "" {
		return "executor"
	}
	if len(joined) > 96 {
		return joined[:96]
	}
	return joined
}
