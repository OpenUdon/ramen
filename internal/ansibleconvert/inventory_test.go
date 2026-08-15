package ansibleconvert

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadStaticInventoryINIAndExactTargets(t *testing.T) {
	inv, diags := loadStaticInventory([]string{filepath.Join("testdata", "inventory.ini")})
	if len(diags) != 0 || inv == nil || inv.invalid {
		t.Fatalf("load inventory: inv=%#v diags=%#v", inv, diags)
	}
	for pattern, want := range map[string][]string{
		"all":        {"web-01", "web-02"},
		"web":        {"web-01", "web-02"},
		"webservers": {"web-01", "web-02"},
		"web-01":     {"web-01"},
	} {
		got, err := inv.selectHosts(pattern)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("selectHosts(%q) = %#v, %v; want %#v", pattern, got, err, want)
		}
	}
	for _, pattern := range []string{"missing", "web,webservers", "web*", "{{ target }}"} {
		if _, err := inv.selectHosts(pattern); err == nil {
			t.Fatalf("selectHosts(%q) succeeded, want bounded-pattern failure", pattern)
		}
	}
}

func TestLoadStaticInventoryYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.yml")
	data := []byte(`all:
  vars:
    env: prod
  children:
    web:
      vars:
        tier: frontend
      hosts:
        web-02:
          zone: b
        web-01:
          zone: a
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	inv, diags := loadStaticInventory([]string{path})
	if len(diags) != 0 || inv == nil || inv.invalid {
		t.Fatalf("load YAML inventory: inv=%#v diags=%#v", inv, diags)
	}
	got, err := inv.selectHosts("web")
	if err != nil || !reflect.DeepEqual(got, []string{"web-01", "web-02"}) {
		t.Fatalf("web selection = %#v, %v", got, err)
	}
}

func TestLoadStaticInventoryFailsClosed(t *testing.T) {
	root := t.TempDir()
	bad := filepath.Join(root, "bad.yml")
	if err := os.WriteFile(bad, []byte("all:\n  plugin: dynamic\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.yml")
	if err := os.Symlink(bad, link); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"malformed": bad, "symlink": link, "missing": filepath.Join(root, "missing.ini"), "directory": root} {
		t.Run(name, func(t *testing.T) {
			inv, diags := loadStaticInventory([]string{path})
			if inv == nil || !inv.invalid || len(diags) == 0 || diags[0].Code != CodeInventoryInvalid || !diags[0].StrictFailure {
				t.Fatalf("inv=%#v diags=%#v", inv, diags)
			}
			if strings.Contains(diags[0].Message, root+string(filepath.Separator)) {
				t.Fatalf("diagnostic disclosed absolute inventory path: %q", diags[0].Message)
			}
		})
	}
}

func TestApplyInventoryTargetFailureSuppressesPlay(t *testing.T) {
	pb := &Playbook{Plays: []*Play{{Name: "remote", Hosts: "missing"}}}
	inv, diags := loadStaticInventory([]string{filepath.Join("testdata", "inventory.ini")})
	if len(diags) != 0 {
		t.Fatalf("load diagnostics: %#v", diags)
	}
	diags = applyInventoryTargets(pb, inv, true)
	if len(diags) != 1 || diags[0].Code != CodeInventoryPattern || !pb.Plays[0].InventoryFailed {
		t.Fatalf("play=%#v diags=%#v", pb.Plays[0], diags)
	}
}

func TestStaticInventoryRejectsEmptySelections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.yml")
	if err := os.WriteFile(path, []byte("all:\n  children:\n    empty: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inv, diags := loadStaticInventory([]string{path})
	if len(diags) != 0 || inv == nil || inv.invalid {
		t.Fatalf("inventory=%#v diagnostics=%#v", inv, diags)
	}
	for _, pattern := range []string{"all", "empty"} {
		if _, err := inv.selectHosts(pattern); err == nil || !strings.Contains(err.Error(), "no hosts") {
			t.Fatalf("selectHosts(%q) error = %v, want empty-selection failure", pattern, err)
		}
	}
	pb := &Playbook{Plays: []*Play{{Name: "empty", Hosts: "all"}}}
	diags = applyInventoryTargets(pb, inv, true)
	if len(diags) != 1 || diags[0].Code != CodeInventoryPattern || !pb.Plays[0].InventoryFailed || pb.Plays[0].InventoryResolved {
		t.Fatalf("play=%#v diagnostics=%#v", pb.Plays[0], diags)
	}
}

func TestConvertRejectsEmptyAllSelection(t *testing.T) {
	root := t.TempDir()
	playbookPath := filepath.Join(root, "playbook.yml")
	inventoryPath := filepath.Join(root, "inventory.yml")
	if err := os.WriteFile(playbookPath, []byte("- name: empty target\n  hosts: all\n  tasks:\n    - name: Must not become a no-op\n      ansible.builtin.file:\n        path: /tmp/never\n        state: directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inventoryPath, []byte("all: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Convert(context.Background(), Options{
		PlaybookPath:   playbookPath,
		InventoryPaths: []string{inventoryPath},
		Argspecs: []ArgspecInput{
			{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
		},
		OutDir: filepath.Join(root, "out"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StrictFailures == 0 || result.UWSPath != "" || result.HCLPath != "" || !hasDiagnostic(result.Diagnostics, CodeInventoryPattern, "empty target", "selects no hosts") {
		t.Fatalf("result=%#v", result)
	}
}

func TestLoadStaticInventoryRejectsGroupCycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cycle.yml")
	data := []byte("all:\n  children:\n    one:\n      children:\n        two:\n          children:\n            one: {}\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	inv, diags := loadStaticInventory([]string{path})
	if inv == nil || !inv.invalid || len(diags) != 1 || !strings.Contains(diags[0].Message, "cycle") {
		t.Fatalf("inv=%#v diags=%#v", inv, diags)
	}
}

func TestInventoryVarsRejectEqualGroupPrecedenceConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conflict.yml")
	data := []byte(`all:
  children:
    blue:
      vars: {tier: blue}
      hosts: {node-1: {}}
    green:
      vars: {tier: green}
      hosts: {node-1: {}}
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	inv, diags := loadStaticInventory([]string{path})
	if len(diags) != 0 || inv.invalid {
		t.Fatalf("inventory structure should load before scope resolution: %#v", diags)
	}
	if _, _, err := inv.varsForHost("node-1"); err == nil || !strings.Contains(err.Error(), "equal precedence") {
		t.Fatalf("varsForHost conflict = %v", err)
	}
}

