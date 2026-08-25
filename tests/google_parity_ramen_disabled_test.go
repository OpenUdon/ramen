//go:build googlelive && !udon

package corpus

import (
	"context"
	"testing"
)

func runGoogleParityY02Live(ctx context.Context, t *testing.T, artifact googleParityArtifact) googleParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'googlelive udon' to run the Y02 live runner")
	return googleParityLiveRecording{}
}

func runGoogleParityY03Live(ctx context.Context, t *testing.T, artifact googleParityArtifact) googleParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'googlelive udon' to run the Y03 live runner")
	return googleParityLiveRecording{}
}

func runGoogleParityY04Live(ctx context.Context, t *testing.T, artifact googleParityArtifact) googleParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'googlelive udon' to run the Y04 live runner")
	return googleParityLiveRecording{}
}

func runGoogleParityY05Live(ctx context.Context, t *testing.T, artifact googleParityArtifact) googleParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'googlelive udon' to run the Y05 live runner")
	return googleParityLiveRecording{}
}

func runGoogleParityY06Live(ctx context.Context, t *testing.T, artifact googleParityArtifact) googleParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'googlelive udon' to run the Y06 live runner")
	return googleParityLiveRecording{}
}

func runGoogleParityY08Live(ctx context.Context, t *testing.T, artifact googleParityArtifact) googleParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'googlelive udon' to run the Y08 live runner")
	return googleParityLiveRecording{}
}
