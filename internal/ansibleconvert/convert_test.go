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
	wantSections := map[string]bool{"roles": false}
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

func normalizeReviewForCorpus(data []byte, outDir, corpusDir string) []byte {
	text := string(data)
	text = strings.ReplaceAll(text, filepath.ToSlash(outDir), "<OUT_DIR>")
	text = strings.ReplaceAll(text, filepath.ToSlash(filepath.Clean(outDir)), "<OUT_DIR>")
	text = strings.ReplaceAll(text, filepath.ToSlash(corpusDir), "<OUT_DIR>")
	text = strings.ReplaceAll(text, filepath.ToSlash(filepath.Clean(corpusDir)), "<OUT_DIR>")
	return []byte(text)
}
