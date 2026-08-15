package ansibleconvert

import "testing"

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
