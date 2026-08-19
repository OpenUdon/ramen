package icot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/authoring/interview"
	"github.com/OpenUdon/authoring/promptcontext"
)

func TestSessionRoundTripPreservesSafetyEvidence(t *testing.T) {
	state := Session{
		Version: SessionVersion,
		Interview: interview.State{
			Version: interview.Version,
			Nodes:   []interview.Node{{ID: nodeSafety, Status: interview.StatusSettled}},
			Answers: []interview.Answer{{ID: "answer-000001", NodeID: nodeSafety, Value: "approve", Source: "user", EvidenceRefs: []string{"evidence-000001"}}},
			Evidence: []interview.Evidence{{
				ID: "evidence-000001", Kind: interview.EvidenceUserDecision, NodeID: nodeSafety,
				Summary: "approve", Attributes: map[string]string{"requires_confirmation": "true", "classification": "side-effect-posture", "confidence": "confirmed"},
			}},
		},
		Boundary:      Boundary{MutationScope: "approved-for-authoring"},
		NetworkPolicy: "never",
		OutDir:        t.TempDir(),
	}
	path := filepath.Join(t.TempDir(), "session.json")
	if err := SaveSession(path, state); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := LoadSession(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := loaded.Interview.Evidence[0].Attributes["requires_confirmation"]; got != "true" {
		t.Fatalf("requires_confirmation = %q", got)
	}
	if !mutationConfirmed(loaded) {
		t.Fatal("linked mutation confirmation was not preserved")
	}
}

func TestValidateSessionRejectsOrphanMutationEvidence(t *testing.T) {
	state := Session{
		Version: SessionVersion, NetworkPolicy: "never", Boundary: Boundary{MutationScope: "approved-for-authoring"},
		Interview: interview.State{Version: interview.Version, Nodes: []interview.Node{{ID: nodeSafety, Status: interview.StatusSettled}}, Evidence: []interview.Evidence{{
			ID: "evidence-000001", Kind: interview.EvidenceUserDecision, NodeID: nodeSafety, Summary: "approve",
			Attributes: map[string]string{"requires_confirmation": "true", "classification": "side-effect-posture", "confidence": "confirmed"},
		}}},
	}
	if err := ValidateSession(state); err == nil || !strings.Contains(err.Error(), "lacks linked durable") {
		t.Fatalf("orphan evidence error = %v", err)
	}
}

func TestValidateSessionRejectsV1PromptContext(t *testing.T) {
	s := SeedSession("List widgets", "widgets", t.TempDir(), "never", nil, nil, promptcontext.Context{Version: "authoring.prompt-context.v1"})
	if err := ValidateSession(s); err == nil || !strings.Contains(err.Error(), "prompt-context version") || !strings.Contains(err.Error(), "v1 inputs are not compatible") {
		t.Fatalf("ValidateSession error = %v", err)
	}
}

func TestValidateSessionRejectsReviewApprovalBypass(t *testing.T) {
	state := Session{
		Version: SessionVersion, NetworkPolicy: "never", Approval: "approve",
		Interview: interview.State{Version: interview.Version, Nodes: []interview.Node{{ID: nodeProposal, Status: interview.StatusSettled}}, Evidence: []interview.Evidence{{
			ID: "evidence-000001", Kind: interview.EvidenceUserDecision, NodeID: nodeProposal, Summary: "approve",
			Attributes: map[string]string{"requires_confirmation": "true", "classification": "proposal-approval", "confidence": "confirmed"},
		}}},
	}
	if err := ValidateSession(state); err == nil || !strings.Contains(err.Error(), "approval lacks linked durable") {
		t.Fatalf("review bypass error = %v", err)
	}
}

func TestLoadSessionRejectsV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	if err := os.WriteFile(path, []byte(`{"version":"ramen.icot-session.v1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSession(path)
	if err == nil || !strings.Contains(err.Error(), "v1 inputs are not compatible") {
		t.Fatalf("error = %v", err)
	}
}

func TestSaveSessionRejectsSymlinkedStateDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".icot")); err != nil {
		t.Fatal(err)
	}
	state := Session{Version: SessionVersion, Interview: interview.State{Version: interview.Version}, NetworkPolicy: "never", OutDir: root}
	err := SaveSession(filepath.Join(root, ".icot", "session.json"), state)
	if err == nil || !strings.Contains(err.Error(), "non-symlink directory") {
		t.Fatalf("symlinked state error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "session.json")); !os.IsNotExist(err) {
		t.Fatalf("session escaped through symlink: %v", err)
	}
}

func TestValidateSessionRejectsDifferingSourceTargetCollision(t *testing.T) {
	state := Session{
		Version:       SessionVersion,
		Interview:     interview.State{Version: interview.Version},
		NetworkPolicy: "never",
		SourcePlans: []SourcePlan{
			{ID: "one", Kind: "openapi", Path: "/one.json", SHA256: strings.Repeat("a", 64), TargetPath: "sources/openapi/api.json"},
			{ID: "two", Kind: "openapi", Path: "/two.json", SHA256: strings.Repeat("b", 64), TargetPath: "sources/openapi/api.json"},
		},
	}
	if err := ValidateSession(state); err == nil || !strings.Contains(err.Error(), "source target collision") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateSessionRejectsDuplicateSourceIDsAndEscapingTargets(t *testing.T) {
	base := Session{Version: SessionVersion, Interview: interview.State{Version: interview.Version}, NetworkPolicy: "never"}
	base.SourcePlans = []SourcePlan{
		{ID: "api", Kind: "openapi", Path: "/one.json", SHA256: strings.Repeat("a", 64), TargetPath: "sources/openapi/one.json"},
		{ID: "api", Kind: "openapi", Path: "/two.json", SHA256: strings.Repeat("b", 64), TargetPath: "sources/openapi/two.json"},
	}
	if err := ValidateSession(base); err == nil || !strings.Contains(err.Error(), "duplicate source id") {
		t.Fatalf("duplicate source error = %v", err)
	}
	base.SourcePlans = []SourcePlan{{ID: "api", Kind: "openapi", Path: "/one.json", SHA256: strings.Repeat("a", 64), TargetPath: "../escape.json"}}
	if err := ValidateSession(base); err == nil || !strings.Contains(err.Error(), "must stay inside") {
		t.Fatalf("escaping source target error = %v", err)
	}
	base.SourcePlans = []SourcePlan{{ID: "api", Kind: "openapi", Path: "/one.json", SHA256: strings.Repeat("a", 64), TargetPath: ".icot/session.json"}}
	if err := ValidateSession(base); err == nil || !strings.Contains(err.Error(), "under the sources directory") {
		t.Fatalf("reserved source target error = %v", err)
	}
}

func TestSessionRejectsInlineSecretLikeOutcome(t *testing.T) {
	s := SeedSession("Create widget with api_key=super-secret-value", "widget", t.TempDir(), "never", nil, nil, promptcontext.Context{Version: promptcontext.Version})
	if err := ValidateSession(s); err == nil || !strings.Contains(err.Error(), "inline secret-like") {
		t.Fatalf("inline secret error = %v", err)
	}
	if len(s.Blockers) == 0 || s.Blockers[0].Deferrable {
		t.Fatalf("inline secret blocker = %#v", s.Blockers)
	}
}

func TestSessionRejectsUnsafePlaceholderButAllowsVariableBinding(t *testing.T) {
	s := SeedSession("Create <widget-name>", "widget", t.TempDir(), "never", nil, nil, promptcontext.Context{Version: promptcontext.Version})
	if err := ValidateSession(s); err == nil || !strings.Contains(err.Error(), "unresolved placeholder") {
		t.Fatalf("placeholder error = %v", err)
	}

	s = SeedSession("Create ${var.widget_name}", "widget", t.TempDir(), "never", nil, nil, promptcontext.Context{Version: promptcontext.Version})
	if err := ValidateSession(s); err != nil {
		t.Fatalf("variable binding rejected: %v", err)
	}
}
