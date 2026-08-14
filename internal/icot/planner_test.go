package icot

import (
	"context"
	"testing"

	"github.com/OpenUdon/apitools"
	sharedicot "github.com/OpenUdon/authoring/icot"
	"github.com/OpenUdon/authoring/promptcontext"
	"github.com/OpenUdon/authoring/readiness"
)

func TestPlannerCompletesMoreThanTwentyDecisionsWithoutBreadthCeiling(t *testing.T) {
	discovery, err := DiscoverLocalSources(context.Background(), DiscoveryOptions{
		Goal: "Create and manage a Kubernetes ConfigMap, read it, update it, and delete it",
		Sources: []apitools.LocalSource{{
			Kind: "openapi", ID: "core", Path: "../../testdata/parity/kubernetes/k03/openapi/core.json",
		}},
	})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	s := SeedSession("Create and manage a Kubernetes ConfigMap, read it, update it, and delete it", "config-map", t.TempDir(), "never", discovery.Plans, discovery.Blockers, discovery.Context)
	answered := 0
	for rounds := 0; rounds < 20 && !Ready(s, CheckReadiness(s)); rounds++ {
		frontier, err := PlanFrontier(s)
		if err != nil {
			t.Fatalf("frontier round %d: %v", rounds+1, err)
		}
		if len(frontier) == 0 {
			t.Fatalf("empty frontier before ready: %#v", CheckReadiness(s))
		}
		answers := make([]sharedicot.RoundAnswer, 0, len(frontier))
		for _, question := range frontier {
			value := question.Recommendation
			source := readiness.DefaultRecommendationSource
			if question.ID == nodeSafety {
				value = "approve"
				source = "user"
			}
			if question.ID == nodeProposal {
				value = "approve"
				source = "user"
			}
			if value == "" {
				t.Fatalf("question %s has no test recommendation", question.ID)
			}
			answers = append(answers, sharedicot.RoundAnswer{QuestionID: question.ID, Value: value, Source: source})
		}
		if err := ApplyRound(&s, answers); err != nil {
			t.Fatalf("apply round %d: %v", rounds+1, err)
		}
		answered += len(answers)
	}
	if !Ready(s, CheckReadiness(s)) {
		t.Fatalf("session not ready: %#v", CheckReadiness(s))
	}
	if answered <= 20 {
		t.Fatalf("answered %d decisions, want more than 20", answered)
	}
	proposal := BuildProposal(s)
	if !proposal.Complete || len(proposal.Resources) != 1 || len(proposal.Sources) != 1 || len(proposal.Steps) == 0 || proposal.FallbackBehavior == "" || len(proposal.Verification) == 0 {
		t.Fatalf("proposal = %#v", proposal)
	}
}

func TestPlannerReturnsWholeIndependentFrontierDeterministically(t *testing.T) {
	s := SeedSession("read widgets", "", "/tmp/out", "never", []SourcePlan{{
		ID: "widget", Kind: "openapi", Path: "/tmp/widget.json", SHA256: "aaaa", TargetPath: "sources/openapi/widget.json",
	}}, nil, promptContextWithOperation())
	frontier, err := PlanFrontier(s)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{nodeActorTrigger, nodeSuccess, nodeProjectName, nodeNonGoals, nodeSource}
	if len(frontier) != len(want) {
		t.Fatalf("frontier = %#v", frontier)
	}
	for i, id := range want {
		if frontier[i].ID != id {
			t.Fatalf("frontier[%d] = %q, want %q: %#v", i, frontier[i].ID, id, frontier)
		}
	}
}

func TestBroadGoalForcesActiveWorkflowSelection(t *testing.T) {
	s := SeedSession("Create widgets and send notifications", "", "/tmp/out", "never", nil, nil, promptContextWithOperation())
	frontier, err := PlanFrontier(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(frontier) != 1 || frontier[0].ID != nodeActiveWorkflow || !frontier[0].Forced {
		t.Fatalf("frontier = %#v", frontier)
	}
}

func promptContextWithOperation() promptcontext.Context {
	return promptcontext.Context{
		Version:    promptcontext.Version,
		Sources:    []promptcontext.SourceDocument{{ID: "widget", Kind: "openapi"}},
		Operations: []promptcontext.OperationCandidate{{ID: "listWidgets", OperationID: "listWidgets", SourceID: "widget", Verb: "GET", Path: "/widgets"}},
	}
}
