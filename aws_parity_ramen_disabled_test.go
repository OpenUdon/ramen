//go:build awslive && !udon

package corpus

import (
	"context"
	"testing"
)

func runAWSParityW01Live(ctx context.Context, t *testing.T, artifact awsParityArtifact) awsParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'awslive udon' to run the W01 live runner")
	return awsParityLiveRecording{}
}
