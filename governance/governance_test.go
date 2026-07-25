package governance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStaticEngineRequiresApprovalAndRoles(t *testing.T) {
	engine := StaticEngine{Policies: []Policy{{Name: "guard", RequireApprovalActions: []string{"delete"}, RequiredApproverRoles: []string{"admin"}}}}
	result := engine.Evaluate(Input{Resources: []Resource{{Address: "example.one", Action: "delete"}}})
	if result.Version != ResultVersion || len(result.ApprovalRequirements) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if err := RequirementsSatisfied(result.ApprovalRequirements, []Approver{{Identity: "alice", Role: "viewer", ApprovedAt: time.Unix(1, 0)}}); err == nil {
		t.Fatalf("viewer unexpectedly satisfied admin requirement")
	}
	if err := RequirementsSatisfied(result.ApprovalRequirements, []Approver{{Identity: "alice", Role: "admin", ApprovedAt: time.Unix(1, 0)}}); err != nil {
		t.Fatalf("admin did not satisfy requirement: %v", err)
	}
}

func TestLoadPolicyFilesValidatesAndEvaluatesPolicies(t *testing.T) {
	root := t.TempDir()
	validPath := filepath.Join(root, "guard.yaml")
	if err := os.WriteFile(validPath, []byte(`
version: ramen.policy.v1
name: guard
deny_actions: [delete]
warn_actions: [update]
max_deletes: 1
`), 0o644); err != nil {
		t.Fatal(err)
	}
	wrongVersionPath := filepath.Join(root, "future.json")
	if err := os.WriteFile(wrongVersionPath, []byte(`{"version":"ramen.policy.v2"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	invalidPath := filepath.Join(root, "invalid.yaml")
	if err := os.WriteFile(invalidPath, []byte("deny_actions: ["), 0o644); err != nil {
		t.Fatal(err)
	}

	engine, decisions := LoadPolicyFiles([]string{
		"",
		validPath,
		wrongVersionPath,
		invalidPath,
		filepath.Join(root, "missing.json"),
	})
	if len(engine.Policies) != 1 || len(engine.Refs) != 1 || engine.Refs[0].Digest == "" {
		t.Fatalf("engine = %#v", engine)
	}
	for _, code := range []string{"policy.version_invalid", "policy.file_parse_error", "policy.file_load_error"} {
		if !hasDecisionCode(decisions, code) {
			t.Fatalf("decisions missing %s: %#v", code, decisions)
		}
	}

	result := engine.Evaluate(Input{Resources: []Resource{
		{Address: "example.delete-one", Action: "delete"},
		{Address: "example.delete-two", Action: "delete"},
		{Address: "example.update", Action: "update"},
	}})
	for _, code := range []string{"policy.deny", "policy.warn", "policy.max_deletes"} {
		if !hasDecisionCode(result.Decisions, code) {
			t.Fatalf("evaluation missing %s: %#v", code, result.Decisions)
		}
	}
}

func TestMergeResultsNormalizesOrdering(t *testing.T) {
	got := MergeResults(
		Result{
			Policies: []PolicyRef{{Name: "z"}},
			Decisions: []Decision{{
				Policy: "z", Severity: "warning", Code: "z",
			}},
		},
		Result{
			Policies: []PolicyRef{{Name: "a"}},
			Decisions: []Decision{{
				Policy: "a", Severity: "error", Code: "a",
			}},
		},
	)
	if got.Version != ResultVersion || got.Policies[0].Name != "a" || got.Decisions[0].Code != "a" {
		t.Fatalf("merged = %#v", got)
	}
}

func TestNormalizeApproverRequiresDeterministicTimestamp(t *testing.T) {
	if _, err := NormalizeApprover(Approver{Identity: "alice"}); err == nil || !strings.Contains(err.Error(), "approved_at") {
		t.Fatalf("missing approved_at error = %v", err)
	}
	approved, err := NormalizeApprover(Approver{Identity: " alice ", Role: " admin ", ApprovedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatalf("normalize approver: %v", err)
	}
	if approved.Identity != "alice" || approved.Role != "admin" || approved.ApprovedAt.Location() != time.UTC {
		t.Fatalf("approved = %#v", approved)
	}
}

func hasDecisionCode(decisions []Decision, code string) bool {
	for _, decision := range decisions {
		if decision.Code == code {
			return true
		}
	}
	return false
}
