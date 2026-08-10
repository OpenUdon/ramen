package governance

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadPolicyFilesIgnoresBlankPathsAndReportsLoadErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	engine, decisions := LoadPolicyFiles([]string{"", "  ", missing})
	if len(engine.Policies) != 0 || len(engine.Refs) != 0 {
		t.Fatalf("blank or missing paths loaded policies: %#v", engine)
	}
	if len(decisions) != 1 || decisions[0].Code != "policy.file_load_error" {
		t.Fatalf("load decisions = %#v", decisions)
	}
}

func TestLoadPolicyFilesParsesYAMLByExtensionAndOtherFilesAsJSON(t *testing.T) {
	root := t.TempDir()
	yamlPath := writePolicyTestFile(t, root, "one.yaml", []byte("deny_actions: [delete]\n"))
	ymlPath := writePolicyTestFile(t, root, "two.yml", []byte("warn_actions: [update]\n"))
	jsonPath := writePolicyTestFile(t, root, "three.policy", []byte(`{"max_deletes":1}`))
	wrongExtensionPath := writePolicyTestFile(t, root, "four.txt", []byte("deny_actions: [delete]\n"))

	engine, decisions := LoadPolicyFiles([]string{yamlPath, ymlPath, jsonPath, wrongExtensionPath})
	if len(engine.Policies) != 3 || len(engine.Refs) != 3 {
		t.Fatalf("parsed engine = %#v", engine)
	}
	if len(decisions) != 1 || decisions[0].Code != "policy.file_parse_error" {
		t.Fatalf("parse decisions = %#v", decisions)
	}
}

func TestLoadPolicyFilesAcceptsAbsentOrV1VersionAndRejectsOtherVersions(t *testing.T) {
	root := t.TempDir()
	absent := writePolicyTestFile(t, root, "absent.json", []byte(`{"name":"absent"}`))
	v1 := writePolicyTestFile(t, root, "v1.json", []byte(`{"version":"ramen.policy.v1","name":"v1"}`))
	future := writePolicyTestFile(t, root, "future.json", []byte(`{"version":"ramen.policy.v2","name":"future"}`))

	engine, decisions := LoadPolicyFiles([]string{absent, v1, future})
	if len(engine.Policies) != 2 || engine.Policies[0].Name != "absent" || engine.Policies[1].Name != "v1" {
		t.Fatalf("version-filtered engine = %#v", engine)
	}
	if len(decisions) != 1 || decisions[0].Code != "policy.version_invalid" {
		t.Fatalf("version decisions = %#v", decisions)
	}
}

func TestLoadPolicyFilesUsesBasenameForUnnamedPolicies(t *testing.T) {
	root := t.TempDir()
	unnamed := writePolicyTestFile(t, root, "guard.yaml", []byte("deny_actions: [delete]\n"))
	named := writePolicyTestFile(t, root, "source.json", []byte(`{"name":"explicit"}`))

	engine, decisions := LoadPolicyFiles([]string{unnamed, named})
	if len(decisions) != 0 {
		t.Fatalf("name decisions = %#v", decisions)
	}
	if engine.Policies[0].Name != "guard.yaml" || engine.Refs[0].Name != "guard.yaml" {
		t.Fatalf("unnamed policy = %#v ref=%#v", engine.Policies[0], engine.Refs[0])
	}
	if engine.Policies[1].Name != "explicit" || engine.Refs[1].Name != "explicit" {
		t.Fatalf("named policy = %#v ref=%#v", engine.Policies[1], engine.Refs[1])
	}
}

func TestLoadPolicyFilesDigestsExactSourceBytes(t *testing.T) {
	root := t.TempDir()
	oneBytes := []byte("{\"name\":\"same\",\"deny_actions\":[\"delete\"]}\n")
	twoBytes := []byte("{ \"name\": \"same\", \"deny_actions\": [\"delete\"] }\n")
	one := writePolicyTestFile(t, root, "one.json", oneBytes)
	two := writePolicyTestFile(t, root, "two.json", twoBytes)

	engine, decisions := LoadPolicyFiles([]string{one, two})
	if len(decisions) != 0 || len(engine.Refs) != 2 {
		t.Fatalf("digest load: engine=%#v decisions=%#v", engine, decisions)
	}
	if engine.Refs[0].Digest != digestBytes(oneBytes) || engine.Refs[1].Digest != digestBytes(twoBytes) {
		t.Fatalf("digests = %#v", engine.Refs)
	}
	if engine.Refs[0].Digest == engine.Refs[1].Digest {
		t.Fatalf("different source bytes produced the same digest: %#v", engine.Refs)
	}
}

func TestLoadPolicyFilesKeepsValidPoliciesWhenSiblingsFail(t *testing.T) {
	root := t.TempDir()
	valid := writePolicyTestFile(t, root, "valid.yaml", []byte("name: valid\ndeny_actions: [delete]\n"))
	invalid := writePolicyTestFile(t, root, "invalid.yaml", []byte("deny_actions: ["))
	missing := filepath.Join(root, "missing.json")

	engine, decisions := LoadPolicyFiles([]string{invalid, valid, missing})
	if len(engine.Policies) != 1 || engine.Policies[0].Name != "valid" {
		t.Fatalf("mixed-validity engine = %#v", engine)
	}
	if !hasDecisionCode(decisions, "policy.file_parse_error") || !hasDecisionCode(decisions, "policy.file_load_error") {
		t.Fatalf("mixed-validity decisions = %#v", decisions)
	}
	result := engine.Evaluate(Input{Resources: []Resource{{Address: "example.one", Action: "delete"}}})
	if !hasDecisionCode(result.Decisions, "policy.deny") {
		t.Fatalf("valid sibling policy was not usable: %#v", result)
	}
}

func TestPolicyEvaluationOrderingIsDeterministic(t *testing.T) {
	root := t.TempDir()
	aPath := writePolicyTestFile(t, root, "a.json", []byte(`{"name":"a","warn_actions":["update"],"require_approval_addresses":["resource.b"]}`))
	zPath := writePolicyTestFile(t, root, "z.json", []byte(`{"name":"z","deny_actions":["delete"]}`))

	firstEngine, firstLoadDecisions := LoadPolicyFiles([]string{zPath, aPath})
	secondEngine, secondLoadDecisions := LoadPolicyFiles([]string{aPath, zPath})
	if len(firstLoadDecisions) != 0 || len(secondLoadDecisions) != 0 {
		t.Fatalf("load decisions: first=%#v second=%#v", firstLoadDecisions, secondLoadDecisions)
	}
	first := firstEngine.Evaluate(Input{Resources: []Resource{
		{Address: "resource.b", Action: "update"},
		{Address: "resource.a", Action: "delete"},
	}})
	second := secondEngine.Evaluate(Input{Resources: []Resource{
		{Address: "resource.a", Action: "delete"},
		{Address: "resource.b", Action: "update"},
	}})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("evaluation order changed result:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if len(first.Policies) != 2 || first.Policies[0].Name != "a" || first.Policies[1].Name != "z" {
		t.Fatalf("policy refs are not deterministic: %#v", first.Policies)
	}
}

func writePolicyTestFile(t *testing.T, root, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
