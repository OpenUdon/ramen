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

func TestProjectTraversesNumericArrayResponsePaths(t *testing.T) {
	mapping := &tfplan.MappingPlan{
		ResponseBindings: []project.ResponseBinding{{
			ResponsePath: "items.1.name",
			StatePath:    "selected_name",
			Computed:     true,
		}},
	}
	result := executor.Result{Computed: map[string]any{
		"items": []any{
			map[string]any{"name": "first"},
			map[string]any{"name": "second"},
		},
	}}

	_, computed := Project(mapping, result)
	if computed["selected_name"] != "second" {
		t.Fatalf("computed = %#v", computed)
	}
}

func TestProjectDoesNotMutateExecutorResultAndPreservesSiblingBindings(t *testing.T) {
	mapping := &tfplan.MappingPlan{
		ResponseBindings: []project.ResponseBinding{
			{ResponsePath: "response.first", StatePath: "projected.first", Computed: true},
			{ResponsePath: "response.second", StatePath: "projected.second", Computed: true},
			{ResponsePath: "response.payload", StatePath: "projected.payload", Computed: true},
		},
	}
	wantResult := executor.Result{Computed: map[string]any{"response": map[string]any{
		"first":  "one",
		"second": "two",
		"payload": map[string]any{
			"items": []any{map[string]any{"name": "original"}},
		},
	}}}
	result := executor.Result{Computed: cloneAnyMap(wantResult.Computed)}

	_, computed := Project(mapping, result)
	if !reflect.DeepEqual(result, wantResult) {
		t.Fatalf("executor result mutated:\n got: %#v\nwant: %#v", result, wantResult)
	}
	if got, ok := dottedAny(computed, "projected.first"); !ok || got != "one" {
		t.Fatalf("first sibling missing: %#v", computed)
	}
	if got, ok := dottedAny(computed, "projected.second"); !ok || got != "two" {
		t.Fatalf("second sibling missing: %#v", computed)
	}
	payload, ok := dottedAny(computed, "projected.payload.items.0")
	if !ok {
		t.Fatalf("projected payload missing: %#v", computed)
	}
	payload.(map[string]any)["name"] = "changed"
	if got, _ := dottedAny(result.Computed, "response.payload.items.0.name"); got != "original" {
		t.Fatalf("projected value aliases executor result: %#v", result.Computed)
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

func TestClassifyReadResultDoesNotReclassifySuccessfulDiagnosticText(t *testing.T) {
	result := executor.Result{
		Success:  true,
		Messages: []string{"the cached prior read returned 404 not found"},
		Events: []executor.Event{{
			Message:  "NoSuchEntity was observed during an earlier attempt",
			Metadata: map[string]any{"status_code": 404},
		}},
	}
	got, err := ClassifyReadResult(result, nil)
	if err != nil || !reflect.DeepEqual(got, result) {
		t.Fatalf("successful result was reclassified: result=%#v err=%v", got, err)
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
			{ResponsePath: "response.placeholder", StatePath: "placeholder", Computed: true},
		},
		Normalizers: []project.Normalizer{
			{Path: "resource.id", Kind: "case_fold"},
			{Path: "tags", Kind: "unordered_collection"},
			{Path: "policy", Kind: "json_semantic"},
			{Path: "empty", Kind: "empty_null_absent_equivalent"},
			{Path: "placeholder", Kind: "sensitive_placeholder"},
		},
	}
	result := executor.Result{
		Identity: map[string]any{"response": map[string]any{"id": "WIDGET-1"}},
		Computed: map[string]any{"response": map[string]any{
			"secret":      "do-not-persist",
			"tags":        []any{"z", "a"},
			"policy":      `{ "b": 2, "a": 1 }`,
			"empty":       " ",
			"placeholder": "replace-me",
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
	if computed["placeholder"] != "${redacted}" {
		t.Fatalf("computed placeholder = %#v", computed["placeholder"])
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
