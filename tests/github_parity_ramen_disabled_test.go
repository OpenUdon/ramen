//go:build githublive && !udon

package corpus

import (
	"context"
	"testing"
)

func runGitHubParityH01Live(ctx context.Context, t *testing.T, artifact githubParityArtifact) githubParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'githublive udon' to run the H01 live runner")
	return githubParityLiveRecording{}
}

func runGitHubParityH02Live(ctx context.Context, t *testing.T, artifact githubParityArtifact) githubParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'githublive udon' to run the H02 live runner")
	return githubParityLiveRecording{}
}

func runGitHubParityH03Live(ctx context.Context, t *testing.T, artifact githubParityArtifact) githubParityLiveRecording {
	t.Helper()
	t.Skip("build with -tags 'githublive udon' to run the H03 live runner")
	return githubParityLiveRecording{}
}
