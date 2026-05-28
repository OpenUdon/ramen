package governance

import (
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
