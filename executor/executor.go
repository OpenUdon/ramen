package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/OpenUdon/ramen/internal/redact"
	"github.com/OpenUdon/uws/uws1"
)

// Action describes one approved plan action handed to a trusted executor.
type Action struct {
	Address     string            `json:"address"`
	Type        string            `json:"type"`
	Provider    string            `json:"provider,omitempty"`
	Action      string            `json:"action"`
	DesiredHash string            `json:"desired_hash,omitempty"`
	Mapping     ActionMapping     `json:"mapping"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type ActionMapping struct {
	Method      string `json:"method,omitempty"`
	SourceKind  string `json:"source_kind,omitempty"`
	SourceID    string `json:"source_id,omitempty"`
	SourcePath  string `json:"source_path,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
}

const (
	FeatureOutputIdentity  = "output.identity"
	FeatureOutputComputed  = "output.computed"
	FeatureMissingEvidence = "output.missing"
	FeatureProgressEvents  = "progress.events"
	FeatureIdempotency     = "idempotency"
	FeatureRetry           = "retry"
	FeatureWaiter          = "waiter"
	FeaturePagination      = "pagination"
)

type CapabilityDescriptor struct {
	Protocols   []string `json:"protocols,omitempty"`
	AuthSchemes []string `json:"auth_schemes,omitempty"`
	Features    []string `json:"features,omitempty"`
}

type CapabilityRequirement struct {
	Protocol string   `json:"protocol,omitempty"`
	Features []string `json:"features,omitempty"`
}

type Capable interface {
	Capabilities() CapabilityDescriptor
}

