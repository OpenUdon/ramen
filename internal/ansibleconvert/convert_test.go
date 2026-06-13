package ansibleconvert

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func runConvert(t *testing.T, fixture string) (*Result, *uws1.Document) {
	t.Helper()
	return runConvertWithOptions(t, fixture, Options{})
}

func runConvertWithOptions(t *testing.T, fixture string, opts Options) (*Result, *uws1.Document) {
	t.Helper()
	outDir := t.TempDir()
	opts.PlaybookPath = filepath.Join("testdata", fixture, "playbook.yml")
	opts.Argspecs = []ArgspecInput{
		{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
	}
	opts.OutDir = outDir
	opts.IgnoreUnsupported = true
	result, err := Convert(context.Background(), Options{
		PlaybookPath:      opts.PlaybookPath,
		Argspecs:          opts.Argspecs,
		OutDir:            opts.OutDir,
		Strict:            opts.Strict,
		ProjectDir:        opts.ProjectDir,
		RolesPaths:        opts.RolesPaths,
		CollectionsPaths:  opts.CollectionsPaths,
		InventoryPaths:    opts.InventoryPaths,
		ExtraVars:         opts.ExtraVars,
		IgnoreUnsupported: opts.IgnoreUnsupported,
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

func TestConvertUnsupportedBlocksWorkflowArtifactsByDefault(t *testing.T) {
	outDir := t.TempDir()
	result, err := Convert(context.Background(), Options{
		PlaybookPath: filepath.Join("testdata", "tier3", "playbook.yml"),
		Argspecs: []ArgspecInput{
			{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
		},
		OutDir: outDir,
	})
	if err != nil {
		t.Fatalf("Convert(tier3) failed: %v", err)
	}
	if result.StrictFailures == 0 {
		t.Fatalf("expected strict failures: %#v", result.Diagnostics)
	}
	if result.UWSPath != "" || result.HCLPath != "" {
		t.Fatalf("unsupported conversion wrote workflow artifacts: %#v", result)
	}
	if _, err := os.Stat(result.DiagnosticsJSON); err != nil {
		t.Fatalf("diagnostics JSON not written: %v", err)
	}
	review, err := os.ReadFile(result.ReviewMD)
	if err != nil {
		t.Fatalf("review not written: %v", err)
	}
	if !strings.Contains(string(review), "workflow artifacts were not written") {
		t.Fatalf("review missing fail gate:\n%s", review)
	}
}

func findStep(steps []*uws1.Step, stepID string) *uws1.Step {
	for _, step := range steps {
		if step == nil {
			continue
		}
		if step.StepID == stepID {
			return step
		}
		if found := findStep(step.Steps, stepID); found != nil {
			return found
		}
		for _, c := range step.Cases {
			if c != nil {
				if found := findStep(c.Steps, stepID); found != nil {
					return found
				}
			}
		}
		if found := findStep(step.Default, stepID); found != nil {
			return found
		}
	}
	return nil
}

func findTopStep(steps []*uws1.Step, stepID string) *uws1.Step {
	for _, step := range steps {
		if step != nil && step.StepID == stepID {
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

func TestConvertInventoryHostFanOutLowersInputsAndForEach(t *testing.T) {
	outDir := t.TempDir()
	result, err := Convert(context.Background(), Options{
		PlaybookPath: filepath.Join("testdata", "hostfanout", "playbook.yml"),
		Argspecs: []ArgspecInput{
			{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
		},
		InventoryPaths: []string{filepath.Join("testdata", "inventory.ini")},
		OutDir:         outDir,
	})
	if err != nil {
		t.Fatalf("Convert(hostfanout) failed: %v", err)
	}
	if result.StrictFailures != 0 {
		t.Fatalf("expected no strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
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
	workflow := doc.Workflows[0]
	if workflow.Inputs == nil || workflow.Inputs.Properties["hosts"].Type != "array" || workflow.Inputs.Properties["hosts"].Items.Type != "string" {
		t.Fatalf("workflow inputs = %#v, want hosts string array", workflow.Inputs)
	}
	step := findStep(workflow.Steps, "ensure_nginx_is_present")
	if step == nil {
		t.Fatalf("host fan-out step missing: %#v", workflow.Steps)
	}
	if step.ForEach != "$inputs.hosts" {
		t.Fatalf("forEach = %q, want $inputs.hosts", step.ForEach)
	}
	if step.Inputs["host"] != "$item" {
		t.Fatalf("step inputs = %#v, want host bound to $item", step.Inputs)
	}
}

func TestConvertInventoryHostFanOutWithHandlerFailsClosed(t *testing.T) {
	result, doc := runConvertWithOptions(t, "nginx", Options{
		InventoryPaths: []string{filepath.Join("testdata", "inventory.ini")},
	})

	if step := findStep(doc.Workflows[0].Steps, "install_nginx"); step == nil || step.ForEach != "$inputs.hosts" {
		t.Fatalf("install step did not fan out over hosts: %#v", step)
	}
	if step := findStep(doc.Workflows[0].Steps, "restart_nginx"); step != nil {
		t.Fatalf("host fan-out handler should fail closed, got %#v", step)
	}
	var sawHandlerDiagnostic bool
	for _, d := range result.Diagnostics {
		if d.Code == CodePlaybookShape && d.Task == "restart nginx" && d.StrictFailure && strings.Contains(d.Message, "per-host changed") {
			sawHandlerDiagnostic = true
		}
	}
	if !sawHandlerDiagnostic {
		t.Fatalf("missing host fan-out handler diagnostic: %#v", result.Diagnostics)
	}
}

func TestConvertSanitizesDottedArgspecSourceID(t *testing.T) {
	// The natural argspec ID is the collection FQCN (e.g. "ansible.builtin"),
	// but UWS sourceDescription names forbid dots. Conversion must sanitize the
	// emitted name rather than fail UWS validation with an internal error.
	result, err := Convert(context.Background(), Options{
		PlaybookPath: filepath.Join("testdata", "nginx", "playbook.yml"),
		Argspecs: []ArgspecInput{
			{ID: "ansible.builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
		},
		OutDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Convert with dotted argspec ID failed: %v", err)
	}
	if result.UWSPath == "" {
		t.Fatalf("expected a UWS document to be written: %#v", result)
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
	if len(doc.SourceDescriptions) != 1 || doc.SourceDescriptions[0].Name != "ansible_builtin" {
		t.Fatalf("source name = %#v, want ansible_builtin", doc.SourceDescriptions)
	}
	for _, op := range doc.Operations {
		if op.SourceDescription != "ansible_builtin" {
			t.Fatalf("operation %q sourceDescription = %q, want ansible_builtin", op.OperationID, op.SourceDescription)
		}
		if op.SourceOperationID != "" && !strings.HasPrefix(op.SourceOperationID, "ansible.builtin.") {
			t.Fatalf("operation %q sourceOperationId = %q, want module FQCN preserved", op.OperationID, op.SourceOperationID)
		}
	}
}

func TestLoadArgspecsRejectsSanitizedSourceNameCollision(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.json")
	second := filepath.Join(root, "second.json")
	if err := os.WriteFile(first, []byte(`{"argspec":"uws.ansible.1.0","collection":"acme.one","modules":{"acme.one.first":{"parameters":{}}}}`), 0o644); err != nil {
		t.Fatalf("write first argspec: %v", err)
	}
	if err := os.WriteFile(second, []byte(`{"argspec":"uws.ansible.1.0","collection":"acme.two","modules":{"acme.two.second":{"parameters":{}}}}`), 0o644); err != nil {
		t.Fatalf("write second argspec: %v", err)
	}
	_, err := LoadArgspecs([]ArgspecInput{
		{ID: "acme.one", Path: first},
		{ID: "acme-one", Path: second},
	})
	if err == nil || !strings.Contains(err.Error(), "sanitized source name") {
		t.Fatalf("LoadArgspecs error = %v, want sanitized source collision", err)
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
		if c.Steps[0].Inputs["service_name"] != "nginx" {
			t.Fatalf("multi-notify handler step lost task-local vars: %#v", c.Steps[0])
		}
	}
	if switchStep.Cases[0].When == switchStep.Cases[1].When {
		t.Fatalf("switch cases gate on the same notifier: %q", switchStep.Cases[0].When)
	}
	op := findOperation(doc, "restart_nginx")
	body, _ := op.Request["body"].(map[string]any)
	if body["name"] != "$inputs.service_name" {
		t.Fatalf("handler operation did not lower task-local var reference: %#v", body)
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
	review, err := os.ReadFile(result.ReviewMD)
	if err != nil {
		t.Fatalf("read review markdown: %v", err)
	}
	for _, want := range []string{"# Ansible Conversion Review", "## Lowered Operations", "`create_app_directories`", "Status: `pass`"} {
		if !strings.Contains(string(review), want) {
			t.Fatalf("review missing %q:\n%s", want, review)
		}
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
	// A block when plus a child when (AND in Ansible) lowers through nested
	// UWS 1.6 switches, keeping the original task step ID inside the wrapper.
	doubleWrapper := findTopStep(doc.Workflows[0].Steps, "doubly_guarded_child_guard_1")
	if doubleWrapper == nil || doubleWrapper.Type != uws1.WorkflowTypeSwitch || len(doubleWrapper.Cases) != 1 {
		t.Fatalf("doubly guarded child should use a switch wrapper, got %#v", doubleWrapper)
	}
	if doubleWrapper.Cases[0].When != `$variables.env == "prod"` {
		t.Fatalf("outer guard = %q", doubleWrapper.Cases[0].When)
	}
	double := findStep(doc.Workflows[0].Steps, "doubly_guarded_child")
	if double == nil || double.OperationRef != "doubly_guarded_child" || double.When != "" {
		t.Fatalf("doubly guarded child original step should stay inside wrapper, got %#v", double)
	}
	// A child relying only on the block guard inherits it.
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
	handlerWrapper := findTopStep(doc.Workflows[0].Steps, "guarded_restart_guard_1")
	if handlerWrapper == nil || handlerWrapper.Type != uws1.WorkflowTypeSwitch || len(handlerWrapper.Cases) != 1 {
		t.Fatalf("handler with its own when should use switch guard wrapper, got %#v", handlerWrapper)
	}
	if handlerWrapper.Cases[0].When != "$steps.consumer_of_skipped_producer.outputs.changed == true" {
		t.Fatalf("handler notifier guard = %q", handlerWrapper.Cases[0].When)
	}
	handler := findStep(doc.Workflows[0].Steps, "guarded_restart")
	if handler == nil || handler.OperationRef != "guarded_restart" || handler.When != "" {
		t.Fatalf("handler original step should stay inside guard wrapper, got %#v", handler)
	}
	if result.StrictFailures < 5 {
		t.Fatalf("expected at least 5 strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
}

func TestConvertPlayLevelPreAndPostTasksLowerWithRolesDiagnostic(t *testing.T) {
	result, doc := runConvert(t, "playsections")

	if findOperation(doc, "main_task") == nil {
		t.Fatalf("main task should still be present for review")
	}
	for _, id := range []string{"pre_task_omitted", "main_task", "post_task_omitted"} {
		if findOperation(doc, id) == nil {
			t.Fatalf("%s should be present for review: %#v", id, doc.Operations)
		}
	}
	var gotOrder []string
	for _, step := range doc.Workflows[0].Steps {
		gotOrder = append(gotOrder, step.OperationRef)
	}
	wantOrder := []string{"pre_task_omitted", "main_task", "post_task_omitted"}
	if !slices.Equal(gotOrder, wantOrder) {
		t.Fatalf("step order = %v, want %v", gotOrder, wantOrder)
	}
	var sawMissingRole bool
	for _, d := range result.Diagnostics {
		if d.Code == CodeStaticResolution && d.StrictFailure && strings.Contains(d.Message, `role "nginx" was not found`) {
			sawMissingRole = true
		}
	}
	if !sawMissingRole {
		t.Fatalf("missing strict diagnostic for unresolved role: %#v", result.Diagnostics)
	}
	if result.StrictFailures < 1 {
		t.Fatalf("strict failures = %d, want at least 1: %#v", result.StrictFailures, result.Diagnostics)
	}
}

func TestConvertStaticImportTasksLowersInOrderWithProvenance(t *testing.T) {
	result, doc := runConvert(t, "imports")
	if result.StrictFailures != 0 {
		t.Fatalf("expected no strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
	var got []string
	for _, step := range doc.Workflows[0].Steps {
		got = append(got, step.OperationRef)
	}
	want := []string{"first_imported", "nested_imported", "final_task"}
	if !slices.Equal(got, want) {
		t.Fatalf("step order = %v, want %v", got, want)
	}
	for _, id := range []string{"first_imported", "nested_imported"} {
		step := findStep(doc.Workflows[0].Steps, id)
		if step == nil || step.When != `$variables.env == "prod"` {
			t.Fatalf("imported task %q did not inherit import when guard: %#v", id, step)
		}
		op := findOperation(doc, id)
		prov, _ := op.Extensions["x-ansible"].(map[string]any)
		if !slices.Contains(asStringSlice(prov["tags"]), "setup") {
			t.Fatalf("imported task %q did not inherit import tags: %#v", id, prov)
		}
	}
	op := findOperation(doc, "nested_imported")
	prov, _ := op.Extensions["x-ansible"].(map[string]any)
	if prov["sourceFile"] == "" || prov["line"] == nil || len(asStringSlice(prov["importStack"])) == 0 {
		t.Fatalf("nested provenance missing source/import stack: %#v", prov)
	}
	hclData, err := os.ReadFile(result.HCLPath)
	if err != nil {
		t.Fatalf("read HCL output: %v", err)
	}
	var hclDoc uws1.Document
	if err := convert.UnmarshalHCL(hclData, &hclDoc); err != nil {
		t.Fatalf("parse HCL output: %v", err)
	}
	hclOp := findOperation(&hclDoc, "nested_imported")
	if hclOp == nil || hclOp.Extensions["x-ansible"] == nil {
		t.Fatalf("HCL round-trip lost x-ansible provenance: %#v", hclOp)
	}
}

func TestConvertStaticImportTasksRejectsCyclesAndTemplates(t *testing.T) {
	for _, fixture := range []string{"importcycle", "importtemplated"} {
		t.Run(fixture, func(t *testing.T) {
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
			if result.StrictFailures == 0 {
				t.Fatalf("expected strict failures: %#v", result.Diagnostics)
			}
			var saw bool
			for _, d := range result.Diagnostics {
				if d.Code == CodeStaticResolution && d.StrictFailure {
					saw = true
				}
			}
			if !saw {
				t.Fatalf("missing static-resolution diagnostic: %#v", result.Diagnostics)
			}
		})
	}
}

func TestConvertStaticRolesVarsHandlersAndListenAliases(t *testing.T) {
	result, doc := runConvertWithOptions(t, "staticroles", Options{
		ProjectDir: filepath.Join("testdata", "staticroles"),
	})
	if result.StrictFailures != 0 {
		t.Fatalf("expected no strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
	for _, name := range []string{"app_pkg", "web_service", "web_enabled"} {
		if _, ok := doc.Components.Variables[name]; !ok {
			t.Fatalf("missing variable %q in %#v", name, doc.Components.Variables)
		}
	}
	var got []string
	for _, step := range doc.Workflows[0].Steps {
		got = append(got, step.OperationRef)
	}
	want := []string{"install_role_package", "deploy_role_config", "extra_role_task", "restart_role_service"}
	if !slices.Equal(got, want) {
		t.Fatalf("step order = %v, want %v", got, want)
	}
	restart := findStep(doc.Workflows[0].Steps, "restart_role_service")
	if restart == nil || restart.When != "$steps.deploy_role_config.outputs.changed == true" {
		t.Fatalf("handler listen alias did not gate on notifier: %#v", restart)
	}
	op := findOperation(doc, "install_role_package")
	prov, _ := op.Extensions["x-ansible"].(map[string]any)
	if prov["role"] != "web" {
		t.Fatalf("role provenance = %#v", prov)
	}
}

func TestConvertFQCNCollectionRoleResolution(t *testing.T) {
	result, doc := runConvertWithOptions(t, "collectionrole", Options{
		ProjectDir:        filepath.Join("testdata", "collectionrole"),
		CollectionsPaths:  []string{filepath.Join("testdata", "collectionrole", "collections")},
		IgnoreUnsupported: true,
	})
	if result.StrictFailures != 0 {
		t.Fatalf("expected no strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
	if findOperation(doc, "collection_role_task") == nil {
		t.Fatalf("collection role task was not lowered: %#v", doc.Operations)
	}
}

func TestConvertStaticVariableConflictFailsClosed(t *testing.T) {
	result, _ := runConvertWithOptions(t, "varconflict", Options{
		ProjectDir: filepath.Join("testdata", "varconflict"),
	})
	if result.StrictFailures == 0 {
		t.Fatalf("expected variable conflict strict failure: %#v", result.Diagnostics)
	}
	var saw bool
	for _, d := range result.Diagnostics {
		if d.Code == CodeVariableConflict && d.StrictFailure {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("missing variable conflict diagnostic: %#v", result.Diagnostics)
	}
}

func TestConvertSemanticDirectivesLowerWhenStaticButRuntimeMetadataIsInfo(t *testing.T) {
	result, doc := runConvert(t, "directives")
	if doc.UWS != "1.6.0" {
		t.Fatalf("semantic directives should stay on UWS 1.6.0, got %q", doc.UWS)
	}
	retryOp := findOperation(doc, "retry_command")
	if retryOp == nil || len(retryOp.SuccessCriteria) != 2 || retryOp.SuccessCriteria[1].Condition != "$response.body.rc == 0" {
		t.Fatalf("retry command did not lower until into successCriteria: %#v", retryOp)
	}
	if len(retryOp.OnFailure) != 1 || retryOp.OnFailure[0].Type != "retry" || retryOp.OnFailure[0].RetryLimit != 2 || retryOp.OnFailure[0].RetryAfter != 1 {
		t.Fatalf("retry command did not lower retries/delay into onFailure retry: %#v", retryOp.OnFailure)
	}
	varsStep := findStep(doc.Workflows[0].Steps, "task_local_vars")
	if varsStep == nil || varsStep.Inputs["local_value"] != "yes" {
		t.Fatalf("task-local vars did not lower to step inputs: %#v", varsStep)
	}
	if ignoreStep := findStep(doc.Workflows[0].Steps, "ignore_errors_command"); ignoreStep != nil {
		t.Fatalf("ignore_errors task should fail closed without UWS continue-on-error semantics: %#v", ignoreStep)
	}
	fatalStep := findStep(doc.Workflows[0].Steps, "fatal_fanout_command")
	if fatalStep == nil {
		t.Fatalf("any_errors_fatal true should use default fail-fast behavior and still lower")
	}
	if findOperation(doc, "runtime_metadata_stays_informational") == nil {
		t.Fatalf("runtime-owned metadata task should still lower")
	}
	var sawInfo, sawIgnore bool
	for _, d := range result.Diagnostics {
		if d.Code == CodeDirectiveTodo && d.Severity == "info" && strings.Contains(d.Message, "runtime-owned") {
			sawInfo = true
		}
		if d.Code == CodeDirectiveTodo && d.Task == "Ignore errors command" && d.StrictFailure && strings.Contains(d.Message, "ignore_errors") {
			sawIgnore = true
		}
	}
	if result.StrictFailures != 1 || !sawInfo || !sawIgnore {
		t.Fatalf("directive diagnostics missing clean/info posture: %#v", result.Diagnostics)
	}
}

func TestConvertInvalidControlDirectivesFailClosed(t *testing.T) {
	result, doc := runConvertWithOptions(t, "directives_invalid", Options{
		IgnoreUnsupported: true,
	})
	wantTasks := map[string]string{
		"Templated retries":          "retries",
		"Invalid retries":            "retries",
		"Invalid delay":              "delay",
		"Negative delay":             "delay",
		"Templated throttle":         "throttle",
		"Invalid throttle":           "throttle",
		"Templated ignore errors":    "ignore_errors",
		"Invalid ignore errors":      "ignore_errors",
		"Templated any errors fatal": "any_errors_fatal",
		"Invalid any errors fatal":   "any_errors_fatal",
	}
	seen := map[string]bool{}
	for _, d := range result.Diagnostics {
		directive, tracked := wantTasks[d.Task]
		if !tracked {
			continue
		}
		if d.Code == CodeDirectiveTodo && d.StrictFailure && strings.Contains(d.Message, directive) && strings.Contains(d.Message, "task was not lowered") {
			seen[d.Task] = true
		}
	}
	for taskName := range wantTasks {
		if !seen[taskName] {
			t.Fatalf("missing strict directive diagnostic for %q: %#v", taskName, result.Diagnostics)
		}
	}
	if result.StrictFailures != len(wantTasks) {
		t.Fatalf("strict failures = %d, want %d: %#v", result.StrictFailures, len(wantTasks), result.Diagnostics)
	}
	if findOperation(doc, "valid_command") == nil {
		t.Fatalf("valid task was not lowered: %#v", doc.Operations)
	}
	for _, id := range []string{
		"templated_retries",
		"invalid_retries",
		"invalid_delay",
		"negative_delay",
		"templated_throttle",
		"invalid_throttle",
		"templated_ignore_errors",
		"invalid_ignore_errors",
		"templated_any_errors_fatal",
		"invalid_any_errors_fatal",
	} {
		if findOperation(doc, id) != nil {
			t.Fatalf("invalid directive task %q was lowered", id)
		}
	}
}

func TestConvertRetriesDelayWithoutUntilStaysWarning(t *testing.T) {
	result, doc := runConvert(t, "directives")
	if result.StrictFailures != 1 {
		t.Fatalf("expected only ignore_errors strict failure, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
	op := findOperation(doc, "retry_metadata_without_until")
	if op == nil {
		t.Fatalf("expected retries/delay without until task to lower")
	}
	if len(op.OnFailure) != 0 {
		t.Fatalf("retries/delay without until should not emit retry action: %#v", op.OnFailure)
	}
	var sawWarning bool
	for _, d := range result.Diagnostics {
		if d.Code == CodeDirectiveTodo && d.Task == "Retry metadata without until" && d.Severity == "warning" && !d.StrictFailure && strings.Contains(d.Message, "retries/delay without until") {
			sawWarning = true
		}
	}
	if !sawWarning {
		t.Fatalf("missing retries/delay without until warning: %#v", result.Diagnostics)
	}
}

func TestConvertThrottleGreaterThanOneFailsForInventoryHostFanOut(t *testing.T) {
	result, doc := runConvertWithOptions(t, "directives", Options{
		InventoryPaths: []string{filepath.Join("testdata", "inventory.ini")},
	})
	if result.StrictFailures < 2 {
		t.Fatalf("expected strict failures for ignore_errors and throttle, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
	step := findStep(doc.Workflows[0].Steps, "fatal_fanout_command")
	if step != nil {
		t.Fatalf("throttle > 1 with host fan-out should fail closed, got %#v", step)
	}
	var sawThrottle bool
	for _, d := range result.Diagnostics {
		if d.Code == CodeDirectiveTodo && d.Task == "Fatal fanout command" && d.StrictFailure && strings.Contains(d.Message, "throttle") {
			sawThrottle = true
		}
	}
	if !sawThrottle {
		t.Fatalf("missing throttle strict diagnostic: %#v", result.Diagnostics)
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
	review, err := os.ReadFile(result.ReviewMD)
	if err != nil {
		t.Fatalf("review must be written even when no document is emitted: %v", err)
	}
	if !strings.Contains(string(review), "No operations were lowered.") {
		t.Fatalf("all-skipped review missing no-operations summary:\n%s", review)
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

func TestAnsibleConversionCorpusDrift(t *testing.T) {
	corpusRoot := filepath.Join("..", "..", "testdata", "ansible-conversion")
	entries, err := os.ReadDir(corpusRoot)
	if err != nil {
		t.Fatalf("read Ansible conversion corpus: %v", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	wantNames := []string{"failclosed", "loop", "multinotify", "nginx"}
	if !slices.Equal(names, wantNames) {
		t.Fatalf("corpus entries = %v, want %v", names, wantNames)
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			outDir := t.TempDir()
			result, err := Convert(context.Background(), Options{
				PlaybookPath: filepath.Join(corpusRoot, name, "input", "playbook.yml"),
				Argspecs: []ArgspecInput{
					{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
				},
				OutDir:            outDir,
				IgnoreUnsupported: true,
			})
			if err != nil {
				t.Fatalf("Convert(%s) failed: %v", name, err)
			}
			actualByRel := map[string]string{
				"expected/diagnostics.json": result.DiagnosticsJSON,
				"expected/diagnostics.md":   result.DiagnosticsMD,
				"expected/review.md":        result.ReviewMD,
			}
			if result.UWSPath != "" {
				actualByRel["workflows/workflow.uws.yaml"] = result.UWSPath
			}
			for _, rel := range sortedMapKeys(actualByRel) {
				actual, err := os.ReadFile(actualByRel[rel])
				if err != nil {
					t.Fatalf("read actual %s: %v", rel, err)
				}
				expectedPath := filepath.Join(corpusRoot, name, rel)
				expected, err := os.ReadFile(expectedPath)
				if err != nil {
					t.Fatalf("read expected %s: %v", expectedPath, err)
				}
				if rel == "expected/review.md" {
					actual = normalizeReviewForCorpus(actual, outDir, filepath.Join(corpusRoot, name))
					expected = normalizeReviewForCorpus(expected, filepath.Join(corpusRoot, name), filepath.Join(corpusRoot, name))
				}
				if string(actual) != string(expected) {
					t.Fatalf("%s drifted for %s\n--- expected\n%s\n--- actual\n%s", rel, name, expected, actual)
				}
			}
		})
	}
}

func sortedMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func asStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func normalizeReviewForCorpus(data []byte, outDir, corpusDir string) []byte {
	text := string(data)
	text = strings.ReplaceAll(text, filepath.ToSlash(outDir), "<OUT_DIR>")
	text = strings.ReplaceAll(text, filepath.ToSlash(filepath.Clean(outDir)), "<OUT_DIR>")
	text = strings.ReplaceAll(text, filepath.ToSlash(corpusDir), "<OUT_DIR>")
	text = strings.ReplaceAll(text, filepath.ToSlash(filepath.Clean(corpusDir)), "<OUT_DIR>")
	return []byte(text)
}
