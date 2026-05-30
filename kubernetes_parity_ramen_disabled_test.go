//go:build !udon

package corpus

import (
	"context"
	"fmt"
	"testing"
)

func runKubernetesParityRamenRuntime(ctx context.Context, t *testing.T, env kubernetesParityLiveEnv, _ string) kubernetesParityRuntimeResult {
	t.Helper()
	return kubernetesParityFailure("ramen", "udon-disabled", fmt.Errorf("build with -tags udon to run Ramen Kubernetes provider parity"))
}

func runKubernetesParityRamenReadMissingRuntime(ctx context.Context, t *testing.T, env kubernetesParityLiveEnv, _ string) kubernetesParityRuntimeResult {
	t.Helper()
	return kubernetesParityFailure("ramen", "udon-disabled", fmt.Errorf("build with -tags udon to run Ramen Kubernetes provider parity"))
}

func runKubernetesParityRamenConfigMapRuntime(ctx context.Context, t *testing.T, env kubernetesParityLiveEnv, _ string) kubernetesParityRuntimeResult {
	t.Helper()
	return kubernetesParityFailure("ramen", "udon-disabled", fmt.Errorf("build with -tags udon to run Ramen Kubernetes provider parity"))
}

func runKubernetesParityRamenServiceAccountRuntime(ctx context.Context, t *testing.T, env kubernetesParityLiveEnv, _ string) kubernetesParityRuntimeResult {
	t.Helper()
	return kubernetesParityFailure("ramen", "udon-disabled", fmt.Errorf("build with -tags udon to run Ramen Kubernetes provider parity"))
}

func runKubernetesParityRamenRoleRuntime(ctx context.Context, t *testing.T, env kubernetesParityLiveEnv, _ string) kubernetesParityRuntimeResult {
	t.Helper()
	return kubernetesParityFailure("ramen", "udon-disabled", fmt.Errorf("build with -tags udon to run Ramen Kubernetes provider parity"))
}

func runKubernetesParityRamenSecretRuntime(ctx context.Context, t *testing.T, env kubernetesParityLiveEnv, _ string) kubernetesParityRuntimeResult {
	t.Helper()
	return kubernetesParityFailure("ramen", "udon-disabled", fmt.Errorf("build with -tags udon to run Ramen Kubernetes provider parity"))
}
