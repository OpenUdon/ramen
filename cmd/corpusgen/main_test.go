package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareCorpusTreesEqual(t *testing.T) {
	got := t.TempDir()
	want := t.TempDir()
	writeTestFile(t, got, "manifest.json", "{\n  \"version\": \"ramen.corpus.v1\",\n  \"entries\": []\n}\n")
	writeTestFile(t, want, "manifest.json", "{\n  \"version\": \"ramen.corpus.v1\",\n  \"entries\": []\n}\n")

	if err := compareCorpusTrees(got, want); err != nil {
		t.Fatalf("equal corpus trees should pass: %v", err)
	}
}

func TestCompareCorpusTreesNormalizesProjectConfigDir(t *testing.T) {
	got := t.TempDir()
	want := t.TempDir()
	writeTestFile(t, got, "entry/project.uws.yaml", projectYAML("/tmp/regenerated/entry/input"))
	writeTestFile(t, want, "entry/project.uws.yaml", projectYAML("testdata/corpus/entry/input"))
	writeTestFile(t, got, "entry/project.uws.hcl", projectHCL("/tmp/regenerated/entry/input"))
	writeTestFile(t, want, "entry/project.uws.hcl", projectHCL("testdata/corpus/entry/input"))

	if err := compareCorpusTrees(got, want); err != nil {
		t.Fatalf("config_dir-only project document differences should pass: %v", err)
	}
}

func TestCompareCorpusTreesChangedGoldenBytesFail(t *testing.T) {
	got := t.TempDir()
	want := t.TempDir()
	writeTestFile(t, got, "entry/plan.json", "{\"ok\":true}\n")
	writeTestFile(t, want, "entry/plan.json", "{\"ok\":false}\n")

	if err := compareCorpusTrees(got, want); err == nil {
		t.Fatal("changed normal file bytes should fail")
	}
}

func TestCompareCorpusTreesMissingAndExtraFilesFail(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		got := t.TempDir()
		want := t.TempDir()
		writeTestFile(t, want, "entry/input/main.tf", "resource \"aws_s3_bucket\" \"test\" {}\n")

		if err := compareCorpusTrees(got, want); err == nil {
			t.Fatal("missing regenerated entry file should fail")
		}
	})
	t.Run("extra", func(t *testing.T) {
		got := t.TempDir()
		want := t.TempDir()
		writeTestFile(t, got, "entry/input/main.tf", "resource \"aws_s3_bucket\" \"test\" {}\n")

		if err := compareCorpusTrees(got, want); err == nil {
			t.Fatal("extra regenerated entry file should fail")
		}
	})
}

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func projectYAML(configDir string) string {
	return `uws: 1.4.0
info:
    title: test
    version: 1.0.0
x-ramen-desired-state:
    metadata:
        action: create
        config_dir: ` + configDir + `
        source: ramen convert tf
    version: ramen.project.v1
`
}

func projectHCL(configDir string) string {
	return `uws = "1.4.0"
info {
  title = "test"
  version = "1.0.0"
}
extensions {
  x-ramen-desired-state {
    metadata {
      action = "create"
      config_dir = "` + configDir + `"
      source = "ramen convert tf"
    }
    version = "ramen.project.v1"
  }
}
`
}