type Idempotency struct {
	Key       string `json:"key,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Supported bool   `json:"supported,omitempty"`
}

type RuntimeHints struct {
	Retry      map[string]any `json:"retry,omitempty"`
	Waiter     map[string]any `json:"waiter,omitempty"`
	Pagination map[string]any `json:"pagination,omitempty"`
}

type Event struct {
	Time      time.Time      `json:"time,omitempty"`
	RunID     int64          `json:"run_id,omitempty"`
	Address   string         `json:"address,omitempty"`
	Action    string         `json:"action,omitempty"`
	Operation string         `json:"operation,omitempty"`
	Phase     string         `json:"phase"`
	Message   string         `json:"message,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type EventSink func(Event)

const FeedbackVersion = "ramen.feedback.v1"

type FeedbackRecord struct {
	Version    string         `json:"version"`
	RunID      int64          `json:"run_id,omitempty"`
	Address    string         `json:"address"`
	Action     string         `json:"action"`
	Operation  string         `json:"operation,omitempty"`
	Success    bool           `json:"success"`
	Missing    bool           `json:"missing,omitempty"`
	ErrorClass string         `json:"error_class,omitempty"`
	Identity   map[string]any `json:"identity,omitempty"`
	Computed   map[string]any `json:"computed,omitempty"`
	Messages   []string       `json:"messages,omitempty"`
	Events     []Event        `json:"events,omitempty"`
	StartedAt  time.Time      `json:"started_at,omitempty"`
	FinishedAt time.Time      `json:"finished_at,omitempty"`
}

// Request is the explicit trusted-executor boundary. Credential material must
// stay in executor-owned configuration and must not be embedded here.
type Request struct {
	RunID        int64                 `json:"run_id,omitempty"`
	Action       Action                `json:"action"`
	Document     *uws1.Document        `json:"-"`
	WorkingDir   string                `json:"working_dir,omitempty"`
	OutDir       string                `json:"out_dir,omitempty"`
	Capabilities CapabilityRequirement `json:"capabilities,omitempty"`
	Idempotency  Idempotency           `json:"idempotency,omitempty"`
	Runtime      RuntimeHints          `json:"runtime,omitempty"`
	Events       EventSink             `json:"-"`
}

// Result captures response-derived facts that Ramen may persist after
// redaction. Raw request and response bodies remain executor-owned.
type Result struct {
	Address    string         `json:"address,omitempty"`
	Operation  string         `json:"operation,omitempty"`
	Success    bool           `json:"success"`
	Missing    bool           `json:"missing,omitempty"`
	Identity   map[string]any `json:"identity,omitempty"`
	Computed   map[string]any `json:"computed,omitempty"`
	Messages   []string       `json:"messages,omitempty"`
	Events     []Event        `json:"events,omitempty"`
	StartedAt  time.Time      `json:"started_at,omitempty"`
	FinishedAt time.Time      `json:"finished_at,omitempty"`
}

// Executor executes one approved action document.
type Executor interface {
	Execute(context.Context, Request) (Result, error)
}

func EnsureSupported(exec Executor, req Request) error {
	if exec == nil {
		return fmt.Errorf("executor is nil")
	}
	capable, ok := exec.(Capable)
	if !ok {
		return fmt.Errorf("executor capability descriptor is required for %s %s", req.Action.Action, req.Action.Address)
	}
	caps := capable.Capabilities()
	if req.Capabilities.Protocol != "" && !contains(caps.Protocols, req.Capabilities.Protocol) {
		return fmt.Errorf("executor capability unsupported protocol %q for %s", req.Capabilities.Protocol, req.Action.Address)
	}
	for _, feature := range req.Capabilities.Features {
		if !contains(caps.Features, feature) {
			return fmt.Errorf("executor capability unsupported feature %q for %s", feature, req.Action.Address)
		}
	}
	return nil
}

func RequirementsForAction(action Action) CapabilityRequirement {
	req := CapabilityRequirement{Protocol: action.Mapping.SourceKind}
	if req.Protocol == "" {
		req.Protocol = "unknown"
	}
	req.Features = append(req.Features, FeatureIdempotency)
	switch action.Action {
	case "read":
		req.Features = append(req.Features, FeatureOutputIdentity, FeatureOutputComputed, FeatureMissingEvidence)
	case "create", "update", "post", "put", "patch":
		req.Features = append(req.Features, FeatureOutputIdentity, FeatureOutputComputed)
	}
	req.Features = append(req.Features, FeatureProgressEvents)
	return req
}

func RequirementsForRuntimeHints(req CapabilityRequirement, hints RuntimeHints) CapabilityRequirement {
	if len(hints.Retry) > 0 && !contains(req.Features, FeatureRetry) {
		req.Features = append(req.Features, FeatureRetry)
	}
	if len(hints.Waiter) > 0 && !contains(req.Features, FeatureWaiter) {
		req.Features = append(req.Features, FeatureWaiter)
	}
	if len(hints.Pagination) > 0 && !contains(req.Features, FeaturePagination) {
		req.Features = append(req.Features, FeaturePagination)
	}
	return req
}

func IdempotencyForAction(action Action) Idempotency {
	payload := []string{
		action.Address,
		action.Type,
		action.Action,
		action.DesiredHash,
		action.Mapping.SourceKind,
		action.Mapping.SourceID,
		action.Mapping.OperationID,
	}
	sum := sha256.Sum256([]byte(strings.Join(payload, "\x00")))
	return Idempotency{Key: "ramen-" + hex.EncodeToString(sum[:16]), Scope: "resource-action", Supported: true}
}

func Emit(req Request, phase, message string, metadata map[string]any) Event {
	event := Event{
		Time:      time.Now().UTC(),
		RunID:     req.RunID,
		Address:   req.Action.Address,
		Action:    req.Action.Action,
		Operation: req.Action.Mapping.OperationID,
		Phase:     phase,
		Message:   message,
		Metadata:  metadata,
	}
	if req.Events != nil {
		req.Events(event)
	}
	return event
}

// MockExecutor is a deterministic executor for public tests and recorded
// examples. It never performs network I/O.
type MockExecutor struct {
	mu        sync.Mutex
	Requests  []Request
	Results   map[string]Result
	ExecuteFn func(context.Context, Request) (Result, error)
}

func (m *MockExecutor) Capabilities() CapabilityDescriptor {
	return CapabilityDescriptor{
		Protocols:   []string{"aws-smithy", "openapi", "google-discovery", "uws", "unknown"},
		AuthSchemes: []string{"mock"},
		Features: []string{
			FeatureOutputIdentity,
			FeatureOutputComputed,
			FeatureMissingEvidence,
			FeatureProgressEvents,
			FeatureIdempotency,
			FeatureRetry,
			FeatureWaiter,
			FeaturePagination,
		},
	}
}

func (m *MockExecutor) Execute(ctx context.Context, req Request) (Result, error) {
	if m == nil {
		return Result{}, fmt.Errorf("mock executor is nil")
	}
	if err := EnsureSupported(m, req); err != nil {
		return Result{}, err
	}
	startEvent := Emit(req, "started", "mock executor started", nil)
	m.mu.Lock()
	m.Requests = append(m.Requests, req)
	m.mu.Unlock()
	if m.ExecuteFn != nil {
		result, err := m.ExecuteFn(ctx, req)
		if err == nil {
			result.Events = append([]Event{startEvent}, result.Events...)
			result.Events = append(result.Events, Emit(req, "finished", "mock executor finished", nil))
		}
		return result, err
	}
	if result, ok := m.Results[req.Action.Address]; ok {
		if result.Address == "" {
			result.Address = req.Action.Address
		}
		if result.Operation == "" {
			result.Operation = req.Action.Mapping.OperationID
		}
		result.Success = true
		now := time.Now().UTC()
		if result.StartedAt.IsZero() {
			result.StartedAt = now
		}
		if result.FinishedAt.IsZero() {
			result.FinishedAt = now
		}
		result.Events = append([]Event{startEvent}, result.Events...)
		result.Events = append(result.Events, Emit(req, "finished", "mock executor finished", nil))
		return result, nil
	}
	now := time.Now().UTC()
	return Result{
		Address:    req.Action.Address,
		Operation:  req.Action.Mapping.OperationID,
		Success:    true,
		Events:     []Event{startEvent, Emit(req, "finished", "mock executor finished", nil)},
		StartedAt:  now,
		FinishedAt: now,
	}, nil
}

func (m *MockExecutor) RequestCount() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Requests)
}

