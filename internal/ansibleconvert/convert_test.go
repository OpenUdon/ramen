package ansibleconvert

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/internal/convertreport"
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
		TargetUWS:         opts.TargetUWS,
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
	if doc.UWS != "1.5.0" {
		t.Fatalf("uws = %q, want 1.5.0", doc.UWS)
	}
	// Ansible module leaves are extension-owned because the managed host does
	// not expose the collection module as a pre-existing named operation.
	if len(doc.SourceDescriptions) != 0 {
		t.Fatalf("sourceDescriptions = %#v, want none", doc.SourceDescriptions)
	}

	install := findOperation(doc, "install_nginx")
	if install == nil {
		t.Fatalf("install_nginx operation missing")
	}
	payload, ok, err := ReadOperationExtension(install.Extensions)
	if err != nil || !ok {
		t.Fatalf("install_nginx missing module-call extension: ok=%v err=%v", ok, err)
	}
	if payload.Module != "ansible.builtin.apt" {
		t.Fatalf("install_nginx module = %q, want ansible.builtin.apt", payload.Module)
	}
	if payload.Argspec == nil || payload.Argspec.SourceID != "builtin" {
		t.Fatalf("install_nginx argspec reference = %#v, want sourceId builtin", payload.Argspec)
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

func TestConvertNginxPlaybookSupportedTargetsUseSameExtensionOwnedLeaves(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		docVersion string
	}{
		{name: "1.5", target: TargetUWS15, docVersion: "1.5.0"},
		{name: "1.6", target: TargetUWS16, docVersion: "1.6.0"},
		{name: "1.7", target: TargetUWS17, docVersion: "1.7.0"},
	}
	var baseline *uws1.Document
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, doc := runConvertWithOptions(t, "nginx", Options{TargetUWS: tt.target})
			if result.StrictFailures != 0 {
				t.Fatalf("expected no strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
			}
			if doc.UWS != tt.docVersion {
				t.Fatalf("uws = %q, want %s", doc.UWS, tt.docVersion)
			}
			if len(doc.SourceDescriptions) != 0 {
				t.Fatalf("target %s should not emit ansible sourceDescriptions: %#v", tt.name, doc.SourceDescriptions)
			}

			install := findOperation(doc, "install_nginx")
			if install == nil {
				t.Fatalf("install_nginx operation missing")
			}
			if install.SourceDescription != "" || install.SourceOperationID != "" || install.SourceOperationRef != "" {
				t.Fatalf("target %s operation should be extension-owned: %#v", tt.name, install)
			}
			if install.Extensions[uws1.ExtensionOperationProfile] != ProfileName {
				t.Fatalf("operation profile = %#v, want %s", install.Extensions[uws1.ExtensionOperationProfile], ProfileName)
			}
			if len(install.Extensions) != 3 || install.Extensions[ExtensionAnsibleModule] == nil || install.Extensions[ExtensionAnsibleProvenance] == nil {
				t.Fatalf("target %s operation extensions = %#v, want exact Ramen selector/module/provenance shape", tt.name, install.Extensions)
			}
			if err := ValidateOperationExtensions(install.Extensions); err != nil {
				t.Fatalf("target %s operation extensions are invalid: %v", tt.name, err)
			}
			payload, ok, err := ReadOperationExtension(install.Extensions)
			if err != nil || !ok {
				t.Fatalf("read ansible module extension ok=%v err=%v extensions=%#v", ok, err, install.Extensions)
			}
			if payload.Module != "ansible.builtin.apt" || payload.Argspec == nil || payload.Argspec.SourceID != "builtin" || payload.Argspec.Collection != "ansible.builtin" {
				t.Fatalf("ansible module payload = %#v", payload)
			}
			if body, _ := install.Request["body"].(map[string]any); body["name"] != "$variables.pkg" {
				t.Fatalf("request body changed for target %s: %#v", tt.name, install.Request)
			}
			if install.Outputs["changed"] != "$response.body.changed" {
				t.Fatalf("outputs changed for target %s: %#v", tt.name, install.Outputs)
			}
			review, err := os.ReadFile(result.ReviewMD)
			if err != nil {
				t.Fatalf("read review: %v", err)
			}
			for _, want := range []string{"`install_nginx`", "`builtin`", "`ansible.builtin.apt`"} {
				if !strings.Contains(string(review), want) {
					t.Fatalf("target %s review missing %q:\n%s", tt.name, want, review)
				}
			}
			for _, path := range []string{result.UWSPath, result.HCLPath} {
				artifact, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read target %s artifact %s: %v", tt.name, path, err)
				}
				for _, retired := range []string{retiredArgspecVersion, retiredProfileName, retiredExtensionModule, retiredExtensionProvenance} {
					if strings.Contains(string(artifact), retired) {
						t.Fatalf("target %s artifact %s contains retired identifier %q:\n%s", tt.name, path, retired, artifact)
					}
				}
			}

			doc.UWS = ""
			if baseline == nil {
				baseline = doc
			} else if !reflect.DeepEqual(doc, baseline) {
				t.Fatalf("target %s document shape differs from target 1.5:\nwant %#v\ngot  %#v", tt.name, baseline, doc)
			}
		})
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
	if workflow.Inputs != nil {
		t.Fatalf("workflow inputs = %#v, want static inventory to avoid runtime hosts input", workflow.Inputs)
	}
	step := findStep(workflow.Steps, "ensure_nginx_is_present")
	if step == nil {
		t.Fatalf("host fan-out step missing: %#v", workflow.Steps)
	}
	if step.ForEach != "$variables.inventory_configure_remote_hosts_hosts" {
		t.Fatalf("forEach = %q, want deterministic static inventory hosts", step.ForEach)
	}
	if step.Inputs["host"] != "$item.host" {
		t.Fatalf("step inputs = %#v, want host bound to the static inventory item", step.Inputs)
	}
}

