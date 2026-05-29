//go:build !udon

package main

import (
	"strings"
	"testing"
)

func TestSelectTrustedExecutorRejectsUdonInPublicBuild(t *testing.T) {
	exec, err := selectTrustedExecutor("udon", false, "")
	if err == nil {
		t.Fatalf("selectTrustedExecutor returned executor %#v, want error", exec)
	}
	if !strings.Contains(err.Error(), "build with -tags udon") {
		t.Fatalf("unexpected error: %v", err)
	}
}
