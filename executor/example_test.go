package executor_test

import (
	"context"
	"fmt"

	"github.com/OpenUdon/ramen/executor"
)

// platformExecutor is a credential-free example of Ramen's in-process trusted
// executor contract. Production adapters keep credentials in executor-owned
// configuration rather than in executor.Request.
type platformExecutor struct{}

func (platformExecutor) Capabilities() executor.CapabilityDescriptor {
	return executor.CapabilityDescriptor{
		Protocols:   []string{"openapi"},
		AuthSchemes: []string{"executor-configured"},
		Features: []string{
			executor.FeatureIdempotency,
			executor.FeatureProgressEvents,
			executor.FeatureOutputIdentity,
			executor.FeatureOutputComputed,
		},
	}
}

func (platformExecutor) Execute(_ context.Context, req executor.Request) (executor.Result, error) {
	return executor.Result{
		Address:   req.Action.Address,
		Operation: req.Action.Mapping.OperationID,
		Success:   true,
		Identity:  map[string]any{"name": "example"},
	}, nil
}

func ExampleExecutor() {
	var adapter executor.Executor = platformExecutor{}
	action := executor.Action{
		Address: "widget.example",
		Action:  "create",
		Mapping: executor.ActionMapping{
			SourceKind:  "openapi",
			OperationID: "createWidget",
		},
	}
	req := executor.Request{
		Action:       action,
		Capabilities: executor.RequirementsForAction(action),
		Idempotency:  executor.IdempotencyForAction(action),
	}
	result, err := adapter.Execute(context.Background(), req)
	fmt.Println(result.Success, result.Operation, err)
	// Output: true createWidget <nil>
}
