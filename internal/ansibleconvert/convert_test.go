package ansibleconvert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func runConvert(t *testing.T, fixture string) (*Result, *uws1.Document) {
	t.Helper()
	outDir := t.TempDir()
	result, err := Convert(context.Background(), Options{
		PlaybookPath: filepath.Join("testdata", fixture, "playbook.yml"),
		Argspecs: []ArgspecInput{
			{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
		},
		OutDir: outDir,
	})
	if err != nil {
		t.Fatalf("Convert(%s) failed: %v", fixture, err)
	}
	data, err := os.ReadFile(result.UWSPath)
	if err != nil {
		t.Fatalf("read emitted UWS document: %v", err)
	}
	var doc uws1.Document
	if err := convert.UnmarshalYAML(data, &doc); err != nil {
		t.Fatalf("parse emitted UWS document: %v", err)
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("emitted UWS document does not validate: %v", err)
	}
	return result, &doc
}

func findStep(steps []*uws1.Step, stepID string) *uws1.Step {
	for _, step := range steps {
		if step.StepID == stepID {
			return step
		}
	}
	return nil
}

func findOperation(doc *uws1.Document, opID string) *uws1.Operation {
	for _, op := range doc.Operations {
		if op.OperationID == opID {
			return op
		}
	}
	return nil
}

func TestConvertNginxPlaybook(t *testing.T) {
	result, doc := runConvert(t, "nginx")
	if result.StrictFailures != 0 {
		t.Fatalf("expected no strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
	if doc.UWS != "1.6.0" {
		t.Fatalf("uws = %q, want 1.6.0", doc.UWS)
	}
	if len(doc.SourceDescriptions) != 1 || doc.SourceDescriptions[0].Type != uws1.SourceDescriptionTypeAnsibleModule {
		t.Fatalf("sourceDescriptions = %#v", doc.SourceDescriptions)
	}
	if doc.SourceDescriptions[0].Name != "builtin" {
		t.Fatalf("source name = %q, want builtin", doc.SourceDescriptions[0].Name)
	}

	install := findOperation(doc, "install_nginx")
	if install == nil || install.SourceOperationID != "ansible.builtin.apt" {
		t.Fatalf("install_nginx operation missing or wrong selector: %#v", install)
	}
	body, _ := install.Request["body"].(map[string]any)
	if body["name"] != "$variables.pkg" {
		t.Fatalf("apt name = %v, want $variables.pkg", body["name"])
	}
	if install.Outputs["changed"] != "$response.body.changed" {
		t.Fatalf("install outputs = %#v", install.Outputs)
	}
	if doc.Components == nil || doc.Components.Variables["pkg"] != "nginx" {
		t.Fatalf("components.variables missing pkg: %#v", doc.Components)
	}

	steps := doc.Workflows[0].Steps
	restart := findStep(steps, "restart_nginx")
	if restart == nil {
		t.Fatalf("handler step missing; steps = %#v", steps)
	}
	wantWhen := "$steps.deploy_nginx_config.outputs.changed == true"
	if restart.When != wantWhen {
		t.Fatalf("handler when = %q, want %q", restart.When, wantWhen)
	}
	if restart.OperationRef != "restart_nginx" {
		t.Fatalf("handler operationRef = %q", restart.OperationRef)
	}
}

func TestConvertMultiNotifyLowersSwitch(t *testing.T) {
	result, doc := runConvert(t, "multinotify")
	if result.StrictFailures != 0 {
		t.Fatalf("expected no strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
	steps := doc.Workflows[0].Steps
	var switchStep *uws1.Step
	for _, step := range steps {
		if step.Type == uws1.WorkflowTypeSwitch {
			switchStep = step
			break
		}
	}
	if switchStep == nil {
		t.Fatalf("no switch step lowered for multi-notify handler; steps = %#v", steps)
	}
	if len(switchStep.Cases) != 2 {
		t.Fatalf("switch cases = %d, want 2", len(switchStep.Cases))
	}
	for _, c := range switchStep.Cases {
		if len(c.Steps) != 1 || c.Steps[0].OperationRef != "restart_nginx" {
			t.Fatalf("switch case does not reference the handler operation: %#v", c)
		}
	}
	if switchStep.Cases[0].When == switchStep.Cases[1].When {
		t.Fatalf("switch cases gate on the same notifier: %q", switchStep.Cases[0].When)
	}
}

func TestConvertLoopLowersForEach(t *testing.T) {
	result, doc := runConvert(t, "loop")
	if result.StrictFailures != 0 {
		t.Fatalf("expected no strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %#v", result.Diagnostics)
	}
	diagnosticsJSON, err := os.ReadFile(result.DiagnosticsJSON)
	if err != nil {
		t.Fatalf("read diagnostics JSON: %v", err)
	}
	if !strings.Contains(string(diagnosticsJSON), `"diagnostics": []`) {
		t.Fatalf("clean diagnostics should serialize as an empty array, got:\n%s", diagnosticsJSON)
	}
	step := findStep(doc.Workflows[0].Steps, "create_app_directories")
	if step == nil {
		t.Fatalf("loop step missing")
	}
	if step.ForEach != "$variables.create_app_directories_items" {
		t.Fatalf("forEach = %q", step.ForEach)
	}
	items, ok := doc.Components.Variables["create_app_directories_items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("hoisted loop items = %#v", doc.Components.Variables)
	}
	op := findOperation(doc, "create_app_directories")
	body, _ := op.Request["body"].(map[string]any)
	if body["path"] != "$item" {
		t.Fatalf("loop body path = %v, want $item", body["path"])
	}
}

func TestConvertTier3EmitsStrictDiagnostics(t *testing.T) {
	result, doc := runConvert(t, "tier3")
	if result.StrictFailures < 5 {
		t.Fatalf("expected at least 5 strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
	wantCodes := map[string]bool{
		CodeJinjaUnsupported:    false,
		CodeDynamicInclude:      false,
		CodeModuleUnknown:       false,
		CodeDelegateUnsupported: false,
	}
	for _, d := range result.Diagnostics {
		if _, tracked := wantCodes[d.Code]; tracked && d.StrictFailure {
			wantCodes[d.Code] = true
		}
	}
	for code, seen := range wantCodes {
		if !seen {
			t.Fatalf("missing strict diagnostic %s: %#v", code, result.Diagnostics)
		}
	}
	// The document is still emitted (review-first) with TODO placeholders.
	op := findOperation(doc, "use_a_filter")
	if op == nil {
		t.Fatalf("shell operation missing")
	}
	body, _ := op.Request["body"].(map[string]any)
	if s, _ := body["cmd"].(string); s == "" || s[:8] != "UWS-TODO" {
		t.Fatalf("filtered arg should be a TODO placeholder, got %v", body["cmd"])
	}
	// Fail-closed: a task whose guard cannot be lowered must NOT become an
	// unconditional step, and a delegated task must NOT lower at all.
	if op := findOperation(doc, "guarded_by_facts"); op != nil {
		t.Fatalf("guarded task lowered despite unsupported when: %#v", op)
	}
	if step := findStep(doc.Workflows[0].Steps, "guarded_by_facts"); step != nil {
		t.Fatalf("guarded step emitted despite unsupported when: %#v", step)
	}
	if op := findOperation(doc, "delegated_restart"); op != nil {
		t.Fatalf("delegated task lowered despite delegate_to: %#v", op)
	}
}

func TestConvertFailClosedGuardsAndTargets(t *testing.T) {
	result, doc := runConvert(t, "failclosed")

	// A delegate_to on a block must skip every task inside the block.
	if op := findOperation(doc, "inside_delegated_block"); op != nil {
		t.Fatalf("block delegate_to leaked a runnable child task: %#v", op)
	}
	// A block when plus a child when (AND in Ansible) must skip the child...
	if op := findOperation(doc, "doubly_guarded_child"); op != nil {
		t.Fatalf("block+task when combination leaked an under-guarded task: %#v", op)
	}
	// ...while a child relying only on the block guard inherits it.
	single := findStep(doc.Workflows[0].Steps, "singly_guarded_child")
	if single == nil || single.When != `$variables.env == "prod"` {
		t.Fatalf("singly guarded child should inherit the block guard, got %#v", single)
	}
	// A consumer of a skipped producer's register must fail closed (TODO), not
	// reference a nonexistent step.
	consumer := findOperation(doc, "consumer_of_skipped_producer")
	if consumer == nil {
		t.Fatalf("consumer task missing")
	}
	body, _ := consumer.Request["body"].(map[string]any)
	cmd, _ := body["cmd"].(string)
	if strings.Contains(cmd, "$steps.guarded_producer") {
		t.Fatalf("consumer references a skipped producer step: %q", cmd)
	}
	if !strings.HasPrefix(cmd, "UWS-TODO") {
		t.Fatalf("consumer arg should be a TODO placeholder, got %q", cmd)
	}
	// A consumer must not be allowed to read a register value produced later in
	// playbook order.
	futureConsumer := findOperation(doc, "consumer_before_future_producer")
	if futureConsumer == nil {
		t.Fatalf("future-register consumer task missing")
	}
	body, _ = futureConsumer.Request["body"].(map[string]any)
	cmd, _ = body["cmd"].(string)
	if strings.Contains(cmd, "$steps.future_producer") {
		t.Fatalf("consumer references a future producer step: %q", cmd)
	}
	if !strings.HasPrefix(cmd, "UWS-TODO") {
		t.Fatalf("future-register consumer arg should be a TODO placeholder, got %q", cmd)
	}
	// A rescue block must not lower only the happy-path body while omitting
	// recovery semantics.
	if op := findOperation(doc, "main_inside_rescue_block"); op != nil {
		t.Fatalf("block with rescue leaked its body without rescue semantics: %#v", op)
	}
	// A handler with its own when must not lower with its guard dropped.
	if op := findOperation(doc, "guarded_restart"); op != nil {
		t.Fatalf("handler with its own when leaked without the guard: %#v", op)
	}
	var handlerDiag bool
	for _, d := range result.Diagnostics {
		if d.Code == CodeJinjaUnsupported && d.Task == "guarded restart" && d.StrictFailure {
			handlerDiag = true
		}
	}
	if !handlerDiag {
		t.Fatalf("missing handler-when diagnostic: %#v", result.Diagnostics)
	}
	if result.StrictFailures < 7 {
		t.Fatalf("expected at least 7 strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
}

func TestConvertPlayLevelSectionsAreStrictDiagnostics(t *testing.T) {
	result, doc := runConvert(t, "playsections")

	if findOperation(doc, "main_task") == nil {
		t.Fatalf("main task should still be present for review")
	}
	if findOperation(doc, "pre_task_omitted") != nil || findOperation(doc, "post_task_omitted") != nil {
		t.Fatalf("unsupported play-level sections should not lower silently: %#v", doc.Operations)
	}
	wantSections := map[string]bool{"pre_tasks": false, "post_tasks": false, "roles": false}
	for _, d := range result.Diagnostics {
		for section := range wantSections {
			if d.Code == CodePlaybookShape && d.StrictFailure && strings.Contains(d.Message, `"`+section+`"`) {
				wantSections[section] = true
			}
		}
	}
	for section, seen := range wantSections {
		if !seen {
			t.Fatalf("missing strict diagnostic for %s: %#v", section, result.Diagnostics)
		}
	}
	if result.StrictFailures < len(wantSections) {
		t.Fatalf("strict failures = %d, want at least %d: %#v", result.StrictFailures, len(wantSections), result.Diagnostics)
	}
}

func TestConvertAllSkippedWritesDiagnosticsWithoutDocument(t *testing.T) {
	outDir := t.TempDir()
	result, err := Convert(context.Background(), Options{
		PlaybookPath: filepath.Join("testdata", "allskipped", "playbook.yml"),
		Argspecs: []ArgspecInput{
			{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
		},
		OutDir: outDir,
	})
	if err != nil {
		t.Fatalf("Convert should not fail when no tasks lower: %v", err)
	}
	if result.UWSPath != "" {
		t.Fatalf("no UWS document should be written, got %q", result.UWSPath)
	}
	if _, err := os.Stat(result.DiagnosticsJSON); err != nil {
		t.Fatalf("diagnostics must be written even when no document is emitted: %v", err)
	}
	if result.StrictFailures < 2 {
		t.Fatalf("expected module_unknown plus no-tasks-lowered strict diagnostics, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
	var sawNoTasks bool
	for _, d := range result.Diagnostics {
		if d.Code == CodePlaybookShape && d.StrictFailure {
			sawNoTasks = true
		}
	}
	if !sawNoTasks {
		t.Fatalf("missing no-tasks-lowered diagnostic: %#v", result.Diagnostics)
	}
}
