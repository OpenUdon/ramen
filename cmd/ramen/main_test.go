package main

import (
	"strings"
	"testing"
)

func TestCLITopLevelCommandDispatchHelp(t *testing.T) {
	tests := []struct {
		command string
		usage   string
	}{
		{command: "apply", usage: "Usage: ramen apply"},
		{command: "author", usage: "Usage: ramen author"},
		{command: "convert", usage: "Usage: ramen convert"},
		{command: "force-unlock", usage: "Usage: ramen force-unlock"},
		{command: "graph", usage: "Usage: ramen graph"},
		{command: "icot", usage: "Usage: ramen icot"},
		{command: "import", usage: "Usage: ramen import"},
		{command: "init", usage: "Usage: ramen init"},
		{command: "plan", usage: "Usage: ramen plan"},
		{command: "refresh", usage: "Usage: ramen refresh"},
		{command: "run", usage: "Usage: ramen run"},
		{command: "show", usage: "Usage: ramen show"},
		{command: "state", usage: "Usage: ramen state"},
		{command: "validate", usage: "Usage: ramen validate"},
		{command: "version", usage: "Usage: ramen version"},
	}

	for _, tc := range tests {
		t.Run(tc.command, func(t *testing.T) {
			output, err := helperCommand(tc.command, "--help").CombinedOutput()
			if err != nil {
				t.Fatalf("%s --help failed: %v\n%s", tc.command, err, output)
			}
			if !strings.Contains(string(output), tc.usage) {
				t.Fatalf("%s --help missing %q:\n%s", tc.command, tc.usage, output)
			}
		})
	}
}
