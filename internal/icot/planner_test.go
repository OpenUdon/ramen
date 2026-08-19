package icot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/OpenUdon/apitools"
	sharedicot "github.com/OpenUdon/authoring/icot"
	"github.com/OpenUdon/authoring/promptcontext"
	"github.com/OpenUdon/authoring/readiness"
	"github.com/OpenUdon/ramen/project"
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

func TestSecurityAlternativeSelectionPreservesOrAndSemanticsAcrossResume(t *testing.T) {
	for _, test := range []struct {
		name   string
		answer string
		want   []string
	}{
		{name: "anonymous", answer: "anonymous"},
		{name: "and-set", answer: "api_key + client_certificate", want: []string{"api_key", "client_certificate"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := promptcontext.Context{
				Version: promptcontext.Version,
				Sources: []promptcontext.SourceDocument{{ID: "widget", Kind: "openapi"}},
				Operations: []promptcontext.OperationCandidate{{
					ID: "listWidgets", OperationID: "listWidgets", SourceID: "widget", Verb: "GET", Path: "/widgets",
					CredentialBindingSets: []promptcontext.CredentialBindingSet{
						{},
						{Bindings: []string{"api_key", "client_certificate"}},
					},
				}},
			}
			s := SeedSession("read widgets", "widgets", "/tmp/out", "never", []SourcePlan{{
				ID: "widget", Kind: "openapi", Path: "/tmp/widget.json", SHA256: strings.Repeat("a", 64), TargetPath: "sources/openapi/widget.json",
			}}, nil, ctx)
			s.Boundary.ActorTrigger = "operator requests reconciliation"
			s.Boundary.SuccessEvidence = []string{"read response validates"}
			s.Boundary.NonGoals = []string{"execute during authoring"}
			s.Intent.SelectedSourceIDs = []string{"widget"}
			s.Intent.SelectedOperationID = "listWidgets"
			Normalize(&s)

			frontier, err := PlanFrontier(s)
			if err != nil {
				t.Fatal(err)
			}
			securityNode := ""
			for _, question := range frontier {
				if strings.HasPrefix(question.ID, "mapping.security.") {
					securityNode = question.ID
					break
				}
			}
			if securityNode == "" {
				t.Fatalf("security choice missing from frontier: %#v", frontier)
			}
			for _, question := range frontier {
				if strings.HasPrefix(question.ID, "mapping.") && !strings.HasPrefix(question.ID, "mapping.security.") {
					t.Fatalf("mapping question %q was ready before the security choice: %#v", question.ID, frontier)
				}
			}
			if err := ApplyRound(&s, []sharedicot.RoundAnswer{{QuestionID: securityNode, Value: test.answer, Source: "user"}}); err != nil {
				t.Fatalf("apply security choice: %v", err)
			}

			encoded, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			var resumed Session
			if err := json.Unmarshal(encoded, &resumed); err != nil {
				t.Fatal(err)
			}
			Normalize(&resumed)
			if len(resumed.Intent.Resources) != 1 {
				t.Fatalf("resources = %#v", resumed.Intent.Resources)
			}
			for roleName, role := range resumed.Intent.Resources[0].Operations {
				if len(role.CredentialBindingAlternatives) != 0 {
					t.Fatalf("role %s retained unresolved alternatives: %#v", roleName, role.CredentialBindingAlternatives)
				}
				if strings.Join(role.CredentialBindings, ",") != strings.Join(test.want, ",") {
					t.Fatalf("role %s bindings = %#v, want %#v", roleName, role.CredentialBindings, test.want)
				}
			}
		})
	}
}

func TestSecurityAlternativeSelectionRejectsAmbiguousLabels(t *testing.T) {
	resource := project.Resource{
		Address: "api.widget", Operations: map[string]project.OperationRole{
			"read": {CredentialBindingAlternatives: [][]string{{"same"}, {"same"}}},
		},
	}
	s := Session{Intent: ActiveIntent{Resources: []project.Resource{resource}}}
	nodeID := securityAlternativeNodeID(resource.Address, "read")
	if err := applySecurityAlternativeAnswer(&s, nodeID, "same"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous label error = %v", err)
	}
	if err := applySecurityAlternativeAnswer(&s, nodeID, "2"); err != nil {
		t.Fatalf("numbered selection failed: %v", err)
	}
	if got := s.Metadata[securityAlternativeMetadataKey(resource.Address, "read")]; got != "2" {
		t.Fatalf("stored security selection = %q, want 2", got)
	}
}

func promptContextWithOperation() promptcontext.Context {
	return promptcontext.Context{
		Version:    promptcontext.Version,
		Sources:    []promptcontext.SourceDocument{{ID: "widget", Kind: "openapi"}},
		Operations: []promptcontext.OperationCandidate{{ID: "listWidgets", OperationID: "listWidgets", SourceID: "widget", Verb: "GET", Path: "/widgets"}},
	}
}
