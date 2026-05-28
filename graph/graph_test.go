package graph

import (
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/project"
)

func TestSortOrdersDependenciesFirst(t *testing.T) {
	got, err := Sort([]Node{
		{Address: "c", DependsOn: []string{"b"}},
		{Address: "a"},
		{Address: "b", DependsOn: []string{"a"}},
	})
	if err != nil {
		t.Fatalf("Sort returned error: %v", err)
	}
	if len(got) != 3 || got[0].Address != "a" || got[1].Address != "b" || got[2].Address != "c" {
		t.Fatalf("order = %#v", got)
	}
}

func TestSortDetectsCycles(t *testing.T) {
	if _, err := Sort([]Node{
		{Address: "a", DependsOn: []string{"b"}},
		{Address: "b", DependsOn: []string{"a"}},
	}); err == nil {
		t.Fatal("Sort returned nil error for cycle")
	}
}

func TestBuildProjectEmitsNodesEdgesOperationsAndDOT(t *testing.T) {
	doc := &project.Document{
		Path: "/tmp/project.uws.json",
		Profile: project.Profile{Resources: []project.Resource{
			{
				Address: "example_resource.app",
				Kind:    "resource",
				Type:    "example_resource",
				Dependencies: []string{
					"example_resource.db",
				},
				Operations: map[string]project.OperationRole{
					"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createApp"},
				},
			},
			{
				Address: "example_resource.db",
				Kind:    "resource",
				Type:    "example_resource",
				Operations: map[string]project.OperationRole{
					"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createDB"},
				},
			},
		}},
	}
	graph := BuildProject(doc)
	if graph.Version != DocumentVersion || len(graph.Nodes) != 2 || len(graph.Edges) != 1 || len(graph.Diagnostics) != 0 {
		t.Fatalf("graph = %#v", graph)
	}
	if graph.Nodes[0].Address != "example_resource.db" || graph.Edges[0].From != "example_resource.db" || graph.Edges[0].To != "example_resource.app" {
		t.Fatalf("graph order/edge = %#v", graph)
	}
	dot := DOT(graph)
	if !strings.Contains(dot, `"example_resource.db" -> "example_resource.app"`) {
		t.Fatalf("DOT missing dependency edge:\n%s", dot)
	}
}

func TestBuildProjectReportsCycleAndMissingDependency(t *testing.T) {
	doc := &project.Document{
		Profile: project.Profile{Resources: []project.Resource{
			{Address: "a", Kind: "resource", Type: "example", Dependencies: []string{"b", "missing"}},
			{Address: "b", Kind: "resource", Type: "example", Dependencies: []string{"a"}},
		}},
	}
	graph := BuildProject(doc)
	if !hasGraphCode(graph, "graph.dependency_missing") || !hasGraphCode(graph, "graph.dependency_cycle") {
		t.Fatalf("graph diagnostics = %#v", graph.Diagnostics)
	}
}

func hasGraphCode(doc Document, code string) bool {
	for _, diag := range doc.Diagnostics {
		if diag.Code == code {
			return true
		}
	}
	return false
}
