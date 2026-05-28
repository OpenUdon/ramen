package graph

import "testing"

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
