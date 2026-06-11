package corpus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/internal/trainingdata"
	"github.com/OpenUdon/ramen/validate"
)

const trainingRoot = "testdata/training"
const expectedRunnableGoldRows = 35

var trainingMarkdownLinkRe = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

type trainingCorpusManifest struct {
	Version string                `json:"version"`
	Entries []trainingCorpusEntry `json:"entries"`
}

type trainingCorpusEntry struct {
	Path string `json:"path"`
}

func TestTrainingManifestCoversParityGoldRows(t *testing.T) {
	manifest := loadTrainingManifest(t)
	byID := trainingEntriesByID(t, manifest)
	rows := parseTrainingParityRows(t)
	goldCount := 0
	for _, entry := range manifest.Entries {
		if entry.Tier == "gold" {
			goldCount++
		}
	}
	if goldCount != len(rows) {
		t.Fatalf("manifest gold entries = %d, want parsed runnable parity rows %d", goldCount, len(rows))
	}
	for row := range rows {
		id := "gold-" + strings.ToLower(row)
		entry, ok := byID[id]
		if !ok {
			t.Fatalf("training manifest missing gold row %s", row)
		}
		if entry.Tier != "gold" || !entry.ParityBacked {
			t.Fatalf("%s tier/parity = %s/%t", id, entry.Tier, entry.ParityBacked)
		}
		if entry.NaturalLanguage.GoalSource != "curated" {
			t.Fatalf("%s goal_source = %q, want curated", id, entry.NaturalLanguage.GoalSource)
		}
	}
	for _, excluded := range []string{"gold-z06", "gold-h04"} {
		if _, ok := byID[excluded]; ok {
			t.Fatalf("%s must not be included as a runnable gold workflow row", excluded)
		}
	}
}

func TestTrainingManifestCoversCorpusSilverRows(t *testing.T) {
	manifest := loadTrainingManifest(t)
	byID := trainingEntriesByID(t, manifest)
	corpusManifest := loadTrainingCorpusManifest(t)
	for _, corpusEntry := range corpusManifest.Entries {
		id := "silver-" + trainingSlug(corpusEntry.Path)
		entry, ok := byID[id]
		if !ok {
			t.Fatalf("training manifest missing silver corpus entry %s", corpusEntry.Path)
		}
		if entry.Tier != "silver" || entry.ParityBacked {
			t.Fatalf("%s tier/parity = %s/%t", id, entry.Tier, entry.ParityBacked)
		}
		if entry.NaturalLanguage.GoalSource != "generated" {
			t.Fatalf("%s goal_source = %q, want generated", id, entry.NaturalLanguage.GoalSource)
		}
	}
}

func TestTrainingManifestExcludesAnsibleConversionFixtures(t *testing.T) {
	manifest := loadTrainingManifest(t)
	for _, entry := range manifest.Entries {
		if strings.Contains(entry.ID, "ansible") {
			t.Fatalf("training manifest includes Ansible conversion entry %s; M45 keeps Ansible workflow conversion evidence out of T01", entry.ID)
		}
		for _, path := range append([]string{entry.PrimaryWorkflowPath, entry.HCLPath, entry.Provenance.SourcePath}, entry.WorkflowPaths...) {
			if strings.Contains(path, "testdata/ansible-conversion") {
				t.Fatalf("training manifest entry %s references Ansible conversion fixture %s", entry.ID, path)
			}
		}
	}
}

func TestTrainingManifestReferencedFilesExist(t *testing.T) {
	manifest := loadTrainingManifest(t)
	for _, entry := range manifest.Entries {
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			requireTrainingPath(t, entry.PrimaryWorkflowPath)
			if !containsString(entry.WorkflowPaths, entry.PrimaryWorkflowPath) {
				t.Fatalf("%s primary workflow is not listed in workflow_paths", entry.ID)
			}
			for _, path := range entry.WorkflowPaths {
				requireTrainingPath(t, path)
			}
			if entry.HCLPath != "" {
				requireTrainingPath(t, entry.HCLPath)
			}
			for _, source := range entry.APISources {
				requireTrainingPath(t, source.Path)
			}
			if entry.Provenance.SourceDoc != "" {
				requireTrainingPath(t, entry.Provenance.SourceDoc)
			}
		})
	}
}

