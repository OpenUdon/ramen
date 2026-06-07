//go:build cloudflarelive

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

func TestCloudflareProviderParityLive(t *testing.T) {
	if os.Getenv(cloudflareParityEnv) != "1" {
		t.Skipf("set %s=1 and %s=<lane> to run the opt-in Cloudflare provider parity harness", cloudflareParityEnv, cloudflareParityLaneEnv)
	}
	selectedLane := strings.ToLower(strings.TrimSpace(os.Getenv(cloudflareParityLaneEnv)))
	if selectedLane == "" {
		t.Fatalf("%s=1 requires explicit %s selection", cloudflareParityEnv, cloudflareParityLaneEnv)
	}
	if !slices.Contains(cloudflareParityLanes, selectedLane) {
		t.Fatalf("%s=%s is not a known Cloudflare parity lane", cloudflareParityLaneEnv, selectedLane)
	}
	artifact := loadCloudflareParityArtifact(t, filepath.Join(cloudflareParityFixtureRoot, selectedLane, "observations.json"))
	assertCloudflareParityArtifact(t, selectedLane, artifact)
	if !artifact.Safety.LiveEnabled {
		t.Skipf("%s=%s is live-disabled by artifact metadata", cloudflareParityLaneEnv, selectedLane)
	}
	requireCloudflareParityLiveEnv(t, artifact)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	var recording cloudflareParityLiveRecording
	switch selectedLane {
	case "c01":
		recording = runCloudflareParityC01Live(ctx, t, artifact)
	case "c02":
		recording = runCloudflareParityC02Live(ctx, t, artifact)
	case "c03":
		recording = runCloudflareParityC03Live(ctx, t, artifact)
	case "c04":
		recording = runCloudflareParityC04Live(ctx, t, artifact)
	case "c05":
		recording = runCloudflareParityC05Live(ctx, t, artifact)
	default:
		t.Skipf("%s=%s is not live-enabled in this implementation", cloudflareParityLaneEnv, selectedLane)
	}
	compareOrUpdateCloudflareParityRecording(t, recording, filepath.Join(cloudflareParityFixtureRoot, selectedLane, "live.observations.json"))
}

func requireCloudflareParityLiveEnv(t *testing.T, artifact cloudflareParityArtifact) {
	t.Helper()
	for _, envName := range artifact.Safety.RequiredEnv {
		if strings.TrimSpace(os.Getenv(envName)) == "" {
			t.Fatalf("%s is required for live Cloudflare provider parity", envName)
		}
	}
}
