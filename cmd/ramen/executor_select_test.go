package main

import "testing"

func TestSelectTrustedExecutorMockCompatibility(t *testing.T) {
	exec, err := selectTrustedExecutor("mock", false, "")
	if err != nil {
		t.Fatalf("selectTrustedExecutor(mock): %v", err)
	}
	if exec == nil {
		t.Fatal("selectTrustedExecutor(mock) returned nil")
	}
	if _, err := selectTrustedExecutor("udon", true, ""); err == nil {
		t.Fatal("selectTrustedExecutor allowed --mock with --executor udon")
	}
}
