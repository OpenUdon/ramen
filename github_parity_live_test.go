//go:build githublive

package corpus

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestGitHubProviderParityLive(t *testing.T) {
	if os.Getenv(githubParityEnv) != "1" {
		t.Skipf("set %s=1 and %s=<lane> to run the opt-in GitHub provider parity harness", githubParityEnv, githubParityLaneEnv)
	}
	selectedLane := strings.ToLower(strings.TrimSpace(os.Getenv(githubParityLaneEnv)))
	if selectedLane == "" {
		t.Fatalf("%s=1 requires explicit %s selection", githubParityEnv, githubParityLaneEnv)
	}
	if !slices.Contains(githubParityLanes, selectedLane) {
		t.Fatalf("%s=%s is not a known GitHub parity lane", githubParityLaneEnv, selectedLane)
	}
	artifact := loadGitHubParityArtifact(t, filepath.Join(githubParityFixtureRoot, selectedLane, "observations.json"))
	assertGitHubParityArtifact(t, selectedLane, artifact)
	if !artifact.Safety.LiveEnabled {
		t.Skipf("%s=%s is live-disabled by artifact metadata", githubParityLaneEnv, selectedLane)
	}
	requireGitHubParityLiveEnv(t, artifact)
	if !slices.Contains(githubParityLiveRunnerLanes, selectedLane) {
		t.Fatalf("%s=%s is marked live-enabled but has no registered GitHub live runner", githubParityLaneEnv, selectedLane)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	var recording githubParityLiveRecording
	switch selectedLane {
	case "h01":
		recording = runGitHubParityH01Live(ctx, t, artifact)
	case "h02":
		recording = runGitHubParityH02Live(ctx, t, artifact)
	case "h03":
		recording = runGitHubParityH03Live(ctx, t, artifact)
	default:
		t.Fatalf("%s=%s is marked live-enabled but has no live runner implementation", githubParityLaneEnv, selectedLane)
	}
	compareOrUpdateGitHubParityRecording(t, recording, filepath.Join(githubParityFixtureRoot, selectedLane, "live.observations.json"))
}

func requireGitHubParityLiveEnv(t *testing.T, artifact githubParityArtifact) {
	t.Helper()
	for _, envName := range artifact.Safety.RequiredEnv {
		if strings.TrimSpace(os.Getenv(envName)) == "" {
			t.Fatalf("%s is required for live GitHub provider parity", envName)
		}
	}
}
