package icot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenUdon/apitools"
)

func TestDiscoverLocalSourcesBuildsDigestBoundPlan(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "widget", "api.openapi.yaml")
	result, err := DiscoverLocalSources(context.Background(), DiscoveryOptions{
		Goal:    "manage widgets",
		Sources: []apitools.LocalSource{{Kind: "openapi", ID: "widget", Path: path}},
	})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Blockers) != 0 {
		t.Fatalf("blockers = %#v", result.Blockers)
	}
	if len(result.Plans) != 1 {
		t.Fatalf("plans = %#v", result.Plans)
	}
	plan := result.Plans[0]
	if plan.ID != "widget" || plan.SHA256 == "" || plan.TargetPath != "sources/openapi/widget.yaml" {
		t.Fatalf("plan = %#v", plan)
	}
	if len(result.Context.Operations) == 0 {
		t.Fatal("prompt context has no operations")
	}
}

func TestDiscoveryAmbiguityIsBlockingEvenWithCandidate(t *testing.T) {
	root := t.TempDir()
	valid := []byte("openapi: 3.0.0\ninfo:\n  title: Widgets\n  version: 1.0.0\npaths:\n  /widgets:\n    get:\n      operationId: listWidgets\n      responses:\n        '200':\n          description: ok\n")
	if err := osWrite(filepath.Join(root, "api.yaml"), valid); err != nil {
		t.Fatal(err)
	}
	if err := osWrite(filepath.Join(root, "ambiguous.json"), []byte(`{"name":"unknown"}`)); err != nil {
		t.Fatal(err)
	}
	result, err := DiscoverLocalSources(context.Background(), DiscoveryOptions{Goal: "list widgets", Roots: []string{root}})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Plans) == 0 {
		t.Fatal("expected the valid source candidate")
	}
	if len(result.Blockers) == 0 || result.Blockers[0].Code != "ramen.icot.discovery_ambiguous" {
		t.Fatalf("blockers = %#v", result.Blockers)
	}
}

func TestDiscoveryOversizedRootFileIsBlockingEvenWithCandidate(t *testing.T) {
	root := t.TempDir()
	valid := []byte("openapi: 3.0.0\ninfo:\n  title: Widgets\n  version: 1.0.0\npaths: {}\n")
	if err := osWrite(filepath.Join(root, "api.yaml"), valid); err != nil {
		t.Fatal(err)
	}
	if err := osWrite(filepath.Join(root, "oversized.json"), make([]byte, len(valid)+32)); err != nil {
		t.Fatal(err)
	}
	result, err := DiscoverLocalSources(context.Background(), DiscoveryOptions{Goal: "list widgets", Roots: []string{root}, MaxBytes: int64(len(valid) + 1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Plans) != 1 {
		t.Fatalf("plans = %#v", result.Plans)
	}
	if len(result.Blockers) != 1 || result.Blockers[0].Code != "ramen.icot.discovery_oversized" {
		t.Fatalf("oversized blockers = %#v", result.Blockers)
	}
}

func osWrite(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}
