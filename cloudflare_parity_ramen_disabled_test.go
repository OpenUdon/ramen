//go:build cloudflarelive && !udon

package corpus

import (
	"context"
	"testing"
)

func runCloudflareParityC01Live(ctx context.Context, t *testing.T, artifact cloudflareParityArtifact) cloudflareParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'cloudflarelive udon' to run the C01 live runner")
	return cloudflareParityLiveRecording{}
}

func runCloudflareParityC02Live(ctx context.Context, t *testing.T, artifact cloudflareParityArtifact) cloudflareParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'cloudflarelive udon' to run the C02 live runner")
	return cloudflareParityLiveRecording{}
}

func runCloudflareParityC03Live(ctx context.Context, t *testing.T, artifact cloudflareParityArtifact) cloudflareParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'cloudflarelive udon' to run the C03 live runner")
	return cloudflareParityLiveRecording{}
}

func runCloudflareParityC04Live(ctx context.Context, t *testing.T, artifact cloudflareParityArtifact) cloudflareParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'cloudflarelive udon' to run the C04 live runner")
	return cloudflareParityLiveRecording{}
}

func runCloudflareParityC05Live(ctx context.Context, t *testing.T, artifact cloudflareParityArtifact) cloudflareParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'cloudflarelive udon' to run the C05 live runner")
	return cloudflareParityLiveRecording{}
}
