// Package corpus holds regression tests for Terraform-to-UWS conversion corpora.
// Re-running conversion on each generated clean-corpus entry must reproduce the
// committed YAML/plan artifacts byte-for-byte, generate HCL that matches the
// regenerated YAML structurally, and stay diagnostic-clean. The diagnostic
// corpus covers intentionally non-clean fixtures with expected diagnostics.
// Regenerate the clean corpus with `go run ./cmd/corpusgen`.
package corpus

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/OpenUdon/ramen/internal/tfconvert"
	"github.com/OpenUdon/uws/convert"
)

const corpusRoot = "testdata/corpus"
const diagnosticCorpusRoot = "testdata/diagnostic-corpus"
const manualCorpusRoot = "testdata/manual-corpus"
const allowMissingCorpusEnv = "RAMEN_CORPUS_ALLOW_MISSING"

type modelRef struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type apiSourceRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Path string `json:"path"`
}

type entryMeta struct {
	Path          string         `json:"path"`
	Provider      string         `json:"provider,omitempty"`
	Service       string         `json:"service"`
	ResourceTypes []string       `json:"resource_types"`
	DataSources   []string       `json:"data_sources,omitempty"`
	APISources    []apiSourceRef `json:"api_sources,omitempty"`
	SmithyModels  []modelRef     `json:"smithy_models,omitempty"`
	SourceDir     string         `json:"source_dir"`
}

type manifest struct {
	Version string      `json:"version"`
	Entries []entryMeta `json:"entries"`
}

type diagnosticEntryMeta struct {
	Path                string     `json:"path"`
	SmithyModels        []modelRef `json:"smithy_models"`
	ExpectedDiagnostics []string   `json:"expected_diagnostics"`
}

type diagnosticManifest struct {
	Version string                `json:"version"`
	Entries []diagnosticEntryMeta `json:"entries"`
}

func loadManifest(t *testing.T) manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(corpusRoot, "manifest.json"))
	if err != nil {
		t.Fatalf("read corpus manifest (run `go run ./cmd/corpusgen`): %v", err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse corpus manifest: %v", err)
	}
	return m
}

func loadDiagnosticManifest(t *testing.T) diagnosticManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(diagnosticCorpusRoot, "manifest.json"))
	if err != nil {
		t.Fatalf("read diagnostic corpus manifest: %v", err)
	}
	var m diagnosticManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse diagnostic corpus manifest: %v", err)
	}
	return m
}

func loadManualManifest(t *testing.T) manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(manualCorpusRoot, "manifest.json"))
	if err != nil {
		t.Fatalf("read manual corpus manifest: %v", err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manual corpus manifest: %v", err)
	}
	return m
}

func TestCorpusManifestNonEmpty(t *testing.T) {
	m := loadManifest(t)
	if len(m.Entries) == 0 {
		t.Fatal("corpus manifest has no entries; run `go run ./cmd/corpusgen`")
	}
}

func TestDiagnosticCorpusManifestNonEmpty(t *testing.T) {
	m := loadDiagnosticManifest(t)
	if len(m.Entries) == 0 {
		t.Fatal("diagnostic corpus manifest has no entries")
	}
}

func TestCorpusConversionsReproduceGoldens(t *testing.T) {
	m := loadManifest(t)
	executed := 0
	for _, entry := range m.Entries {
		entry := entry
		t.Run(entry.Path, func(t *testing.T) {
			entryDir := filepath.Join(corpusRoot, filepath.FromSlash(entry.Path))
			sources := apiSourcesForEntry(t, entry)
			executed++

			tmp := t.TempDir()
			res, err := tfconvert.Convert(context.Background(), tfconvert.Options{
				ConfigDir:  filepath.Join(entryDir, "input"),
				APISources: sources,
				Action:     "create",
				OutDir:     tmp,
			})
			if err != nil {
				t.Fatalf("convert: %v", err)
			}
			for _, d := range res.Diagnostics {
				if d.Severity == "error" {
					t.Fatalf("conversion produced error diagnostic %s: %s", d.Code, d.Message)
				}
			}

			// YAML and plan are deterministic, so byte-compare them.
			assertSameFile(t, res.NativeProjectPath, filepath.Join(entryDir, "project.uws.yaml"))
			assertSameFile(t, res.PlanJSONPath, filepath.Join(entryDir, "plan.json"))
			// The HCL marshaller is not key-order-stable, so verify the freshly
			// generated .hcl describes the same document as the generated .yaml.
			assertHCLMatchesYAML(t, res.NativeProjectHCLPath, res.NativeProjectPath)
		})
	}
	if allowMissingCorpusFixtures() {
		if executed == 0 {
			t.Fatal("corpus conversion test executed zero entries; ensure referenced API source fixtures are available")
		}
		return
	}
	if executed != len(m.Entries) {
		t.Fatalf("corpus conversion test executed %d of %d entries", executed, len(m.Entries))
	}
}