func TestLoadStaticInventoryRejectsCredentialShapedVariables(t *testing.T) {
	for name, body := range map[string]string{
		"group":  "all:\n  vars:\n    db_password: group-do-not-disclose\n  hosts:\n    node-1: {}\n",
		"host":   "all:\n  hosts:\n    node-1:\n      api_token: host-do-not-disclose\n",
		"nested": "all:\n  vars:\n    config:\n      accounts:\n        - private_key: nested-do-not-disclose\n  hosts:\n    node-1: {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "inventory.yml")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			inv, diags := loadStaticInventory([]string{path})
			if inv == nil || !inv.invalid || len(diags) != 1 || diags[0].Code != CodeInventoryInvalid {
				t.Fatalf("inventory=%#v diagnostics=%#v", inv, diags)
			}
			for _, literal := range []string{"group-do-not-disclose", "host-do-not-disclose", "nested-do-not-disclose"} {
				if strings.Contains(diags[0].Message, literal) {
					t.Fatalf("diagnostic disclosed rejected literal %q: %s", literal, diags[0].Message)
				}
			}
		})
	}
}

func TestLoadStaticInventoryKeepsConnectionCredentialsRuntimeOwned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.yml")
	data := []byte("all:\n  hosts:\n    node-1:\n      ansible_password: runtime-owned\n      tier: frontend\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	inv, diags := loadStaticInventory([]string{path})
	if len(diags) != 0 || inv == nil || inv.invalid {
		t.Fatalf("inventory=%#v diagnostics=%#v", inv, diags)
	}
	vars, runtimeNames, err := inv.varsForHost("node-1")
	if err != nil {
		t.Fatal(err)
	}
	if vars["tier"] != "frontend" || vars["ansible_password"] != nil || !runtimeNames["ansible_password"] {
		t.Fatalf("vars=%#v runtimeNames=%#v", vars, runtimeNames)
	}
}

