package stateprojection

import (
	"errors"
	"reflect"
	"testing"

	"github.com/OpenUdon/ramen/executor"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
)

func TestProjectTraversesJSONStringResponsePaths(t *testing.T) {
	mapping := &tfplan.MappingPlan{
		ResponseBindings: []project.ResponseBinding{{
			OperationRole: "read",
			ResponsePath:  "Policy.Statement.0.Sid",
			StatePath:     "policy_statement_sid",
			Computed:      true,
		}},
	}
	result := executor.Result{
		Success: true,
		Computed: map[string]any{
			"Policy": `{"Version":"2012-10-17","Statement":[{"Sid":"AllowInvoke","Effect":"Allow"}]}`,
		},
	}

	_, computed := Project(mapping, result)
	if computed["policy_statement_sid"] != "AllowInvoke" {
		t.Fatalf("computed = %#v", computed)
	}
}

func TestClassifyReadResultRecognizesMissingEvidence(t *testing.T) {
	tests := []struct {
		name   string
		result executor.Result
		err    error
	}{
		{name: "explicit", result: executor.Result{Missing: true}},
		{name: "error", err: errors.New("remote object was not found")},
		{name: "message", result: executor.Result{Messages: []string{"NoSuchEntity"}}},
		{name: "event metadata", result: executor.Result{Events: []executor.Event{{Metadata: map[string]any{"status_code": 404}}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ClassifyReadResult(tc.result, tc.err)
			if err != nil || !got.Success || !got.Missing {
				t.Fatalf("result=%#v err=%v", got, err)
			}
			if tc.err != nil && !reflect.DeepEqual(got.Messages, []string{tc.err.Error()}) {
				t.Fatalf("messages = %#v", got.Messages)
			}
		})
	}
}

func TestClassifyReadResultPreservesUnrelatedFailure(t *testing.T) {
	wantErr := errors.New("permission denied")
	result := executor.Result{Success: false, Messages: []string{"request failed"}}
	got, err := ClassifyReadResult(result, wantErr)
	if !reflect.DeepEqual(got, result) || !errors.Is(err, wantErr) {
		t.Fatalf("result=%#v err=%v", got, err)
	}
}

func TestProjectRehomesRedactsAndNormalizesBindings(t *testing.T) {
	mapping := &tfplan.MappingPlan{
		ResponseBindings: []project.ResponseBinding{
			{ResponsePath: "response.id", StatePath: "resource.id", Identity: true},
			{ResponsePath: "response.secret", StatePath: "resource.secret", Computed: true, Sensitive: true},
			{ResponsePath: "response.tags", StatePath: "tags", Computed: true},
			{ResponsePath: "response.policy", StatePath: "policy", Computed: true},
			{ResponsePath: "response.empty", StatePath: "empty", Computed: true},
		},
		Normalizers: []project.Normalizer{
			{Path: "resource.id", Kind: "case_fold"},
			{Path: "tags", Kind: "unordered_collection"},
			{Path: "policy", Kind: "json_semantic"},
			{Path: "empty", Kind: "empty_null_absent_equivalent"},
		},
	}
	result := executor.Result{
		Identity: map[string]any{"response": map[string]any{"id": "WIDGET-1"}},
		Computed: map[string]any{"response": map[string]any{
			"secret": "do-not-persist",
			"tags":   []any{"z", "a"},
			"policy": `{ "b": 2, "a": 1 }`,
			"empty":  " ",
		}},
	}

	identity, computed := Project(mapping, result)
	if got, ok := dottedAny(identity, "resource.id"); !ok || got != "widget-1" {
		t.Fatalf("identity = %#v", identity)
	}
	if _, ok := identity["response"]; ok {
		t.Fatalf("identity retained emptied response wrapper: %#v", identity)
	}
	if got, ok := dottedAny(computed, "resource.secret"); !ok || got != "${redacted}" {
		t.Fatalf("computed sensitive value = %#v", computed)
	}
	if !reflect.DeepEqual(computed["tags"], []any{"a", "z"}) {
		t.Fatalf("computed tags = %#v", computed["tags"])
	}
	if computed["policy"] != `{"a":1,"b":2}` {
		t.Fatalf("computed policy = %#v", computed["policy"])
	}
	if _, ok := computed["empty"]; ok {
		t.Fatalf("computed retained empty value: %#v", computed)
	}
	if _, ok := computed["response"]; ok {
		t.Fatalf("computed retained emptied response wrapper: %#v", computed)
	}
}

func TestProjectToleratesMissingAndInvalidResponsePaths(t *testing.T) {
	mapping := &tfplan.MappingPlan{
		ResponseBindings: []project.ResponseBinding{
			{ResponsePath: "missing.value", StatePath: "ignored", Computed: true},
			{ResponsePath: "array.not-an-index", StatePath: "ignored", Computed: true},
			{ResponsePath: "invalid.child", StatePath: "ignored", Computed: true},
		},
	}
	result := executor.Result{Computed: map[string]any{
		"array":   []any{"first"},
		"invalid": "not-json",
	}}
	_, computed := Project(mapping, result)
	if !reflect.DeepEqual(computed, result.Computed) {
		t.Fatalf("computed=%#v want=%#v", computed, result.Computed)
	}
}