func TestManualCleanCorpusReproducesKeyArtifacts(t *testing.T) {
	m := loadManualManifest(t)
	if len(m.Entries) == 0 {
		t.Fatal("manual corpus manifest has no entries")
	}
	executed := 0
	for _, entry := range m.Entries {
		entry := entry
		t.Run(entry.Path, func(t *testing.T) {
			entryDir := filepath.Join(manualCorpusRoot, filepath.FromSlash(entry.Path))
			res, err := tfconvert.Convert(context.Background(), tfconvert.Options{
				ConfigDir:  filepath.Join(entryDir, "input"),
				APISources: apiSourcesForEntry(t, entry),
				Action:     "create",
				OutDir:     t.TempDir(),
			})
			if err != nil {
				t.Fatalf("convert: %v", err)
			}
			for _, d := range res.Diagnostics {
				if d.Severity == "error" {
					t.Fatalf("conversion produced error diagnostic %s: %s", d.Code, d.Message)
				}
			}
			executed++

			assertSameFile(t, res.NativeProjectPath, filepath.Join(entryDir, "project.uws.yaml"))
			assertSameFile(t, res.PlanJSONPath, filepath.Join(entryDir, "expected", "plan.json"))
			assertHCLMatchesYAML(t, res.NativeProjectHCLPath, res.NativeProjectPath)
		})
	}
	assertCorpusExecuted(t, "manual clean corpus", executed, len(m.Entries))
}

func TestCorpusMissingFixturePolicy(t *testing.T) {
	t.Setenv(allowMissingCorpusEnv, "")
	if allowMissingCorpusFixtures() {
		t.Fatalf("%s unset should fail missing corpus fixtures", allowMissingCorpusEnv)
	}
	t.Setenv(allowMissingCorpusEnv, "0")
	if allowMissingCorpusFixtures() {
		t.Fatalf("%s=0 should fail missing corpus fixtures", allowMissingCorpusEnv)
	}
	t.Setenv(allowMissingCorpusEnv, "1")
	if !allowMissingCorpusFixtures() {
		t.Fatalf("%s=1 should skip missing corpus fixtures", allowMissingCorpusEnv)
	}
}

func allowMissingCorpusFixtures() bool {
	return os.Getenv(allowMissingCorpusEnv) == "1"
}

func requireCorpusFixture(t *testing.T, path, kind string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		if allowMissingCorpusFixtures() {
			t.Skipf("%s unavailable (%s): %v", kind, path, err)
		}
		t.Fatalf("%s unavailable (%s); set %s=1 to skip missing fixtures in partial checkouts: %v", kind, path, allowMissingCorpusEnv, err)
	}
}

func assertCorpusExecuted(t *testing.T, name string, executed, total int) {
	t.Helper()
	if msg := corpusExecutionError(name, executed, total); msg != "" {
		t.Fatal(msg)
	}
}

func corpusExecutionError(name string, executed, total int) string {
	if allowMissingCorpusFixtures() {
		if executed == 0 {
			return name + " conversion test executed zero entries; ensure referenced API source fixtures are available"
		}
		return ""
	}
	if executed != total {
		return name + " conversion test did not execute every manifest entry"
	}
	return ""
}

func TestCorpusExecutionPolicyRequiresAllEntries(t *testing.T) {
	t.Setenv(allowMissingCorpusEnv, "")
	if msg := corpusExecutionError("test", 2, 2); msg != "" {
		t.Fatalf("complete corpus should pass: %s", msg)
	}
	if msg := corpusExecutionError("test", 1, 2); msg == "" {
		t.Fatal("partial corpus should fail by default")
	}
	t.Setenv(allowMissingCorpusEnv, "1")
	if msg := corpusExecutionError("test", 1, 2); msg != "" {
		t.Fatalf("allow-missing partial corpus should pass: %s", msg)
	}
	if msg := corpusExecutionError("test", 0, 2); msg == "" {
		t.Fatal("allow-missing corpus should still require at least one executed entry")
	}
}