func TestInventoryCredentialLiteralsNeverEnterPartialArtifacts(t *testing.T) {
	root := t.TempDir()
	playbookPath := filepath.Join(root, "playbook.yml")
	inventoryPath := filepath.Join(root, "inventory.yml")
	playbook := `- name: local safe work
  hosts: localhost
  tasks:
    - name: Keep local task
      ansible.builtin.file:
        path: /tmp/safe
        state: directory
- name: remote rejected work
  hosts: all
  tasks:
    - name: Reject remote task
      ansible.builtin.file:
        path: /tmp/remote
        state: directory
`
	inventory := `all:
  vars:
    config:
      password: inventory-do-not-disclose
  hosts:
    node-1: {}
`
	if err := os.WriteFile(playbookPath, []byte(playbook), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inventoryPath, []byte(inventory), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Convert(context.Background(), Options{
		PlaybookPath: playbookPath, InventoryPaths: []string{inventoryPath},
		Argspecs: []ArgspecInput{{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")}},
		Mode:     "partial", OutDir: filepath.Join(root, "out"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StrictFailures == 0 || result.UWSPath == "" || result.HCLPath == "" {
		t.Fatalf("result=%#v", result)
	}
	for _, path := range []string{result.UWSPath, result.HCLPath, result.ReviewMD, result.ManifestPath, result.DiagnosticsJSON, result.DiagnosticsMD} {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(data), "inventory-do-not-disclose") {
			t.Fatalf("artifact %s disclosed rejected inventory credential:\n%s", path, data)
		}
	}
}

func TestStaticInventoryPrecomputesSharedGroupMembershipAtBounds(t *testing.T) {
	inv := newStaticInventory()
	leaf := inv.group("leaf")
	for hostIndex := 0; hostIndex < maxInventoryHosts; hostIndex++ {
		host := fmt.Sprintf("node-%04d", hostIndex)
		inv.hosts[host] = map[string]any{}
		leaf.hosts[host] = true
	}
	for groupIndex := 1; groupIndex < maxInventoryGroups; groupIndex++ {
		name := fmt.Sprintf("parent-%04d", groupIndex)
		group := inv.group(name)
		group.children["leaf"] = true
		group.vars["tier"] = "shared"
	}
	if err := inv.precomputeGroupHosts(); err != nil {
		t.Fatal(err)
	}
	if len(inv.groupHostBits) != maxInventoryGroups || len(inv.sortedHosts) != maxInventoryHosts {
		t.Fatalf("cached groups/hosts = %d/%d", len(inv.groupHostBits), len(inv.sortedHosts))
	}
	hosts, err := inv.selectHosts("parent-0001")
	if err != nil || len(hosts) != maxInventoryHosts || hosts[0] != "node-0000" || hosts[len(hosts)-1] != "node-4095" {
		t.Fatalf("selection len=%d first/last=%q/%q error=%v", len(hosts), hosts[0], hosts[len(hosts)-1], err)
	}
	for _, host := range inv.sortedHosts {
		vars, _, err := inv.varsForHost(host)
		if err != nil || vars["tier"] != "shared" {
			t.Fatalf("varsForHost(%q) = %#v, %v", host, vars, err)
		}
	}
}