func TestConvertInventoryHostFanOutWithHandlerFailsClosed(t *testing.T) {
	result, doc := runConvertWithOptions(t, "nginx", Options{
		InventoryPaths: []string{filepath.Join("testdata", "inventory.ini")},
	})

	if step := findStep(doc.Workflows[0].Steps, "install_nginx"); step == nil || step.ForEach != "$variables.inventory_configure_nginx_hosts" {
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

func TestConvertPreservesDottedArgspecSourceID(t *testing.T) {
	// The natural argspec ID is often the collection FQCN. Extension-owned
	// module calls preserve it because no sourceDescription name is emitted.
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
	if len(doc.SourceDescriptions) != 0 {
		t.Fatalf("sourceDescriptions = %#v, want none", doc.SourceDescriptions)
	}
	for _, op := range doc.Operations {
		payload, ok, err := ReadOperationExtension(op.Extensions)
		if err != nil || !ok {
			t.Fatalf("operation %q missing module-call extension: ok=%v err=%v", op.OperationID, ok, err)
		}
		if !strings.HasPrefix(payload.Module, "ansible.builtin.") {
			t.Fatalf("operation %q module = %q, want module FQCN preserved", op.OperationID, payload.Module)
		}
		// The dotted argspec ID is carried verbatim in the review reference; it
		// no longer has to satisfy the UWS sourceDescription name pattern.
		if payload.Argspec == nil || payload.Argspec.SourceID != "ansible.builtin" {
			t.Fatalf("operation %q argspec reference = %#v, want sourceId ansible.builtin", op.OperationID, payload.Argspec)
		}
	}
}

func TestLoadArgspecsPreservesDistinctRawSourceIDs(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.json")
	second := filepath.Join(root, "second.json")
	if err := os.WriteFile(first, []byte(`{"argspec":"ramen.ansible.1.0","collection":"acme.one","modules":{"acme.one.first":{"parameters":{}}}}`), 0o644); err != nil {
		t.Fatalf("write first argspec: %v", err)
	}
	if err := os.WriteFile(second, []byte(`{"argspec":"ramen.ansible.1.0","collection":"acme.two","modules":{"acme.two.second":{"parameters":{}}}}`), 0o644); err != nil {
		t.Fatalf("write second argspec: %v", err)
	}
	idx, err := LoadArgspecs([]ArgspecInput{
		{ID: "acme.one", Path: first},
		{ID: "acme-one", Path: second},
	})
	if err != nil {
		t.Fatalf("LoadArgspecs rejected distinct raw IDs: %v", err)
	}
	for fqcn, wantSourceID := range map[string]string{
		"acme.one.first":  "acme.one",
		"acme.two.second": "acme-one",
	} {
		gotSourceID, _, ok := idx.Lookup(fqcn)
		if !ok || gotSourceID != wantSourceID {
			t.Fatalf("Lookup(%q) = (%q, %v), want source ID %q", fqcn, gotSourceID, ok, wantSourceID)
		}
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
	// UWS switch steps keep the original task step ID inside the wrapper.
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

func TestConvertPortableConditionCombinations(t *testing.T) {
	result, doc := runConvert(t, "conditions")
	if result.StrictFailures != 0 {
		t.Fatalf("expected no strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}

	andWrapper := findTopStep(doc.Workflows[0].Steps, "and_condition_guard_1")
	if andWrapper == nil || andWrapper.Type != uws1.WorkflowTypeSwitch || len(andWrapper.Cases) != 1 {
		t.Fatalf("AND condition should lower to nested switch guard, got %#v", andWrapper)
	}
	if andWrapper.Cases[0].When != `$variables.env == "prod"` {
		t.Fatalf("AND first guard = %q", andWrapper.Cases[0].When)
	}
	if inner := findStep(andWrapper.Cases[0].Steps, "and_condition_guard_2"); inner == nil || inner.Cases[0].When != "$variables.enabled == true" {
		t.Fatalf("AND second guard missing: %#v", andWrapper)
	}

	orWrapper := findTopStep(doc.Workflows[0].Steps, "or_condition_guard_or")
	if orWrapper == nil || orWrapper.Type != uws1.WorkflowTypeSwitch || len(orWrapper.Cases) != 2 {
		t.Fatalf("OR condition should lower to switch cases, got %#v", orWrapper)
	}
	gotOR := []string{orWrapper.Cases[0].When, orWrapper.Cases[1].When}
	wantOR := []string{`$variables.env == "prod"`, "$inputs.missing_flag == true"}
	if !slices.Equal(gotOR, wantOR) {
		t.Fatalf("OR guards = %v, want %v", gotOR, wantOR)
	}
	for _, c := range orWrapper.Cases {
		if len(c.Steps) != 1 || c.Steps[0].OperationRef != "or_condition" {
			t.Fatalf("OR case should reference operation once: %#v", c)
		}
	}

	nestedWrapper := findTopStep(doc.Workflows[0].Steps, "nested_condition_guard_or")
	if nestedWrapper == nil || nestedWrapper.Type != uws1.WorkflowTypeSwitch || len(nestedWrapper.Cases) != 2 {
		t.Fatalf("nested OR/AND condition should lower to switch cases, got %#v", nestedWrapper)
	}
	if nestedWrapper.Cases[0].When != `$variables.env == "prod"` {
		t.Fatalf("nested first case guard = %q", nestedWrapper.Cases[0].When)
	}
	if len(nestedWrapper.Cases[0].Steps) != 1 || nestedWrapper.Cases[0].Steps[0].When != "$variables.enabled == true" {
		t.Fatalf("nested AND branch missing second guard: %#v", nestedWrapper.Cases[0].Steps)
	}
	if nestedWrapper.Cases[1].When != "$inputs.missing_flag == true" {
		t.Fatalf("nested OR second case = %q", nestedWrapper.Cases[1].When)
	}

	notStep := findStep(doc.Workflows[0].Steps, "not_condition")
	if notStep == nil || notStep.When != "$inputs.missing_flag != true" {
		t.Fatalf("not condition step = %#v", notStep)
	}
	definedStep := findStep(doc.Workflows[0].Steps, "defined_condition")
	if definedStep == nil || definedStep.When != "$inputs.missing_flag == null" {
		t.Fatalf("defined condition step = %#v", definedStep)
	}
}

func TestConvertORNotifyTaskFailsClosed(t *testing.T) {
	root := t.TempDir()
	playbookPath := filepath.Join(root, "playbook.yml")
	if err := os.WriteFile(playbookPath, []byte(`- name: OR notify case
  hosts: localhost
  vars:
    env: prod
  tasks:
    - name: Safe task
      ansible.builtin.shell:
        cmd: echo safe

    - name: Deploy config
      ansible.builtin.template:
        src: nginx.conf.j2
        dest: /etc/nginx/nginx.conf
      when: env == "prod" or force_deploy
      notify: restart nginx
  handlers:
    - name: restart nginx
      ansible.builtin.service:
        name: nginx
        state: restarted
`), 0o644); err != nil {
		t.Fatalf("write playbook: %v", err)
	}

	result, err := Convert(context.Background(), Options{
		PlaybookPath: playbookPath,
		Argspecs: []ArgspecInput{
			{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
		},
		OutDir:            filepath.Join(root, "out"),
		IgnoreUnsupported: true,
	})
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	if result.StrictFailures == 0 {
		t.Fatalf("expected OR notify strict failure: %#v", result.Diagnostics)
	}
	var sawORNotify bool
	for _, d := range result.Diagnostics {
		if d.Code == CodeDirectiveTodo && d.Task == "Deploy config" && d.StrictFailure && strings.Contains(d.Message, "notifying task") {
			sawORNotify = true
		}
	}
	if !sawORNotify {
		t.Fatalf("missing OR notify diagnostic: %#v", result.Diagnostics)
	}

	data, err := os.ReadFile(result.UWSPath)
	if err != nil {
		t.Fatalf("read emitted UWS document: %v", err)
	}
	var doc uws1.Document
	if err := convert.UnmarshalYAML(data, &doc); err != nil {
		t.Fatalf("parse emitted UWS document: %v", err)
	}
	if findStep(doc.Workflows[0].Steps, "deploy_config") != nil || findStep(doc.Workflows[0].Steps, "restart_nginx") != nil {
		t.Fatalf("OR notify task or handler leaked into document: %#v", doc.Workflows[0].Steps)
	}
	if safe := findStep(doc.Workflows[0].Steps, "safe_task"); safe == nil {
		t.Fatalf("unrelated safe task should still lower: %#v", doc.Workflows[0].Steps)
	}
}

func TestConvertWrappedGuardRegisterAndNotifyFailClosed(t *testing.T) {
	root := t.TempDir()
	playbookPath := filepath.Join(root, "playbook.yml")
	if err := os.WriteFile(playbookPath, []byte(`- name: wrapped guard producer case
  hosts: localhost
  vars:
    env: prod
    enabled: true
  tasks:
    - name: Safe task
      ansible.builtin.shell:
        cmd: echo safe

    - name: Producer
      ansible.builtin.shell:
        cmd: echo producer
      when:
        - env == "prod"
        - enabled
      register: produced

    - name: Consumer
      ansible.builtin.shell:
        cmd: "{{ produced.rc }}"

    - name: Deploy config
      ansible.builtin.template:
        src: nginx.conf.j2
        dest: /etc/nginx/nginx.conf
      when:
        - env == "prod"
        - enabled
      notify: restart nginx
  handlers:
    - name: restart nginx
      ansible.builtin.service:
        name: nginx
        state: restarted
`), 0o644); err != nil {
		t.Fatalf("write playbook: %v", err)
	}

	result, err := Convert(context.Background(), Options{
		PlaybookPath: playbookPath,
		Argspecs: []ArgspecInput{
			{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
		},
		OutDir:            filepath.Join(root, "out"),
		IgnoreUnsupported: true,
	})
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	if result.StrictFailures < 2 {
		t.Fatalf("expected register and notify strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
	var sawRegister, sawNotify bool
	for _, d := range result.Diagnostics {
		switch {
		case d.Code == CodeDirectiveTodo && d.Task == "Producer" && d.StrictFailure && strings.Contains(d.Message, "registered task"):
			sawRegister = true
		case d.Code == CodeDirectiveTodo && d.Task == "Deploy config" && d.StrictFailure && strings.Contains(d.Message, "notifying task"):
			sawNotify = true
		}
	}
	if !sawRegister || !sawNotify {
		t.Fatalf("missing wrapped-guard diagnostics register=%v notify=%v: %#v", sawRegister, sawNotify, result.Diagnostics)
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
	for _, leaked := range []string{"producer", "deploy_config", "restart_nginx"} {
		if findOperation(&doc, leaked) != nil || findStep(doc.Workflows[0].Steps, leaked) != nil {
			t.Fatalf("wrapped-guard task %q leaked into document: %#v", leaked, doc.Workflows[0].Steps)
		}
	}
	consumer := findOperation(&doc, "consumer")
	if consumer == nil {
		t.Fatalf("consumer task missing")
	}
	body, _ := consumer.Request["body"].(map[string]any)
	cmd, _ := body["cmd"].(string)
	if strings.Contains(cmd, "$steps.producer") {
		t.Fatalf("consumer references skipped wrapped producer: %q", cmd)
	}
	if !strings.HasPrefix(cmd, "UWS-TODO") {
		t.Fatalf("consumer arg should be a TODO placeholder, got %q", cmd)
	}
	if safe := findStep(doc.Workflows[0].Steps, "safe_task"); safe == nil {
		t.Fatalf("unrelated safe task should still lower: %#v", doc.Workflows[0].Steps)
	}
}

func TestConvertBadTaskShapesWriteDiagnosticsAndPartialArtifacts(t *testing.T) {
	root := t.TempDir()
	playbookPath := filepath.Join(root, "playbook.yml")
	if err := os.WriteFile(playbookPath, []byte(`- name: bad task shapes
  hosts: localhost
  tasks:
    - name: Safe task
      ansible.builtin.shell:
        cmd: echo safe

    - name: Missing module
      tags: bad

    - name: Multiple modules
      ansible.builtin.shell:
        cmd: echo one
      ansible.builtin.service:
        name: nginx
        state: started

    - name: Bad arg shape
      ansible.builtin.shell:
        - echo bad

    - name: Unsupported legacy loop
      ansible.builtin.shell:
        cmd: echo bad
      with_dict:
        key: value
`), 0o644); err != nil {
		t.Fatalf("write playbook: %v", err)
	}

	result, err := Convert(context.Background(), Options{
		PlaybookPath: playbookPath,
		Argspecs: []ArgspecInput{
			{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
		},
		OutDir:            filepath.Join(root, "out"),
		IgnoreUnsupported: true,
	})
	if err != nil {
		t.Fatalf("Convert should not abort on task-shape diagnostics: %v", err)
	}
	if result.UWSPath == "" {
		t.Fatalf("partial workflow should be written when a safe task lowers: %#v", result)
	}
	for _, path := range []string{result.DiagnosticsJSON, result.DiagnosticsMD, result.ReviewMD, result.UWSPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
	}
	if result.StrictFailures < 4 {
		t.Fatalf("expected strict diagnostics for bad shapes and legacy loop, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
	for _, want := range []string{"no module invocation", "multiple module keys", "unsupported argument shape", "with_dict"} {
		var found bool
		for _, d := range result.Diagnostics {
			if d.StrictFailure && strings.Contains(d.Message, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing diagnostic containing %q: %#v", want, result.Diagnostics)
		}
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
	if findOperation(&doc, "safe_task") == nil {
		t.Fatalf("safe task did not lower: %#v", doc.Operations)
	}
	for _, skipped := range []string{"missing_module", "multiple_modules", "bad_arg_shape", "unsupported_legacy_loop"} {
		if findOperation(&doc, skipped) != nil || findStep(doc.Workflows[0].Steps, skipped) != nil {
			t.Fatalf("bad task %q leaked into partial workflow", skipped)
		}
	}
}

func TestConvertChangedFailedWhenUseCurrentResponseAndInvertComparisons(t *testing.T) {
	root := t.TempDir()
	playbookPath := filepath.Join(root, "playbook.yml")
	if err := os.WriteFile(playbookPath, []byte(`- name: condition outputs
  hosts: localhost
  tasks:
    - name: Changed from response
      ansible.builtin.shell:
        cmd: echo changed
      register: changed_result
      changed_when: changed_result.rc == 0

    - name: Failed eq
      ansible.builtin.shell:
        cmd: echo eq
      register: eq_result
      failed_when: eq_result.rc == 1

    - name: Failed ne
      ansible.builtin.shell:
        cmd: echo ne
      register: ne_result
      failed_when: ne_result.rc != 1

    - name: Failed lt
      ansible.builtin.shell:
        cmd: echo lt
      register: lt_result
      failed_when: lt_result.rc < 1

    - name: Failed le
      ansible.builtin.shell:
        cmd: echo le
      register: le_result
      failed_when: le_result.rc <= 1

    - name: Failed gt
      ansible.builtin.shell:
        cmd: echo gt
      register: gt_result
      failed_when: gt_result.rc > 1

    - name: Failed ge
      ansible.builtin.shell:
        cmd: echo ge
      register: ge_result
      failed_when: ge_result.rc >= 1
`), 0o644); err != nil {
		t.Fatalf("write playbook: %v", err)
	}

	result, err := Convert(context.Background(), Options{
		PlaybookPath: playbookPath,
		Argspecs: []ArgspecInput{
			{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
		},
		OutDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
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
	changed := findOperation(&doc, "changed_from_response")
	if changed == nil || changed.Outputs["changed"] != "$response.body.rc == 0" {
		t.Fatalf("changed_when did not use current response: %#v", changed)
	}
	wantCriteria := map[string]string{
		"failed_eq": "$response.body.rc != 1",
		"failed_ne": "$response.body.rc == 1",
		"failed_lt": "$response.body.rc >= 1",
		"failed_le": "$response.body.rc > 1",
		"failed_gt": "$response.body.rc <= 1",
		"failed_ge": "$response.body.rc < 1",
	}
	for opID, want := range wantCriteria {
		op := findOperation(&doc, opID)
		if op == nil || len(op.SuccessCriteria) != 1 || op.SuccessCriteria[0].Condition != want {
			t.Fatalf("%s successCriteria = %#v, want %q", opID, op, want)
		}
	}
}

func TestConvertStaticImportInheritsSemanticDirectives(t *testing.T) {
	result, doc := runConvert(t, "importdirectives")
	if result.StrictFailures != 0 {
		t.Fatalf("expected no strict failures, got %d: %#v", result.StrictFailures, result.Diagnostics)
	}
	step := findStep(doc.Workflows[0].Steps, "imported_semantic_task")
	if step == nil || step.When != `$variables.env == "prod"` {
		t.Fatalf("imported task should inherit wrapper when as step guard, got %#v", step)
	}
	op := findOperation(doc, "imported_semantic_task")
	if op == nil {
		t.Fatalf("imported operation missing")
	}
	if op.Outputs["changed"] != "$inputs.changed_flag == true" {
		t.Fatalf("changed_when inheritance = %#v", op.Outputs)
	}
	if len(op.SuccessCriteria) != 2 || op.SuccessCriteria[0].Condition != "$inputs.failed_flag != true" || op.SuccessCriteria[1].Condition != "$inputs.ready_flag == true" {
		t.Fatalf("failed_when/until inheritance criteria = %#v", op.SuccessCriteria)
	}
	if len(op.OnFailure) != 1 || op.OnFailure[0].Type != "retry" || op.OnFailure[0].RetryLimit != 3 || op.OnFailure[0].RetryAfter != 2 {
		t.Fatalf("retry inheritance = %#v", op.OnFailure)
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
		prov, _ := op.Extensions[ExtensionAnsibleProvenance].(map[string]any)
		if !slices.Contains(asStringSlice(prov["tags"]), "setup") {
			t.Fatalf("imported task %q did not inherit import tags: %#v", id, prov)
		}
	}
	op := findOperation(doc, "nested_imported")
	prov, _ := op.Extensions[ExtensionAnsibleProvenance].(map[string]any)
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
	if hclOp == nil || hclOp.Extensions[ExtensionAnsibleProvenance] == nil {
		t.Fatalf("HCL round-trip lost x-ramen-ansible-provenance provenance: %#v", hclOp)
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
	prov, _ := op.Extensions[ExtensionAnsibleProvenance].(map[string]any)
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

func TestConvertLiteralIncludeTasksAndRole(t *testing.T) {
	result, doc := runConvertWithOptions(t, "includes", Options{
		ProjectDir: filepath.Join("testdata", "includes"),
	})
	if result.StrictFailures != 0 {
		t.Fatalf("literal includes should lower: %#v", result.Diagnostics)
	}
	if got := []string{doc.Workflows[0].Steps[0].OperationRef, doc.Workflows[0].Steps[1].OperationRef}; !slices.Equal(got, []string{"included_task", "included_role_task"}) {
		t.Fatalf("include operation order = %#v", got)
	}
	step := findStep(doc.Workflows[0].Steps, "included_task")
	if step == nil || step.When != "$inputs.enabled == true" || step.Inputs["local_value"] != "included" {
		t.Fatalf("included task wrapper = %#v", step)
	}
	op := findOperation(doc, "included_task")
	body, _ := op.Request["body"].(map[string]any)
	if body["cmd"] != "$inputs.local_value" {
		t.Fatalf("included task body = %#v", body)
	}
	roleOp := findOperation(doc, "included_role_task")
	prov, _ := roleOp.Extensions[ExtensionAnsibleProvenance].(map[string]any)
	if prov["role"] != "extra" {
		t.Fatalf("included role provenance = %#v", prov)
	}
}

func TestConvertDynamicIncludeOptionsRemainStrict(t *testing.T) {
	root := t.TempDir()
	playbook := filepath.Join(root, "playbook.yml")
	if err := os.WriteFile(playbook, []byte(`- name: dynamic includes
  hosts: localhost
  tasks:
    - name: templated tasks
      include_tasks: "{{ file_name }}"
    - name: applied role
      include_role:
        name: extra
        apply:
          tags: always
    - name: looped tasks
      include_tasks: tasks.yml
      loop: [one]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Convert(context.Background(), Options{
		PlaybookPath: playbook,
		Argspecs: []ArgspecInput{
			{ID: "builtin", Path: filepath.Join("testdata", "argspec", "ansible-builtin.argspec.json")},
		},
		Mode: "partial", OutDir: filepath.Join(root, "out"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StrictFailures < 3 || result.UWSPath != "" {
		t.Fatalf("dynamic include result = %#v", result)
	}
	for _, name := range []string{"templated tasks", "applied role", "looped tasks"} {
		var found bool
		for _, diag := range result.Diagnostics {
			if diag.Task == name && diag.Code == CodeDynamicInclude && diag.StrictFailure {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing dynamic include diagnostic for %q: %#v", name, result.Diagnostics)
		}
	}
}

func TestConvertStaticVariablePrecedenceKeepsPlayVarsAboveVarsFiles(t *testing.T) {
	result, doc := runConvertWithOptions(t, "varconflict", Options{
		ProjectDir: filepath.Join("testdata", "varconflict"),
	})
	if result.StrictFailures != 0 {
		t.Fatalf("play vars should override vars_files: %#v", result.Diagnostics)
	}
	if doc.Components == nil || doc.Components.Variables["app_pkg"] != "nginx" {
		t.Fatalf("app_pkg = %#v, want play-level nginx", doc.Components)
	}
	for _, d := range result.Diagnostics {
		if d.Code == CodeVariableConflict {
			t.Fatalf("unexpected precedence conflict: %#v", result.Diagnostics)
		}
	}
}

func TestConvertStaticInventoryVariablesAndPrecedence(t *testing.T) {
	result, doc := runConvertWithOptions(t, "inventoryvars", Options{
		InventoryPaths: []string{filepath.Join("testdata", "inventoryvars", "inventory.yml")},
	})
	if result.StrictFailures != 0 {
		t.Fatalf("inventory vars conversion failed: %#v", result.Diagnostics)
	}
	if doc.Components == nil || doc.Components.Variables["env"] != "play" || doc.Components.Variables["common_value"] != "from_file" {
		t.Fatalf("global precedence variables = %#v", doc.Components)
	}
	hosts, ok := doc.Components.Variables["inventory_inventory_variable_scope_hosts"].([]any)
	if !ok || len(hosts) != 2 {
		t.Fatalf("inventory host facts = %#v", doc.Components.Variables)
	}
	for _, value := range hosts {
		host, ok := value.(map[string]any)
		if !ok || host["host"] == "" {
			t.Fatalf("host fact = %#v", value)
		}
		vars, _ := host["vars"].(map[string]any)
		if vars["tier"] != "frontend" || vars["zone"] == nil || vars["base"] != true {
			t.Fatalf("host vars = %#v", vars)
		}
		for name := range vars {
			if strings.HasPrefix(name, "ansible_") || name == "env" {
				t.Fatalf("runtime/shadowed inventory variable leaked: %#v", vars)
			}
		}
	}
	checks := map[string]any{
		"global_precedence": "$variables.env",
		"group_variable":    "$item.vars.tier",
		"host_variable":     "$item.vars.zone",
	}
	for operationID, want := range checks {
		op := findOperation(doc, operationID)
		body, _ := op.Request["body"].(map[string]any)
		if body["cmd"] != want {
			t.Fatalf("%s cmd = %#v, want %#v", operationID, body["cmd"], want)
		}
		step := findStep(doc.Workflows[0].Steps, operationID)
		if step == nil || step.ForEach != "$variables.inventory_inventory_variable_scope_hosts" || step.Inputs["host"] != "$item.host" {
			t.Fatalf("%s static host fan-out = %#v", operationID, step)
		}
	}
}

func TestConvertSemanticDirectivesLowerWhenStaticButRuntimeMetadataIsInfo(t *testing.T) {
	result, doc := runConvert(t, "directives")
	if doc.UWS != "1.5.0" {
		t.Fatalf("semantic directives should stay on UWS 1.5.0, got %q", doc.UWS)
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
				"expected/manifest.json":    result.ManifestPath,
				"expected/review.md":        result.ReviewMD,
			}
			if result.UWSPath != "" {
				actualByRel["workflows/workflow.uws.yaml"] = result.UWSPath
			}
			if result.HCLPath != "" {
				actualByRel["workflows/workflow.hcl"] = result.HCLPath
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
				if rel == "expected/manifest.json" {
					if err := convertreport.ValidateManifest(actual); err != nil {
						t.Fatalf("manifest for %s is invalid: %v", name, err)
					}
				}
				if strings.HasPrefix(rel, "workflows/") {
					for _, retired := range []string{retiredArgspecVersion, retiredProfileName, retiredExtensionModule, retiredExtensionProvenance} {
						if strings.Contains(string(actual), retired) {
							t.Fatalf("%s for %s contains retired identifier %q", rel, name, retired)
						}
					}
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
