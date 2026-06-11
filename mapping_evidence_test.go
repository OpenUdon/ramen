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

}

func TestKubernetesRoleBindingUpdateMappingIsGatedByK09Evidence(t *testing.T) {
	evidence := loadMappingEvidenceArtifact(t, filepath.Join("testdata", "parity", "kubernetes", "k09", "observations.json"))
	if evidence.Lane != "K09" || evidence.Status != "recorded" {
		t.Fatalf("K09 evidence lane/status = %s/%s, want K09/recorded", evidence.Lane, evidence.Status)
	}
	scenario := findMappingEvidenceScenario(evidence, "kubernetes_role_binding_v1")
	if scenario == nil {
		t.Fatal("K09 evidence does not record kubernetes_role_binding_v1")
	}
	for _, transition := range []string{"create", "update", "read", "no-op", "destroy"} {
		if !slices.Contains(scenario.ExpectedTransitions, transition) {
			t.Fatalf("K09 RoleBinding evidence missing %q transition: %#v", transition, scenario.ExpectedTransitions)
		}
	}
	for _, artifact := range scenario.ObservationArtifacts {
		if _, err := os.Stat(artifact); err != nil {
			t.Fatalf("K09 RoleBinding observation artifact %s unavailable: %v", artifact, err)
		}
	}

	registry := tfmapping.DefaultRegistry()
	mapping := registry.MapObject(tfmapping.Object{Kind: "resource", Type: "kubernetes_role_binding_v1", Provider: "provider.kubernetes"}, "update", "update")
	if len(mapping.Diagnostics) != 0 {
		t.Fatalf("role binding update diagnostics = %#v", mapping.Diagnostics)
	}
	if len(mapping.Target.OperationIDs) == 0 || mapping.Target.OperationIDs[0] != "replaceRbacAuthorizationV1NamespacedRoleBinding" {
		t.Fatalf("role binding update operation IDs = %#v", mapping.Target.OperationIDs)
	}
}

func TestKubernetesClusterRoleUpdateMappingIsGatedByK10Evidence(t *testing.T) {
	evidence := loadMappingEvidenceArtifact(t, filepath.Join("testdata", "parity", "kubernetes", "k10", "observations.json"))
	if evidence.Lane != "K10" || evidence.Status != "recorded" {
		t.Fatalf("K10 evidence lane/status = %s/%s, want K10/recorded", evidence.Lane, evidence.Status)
	}
	scenario := findMappingEvidenceScenario(evidence, "kubernetes_cluster_role_v1")
	if scenario == nil {
		t.Fatal("K10 evidence does not record kubernetes_cluster_role_v1")
	}
	for _, transition := range []string{"create", "update", "read", "no-op", "destroy"} {
		if !slices.Contains(scenario.ExpectedTransitions, transition) {
			t.Fatalf("K10 ClusterRole evidence missing %q transition: %#v", transition, scenario.ExpectedTransitions)
		}
	}
	for _, artifact := range scenario.ObservationArtifacts {
		if _, err := os.Stat(artifact); err != nil {
			t.Fatalf("K10 ClusterRole observation artifact %s unavailable: %v", artifact, err)
		}
	}

	registry := tfmapping.DefaultRegistry()
	mapping := registry.MapObject(tfmapping.Object{Kind: "resource", Type: "kubernetes_cluster_role_v1", Provider: "provider.kubernetes"}, "update", "update")
	if len(mapping.Diagnostics) != 0 {
		t.Fatalf("cluster role update diagnostics = %#v", mapping.Diagnostics)
	}
	if len(mapping.Target.OperationIDs) == 0 || mapping.Target.OperationIDs[0] != "replaceRbacAuthorizationV1ClusterRole" {
		t.Fatalf("cluster role update operation IDs = %#v", mapping.Target.OperationIDs)
	}
}

