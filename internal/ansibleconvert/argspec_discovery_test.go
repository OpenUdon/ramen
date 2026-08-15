package ansibleconvert

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverArgspecsUsesValidatedCollectionIDs(t *testing.T) {
	root := t.TempDir()
	data, err := os.ReadFile(filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "nested", "builtin.argspec.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.json"), []byte("not an argspec"), 0o600); err != nil {
		t.Fatal(err)
	}
	inputs, err := DiscoverArgspecs([]string{root})
	if err != nil {
		t.Fatalf("DiscoverArgspecs: %v", err)
	}
	if len(inputs) != 1 || inputs[0].ID != "ansible.builtin" || inputs[0].Path != path {
		t.Fatalf("inputs = %#v", inputs)
	}
	idx, err := LoadArgspecs(inputs)
	if err != nil {
		t.Fatalf("LoadArgspecs(discovered): %v", err)
	}
	if sourceID, _, ok := idx.Lookup("ansible.builtin.shell"); !ok || sourceID != "ansible.builtin" {
		t.Fatalf("lookup = %q, %v", sourceID, ok)
	}
}

func TestDiscoverArgspecsRejectsSymlinksEmptyAndDuplicates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Run("symlink entry", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "target.argspec.json")
		if err := os.WriteFile(target, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(root, "linked.argspec.json")); err != nil {
			t.Fatal(err)
		}
		if _, err := DiscoverArgspecs([]string{root}); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if _, err := DiscoverArgspecs([]string{t.TempDir()}); err == nil || !strings.Contains(err.Error(), "no *.argspec.json") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("duplicate collection", func(t *testing.T) {
		root := t.TempDir()
		for _, name := range []string{"a.argspec.json", "b.argspec.json"} {
			if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		inputs, err := DiscoverArgspecs([]string{root})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := LoadArgspecs(inputs); err == nil || !strings.Contains(err.Error(), "duplicate source ID") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestLoadArgspecsRejectsSymlinkInput(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.json")
	data, err := os.ReadFile(filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadArgspecs([]ArgspecInput{{ID: "builtin", Path: link}}); err == nil || !strings.Contains(err.Error(), "symlinks") {
		t.Fatalf("error = %v", err)
	}
}
