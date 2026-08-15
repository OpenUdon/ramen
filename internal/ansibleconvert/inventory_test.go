package ansibleconvert

import (
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
