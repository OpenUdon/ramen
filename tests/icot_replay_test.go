package corpus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/OpenUdon/apitools"
	sharedicot "github.com/OpenUdon/authoring/icot"
	"github.com/OpenUdon/authoring/prompt"
	"github.com/OpenUdon/authoring/promptcontext"
	"github.com/OpenUdon/authoring/readiness"
	ramenauthoring "github.com/OpenUdon/ramen/authoring"
	ramenicot "github.com/OpenUdon/ramen/internal/icot"
	"github.com/OpenUdon/ramen/project"
)

const icotReplayInventoryPath = "testdata/parity/icot-replay.json"

type icotReplayInventory struct {
	Version string            `json:"version"`
	Entries []icotReplayEntry `json:"entries"`
}

type icotReplayEntry struct {
	Row             string                `json:"row"`
	ProjectName     string                `json:"project_name"`
	Goal            string                `json:"goal"`
	APISources      []icotReplayAPISource `json:"api_sources"`
	SeedOperation   string                `json:"seed_operation"`
	ApprovedFixture string                `json:"approved_fixture"`
	RoleMatch       string                `json:"role_match"`
}

type icotReplayAPISource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Path string `json:"path"`
}

func TestICoTReplayProjectsValidate(t *testing.T) {
	entries := loadICoTReplayInventory(t)
	executed := 0
	exactMatches := 0
	for _, entry := range entries {
		t.Run(entry.Row, func(t *testing.T) {
			result, _ := draftICoTReplayProject(t, entry)
			if result.Validation == nil {
				t.Fatalf("validation result is nil")
			}
			if !result.Validation.Valid {
				t.Fatalf("generated project did not validate: %#v", result.Validation)
			}
			generated := operationRoleSet(t, result.ProjectPath)
			approved := operationRoleSet(t, entry.ApprovedFixture)
			if entry.RoleMatch != "exact" || !reflect.DeepEqual(generated, approved) {
				t.Fatalf("generated role set = %#v, want exact approved %#v", generated, approved)
			}
			exactMatches++
			executed++
		})
	}
	if executed != len(entries) {
		t.Fatalf("executed %d replay rows, want %d", executed, len(entries))
	}
	if exactMatches != 32 {
		t.Fatalf("role match count exact=%d, want exact=32", exactMatches)
	}
}

func TestICoTReplayInventoryCoversParityFixtures(t *testing.T) {
	entries := loadICoTReplayInventory(t)
	rows := map[string]bool{}
	for _, entry := range entries {
		if rows[entry.Row] {
			t.Fatalf("duplicate replay row %s", entry.Row)
		}
		rows[entry.Row] = true
	}
	if len(rows) != 32 {
		t.Fatalf("inventory rows = %d, want 32", len(rows))
	}
	for _, provider := range []string{"kubernetes", "azure", "google", "cloudflare"} {
		matches, err := filepath.Glob(filepath.Join("testdata", "parity", provider, "*", "ramen", "project.uws.yaml"))
		if err != nil {
			t.Fatalf("glob %s parity fixtures: %v", provider, err)
		}
		for _, fixture := range matches {
			row := strings.ToUpper(filepath.Base(filepath.Dir(filepath.Dir(fixture))))
			if !rows[row] {
				t.Fatalf("parity fixture %s has no iCoT replay row %s", fixture, row)
			}
		}
	}
	if rows["Z06"] {
		t.Fatalf("Z06 is observations-only and must not be a runnable replay row")
	}
}

func TestICoTReplayInventoryMatchesParityDoc(t *testing.T) {
	entries := loadICoTReplayInventory(t)
	doc, err := os.ReadFile(filepath.Join("docs", "parity_nl.md"))
	if err != nil {
		t.Fatalf("read parity natural-language doc: %v", err)
	}
	text := string(doc)
	for _, entry := range entries {
		if !strings.Contains(text, "| "+entry.Row+" |") {
			t.Fatalf("docs/parity_nl.md missing row %s", entry.Row)
		}
		if !strings.Contains(text, entry.ProjectName) {
			t.Fatalf("docs/parity_nl.md missing project name %s for row %s", entry.ProjectName, entry.Row)
		}
		if !strings.Contains(text, "../"+entry.ApprovedFixture) {
			t.Fatalf("docs/parity_nl.md missing approved fixture %s for row %s", entry.ApprovedFixture, entry.Row)
		}
		for _, source := range entry.APISources {
			if !strings.Contains(text, source.Kind+":"+source.ID+"="+source.Path) {
				t.Fatalf("docs/parity_nl.md missing API source %s:%s=%s for row %s", source.Kind, source.ID, source.Path, entry.Row)
			}
		}
	}
	if !strings.Contains(text, "| Z06 |") {
		t.Fatalf("docs/parity_nl.md must keep observations-only Z06 visible")
	}
}

func TestICoTReplayRejectsV1WithoutCompatibilityDecode(t *testing.T) {
	var inventory icotReplayInventory
	err := decodeICoTReplayInventory([]byte(`{"version":"ramen.icot-replay.v1","entries":[]}`), &inventory)
	if err == nil || !strings.Contains(err.Error(), "v1 inputs are not compatible") {
		t.Fatalf("v1 replay error = %v", err)
	}
}