func apiSourcesForEntry(t *testing.T, entry entryMeta) []tfconvert.APISourceInput {
	t.Helper()
	if len(entry.APISources) > 0 {
		sources := make([]tfconvert.APISourceInput, 0, len(entry.APISources))
		for _, src := range entry.APISources {
			requireCorpusFixture(t, src.Path, "API source")
			sources = append(sources, tfconvert.APISourceInput{
				Kind: src.Kind,
				ID:   src.ID,
				Path: src.Path,
			})
		}
		return sources
	}

	sources := make([]tfconvert.APISourceInput, 0, len(entry.SmithyModels))
	for _, model := range entry.SmithyModels {
		requireCorpusFixture(t, model.Path, "smithy model")
		sources = append(sources, tfconvert.APISourceInput{
			Kind: "aws-smithy",
			ID:   model.ID,
			Path: model.Path,
		})
	}
	return sources
}

func TestDiagnosticCorpusProducesExpectedDiagnostics(t *testing.T) {
	m := loadDiagnosticManifest(t)
	executed := 0
	for _, entry := range m.Entries {
		entry := entry
		t.Run(entry.Path, func(t *testing.T) {
			entryDir := filepath.Join(diagnosticCorpusRoot, filepath.FromSlash(entry.Path))
			sources := make([]tfconvert.APISourceInput, 0, len(entry.SmithyModels))
			for _, model := range entry.SmithyModels {
				requireCorpusFixture(t, model.Path, "smithy model")
				sources = append(sources, tfconvert.APISourceInput{
					Kind: "aws-smithy",
					ID:   model.ID,
					Path: model.Path,
				})
			}
			executed++

			res, err := tfconvert.Convert(context.Background(), tfconvert.Options{
				ConfigDir:  filepath.Join(entryDir, "input"),
				APISources: sources,
				Action:     "create",
				OutDir:     t.TempDir(),
			})
			if err != nil {
				t.Fatalf("convert: %v", err)
			}
			for _, code := range entry.ExpectedDiagnostics {
				if !hasDiagnostic(res.Diagnostics, code) {
					t.Fatalf("diagnostics missing %s: %#v", code, res.Diagnostics)
				}
			}
		})
	}
	assertCorpusExecuted(t, "diagnostic corpus", executed, len(m.Entries))
}

func assertHCLMatchesYAML(t *testing.T, hclPath, yamlPath string) {
	t.Helper()
	hclData, err := os.ReadFile(hclPath)
	if err != nil {
		t.Fatalf("read project.uws.hcl: %v", err)
	}
	yamlData, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read project.uws.yaml: %v", err)
	}
	fromHCL, err := convert.HCLToJSON(hclData)
	if err != nil {
		t.Fatalf("parse project.uws.hcl: %v", err)
	}
	fromYAML, err := convert.YAMLToJSON(yamlData)
	if err != nil {
		t.Fatalf("parse project.uws.yaml: %v", err)
	}
	// The two serializations order JSON keys differently, so compare the decoded
	// documents structurally rather than byte-for-byte.
	var docHCL, docYAML any
	if err := json.Unmarshal(fromHCL, &docHCL); err != nil {
		t.Fatalf("decode HCL-derived JSON: %v", err)
	}
	if err := json.Unmarshal(fromYAML, &docYAML); err != nil {
		t.Fatalf("decode YAML-derived JSON: %v", err)
	}
	if !reflect.DeepEqual(docHCL, docYAML) {
		t.Errorf("%s and %s describe different documents", hclPath, yamlPath)
	}
}

func hasDiagnostic(diags []tfconvert.Diagnostic, code string) bool {
	for _, diag := range diags {
		if diag.Code == code {
			return true
		}
	}
	return false
}

func assertSameFile(t *testing.T, gotPath, wantPath string) {
	t.Helper()
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("read regenerated %s: %v", gotPath, err)
	}
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", wantPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("regenerated output differs from golden %s; re-run `go run ./cmd/corpusgen`", wantPath)
	}
}