func TestKubernetesClusterRoleBindingMappingIsGatedByK12Evidence(t *testing.T) {
	evidence := loadMappingEvidenceArtifact(t, filepath.Join("testdata", "parity", "kubernetes", "k12", "observations.json"))
	if evidence.Lane != "K12" || evidence.Status != "recorded" {
		t.Fatalf("K12 evidence lane/status = %s/%s, want K12/recorded", evidence.Lane, evidence.Status)
	}
	scenario := findMappingEvidenceScenario(evidence, "kubernetes_cluster_role_binding_v1")
	if scenario == nil {
		t.Fatal("K12 evidence does not record kubernetes_cluster_role_binding_v1")
	}
	for _, transition := range []string{"create", "read", "no-op", "destroy"} {
		if !slices.Contains(scenario.ExpectedTransitions, transition) {
			t.Fatalf("K12 ClusterRoleBinding evidence missing %q transition: %#v", transition, scenario.ExpectedTransitions)
		}
	}
	for _, artifact := range scenario.ObservationArtifacts {
		if _, err := os.Stat(artifact); err != nil {
			t.Fatalf("K12 ClusterRoleBinding observation artifact %s unavailable: %v", artifact, err)
		}
	}

	registry := tfmapping.DefaultRegistry()
	for _, tt := range []struct {
		purpose string
		action  string
		want    string
	}{
		{purpose: "create", action: "create", want: "createRbacAuthorizationV1ClusterRoleBinding"},
		{purpose: "read", action: "read", want: "readRbacAuthorizationV1ClusterRoleBinding"},
		{purpose: "delete", action: "delete", want: "deleteRbacAuthorizationV1ClusterRoleBinding"},
	} {
		mapping := registry.MapObject(tfmapping.Object{Kind: "resource", Type: "kubernetes_cluster_role_binding_v1", Provider: "provider.kubernetes"}, tt.purpose, tt.action)
		if len(mapping.Diagnostics) != 0 {
			t.Fatalf("%s mapping diagnostics = %#v", tt.purpose, mapping.Diagnostics)
		}
		if len(mapping.Target.OperationIDs) == 0 || mapping.Target.OperationIDs[0] != tt.want {
			t.Fatalf("%s operation IDs = %#v, want first %q", tt.purpose, mapping.Target.OperationIDs, tt.want)
		}
	}
}

func TestKubernetesClusterRoleBindingUpdateMappingIsGatedByK13Evidence(t *testing.T) {
	evidence := loadMappingEvidenceArtifact(t, filepath.Join("testdata", "parity", "kubernetes", "k13", "observations.json"))
	if evidence.Lane != "K13" || evidence.Status != "recorded" {
		t.Fatalf("K13 evidence lane/status = %s/%s, want K13/recorded", evidence.Lane, evidence.Status)
	}
	scenario := findMappingEvidenceScenario(evidence, "kubernetes_cluster_role_binding_v1")
	if scenario == nil {
		t.Fatal("K13 evidence does not record kubernetes_cluster_role_binding_v1")
	}
	for _, transition := range []string{"create", "update", "read", "no-op", "destroy"} {
		if !slices.Contains(scenario.ExpectedTransitions, transition) {
			t.Fatalf("K13 ClusterRoleBinding evidence missing %q transition: %#v", transition, scenario.ExpectedTransitions)
		}
	}
	for _, artifact := range scenario.ObservationArtifacts {
		if _, err := os.Stat(artifact); err != nil {
			t.Fatalf("K13 ClusterRoleBinding observation artifact %s unavailable: %v", artifact, err)
		}
	}

	registry := tfmapping.DefaultRegistry()
	mapping := registry.MapObject(tfmapping.Object{Kind: "resource", Type: "kubernetes_cluster_role_binding_v1", Provider: "provider.kubernetes"}, "update", "update")
	if len(mapping.Diagnostics) != 0 {
		t.Fatalf("cluster role binding update diagnostics = %#v", mapping.Diagnostics)
	}
	if len(mapping.Target.OperationIDs) == 0 || mapping.Target.OperationIDs[0] != "replaceRbacAuthorizationV1ClusterRoleBinding" {
		t.Fatalf("cluster role binding update operation IDs = %#v", mapping.Target.OperationIDs)
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
