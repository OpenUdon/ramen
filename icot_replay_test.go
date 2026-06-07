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
	divergentMatches := 0
	for _, entry := range entries {
		t.Run(entry.Row, func(t *testing.T) {
			result, _ := draftICoTReplayProject(t, entry)
			generated := operationRoleSet(t, result.ProjectPath)
			approved := operationRoleSet(t, entry.ApprovedFixture)
			switch entry.RoleMatch {
			case "exact":
				if !reflect.DeepEqual(generated, approved) {
					t.Fatalf("generated role set = %#v, want approved %#v", generated, approved)
				}
				exactMatches++
			case "divergent":
				assertPinnedAzureSQLDivergence(t, entry.Row, generated, approved)
				divergentMatches++
			default:
				t.Fatalf("unknown role_match %q", entry.RoleMatch)
			}
		})
	}
	if exactMatches != 22 || divergentMatches != 2 {
		t.Fatalf("role match counts exact=%d divergent=%d, want exact=22 divergent=2", exactMatches, divergentMatches)
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
	if len(rows) != 24 {
		t.Fatalf("inventory rows = %d, want 24", len(rows))
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

func assertPinnedAzureSQLDivergence(t *testing.T, row string, generated, approved map[string]string) {
	t.Helper()
	if row != "Z01" && row != "Z05" {
		t.Fatalf("only Z01/Z05 may be divergent, got %s", row)
	}
	if reflect.DeepEqual(generated, approved) {
		t.Fatalf("%s unexpectedly became exact; update the inventory and docs intentionally", row)
	}
	if generated["create"] != "Databases_CreateOrUpdate" || approved["put"] != "Databases_CreateOrUpdate" {
		t.Fatalf("%s create/put divergence = generated %#v approved %#v", row, generated, approved)
	}
	if _, ok := generated["put"]; ok {
		t.Fatalf("%s generated role set now includes put: %#v", row, generated)
	}
	if _, ok := approved["create"]; ok {
		t.Fatalf("%s approved role set unexpectedly includes create: %#v", row, approved)
	}
	for _, role := range []string{"read", "delete"} {
		if generated[role] != approved[role] {
			t.Fatalf("%s role %s operation = %q, want approved %q", row, role, generated[role], approved[role])
		}
	}
	if len(generated) != len(approved) {
		t.Fatalf("%s generated role count = %d, want approved count %d", row, len(generated), len(approved))
	}
}

func firstNonEmptyForTest(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