type Recording struct {
	Version string         `json:"version"`
	Calls   []RecordedCall `json:"calls"`
}

type RecordedCall struct {
	Key     string  `json:"key"`
	Request Request `json:"request"`
	Result  Result  `json:"result"`
	Error   string  `json:"error,omitempty"`
}

type RecordedExecutor struct {
	mu       sync.Mutex
	Records  map[string]RecordedCall
	Recorder Executor
	Calls    []RecordedCall
}

func NewRecordedExecutor(calls []RecordedCall) *RecordedExecutor {
	records := map[string]RecordedCall{}
	for _, call := range calls {
		key := call.Key
		if key == "" {
			key = RequestKey(call.Request)
			call.Key = key
		}
		records[key] = call
	}
	return &RecordedExecutor{Records: records}
}

func LoadRecording(path string) (*RecordedExecutor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var recording Recording
	if err := json.Unmarshal(data, &recording); err != nil {
		return nil, err
	}
	return NewRecordedExecutor(recording.Calls), nil
}

func (r *RecordedExecutor) Save(path string) error {
	if r == nil {
		return fmt.Errorf("recorded executor is nil")
	}
	r.mu.Lock()
	calls := slices.Clone(r.Calls)
	r.mu.Unlock()
	data, err := json.MarshalIndent(Recording{Version: "ramen.executor.recording.v1", Calls: calls}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (r *RecordedExecutor) Capabilities() CapabilityDescriptor {
	if r != nil && r.Recorder != nil {
		if capable, ok := r.Recorder.(Capable); ok {
			return capable.Capabilities()
		}
	}
	return (&MockExecutor{}).Capabilities()
}

func (r *RecordedExecutor) Execute(ctx context.Context, req Request) (Result, error) {
	if r == nil {
		return Result{}, fmt.Errorf("recorded executor is nil")
	}
	if err := EnsureSupported(r, req); err != nil {
		return Result{}, err
	}
	key := RequestKey(req)
	r.mu.Lock()
	if call, ok := r.Records[key]; ok {
		r.mu.Unlock()
		if call.Error != "" {
			return Result{}, fmt.Errorf("%s", call.Error)
		}
		Emit(req, "replayed", "recorded executor replayed result", nil)
		return call.Result, nil
	}
	r.mu.Unlock()
	if r.Recorder == nil {
		return Result{}, fmt.Errorf("recorded executor missing replay for %s", key)
	}
	result, err := r.Recorder.Execute(ctx, req)
	call := RecordedCall{Key: key, Request: req, Result: RedactResult(result)}
	if err != nil {
		call.Error = err.Error()
	}
	r.mu.Lock()
	r.Calls = append(r.Calls, call)
	r.mu.Unlock()
	return result, err
}

func RedactResult(result Result) Result {
	result.Identity = redact.Map(result.Identity)
	result.Computed = redact.Map(result.Computed)
	for i, msg := range result.Messages {
		result.Messages[i] = redact.String(msg)
	}
	for i := range result.Events {
		result.Events[i].Message = redact.String(result.Events[i].Message)
		result.Events[i].Metadata = redact.Map(result.Events[i].Metadata)
	}
	return result
}

func FeedbackFromResult(req Request, result Result, err error) FeedbackRecord {
	redacted := RedactResult(result)
	record := FeedbackRecord{
		Version:    FeedbackVersion,
		RunID:      req.RunID,
		Address:    req.Action.Address,
		Action:     req.Action.Action,
		Operation:  firstNonEmpty(redacted.Operation, req.Action.Mapping.OperationID),
		Success:    redacted.Success && err == nil,
		Missing:    redacted.Missing,
		Identity:   redacted.Identity,
		Computed:   redacted.Computed,
		Messages:   redacted.Messages,
		Events:     redacted.Events,
		StartedAt:  redacted.StartedAt,
		FinishedAt: redacted.FinishedAt,
	}
	if err != nil {
		record.ErrorClass = "executor_error"
		record.Messages = append(record.Messages, redact.String(err.Error()))
	} else if !redacted.Success {
		record.ErrorClass = "executor_unsuccessful"
	}
	return record
}

func RequestKey(req Request) string {
	payload := []string{
		req.Action.Address,
		req.Action.Type,
		req.Action.Action,
		req.Action.DesiredHash,
		req.Action.Mapping.SourceKind,
		req.Action.Mapping.SourceID,
		req.Action.Mapping.OperationID,
		req.Idempotency.Key,
	}
	sum := sha256.Sum256([]byte(strings.Join(payload, "\x00")))
	return hex.EncodeToString(sum[:])
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