func loadICoTReplayInventory(t *testing.T) []icotReplayEntry {
	t.Helper()
	data, err := os.ReadFile(icotReplayInventoryPath)
	if err != nil {
		t.Fatalf("read iCoT replay inventory: %v", err)
	}
	var inventory icotReplayInventory
	if err := decodeICoTReplayInventory(data, &inventory); err != nil {
		t.Fatalf("parse iCoT replay inventory: %v", err)
	}
	if len(inventory.Entries) == 0 {
		t.Fatalf("inventory has no entries")
	}
	return inventory.Entries
}

func decodeICoTReplayInventory(data []byte, inventory *icotReplayInventory) error {
	if err := json.Unmarshal(data, inventory); err != nil {
		return err
	}
	if inventory.Version != "ramen.icot-replay.v2" {
		return fmt.Errorf("unsupported Ramen iCoT replay version %q; want %q; v1 inputs are not compatible", inventory.Version, "ramen.icot-replay.v2")
	}
	return nil
}

func draftICoTReplayProject(t *testing.T, entry icotReplayEntry) (ramenauthoring.Result, promptcontext.Context) {
	t.Helper()
	inputs := make([]ramenauthoring.APISourceInput, 0, len(entry.APISources))
	for _, source := range entry.APISources {
		inputs = append(inputs, ramenauthoring.APISourceInput{
			Kind: source.Kind,
			ID:   source.ID,
			Path: source.Path,
		})
	}
	localSources := make([]apitools.LocalSource, 0, len(inputs))
	for _, input := range inputs {
		localSources = append(localSources, apitools.LocalSource{Kind: input.Kind, ID: input.ID, Path: input.Path})
	}
	discovered, err := ramenicot.DiscoverLocalSources(context.Background(), ramenicot.DiscoveryOptions{Goal: entry.Goal, Sources: localSources})
	if err != nil {
		t.Fatalf("discover local sources: %v", err)
	}
	ctx := discovered.Context
	seed := findICoTReplaySeed(t, ctx, entry.SeedOperation)
	_ = seed
	outDir := t.TempDir()
	session := ramenicot.SeedSession(entry.Goal, entry.ProjectName, outDir, "never", discovered.Plans, discovered.Blockers, discovered.Context)
	for !ramenicot.Ready(session, ramenicot.CheckReadiness(session)) {
		frontier, err := ramenicot.PlanFrontier(session)
		if err != nil {
			t.Fatalf("plan frontier: %v", err)
		}
		if len(frontier) == 0 {
			t.Fatal("v2 replay reached an empty unresolved frontier")
		}
		answers := make([]sharedicot.RoundAnswer, 0, len(frontier))
		for _, question := range frontier {
			value := question.Recommendation
			source := readiness.DefaultRecommendationSource
			switch question.ID {
			case "operation.seed":
				value, source = entry.SeedOperation, "replay"
			case "safety.mutation_posture":
				value, source = "approve", "replay"
			case "output.verification":
				value, source = "validate", "replay"
			case "proposal.approval":
				value, source = "approve", "replay"
			}
			if value == "" {
				t.Fatalf("v2 replay question %s has no deterministic answer", question.ID)
			}
			answers = append(answers, sharedicot.RoundAnswer{QuestionID: question.ID, Value: value, Source: source})
		}
		if err := ramenicot.ApplyRound(&session, answers); err != nil {
			t.Fatalf("apply replay round: %v", err)
		}
	}
	run, err := ramenicot.Run(context.Background(), ramenicot.RunOptions{
		Session: session, DefaultMode: prompt.DefaultsSilent, NoTranscript: true, Validate: true,
	})
	if err != nil {
		t.Fatalf("run v2 replay: %v", err)
	}
	result := ramenauthoring.Result{ProjectPath: run.Artifact.ProjectPath, ProjectHCLPath: run.Artifact.ProjectHCLPath, Validation: run.Artifact.Validation}
	if result.ProjectPath == "" {
		t.Fatalf("draft result missing project path: %#v", result)
	}
	return result, ctx
}

func findICoTReplaySeed(t *testing.T, ctx promptcontext.Context, operationID string) promptcontext.OperationCandidate {
	t.Helper()
	var ids []string
	for _, op := range ctx.Operations {
		id := strings.TrimSpace(firstNonEmptyForTest(op.OperationID, op.ID))
		ids = append(ids, id)
		if id == operationID {
			return op
		}
	}
	slices.Sort(ids)
	t.Fatalf("seed operation %q not found in prompt context operations %#v", operationID, ids)
	return promptcontext.OperationCandidate{}
}

func operationRoleSet(t *testing.T, path string) map[string]string {
	t.Helper()
	doc, err := project.Load(path)
	if err != nil {
		t.Fatalf("load project %s: %v", path, err)
	}
	if len(doc.Profile.Resources) != 1 {
		t.Fatalf("%s resources = %d, want 1", path, len(doc.Profile.Resources))
	}
	out := map[string]string{}
	for purpose, role := range doc.Profile.Resources[0].Operations {
		out[purpose] = role.OperationID
	}
	return out
}

func firstNonEmptyForTest(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
