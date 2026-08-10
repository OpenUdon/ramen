package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/state"
)

func TestCLIDestroyCommandRemoved(t *testing.T) {
	cmd := helperCommand("destroy", "--help")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("destroy command unexpectedly succeeded:\n%s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
		t.Fatalf("destroy exit = %v, output:\n%s", err, output)
	}
	if !strings.Contains(string(output), `unknown command "destroy"`) {
		t.Fatalf("destroy output missing unknown-command diagnostic:\n%s", output)
	}
}

func TestCLIShowIncludesAPIMethodSummary(t *testing.T) {
	root := t.TempDir()
	planPath := filepath.Join(root, "method-plan.json")
	data, err := json.Marshal(tfplan.Document{
		Version: tfplan.Version,
		Action:  "post",
		Summary: tfplan.Summary{Post: 1, Put: 1, Patch: 1, Read: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustWriteCLIFile(t, planPath, data)
	cmd := helperCommand("show", planPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("show failed: %v\n%s", err, output)
	}
	for _, expected := range []string{"post=1", "put=1", "patch=1", "read=1"} {
		if !strings.Contains(string(output), expected) {
			t.Fatalf("show output missing %q:\n%s", expected, output)
		}
	}
}

func TestCLIVersionOutputsPlainTextJSONAndHelp(t *testing.T) {
	cmd := helperCommand("version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version failed: %v\n%s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != version {
		t.Fatalf("version output = %q, want %q", got, version)
	}

	cmd = helperCommand("version", "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version --json failed: %v\n%s", err, output)
	}
	var info versionInfo
	if err := json.Unmarshal(output, &info); err != nil {
		t.Fatalf("version JSON is not parseable: %v\n%s", err, output)
	}
	if info.Version != version {
		t.Fatalf("version JSON version = %q, want %q", info.Version, version)
	}
	if info.Module != "github.com/OpenUdon/ramen" {
		t.Fatalf("version JSON module = %q, want github.com/OpenUdon/ramen", info.Module)
	}

	cmd = helperCommand("version", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version help failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"Usage: ramen version", "--json", "does not check networks"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("version help missing %q:\n%s", expected, text)
		}
	}
}

func TestCollectVersionInfoUsesInjectedReleaseVersion(t *testing.T) {
	previous := version
	version = "0.1.0"
	t.Cleanup(func() {
		version = previous
	})

	info := collectVersionInfo()
	if info.Version != "0.1.0" {
		t.Fatalf("version = %q, want 0.1.0", info.Version)
	}
	if info.Module != "github.com/OpenUdon/ramen" {
		t.Fatalf("module = %q, want github.com/OpenUdon/ramen", info.Module)
	}
}

func TestCLIForceUnlockRequiresExactHolder(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.db")
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.AcquireLock(context.Background(), "state", "holder-1", time.Minute); err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	_ = store.Close()

	cmd := helperCommand("force-unlock", "wrong-holder", "--state", statePath)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("force-unlock unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), "holder-1") || !strings.Contains(string(output), "wrong-holder") {
		t.Fatalf("force-unlock mismatch output missing holder detail:\n%s", output)
	}

	cmd = helperCommand("force-unlock", "holder-1", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("force-unlock failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "force-unlocked state held by holder-1") {
		t.Fatalf("force-unlock output missing summary:\n%s", output)
	}
	verify, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("reopen state: %v", err)
	}
	if lock, err := verify.CurrentLock(context.Background(), "state"); err != nil || lock != nil {
		t.Fatalf("lock after force unlock = %#v err=%v", lock, err)
	}
	_ = verify.Close()

	cmd = helperCommand("force-unlock", "holder-1", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("force-unlock missing lock unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), `state lock "state" is not held`) {
		t.Fatalf("force-unlock missing output:\n%s", output)
	}

	expired, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open expired state: %v", err)
	}
	if err := expired.AcquireLock(context.Background(), "state", "expired-holder", time.Nanosecond); err != nil {
		t.Fatalf("acquire expired lock: %v", err)
	}
	_ = expired.Close()
	time.Sleep(time.Millisecond)
	cmd = helperCommand("force-unlock", "expired-holder", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("force-unlock expired unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), `state lock "state" is not held`) {
		t.Fatalf("force-unlock expired output:\n%s", output)
	}

	cmd = helperCommand("force-unlock", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("force-unlock malformed args unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), "Usage: ramen force-unlock") {
		t.Fatalf("force-unlock malformed output missing usage:\n%s", output)
	}
}
