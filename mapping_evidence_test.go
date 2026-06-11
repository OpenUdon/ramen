package corpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/OpenUdon/ramen/tfmapping"
)

type mappingEvidenceArtifact struct {
	Version   string                    `json:"version"`
	Lane      string                    `json:"lane"`
	Status    string                    `json:"status"`
	Scenarios []mappingEvidenceScenario `json:"scenarios"`
}

type mappingEvidenceScenario struct {
	ResourceType         string   `json:"resource_type"`
	ExpectedTransitions  []string `json:"expected_transitions"`
	ObservationArtifacts []string `json:"observation_artifacts"`
}

func TestKubernetesRoleBindingMappingIsGatedByK07Evidence(t *testing.T) {
	evidence := loadMappingEvidenceArtifact(t, filepath.Join("testdata", "parity", "kubernetes", "k07", "observations.json"))
	if evidence.Version != kubernetesParityArtifactV1 {
		t.Fatalf("K07 evidence version = %q, want %q", evidence.Version, kubernetesParityArtifactV1)
	}
	if evidence.Lane != "K07" || evidence.Status != "recorded" {
		t.Fatalf("K07 evidence lane/status = %s/%s, want K07/recorded", evidence.Lane, evidence.Status)
	}
	scenario := findMappingEvidenceScenario(evidence, "kubernetes_role_binding_v1")
	if scenario == nil {
		t.Fatal("K07 evidence does not record kubernetes_role_binding_v1")
	}
	for _, transition := range []string{"create", "read", "no-op", "destroy"} {
		if !slices.Contains(scenario.ExpectedTransitions, transition) {
			t.Fatalf("K07 RoleBinding evidence missing %q transition: %#v", transition, scenario.ExpectedTransitions)
		}
	}
	for _, artifact := range scenario.ObservationArtifacts {
		if _, err := os.Stat(artifact); err != nil {
			t.Fatalf("K07 RoleBinding observation artifact %s unavailable: %v", artifact, err)
		}
	}

	registry := tfmapping.DefaultRegistry()
	for _, tt := range []struct {
		purpose string
		action  string
		want    string
	}{
		{purpose: "create", action: "create", want: "createRbacAuthorizationV1NamespacedRoleBinding"},
		{purpose: "read", action: "read", want: "readRbacAuthorizationV1NamespacedRoleBinding"},
		{purpose: "delete", action: "delete", want: "deleteRbacAuthorizationV1NamespacedRoleBinding"},
	} {
		mapping := registry.MapObject(tfmapping.Object{Kind: "resource", Type: "kubernetes_role_binding_v1", Provider: "provider.kubernetes"}, tt.purpose, tt.action)
		if len(mapping.Diagnostics) != 0 {
			t.Fatalf("%s mapping diagnostics = %#v", tt.purpose, mapping.Diagnostics)
		}
		if len(mapping.Target.OperationIDs) == 0 || mapping.Target.OperationIDs[0] != tt.want {
			t.Fatalf("%s operation IDs = %#v, want first %q", tt.purpose, mapping.Target.OperationIDs, tt.want)
		}
	}

	mapping := registry.MapObject(tfmapping.Object{Kind: "resource", Type: "kubernetes_role_binding_v1", Provider: "provider.kubernetes"}, "update", "update")
	if len(mapping.Diagnostics) != 1 {
		t.Fatalf("role binding update diagnostics = %#v, want one", mapping.Diagnostics)
	}
	if mapping.Diagnostics[0].Code != tfmapping.DiagnosticCodeUnsupportedAction {
		t.Fatalf("role binding update diagnostic = %q, want %q", mapping.Diagnostics[0].Code, tfmapping.DiagnosticCodeUnsupportedAction)
	}
}

func TestKubernetesClusterRoleMappingIsGatedByK08Evidence(t *testing.T) {
	evidence := loadMappingEvidenceArtifact(t, filepath.Join("testdata", "parity", "kubernetes", "k08", "observations.json"))
	if evidence.Lane != "K08" || evidence.Status != "recorded" {
		t.Fatalf("K08 evidence lane/status = %s/%s, want K08/recorded", evidence.Lane, evidence.Status)
	}
	scenario := findMappingEvidenceScenario(evidence, "kubernetes_cluster_role_v1")
	if scenario == nil {
		t.Fatal("K08 evidence does not record kubernetes_cluster_role_v1")
	}
	for _, transition := range []string{"create", "read", "no-op", "destroy"} {
		if !slices.Contains(scenario.ExpectedTransitions, transition) {
			t.Fatalf("K08 ClusterRole evidence missing %q transition: %#v", transition, scenario.ExpectedTransitions)
		}
	}
	for _, artifact := range scenario.ObservationArtifacts {
		if _, err := os.Stat(artifact); err != nil {
			t.Fatalf("K08 ClusterRole observation artifact %s unavailable: %v", artifact, err)
		}
	}

	registry := tfmapping.DefaultRegistry()
	for _, tt := range []struct {
		purpose string
		action  string
		want    string
	}{
		{purpose: "create", action: "create", want: "createRbacAuthorizationV1ClusterRole"},
		{purpose: "read", action: "read", want: "readRbacAuthorizationV1ClusterRole"},
		{purpose: "delete", action: "delete", want: "deleteRbacAuthorizationV1ClusterRole"},
	} {
		mapping := registry.MapObject(tfmapping.Object{Kind: "resource", Type: "kubernetes_cluster_role_v1", Provider: "provider.kubernetes"}, tt.purpose, tt.action)
		if len(mapping.Diagnostics) != 0 {
			t.Fatalf("%s mapping diagnostics = %#v", tt.purpose, mapping.Diagnostics)
		}
		if len(mapping.Target.OperationIDs) == 0 || mapping.Target.OperationIDs[0] != tt.want {
			t.Fatalf("%s operation IDs = %#v, want first %q", tt.purpose, mapping.Target.OperationIDs, tt.want)
		}
	}

	mapping := registry.MapObject(tfmapping.Object{Kind: "resource", Type: "kubernetes_cluster_role_v1", Provider: "provider.kubernetes"}, "update", "update")
	if len(mapping.Diagnostics) != 1 {
		t.Fatalf("cluster role update diagnostics = %#v, want one", mapping.Diagnostics)
	}
	if mapping.Diagnostics[0].Code != tfmapping.DiagnosticCodeUnsupportedAction {
		t.Fatalf("cluster role update diagnostic = %q, want %q", mapping.Diagnostics[0].Code, tfmapping.DiagnosticCodeUnsupportedAction)
	}
}

func loadMappingEvidenceArtifact(t *testing.T, path string) mappingEvidenceArtifact {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read mapping evidence %s: %v", path, err)
	}
	var evidence mappingEvidenceArtifact
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatalf("parse mapping evidence %s: %v", path, err)
	}
	return evidence
}

func findMappingEvidenceScenario(evidence mappingEvidenceArtifact, resourceType string) *mappingEvidenceScenario {
	for i := range evidence.Scenarios {
		if evidence.Scenarios[i].ResourceType == resourceType {
			return &evidence.Scenarios[i]
		}
	}
	return nil
}
