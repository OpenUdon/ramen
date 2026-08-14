package icot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/apitools"
	sharedicot "github.com/OpenUdon/authoring/icot"
	"github.com/OpenUdon/authoring/prompt"
	"github.com/OpenUdon/authoring/promptcontext"
	"github.com/OpenUdon/authoring/readiness"
	"github.com/OpenUdon/ramen/project"
)

func TestRunNormalAppliesFrontierDefaultsAndWritesOnlyAfterApproval(t *testing.T) {
	discovery, err := DiscoverLocalSources(context.Background(), DiscoveryOptions{
		Goal:    "Create a widget",
		Sources: []apitools.LocalSource{{Kind: "openapi", ID: "widget", Path: "../../examples/widget/api.openapi.yaml"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "project")
	s := SeedSession("Create a widget", "widget", outDir, "never", discovery.Plans, discovery.Blockers, discovery.Context)
	s.Intent.Verification = []string{"validate"}
	var output strings.Builder
	result, err := Run(context.Background(), RunOptions{
		Session: s, Input: strings.NewReader("putWidget\napprove\napprove\n"), Output: &output,
		DefaultMode: prompt.DefaultsShow, AutosavePath: filepath.Join(outDir, "session.json"),
		TranscriptPath: filepath.Join(outDir, "transcript.json"), Validate: true,
	})
	if err != nil {
		t.Fatalf("run: %v\n%s", err, output.String())
	}
	if result.Status != "complete" || !result.Completed {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(outDir, project.DefaultFile)); err != nil {
		t.Fatalf("project missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "sources", "openapi", "widget.yaml")); err != nil {
		t.Fatalf("source missing: %v", err)
	}
	if !strings.Contains(output.String(), "Complete Ramen proposal:") || !strings.Contains(output.String(), "Round ") {
		t.Fatalf("output did not show rounds and proposal:\n%s", output.String())
	}
	if len(result.Session.Interview.Evidence) == 0 {
		t.Fatal("result lost interview evidence")
	}
}

func TestRunAgentReturnsWholeFrontierWithoutWrites(t *testing.T) {
	discovery, err := DiscoverLocalSources(context.Background(), DiscoveryOptions{
		Goal:    "Create a widget",
		Sources: []apitools.LocalSource{{Kind: "openapi", ID: "widget", Path: "../../examples/widget/api.openapi.yaml"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "project")
	s := SeedSession("Create a widget", "widget", outDir, "never", discovery.Plans, discovery.Blockers, discovery.Context)
	result, err := Run(context.Background(), RunOptions{Session: s, Agent: true, DefaultMode: prompt.DefaultsSilent})
	if !errors.Is(err, sharedicot.ErrNeedsInput) {
		t.Fatalf("error = %v", err)
	}
	if result.Status != "needs_input" || len(result.Frontier) == 0 || len(result.SourceCandidates) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("agent mode wrote output: %v", err)
	}
}

func TestRunCancelLeavesOnlyResumableState(t *testing.T) {
	discovery, err := DiscoverLocalSources(context.Background(), DiscoveryOptions{
		Goal:    "Create a widget",
		Sources: []apitools.LocalSource{{Kind: "openapi", ID: "widget", Path: "../../examples/widget/api.openapi.yaml"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "project")
	s := SeedSession("Create a widget", "widget", outDir, "never", discovery.Plans, discovery.Blockers, discovery.Context)
	result, err := Run(context.Background(), RunOptions{
		Session: s, Input: strings.NewReader("cancel\n"), DefaultMode: prompt.DefaultsShow,
		AutosavePath: filepath.Join(outDir, "session.json"), NoTranscript: true,
	})
	if !errors.Is(err, sharedicot.ErrCanceled) || result.Status != "canceled" {
		t.Fatalf("result/error = %#v / %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(outDir, project.DefaultFile)); !os.IsNotExist(err) {
		t.Fatalf("cancel wrote deliverable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "session.json")); err != nil {
		t.Fatalf("cancel did not preserve resumable state: %v", err)
	}
}

func TestRunPrintOnlyLeavesNoFiles(t *testing.T) {
	discovery, err := DiscoverLocalSources(context.Background(), DiscoveryOptions{
		Goal:    "Create a widget",
		Sources: []apitools.LocalSource{{Kind: "openapi", ID: "widget", Path: "../../examples/widget/api.openapi.yaml"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "project")
	s := SeedSession("Create a widget", "widget", outDir, "never", discovery.Plans, discovery.Blockers, discovery.Context)
	result, err := Run(context.Background(), RunOptions{
		Session: s, Input: strings.NewReader("putWidget\napprove\napprove\n"), DefaultMode: prompt.DefaultsShow,
		PrintOnly: true, AutosavePath: filepath.Join(outDir, "session.json"), TranscriptPath: filepath.Join(outDir, "transcript.json"),
	})
	if err != nil || result.Status != "proposal" {
		t.Fatalf("print result/error = %#v / %v", result, err)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("print-only wrote output: %v", err)
	}
}

func TestRunFullShowsEveryDecisionAndFastHidesSafeDefaults(t *testing.T) {
	discovery, err := DiscoverLocalSources(context.Background(), DiscoveryOptions{
		Goal:    "Create a widget",
		Sources: []apitools.LocalSource{{Kind: "openapi", ID: "widget", Path: "../../examples/widget/api.openapi.yaml"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := SeedSession("Create a widget", "widget", filepath.Join(t.TempDir(), "project"), "never", discovery.Plans, discovery.Blockers, discovery.Context)
	fullInput := deterministicRunInput(t, base, true)
	var fullOutput strings.Builder
	full, err := Run(context.Background(), RunOptions{
		Session: base, Input: strings.NewReader(fullInput), Output: &fullOutput,
		DefaultMode: prompt.DefaultsAsk, PrintOnly: true, NoTranscript: true,
	})
	if err != nil || full.Status != "proposal" || !strings.Contains(fullOutput.String(), "Confirm who requests reconciliation") {
		t.Fatalf("full mode result/error/output = %#v / %v\n%s", full, err, fullOutput.String())
	}

	fastSession := base
	fastSession.OutDir = filepath.Join(t.TempDir(), "fast")
	var fastOutput strings.Builder
	fast, err := Run(context.Background(), RunOptions{
		Session: fastSession, Input: strings.NewReader("putWidget\napprove\napprove\n"), Output: &fastOutput,
		DefaultMode: prompt.DefaultsSilent, NoTranscript: true,
	})
	if err != nil || fast.Status != "complete" {
		t.Fatalf("fast mode result/error = %#v / %v\n%s", fast, err, fastOutput.String())
	}
	if strings.Contains(fastOutput.String(), "Confirm who requests reconciliation") {
		t.Fatalf("fast mode displayed a safe default:\n%s", fastOutput.String())
	}
}

func deterministicRunInput(t *testing.T, session Session, answerEveryQuestion bool) string {
	t.Helper()
	var lines []string
	for !Ready(session, CheckReadiness(session)) {
		frontier, err := PlanFrontier(session)
		if err != nil {
			t.Fatal(err)
		}
		answers := make([]sharedicot.RoundAnswer, 0, len(frontier))
		for _, question := range frontier {
			value := question.Recommendation
			source := readiness.DefaultRecommendationSource
			switch question.ID {
			case nodeOperation:
				value, source = "putWidget", "user"
			case nodeSafety:
				value, source = "approve", "user"
			case nodeProposal:
				value, source = "approve", "user"
			}
			if value == "" {
				t.Fatalf("missing deterministic answer for %s", question.ID)
			}
			if answerEveryQuestion || question.Forced || question.Recommendation == "" {
				lines = append(lines, value)
			}
			answers = append(answers, sharedicot.RoundAnswer{QuestionID: question.ID, Value: value, Source: source})
		}
		if err := ApplyRound(&session, answers); err != nil {
			t.Fatal(err)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestRunDraftResumeAndAtomicPromotion(t *testing.T) {
	discovery, err := DiscoverLocalSources(context.Background(), DiscoveryOptions{
		Goal:    "Create a widget",
		Sources: []apitools.LocalSource{{Kind: "openapi", ID: "widget", Path: "../../examples/widget/api.openapi.yaml"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "project")
	sessionPath := filepath.Join(outDir, "session.json")
	s := SeedSession("Create a widget", "widget", outDir, "never", discovery.Plans, discovery.Blockers, discovery.Context)
	deferred := false
	for rounds := 0; rounds < 20 && !Ready(s, CheckReadiness(s)); rounds++ {
		frontier, err := PlanFrontier(s)
		if err != nil {
			t.Fatal(err)
		}
		answers := make([]sharedicot.RoundAnswer, 0, len(frontier))
		for _, question := range frontier {
			value := question.Recommendation
			source := readiness.DefaultRecommendationSource
			if question.ID == nodeSafety {
				value, source = "approve", "user"
			}
			if strings.HasPrefix(question.ID, "mapping.") && !deferred {
				value, source = "defer:api-team|mapping is not yet confirmed|schema owner confirms field semantics|resume the interview", "user"
				deferred = true
			}
			if question.ID == nodeProposal {
				value, source = "save-draft", "user"
			}
			if value == "" {
				t.Fatalf("missing answer for %s", question.ID)
			}
			answers = append(answers, sharedicot.RoundAnswer{QuestionID: question.ID, Value: value, Source: source})
		}
		if err := ApplyRound(&s, answers); err != nil {
			t.Fatal(err)
		}
	}
	if !deferred || s.Approval != "save-draft" {
		t.Fatalf("draft session = %#v", s)
	}
	draft, err := Run(context.Background(), RunOptions{Session: s, DefaultMode: prompt.DefaultsShow, AutosavePath: sessionPath, NoTranscript: true})
	if err != nil || draft.Status != "draft" {
		t.Fatalf("draft result/error = %#v / %v", draft, err)
	}
	for _, path := range []string{project.DraftFile, project.DraftHCL, "sources/openapi/widget.yaml", "session.json"} {
		if _, err := os.Stat(filepath.Join(outDir, filepath.FromSlash(path))); err != nil {
			t.Fatalf("draft artifact %s missing: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, project.DefaultFile)); !os.IsNotExist(err) {
		t.Fatalf("draft wrote runnable project: %v", err)
	}

	resumed, err := LoadSession(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	PrepareResume(&resumed)
	for rounds := 0; rounds < 20 && !Ready(resumed, CheckReadiness(resumed)); rounds++ {
		frontier, err := PlanFrontier(resumed)
		if err != nil {
			t.Fatal(err)
		}
		answers := make([]sharedicot.RoundAnswer, 0, len(frontier))
		for _, question := range frontier {
			value := question.Recommendation
			source := readiness.DefaultRecommendationSource
			if question.ID == nodeProposal {
				value = "approve"
				source = "user"
			}
			if value == "" {
				t.Fatalf("missing resume answer for %s", question.ID)
			}
			answers = append(answers, sharedicot.RoundAnswer{QuestionID: question.ID, Value: value, Source: source})
		}
		if err := ApplyRound(&resumed, answers); err != nil {
			t.Fatal(err)
		}
	}
	complete, err := Run(context.Background(), RunOptions{Session: resumed, DefaultMode: prompt.DefaultsShow, AutosavePath: sessionPath, NoTranscript: true})
	if err != nil || complete.Status != "complete" {
		t.Fatalf("promotion result/error = %#v / %v", complete, err)
	}
	if _, err := os.Stat(filepath.Join(outDir, project.DefaultFile)); err != nil {
		t.Fatalf("promoted project missing: %v", err)
	}
	for _, obsolete := range []string{project.DraftFile, project.DraftHCL, "session.json"} {
		if _, err := os.Stat(filepath.Join(outDir, obsolete)); !os.IsNotExist(err) {
			t.Fatalf("obsolete %s survived promotion: %v", obsolete, err)
		}
	}
}

func TestRunSourceDeferralWritesExplicitlyNonExecutableDraft(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "project")
	sessionPath := filepath.Join(outDir, "session.json")
	s := SeedSession("Create a widget", "widget", outDir, "never", nil, nil, promptcontext.Context{Version: promptcontext.Version})
	for rounds := 0; rounds < 10 && !Ready(s, CheckReadiness(s)); rounds++ {
		frontier, err := PlanFrontier(s)
		if err != nil {
			t.Fatal(err)
		}
		answers := make([]sharedicot.RoundAnswer, 0, len(frontier))
		for _, question := range frontier {
			value := question.Recommendation
			source := readiness.DefaultRecommendationSource
			switch question.ID {
			case nodeSourceInput:
				value, source = "defer:api-team|source is unavailable|provider publishes the schema|resume with --api-source", "user"
			case nodeSafety:
				value, source = "approve", "user"
			case nodeProposal:
				value, source = "save-draft", "user"
			}
			if value == "" {
				t.Fatalf("missing answer for %s", question.ID)
			}
			answers = append(answers, sharedicot.RoundAnswer{QuestionID: question.ID, Value: value, Source: source})
		}
		if err := ApplyRound(&s, answers); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Run(context.Background(), RunOptions{Session: s, DefaultMode: prompt.DefaultsShow, AutosavePath: sessionPath, NoTranscript: true})
	if err != nil || result.Status != "draft" {
		t.Fatalf("draft result/error = %#v / %v", result, err)
	}
	data, err := os.ReadFile(filepath.Join(outDir, project.DraftFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "operations: null") || strings.Contains(string(data), "workflows:") {
		t.Fatalf("source-deferred draft became executable:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(outDir, project.DefaultFile)); !os.IsNotExist(err) {
		t.Fatalf("source deferral wrote runnable project: %v", err)
	}
}

func TestRunOperationDeferralWritesDraftWithConfirmedSource(t *testing.T) {
	discovery, err := DiscoverLocalSources(context.Background(), DiscoveryOptions{
		Goal:    "Create a widget",
		Sources: []apitools.LocalSource{{Kind: "openapi", ID: "widget", Path: "../../examples/widget/api.openapi.yaml"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "project")
	s := SeedSession("Create a widget", "widget", outDir, "never", discovery.Plans, discovery.Blockers, discovery.Context)
	deferred := false
	for rounds := 0; rounds < 10 && !Ready(s, CheckReadiness(s)); rounds++ {
		frontier, err := PlanFrontier(s)
		if err != nil {
			t.Fatal(err)
		}
		answers := make([]sharedicot.RoundAnswer, 0, len(frontier))
		for _, question := range frontier {
			value := question.Recommendation
			source := readiness.DefaultRecommendationSource
			switch question.ID {
			case nodeOperation:
				value, source = "defer:api-team|operation is not yet approved|API owner selects an operation|resume the interview", "user"
				deferred = true
			case nodeSafety:
				value, source = "approve", "user"
			case nodeProposal:
				value, source = "save-draft", "user"
			}
			if value == "" {
				t.Fatalf("missing answer for %s", question.ID)
			}
			answers = append(answers, sharedicot.RoundAnswer{QuestionID: question.ID, Value: value, Source: source})
		}
		if err := ApplyRound(&s, answers); err != nil {
			t.Fatal(err)
		}
	}
	if !deferred {
		t.Fatal("operation decision was not offered")
	}
	result, err := Run(context.Background(), RunOptions{Session: s, DefaultMode: prompt.DefaultsShow, AutosavePath: filepath.Join(outDir, "session.json"), NoTranscript: true})
	if err != nil || result.Status != "draft" {
		t.Fatalf("draft result/error = %#v / %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "sources", "openapi", "widget.yaml")); err != nil {
		t.Fatalf("confirmed source was not materialized with draft: %v", err)
	}
}
