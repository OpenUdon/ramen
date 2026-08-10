package main

import (
	"testing"
)

func TestPositionalFirstLast(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "empty", args: nil, want: nil},
		{name: "flags only", args: []string{"--state", "state.db"}, want: []string{"--state", "state.db"}},
		{name: "single positional", args: []string{"addr"}, want: []string{"addr"}},
		{name: "positional before flags", args: []string{"addr", "--json"}, want: []string{"--json", "addr"}},
		{name: "flag before positional unchanged", args: []string{"--json", "addr"}, want: []string{"--json", "addr"}},
	}
	for _, tt := range tests {
		got := positionalFirstLast(tt.args)
		if len(got) != len(tt.want) {
			t.Fatalf("%s length = %d, want %d (%#v)", tt.name, len(got), len(tt.want), got)
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Fatalf("%s[%d] = %q, want %q (%#v)", tt.name, i, got[i], tt.want[i], got)
			}
		}
	}
}
