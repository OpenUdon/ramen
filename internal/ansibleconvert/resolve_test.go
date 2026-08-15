package ansibleconvert

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeVarsPreservesEqualLayerPriorityAndSource(t *testing.T) {
	for name, priority := range map[string]int{
		"role_defaults": varPriorityRoleDefault,
		"vars_files":    varPriorityVarsFile,
		"role_vars":     varPriorityRoleVars,
	} {
		t.Run(name, func(t *testing.T) {
			play := &Play{Name: "precedence", Vars: map[string]any{}}
			resolver := &staticResolver{}
			resolver.mergeVars(play, map[string]any{"setting": "first"}, priority, "first source", play.Name)
			resolver.mergeVars(play, map[string]any{"setting": "second"}, priority, "second source", play.Name)

			if got := play.Vars["setting"]; got != "first" {
				t.Fatalf("setting = %#v, want first equal-layer value retained", got)
			}
			if got := play.VarPriorities["setting"]; got != priority {
				t.Fatalf("priority = %d, want %d", got, priority)
			}
			if got := play.VarSources["setting"]; got != "first source" {
				t.Fatalf("source = %q, want first source", got)
			}
			if !play.StaticScopeFailed || len(resolver.diags) != 1 || resolver.diags[0].Code != CodeVariableConflict {
				t.Fatalf("play=%#v diagnostics=%#v", play, resolver.diags)
			}
		})
	}
}

func TestMergeVarsKeepsDocumentedCrossLayerPrecedence(t *testing.T) {
	play := &Play{Name: "precedence", Vars: map[string]any{"setting": "play"}}
	resolver := &staticResolver{}
	resolver.initializePlayVars(play)
	resolver.mergeVars(play, map[string]any{"setting": "file"}, varPriorityVarsFile, "vars file", play.Name)
	resolver.mergeVars(play, map[string]any{"setting": "role"}, varPriorityRoleVars, "role vars", play.Name)

	if got := play.Vars["setting"]; got != "role" {
		t.Fatalf("setting = %#v, want higher-priority role value", got)
	}
	if got := play.VarPriorities["setting"]; got != varPriorityRoleVars {
		t.Fatalf("priority = %d, want %d", got, varPriorityRoleVars)
	}
	if len(resolver.diags) != 0 || play.StaticScopeFailed {
		t.Fatalf("play=%#v diagnostics=%#v", play, resolver.diags)
	}
}

func TestExpandPrivateIncludeRoleRejectsDefaultsAndVars(t *testing.T) {
	for _, variableDir := range []string{"defaults", "vars"} {
		t.Run(variableDir, func(t *testing.T) {
			root := t.TempDir()
			roleRoot := filepath.Join(root, "roles", "private")
			if err := os.MkdirAll(filepath.Join(roleRoot, variableDir), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(roleRoot, "tasks"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(roleRoot, variableDir, "main.yml"), []byte("scoped_value: role-private\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(roleRoot, "tasks", "main.yml"), []byte("- name: private task\n  ansible.builtin.file:\n    path: /tmp/private\n    state: directory\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			play := &Play{Name: "private include", Vars: map[string]any{"play_value": true}}
			resolver := &staticResolver{opts: Options{ProjectDir: root}}
			tasks := resolver.expandRole(play, "private", filepath.Join(root, "playbook.yml"), 1, 1, nil, "tasks", true)
			if len(tasks) != 0 || len(resolver.diags) != 1 || resolver.diags[0].Code != CodeStaticResolution || play.StaticScopeFailed || play.Vars["scoped_value"] != nil {
				t.Fatalf("tasks=%#v play=%#v diagnostics=%#v", tasks, play, resolver.diags)
			}
		})
	}
}
