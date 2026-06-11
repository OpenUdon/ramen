package corpus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/OpenUdon/authoring/promptcontext"
	ramenauthoring "github.com/OpenUdon/ramen/authoring"
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
	for _, entry := range entries {
		t.Run(entry.Row, func(t *testing.T) {
			result, _ := draftICoTReplayProject(t, entry)
			if result.Validation == nil {
				t.Fatalf("validation result is nil")
			}
			if !result.Validation.Valid {
				t.Fatalf("generated project did not validate: %#v", result.Validation)
			}
			executed++
		})
	}
	if executed != len(entries) {
		t.Fatalf("executed %d replay rows, want %d", executed, len(entries))
	}
}

func TestICoTReplayRoleSetsMatchApproved(t *testing.T) {
	entries := loadICoTReplayInventory(t)
	exactMatches := 0
	for _, entry := range entries {
		t.Run(entry.Row, func(t *testing.T) {
			result, _ := draftICoTReplayProject(t, entry)
			generated := operationRoleSet(t, result.ProjectPath)
			approved := operationRoleSet(t, entry.ApprovedFixture)
			if entry.RoleMatch != "exact" {
				t.Fatalf("unknown role_match %q", entry.RoleMatch)
			}
			if !reflect.DeepEqual(generated, approved) {
				t.Fatalf("generated role set = %#v, want approved %#v", generated, approved)
			}
			exactMatches++
		})
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

func loadICoTReplayInventory(t *testing.T) []icotReplayEntry {
	t.Helper()
	data, err := os.ReadFile(icotReplayInventoryPath)
	if err != nil {
		t.Fatalf("read iCoT replay inventory: %v", err)
	}
	var inventory icotReplayInventory
	if err := json.Unmarshal(data, &inventory); err != nil {
		t.Fatalf("parse iCoT replay inventory: %v", err)
	}
	if inventory.Version != "ramen.icot-replay.v1" {
		t.Fatalf("inventory version = %q", inventory.Version)
	}
	if len(inventory.Entries) == 0 {
		t.Fatalf("inventory has no entries")
	}
	return inventory.Entries
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
	ctx, err := ramenauthoring.PromptContextFromAPISources(context.Background(), entry.Goal, inputs)
	if err != nil {
		t.Fatalf("build prompt context: %v", err)
	}
	seed := findICoTReplaySeed(t, ctx, entry.SeedOperation)
	resource := ramenauthoring.APILifecycleResource(ctx, seed, entry.Goal, entry.ProjectName)
	result, err := ramenauthoring.DraftProject(context.Background(), ramenauthoring.Options{
		Goal:        entry.Goal,
		ProjectName: entry.ProjectName,
		Context:     ctx,
		Resources:   []project.Resource{resource},
		OutDir:      t.TempDir(),
		Validate:    true,
	})
	if err != nil {
		t.Fatalf("draft project: %v", err)
	}
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
