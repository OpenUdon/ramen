//go:build azurelive && !udon

package corpus

import (
	"context"
	"testing"
)

func runAzureParityZ01Live(ctx context.Context, t *testing.T, artifact azureParityArtifact) azureParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'azurelive udon' to run the Z01 live runner")
	return azureParityLiveRecording{}
}

func runAzureParityZ02Live(ctx context.Context, t *testing.T, artifact azureParityArtifact) azureParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'azurelive udon' to run the Z02 live runner")
	return azureParityLiveRecording{}
}
