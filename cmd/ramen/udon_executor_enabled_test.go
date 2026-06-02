//go:build udon

package main

import (
	"testing"

	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/uws/uws1"
)

func TestIdentityProjectionFallsBackToDeepID(t *testing.T) {
	body := map[string]any{
		"value": []any{map[string]any{
			"id": "/subscriptions/example/resourceGroups/rg/providers/example/type/name",
		}},
	}
	identity, err := identityProjection(zeroReadExecutorRequest(), body, computedProjection(body))
	if err != nil {
		t.Fatalf("identity projection failed: %v", err)
	}
	if identity["id"] != "/subscriptions/example/resourceGroups/rg/providers/example/type/name" {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestProjectUdonExecutorOutputAllowsAsyncMutationWithoutIdentity(t *testing.T) {
	req := executor.Request{
		Action: executor.Action{
			Address: "resource.sql_database_ramen",
			Action:  "create",
			Mapping: executor.ActionMapping{
				OperationID: "Databases_CreateOrUpdate",
			},
		},
	}
	result, err := projectUdonExecutorOutputForBody(req, map[string]any{
		"operation": "UpsertDatabase",
		"startTime": "2026-05-31T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("project output failed: %v", err)
	}
	if len(result.Identity) != 0 {
		t.Fatalf("identity = %#v", result.Identity)
	}
	if result.Computed["operation"] != "UpsertDatabase" {
		t.Fatalf("computed = %#v", result.Computed)
	}
}

func TestIdentityProjectionFallsBackToIdentityAttributeNameInRequest(t *testing.T) {
	req := executor.Request{
		Action: executor.Action{
			Address: "resource.sql_database_ramen",
			Action:  "put",
			Metadata: map[string]string{
				"identity_attributes": `[{"name":"databaseName","terraform_path":"databaseName","required":true}]`,
			},
		},
		Document: testUdonDocumentRequest(map[string]any{
			"path": map[string]any{
				"databaseName": "ramen-m27",
			},
		}),
	}

	identity, err := identityProjection(req, map[string]any{"operation": "UpsertDatabase"}, map[string]any{"operation": "UpsertDatabase"})
	if err != nil {
		t.Fatalf("identity projection failed: %v", err)
	}
	if identity["databaseName"] != "ramen-m27" {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestComputedProjectionParsesJSONResponseString(t *testing.T) {
	computed := computedProjection(`{"value":[{"id":"/subscriptions/example/resourceGroups/rg/providers/example/type/name","name":"name"}]}`)
	if len(computed) == 0 {
		t.Fatal("computed projection is empty")
	}
	value, ok := computed["value"].([]any)
	if !ok || len(value) != 1 {
		t.Fatalf("computed value = %#v", computed["value"])
	}
	first, ok := value[0].(map[string]any)
	if !ok || first["id"] == "" {
		t.Fatalf("computed first value = %#v", value[0])
	}
}

func TestLookupPathDeepFindsValuesInTypedSlices(t *testing.T) {
	body := map[string]any{
		"value": []map[string]any{{
			"id": "/subscriptions/example/resourceGroups/rg/providers/example/type/name",
		}},
	}
	value, ok := lookupPathDeep(body, []string{"id"})
	if !ok {
		t.Fatalf("id not found in %#v", body)
	}
	if value != "/subscriptions/example/resourceGroups/rg/providers/example/type/name" {
		t.Fatalf("id = %#v", value)
	}
}

func zeroReadExecutorRequest() executor.Request {
	return executor.Request{Action: executor.Action{Action: "read"}}
}

func testUdonDocumentRequest(request map[string]any) *uws1.Document {
	return &uws1.Document{
		Operations: []*uws1.Operation{{
			OperationID: "resource_sql_database_ramen_put",
			Request:     request,
		}},
	}
}

func projectUdonExecutorOutputForBody(req executor.Request, body any) (executor.Result, error) {
	computed := computedProjection(body)
	identity, err := identityProjection(req, body, computed)
	if err != nil {
		return executor.Result{}, err
	}
	if len(identity) == 0 && isAsyncAcceptedMutation(req, computed) {
		return executor.Result{
			Address:   req.Action.Address,
			Operation: req.Action.Mapping.OperationID,
			Computed:  computed,
		}, nil
	}
	return executor.Result{Identity: identity, Computed: computed}, nil
}
