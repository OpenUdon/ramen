package graph

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/OpenUdon/ramen/project"
)

const DocumentVersion = "ramen.graph.v1"

type Document struct {
	Version     string       `json:"version"`
	ProjectPath string       `json:"project_path,omitempty"`
	Nodes       []GraphNode  `json:"nodes,omitempty"`
	Edges       []GraphEdge  `json:"edges,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type GraphNode struct {
	Address    string         `json:"address"`
	Kind       string         `json:"kind,omitempty"`
	Type       string         `json:"type,omitempty"`
	Operations []OperationRef `json:"operations,omitempty"`
}

type OperationRef struct {
	Role        string `json:"role"`
	SourceKind  string `json:"source_kind,omitempty"`
	SourceID    string `json:"source_id,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
}

type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

type Diagnostic struct {
	Code          string `json:"code"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	Address       string `json:"address,omitempty"`
	APISourceKind string `json:"api_source_kind,omitempty"`
	APISourceID   string `json:"api_source_id,omitempty"`
	OperationID   string `json:"operation_id,omitempty"`
}

func BuildProject(doc *project.Document) Document {
	out := Document{Version: DocumentVersion}
	if doc == nil {
		out.Diagnostics = append(out.Diagnostics, Diagnostic{Code: "graph.project_required", Severity: "error", Message: "project document is required"})
		return out
	}
	out.ProjectPath = doc.Path
	addresses := map[string]bool{}
	for _, resource := range doc.Profile.Resources {
		addresses[resource.Address] = true
	}
	var sortNodes []Node
	for _, resource := range doc.Profile.Resources {
		sortNodes = append(sortNodes, Node{Address: resource.Address, DependsOn: resource.Dependencies})
		for _, dep := range resource.Dependencies {
			if !addresses[dep] {
				out.Diagnostics = append(out.Diagnostics, Diagnostic{Code: "graph.dependency_missing", Severity: "error", Message: fmt.Sprintf("resource %s depends on missing resource %s", resource.Address, dep), Address: resource.Address})
			}
			out.Edges = append(out.Edges, GraphEdge{From: dep, To: resource.Address, Type: "dependency"})
		}
	}
	ordered, err := Sort(sortNodes)
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, Diagnostic{Code: "graph.dependency_cycle", Severity: "error", Message: err.Error()})
		ordered = sortNodes
		slices.SortFunc(ordered, func(a, b Node) int { return strings.Compare(a.Address, b.Address) })
	}
	byAddress := map[string]project.Resource{}
	for _, resource := range doc.Profile.Resources {
		byAddress[resource.Address] = resource
	}
	for _, sorted := range ordered {
		resource, ok := byAddress[sorted.Address]
		if !ok {
			continue
		}
		node := GraphNode{
			Address: resource.Address,
			Kind:    resource.Kind,
			Type:    resource.Type,
		}
		for role, op := range resource.Operations {
			node.Operations = append(node.Operations, OperationRef{Role: firstNonEmpty(op.Purpose, role), SourceKind: op.SourceKind, SourceID: op.SourceID, OperationID: op.OperationID})
		}
		slices.SortFunc(node.Operations, func(a, b OperationRef) int { return strings.Compare(a.Role, b.Role) })
		out.Nodes = append(out.Nodes, node)
	}
	slices.SortFunc(out.Edges, func(a, b GraphEdge) int {
		if diff := strings.Compare(a.From, b.From); diff != 0 {
			return diff
		}
		return strings.Compare(a.To, b.To)
	})
	slices.SortFunc(out.Diagnostics, func(a, b Diagnostic) int {
		left := []string{a.Severity, a.Code, a.Address, a.Message}
		right := []string{b.Severity, b.Code, b.Address, b.Message}
		return slices.Compare(left, right)
	})
	return out
}

func DOT(doc Document) string {
	var b strings.Builder
	b.WriteString("digraph ramen {\n")
	b.WriteString("  rankdir=LR;\n")
	for _, node := range doc.Nodes {
		fmt.Fprintf(&b, "  %s [label=%s];\n", strconv.Quote(node.Address), strconv.Quote(node.Address))
	}
	for _, edge := range doc.Edges {
		fmt.Fprintf(&b, "  %s -> %s;\n", strconv.Quote(edge.From), strconv.Quote(edge.To))
	}
	b.WriteString("}\n")
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
