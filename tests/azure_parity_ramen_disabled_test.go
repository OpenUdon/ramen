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

func runAzureParityZ04Live(ctx context.Context, t *testing.T, artifact azureParityArtifact) azureParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'azurelive udon' to run the Z04 live runner")
	return azureParityLiveRecording{}
}

func runAzureParityZ05Live(ctx context.Context, t *testing.T, artifact azureParityArtifact) azureParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'azurelive udon' to run the Z05 live runner")
	return azureParityLiveRecording{}
}

func runAzureParityZ08Live(ctx context.Context, t *testing.T, artifact azureParityArtifact) azureParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'azurelive udon' to run the Z08 live runner")
	return azureParityLiveRecording{}
}

func runAzureParityZ09Live(ctx context.Context, t *testing.T, artifact azureParityArtifact) azureParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'azurelive udon' to run the Z09 live runner")
	return azureParityLiveRecording{}
}
