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
