package executor

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	SourceKind  string `json:"source_kind,omitempty"`
	SourceID    string `json:"source_id,omitempty"`
	SourcePath  string `json:"source_path,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
}

// Request is the explicit trusted-executor boundary. Credential material must
// stay in executor-owned configuration and must not be embedded here.
type Request struct {
	RunID      int64          `json:"run_id,omitempty"`
	Action     Action         `json:"action"`
	Document   *uws1.Document `json:"-"`
	WorkingDir string         `json:"working_dir,omitempty"`
	OutDir     string         `json:"out_dir,omitempty"`
}

// Result captures response-derived facts that Ramen may persist after
// redaction. Raw request and response bodies remain executor-owned.
type Result struct {
	Address    string         `json:"address,omitempty"`
	Operation  string         `json:"operation,omitempty"`
	Success    bool           `json:"success"`
	Identity   map[string]any `json:"identity,omitempty"`
	Computed   map[string]any `json:"computed,omitempty"`
	Messages   []string       `json:"messages,omitempty"`
	StartedAt  time.Time      `json:"started_at,omitempty"`
	FinishedAt time.Time      `json:"finished_at,omitempty"`
}

// Executor executes one approved action document.
type Executor interface {
	Execute(context.Context, Request) (Result, error)
}

// MockExecutor is a deterministic executor for public tests and recorded
// examples. It never performs network I/O.
type MockExecutor struct {
	mu        sync.Mutex
	Requests  []Request
	Results   map[string]Result
	ExecuteFn func(context.Context, Request) (Result, error)
}

func (m *MockExecutor) Execute(ctx context.Context, req Request) (Result, error) {
	if m == nil {
		return Result{}, fmt.Errorf("mock executor is nil")
	}
	m.mu.Lock()
	m.Requests = append(m.Requests, req)
	m.mu.Unlock()
	if m.ExecuteFn != nil {
		return m.ExecuteFn(ctx, req)
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
		return result, nil
	}
	now := time.Now().UTC()
	return Result{
		Address:    req.Action.Address,
		Operation:  req.Action.Mapping.OperationID,
		Success:    true,
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