func TestTrainingManifestEntriesStrictValidate(t *testing.T) {
	manifest := loadTrainingManifest(t)
	for _, entry := range manifest.Entries {
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			inputs := make([]validate.APISourceInput, 0, len(entry.APISources))
			for _, source := range entry.APISources {
				inputs = append(inputs, validate.APISourceInput{Kind: source.Kind, ID: source.ID, Path: source.Path})
			}
			result, err := validate.Run(context.Background(), validate.Options{
				ProjectPath: entry.PrimaryWorkflowPath,
				APISources:  inputs,
				Strict:      true,
			})
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			if !result.Valid {
				t.Fatalf("strict validation failed: %#v", result.Diagnostics)
			}
			if entry.Validation.Status != "valid" || entry.Validation.Summary != result.Summary {
				t.Fatalf("manifest validation summary = %#v, want %#v", entry.Validation, result.Summary)
			}
		})
	}
}

func loadTrainingManifest(t *testing.T) trainingdata.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(trainingRoot, "manifest.json"))
	if err != nil {
		t.Fatalf("read training manifest (run `go run ./cmd/traininggen`): %v", err)
	}
	var manifest trainingdata.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse training manifest: %v", err)
	}
	if manifest.Version != trainingdata.Version {
		t.Fatalf("training manifest version = %q", manifest.Version)
	}
	if len(manifest.Entries) == 0 {
		t.Fatal("training manifest has no entries")
	}
	return manifest
}

func trainingEntriesByID(t *testing.T, manifest trainingdata.Manifest) map[string]trainingdata.Entry {
	t.Helper()
	out := map[string]trainingdata.Entry{}
	for _, entry := range manifest.Entries {
		if out[entry.ID].ID != "" {
			t.Fatalf("duplicate training entry id %s", entry.ID)
		}
		out[entry.ID] = entry
	}
	return out
}

func loadTrainingCorpusManifest(t *testing.T) trainingCorpusManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(corpusRoot, "manifest.json"))
	if err != nil {
		t.Fatalf("read corpus manifest: %v", err)
	}
	var manifest trainingCorpusManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse corpus manifest: %v", err)
	}
	return manifest
}

func parseTrainingParityRows(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("docs", "parity_nl.md"))
	if err != nil {
		t.Fatalf("read parity natural-language doc: %v", err)
	}
	text := string(data)
	idx := strings.Index(text, "## Detailed Prompt Inventory")
	if idx < 0 {
		t.Fatalf("docs/parity_nl.md missing Detailed Prompt Inventory")
	}
	rows := map[string]bool{}
	for _, line := range strings.Split(text[idx:], "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|---") || strings.HasPrefix(line, "| Entry ") {
			continue
		}
		cols := strings.Split(strings.Trim(line, "|"), "|")
		if len(cols) != 7 {
			continue
		}
		row := strings.TrimSpace(cols[0])
		if row == "Z06" || row == "H04" {
			continue
		}
		if len(trainingMarkdownLinkRe.FindAllStringSubmatch(cols[2], -1)) == 0 {
			continue
		}
		rows[row] = true
	}
	if rows["Z06"] || rows["H04"] {
		t.Fatalf("non-workflow rows must be excluded from parsed training gold rows")
	}
	if len(rows) != expectedRunnableGoldRows {
		t.Fatalf("parsed runnable parity rows = %d, want %d", len(rows), expectedRunnableGoldRows)
	}
	return rows
}

func requireTrainingPath(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("referenced path %s does not exist: %v", path, err)
	}
}

func trainingSlug(path string) string {
	path = strings.ToLower(filepath.ToSlash(path))
	var b strings.Builder
	for _, r := range path {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
