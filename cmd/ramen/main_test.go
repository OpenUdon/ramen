package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	sharedicotcli "github.com/OpenUdon/authoring/icotcli"
	sharedpromptcontext "github.com/OpenUdon/authoring/promptcontext"
	sharedreport "github.com/OpenUdon/authoring/report"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	ramenrun "github.com/OpenUdon/ramen/run"
	"github.com/OpenUdon/ramen/state"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestCLIConvertHelpIncludesContract(t *testing.T) {
	cmd := helperCommand("--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("top-level help failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "author") {
		t.Fatalf("top-level help missing author command:\n%s", output)
	}
	if !strings.Contains(string(output), "icot") {
		t.Fatalf("top-level help missing icot command:\n%s", output)
	}
	if strings.Contains(string(output), "destroy") {
		t.Fatalf("top-level help still advertises deprecated destroy command:\n%s", output)
	}

	cmd = helperCommand("author", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("author help failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"Usage: ramen author", "--context", "--goal", "--out", "--validate", "--graph", "--plan", "does not execute"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("author help missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "destroy") {
		t.Fatalf("author help still names deprecated destroy command:\n%s", text)
	}

	cmd = helperCommand("convert", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert help failed: %v\n%s", err, output)
	}
	text = string(output)
	for _, expected := range []string{"Usage: ramen convert", "--config-dir", "--api-source", "--openapi", "--action", "--target", "--strict", "does not execute Terraform"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("convert help missing %q:\n%s", expected, text)
		}
	}
	for _, expected := range []string{"ansible", "--playbook"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("convert help missing ansible subcommand %q:\n%s", expected, text)
		}
	}

	for _, helpArg := range []string{"-h", "help"} {
		cmd = helperCommand("convert", helpArg)
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("convert %s failed: %v\n%s", helpArg, err, output)
		}
		if !strings.Contains(string(output), "Usage: ramen convert") {
			t.Fatalf("convert %s output missing usage:\n%s", helpArg, output)
		}
	}

	cmd = helperCommand("convert", "tf", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert tf --help failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "Usage: ramen convert") {
		t.Fatalf("convert tf help missing usage:\n%s", output)
	}

	cmd = helperCommand("convert", "ansible", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert ansible --help failed: %v\n%s", err, output)
	}
	for _, expected := range []string{"Usage: ramen convert ansible", "--playbook", "--argspec", "--project-dir", "--roles-path", "--collections-path", "--inventory", "--extra-var", "--target-uws", "--ignore-unsupported", "ansible-module", "resolving play roles/import_role", "host fan-out over $inputs.hosts", "not used for static expression lowering"} {
		if !strings.Contains(string(output), expected) {
			t.Fatalf("convert ansible help missing %q:\n%s", expected, output)
		}
	}

	cmd = helperCommand("icot", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("icot help failed: %v\n%s", err, output)
	}
	text = string(output)
	for _, expected := range []string{"Usage: ramen icot", "--goal", "--api-source", "--prompt-mode", "--no-llm", "--answers", "never executes"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("icot help missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "destroy") {
		t.Fatalf("icot help still names deprecated destroy command:\n%s", text)
	}
}

func TestCLIAuthorDraftsProject(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	writeAuthorOpenAPIForCLITest(t, sourcePath)
	contextPath := writeAuthorContextForCLITest(t, root, sharedpromptcontext.Context{
		Version: sharedpromptcontext.Version,
		Sources: []sharedpromptcontext.SourceDocument{{
			ID:   "widgets",
			Kind: "openapi",
			URI:  sourcePath,
		}},
		Operations: []sharedpromptcontext.OperationCandidate{{
			ID:              "widgets#createWidget",
			SourceID:        "widgets",
			OperationID:     "createWidget",
			Verb:            "POST",
			Path:            "/widgets",
			RequestSchemaID: "createWidgetRequest",
		}},
		Schemas: []sharedpromptcontext.SchemaHint{{
			ID:       "createWidgetRequest",
			Required: []string{"name"},
			Fields: []sharedpromptcontext.FieldHint{
				{Name: "name", Type: "string", Required: true},
			},
		}},
	})
	outDir := filepath.Join(root, "draft")
	cmd := helperCommand("author", "--context", contextPath, "--goal", "Manage widgets", "--out", outDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("author failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "ramen: author status=complete") {
		t.Fatalf("author output missing complete status:\n%s", output)
	}
	projectPath := filepath.Join(outDir, project.DefaultFile)
	if _, err := os.Stat(projectPath); err != nil {
		t.Fatalf("project file missing: %v", err)
	}
	if _, err := project.Load(projectPath); err != nil {
		t.Fatalf("generated project does not load: %v", err)
	}
}

func TestCLIAuthorJSONWithGates(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	writeAuthorOpenAPIForCLITest(t, sourcePath)
	contextPath := writeAuthorContextForCLITest(t, root, sharedpromptcontext.Context{
		Version: sharedpromptcontext.Version,
		Sources: []sharedpromptcontext.SourceDocument{{
			ID:   "widgets",
			Kind: "openapi",
			URI:  sourcePath,
		}},
		Operations: []sharedpromptcontext.OperationCandidate{{
			ID:              "widgets#createWidget",
			SourceID:        "widgets",
			OperationID:     "createWidget",
			RequestSchemaID: "createWidgetRequest",
		}},
		Schemas: []sharedpromptcontext.SchemaHint{{
			ID:       "createWidgetRequest",
			Required: []string{"name"},
			Fields: []sharedpromptcontext.FieldHint{
				{Name: "name", Type: "string", Required: true},
			},
		}},
	})
	cmd := helperCommand("author", "--context", contextPath, "--goal", "Manage widgets", "--out", filepath.Join(root, "draft"), "--json", "--validate", "--graph", "--plan")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("author json gates failed: %v\n%s", err, output)
	}
	var result authorCLIResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("author JSON is not parseable: %v\n%s", err, output)
	}
	if result.Report.Status != sharedreport.StatusComplete || result.ProjectPath == "" {
		t.Fatalf("author JSON report = %#v\n%s", result, output)
	}
	if result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("author validation = %#v\n%s", result.Validation, output)
	}
	if result.Graph == nil || len(result.Graph.Nodes) == 0 {
		t.Fatalf("author graph = %#v\n%s", result.Graph, output)
	}
	if result.Plan == nil || result.Plan.Plan.Version != tfplan.Version {
		t.Fatalf("author plan = %#v\n%s", result.Plan, output)
	}
}

func TestCLIAuthorNeedsInputWithoutOperations(t *testing.T) {
	root := t.TempDir()
	contextPath := writeAuthorContextForCLITest(t, root, sharedpromptcontext.Context{
		Version: sharedpromptcontext.Version,
		Sources: []sharedpromptcontext.SourceDocument{{
			ID:   "widgets",
			Kind: "openapi",
			URI:  filepath.Join(root, "api.yaml"),
		}},
	})
	outDir := filepath.Join(root, "draft")
	cmd := helperCommand("author", "--context", contextPath, "--goal", "Manage widgets", "--out", outDir, "--json")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("author without operations unexpectedly succeeded:\n%s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("author exit = %v, output:\n%s", err, output)
	}
	if !strings.Contains(string(output), "ramen.authoring.missing_mapping_metadata") {
		t.Fatalf("author needs-input output missing diagnostic:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(outDir, project.DefaultFile)); !os.IsNotExist(err) {
		t.Fatalf("author wrote project despite needs_input: %v", err)
	}

	sourcePath := filepath.Join(root, "api.yaml")
	writeAuthorOpenAPIForCLITest(t, sourcePath)
	contextPath = writeAuthorContextForCLITest(t, root, sharedpromptcontext.Context{
		Version: sharedpromptcontext.Version,
		Sources: []sharedpromptcontext.SourceDocument{{
			ID:   "widgets",
			Kind: "openapi",
			URI:  sourcePath,
		}},
		Operations: []sharedpromptcontext.OperationCandidate{{
			ID:          "widgets#createWidget",
			SourceID:    "widgets",
			OperationID: "createWidget",
		}},
	})
	outDir = filepath.Join(root, "missing-goal")
	cmd = helperCommand("author", "--context", contextPath, "--out", outDir, "--json")
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("author without goal unexpectedly succeeded:\n%s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("author missing-goal exit = %v, output:\n%s", err, output)
	}
	if !strings.Contains(string(output), "ramen.authoring.missing_goal") {
		t.Fatalf("author missing-goal output missing diagnostic:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(outDir, project.DefaultFile)); !os.IsNotExist(err) {
		t.Fatalf("author wrote project despite missing goal: %v", err)
	}
}

func TestCLIAuthorInputErrorsExitTwo(t *testing.T) {
	root := t.TempDir()
	for _, tt := range []struct {
		name     string
		args     []string
		expected string
	}{
		{name: "missing context", args: []string{"author", "--goal", "Manage widgets"}, expected: "Usage: ramen author"},
		{name: "invalid json", args: []string{"author", "--context", filepath.Join(root, "bad.json"), "--goal", "Manage widgets"}, expected: "author.context_parse_error"},
		{name: "missing file", args: []string{"author", "--context", filepath.Join(root, "missing.json"), "--goal", "Manage widgets"}, expected: "author.context_read_error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "invalid json" {
				mustWriteCLIFile(t, filepath.Join(root, "bad.json"), []byte(`{`))
			}
			cmd := helperCommand(tt.args...)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("author input error unexpectedly succeeded:\n%s", output)
			}
			if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
				t.Fatalf("author exit = %v, output:\n%s", err, output)
			}
			if !strings.Contains(string(output), tt.expected) {
				t.Fatalf("author error missing %q:\n%s", tt.expected, output)
			}
		})
	}
}

func TestCLIIcotNoLLMWritesProject(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	writeICOTOpenAPIForCLITest(t, sourcePath)
	outDir := filepath.Join(root, "icot")
	answersPath := filepath.Join(root, "answers.txt")
	mustWriteCLIFile(t, answersPath, []byte("createWidget\n"))
	cmd := helperCommand("icot", "--agent", "--no-llm", "--answers", answersPath, "--goal", "Create a widget", "--api-source", "openapi:widgets="+sourcePath, "--out", outDir, "--json")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("icot failed: %v\nstdout:\n%s\nstderr:\n%s", err, output, stderr.String())
	}
	var result authorCLIResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("icot JSON is not parseable: %v\n%s", err, output)
	}
	if result.Report.Status != sharedreport.StatusComplete || result.ProjectPath == "" {
		t.Fatalf("icot result = %#v\n%s", result, output)
	}
	if result.ProjectHCLPath == "" {
		t.Fatalf("icot result missing HCL path: %#v\n%s", result, output)
	}
	hclData, err := os.ReadFile(result.ProjectHCLPath)
	if err != nil {
		t.Fatalf("generated icot HCL project is missing: %v", err)
	}
	if _, err := uwsconvert.HCLToJSON(hclData); err != nil {
		t.Fatalf("generated icot HCL project does not parse: %v", err)
	}
	doc, err := project.Load(filepath.Join(outDir, project.DefaultFile))
	if err != nil {
		t.Fatalf("generated icot project does not load: %v", err)
	}
	if got := doc.Profile.APISources[0].Path; got != sourcePath {
		t.Fatalf("api source path = %q, want %q", got, sourcePath)
	}
	projectText, err := os.ReadFile(filepath.Join(outDir, project.DefaultFile))
	if err != nil {
		t.Fatalf("read generated icot project: %v", err)
	}
	if !strings.Contains(string(projectText), "../api.yaml") || strings.Contains(string(projectText), "widgets.yaml") {
		t.Fatalf("generated project did not preserve local API source path:\n%s", projectText)
	}
	validateCmd := helperCommand("validate", "--project", outDir)
	if validateOutput, err := validateCmd.CombinedOutput(); err != nil {
		t.Fatalf("validate without --api-source failed: %v\n%s", err, validateOutput)
	}
	planPath := filepath.Join(root, "plan.json")
	planCmd := helperCommand("plan", "--project", outDir, "--state", filepath.Join(root, "state.db"), "--out", planPath)
	if planOutput, err := planCmd.CombinedOutput(); err != nil {
		t.Fatalf("plan without --api-source failed: %v\n%s", err, planOutput)
	}
	if got := doc.Profile.Resources[0].RequiredOperations; len(got) != 1 || got[0] != "post" {
		t.Fatalf("required operations = %#v", got)
	}
	if role := doc.Profile.Resources[0].Operations["post"]; role.Method != "POST" || role.OperationID != "createWidget" {
		t.Fatalf("post operation role = %#v", role)
	}
}

func TestCLIIcotAgentAmbiguousOperationNeedsInput(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	writeICOTOpenAPIForCLITest(t, sourcePath)
	outDir := filepath.Join(root, "ambiguous")
	cmd := helperCommand("icot", "--agent", "--no-llm", "--goal", "Manage widgets", "--api-source", "openapi:widgets="+sourcePath, "--out", outDir, "--json")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err == nil {
		t.Fatalf("ambiguous icot unexpectedly succeeded:\nstdout:\n%s\nstderr:\n%s", output, stderr.String())
	}
	var result authorCLIResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("icot JSON is not parseable: %v\n%s", err, output)
	}
	if result.Report.Status != sharedreport.StatusNeedsInput || !strings.Contains(string(output), "ramen.icot.operation_ambiguous") {
		t.Fatalf("ambiguous result = %#v\n%s", result, output)
	}
	if _, err := os.Stat(filepath.Join(outDir, project.DefaultFile)); !os.IsNotExist(err) {
		t.Fatalf("ambiguous icot wrote project: %v", err)
	}
}

func TestCLIIcotAgentExpandsLifecycleOperations(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	writeLifecycleICOTOpenAPIForCLITest(t, sourcePath)
	outDir := filepath.Join(root, "lifecycle")
	answersPath := filepath.Join(root, "answers.txt")
	mustWriteCLIFile(t, answersPath, []byte("createWidget\n"))
	cmd := helperCommand("icot", "--agent", "--no-llm", "--answers", answersPath, "--goal", "Create, read, update, and delete a widget named ramen", "--api-source", "openapi:widgets="+sourcePath, "--out", outDir, "--json", "--validate")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("lifecycle icot failed: %v\nstdout:\n%s\nstderr:\n%s", err, output, stderr.String())
	}
	doc, err := project.Load(filepath.Join(outDir, project.DefaultFile))
	if err != nil {
		t.Fatalf("lifecycle project does not load: %v", err)
	}
	resource := doc.Profile.Resources[0]
	if got := resource.RequiredOperations; !slices.Equal(got, []string{"create", "delete", "read", "update"}) {
		t.Fatalf("required operations = %#v", got)
	}
	for role, operationID := range map[string]string{"create": "createWidget", "read": "getWidget", "update": "patchWidget", "delete": "deleteWidget"} {
		if resource.Operations[role].OperationID != operationID {
			t.Fatalf("%s role = %#v", role, resource.Operations[role])
		}
	}
	if len(doc.UWS.Operations) != 4 || len(doc.UWS.Workflows[0].Steps) != 4 {
		t.Fatalf("generated UWS operations/steps = %#v %#v", doc.UWS.Operations, doc.UWS.Workflows[0].Steps)
	}
}

func TestCLIIcotReadOnlyValidateGraphPlan(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	writeICOTOpenAPIForCLITest(t, sourcePath)
	outDir := filepath.Join(root, "read")
	answersPath := filepath.Join(root, "answers.txt")
	mustWriteCLIFile(t, answersPath, []byte("listWidgets\n"))
	cmd := helperCommand("icot", "--agent", "--no-llm", "--answers", answersPath, "--goal", "List all widgets", "--api-source", "openapi:widgets="+sourcePath, "--out", outDir, "--json", "--validate", "--graph", "--plan")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("read-only icot with plan failed: %v\nstdout:\n%s\nstderr:\n%s", err, output, stderr.String())
	}
	var result authorCLIResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("icot JSON is not parseable: %v\n%s", err, output)
	}
	if result.Report.Status != sharedreport.StatusComplete {
		t.Fatalf("icot read-only result = %#v\n%s", result, output)
	}
	if result.Validation == nil || !result.Validation.Valid || result.Graph == nil || result.Plan == nil || result.Plan.Plan.Summary.Read != 1 {
		t.Fatalf("read-only gates missing: validation=%#v graph=%#v plan=%#v\n%s", result.Validation, result.Graph, result.Plan, output)
	}
	doc, err := project.Load(filepath.Join(outDir, project.DefaultFile))
	if err != nil {
		t.Fatalf("read-only project does not load: %v", err)
	}
	resource := doc.Profile.Resources[0]
	if len(resource.RequiredOperations) != 1 || resource.RequiredOperations[0] != "read" {
		t.Fatalf("read-only required operations = %#v", resource.RequiredOperations)
	}
	if _, ok := resource.Operations["read"]; !ok {
		t.Fatalf("read operation role missing: %#v", resource.Operations)
	}
	if len(doc.UWS.Workflows) == 0 || len(doc.UWS.Workflows[0].Steps) == 0 || !strings.HasPrefix(doc.UWS.Workflows[0].Steps[0].StepID, "read_") {
		t.Fatalf("read-only workflow step = %#v", doc.UWS.Workflows)
	}
}

func TestCLIIcotReadOnlyValidateGraphPlanWithAzureFixture(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "azure", "resources.json")
	mustWriteCLIFile(t, sourcePath, []byte(`openapi: 3.0.0
info:
  title: Azure resources list
  version: v1
paths:
  /resources:
    get:
      operationId: Resources_List
      responses:
        "200":
          description: ok
`))

	outDir := filepath.Join(root, "read")
	answersPath := filepath.Join(root, "answers.txt")
	mustWriteCLIFile(t, answersPath, []byte("Resources_List\n"))
	cmd := helperCommand("icot", "--agent", "--no-llm", "--answers", answersPath, "--goal", "List Azure resources in the selected subscription", "--api-source", "openapi:azure-resources="+sourcePath, "--out", outDir, "--json", "--validate", "--graph", "--plan")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("read-only azure fixture icot with plan failed: %v\nstdout:\n%s\nstderr:\n%s", err, output, stderr.String())
	}
	var result authorCLIResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("icot JSON is not parseable: %v\n%s", err, output)
	}
	if result.Report.Status != sharedreport.StatusComplete {
		t.Fatalf("icot azure read-only result = %#v\n%s", result, output)
	}
	if result.Validation == nil || !result.Validation.Valid || result.Graph == nil || result.Plan == nil || result.Plan.Plan.Summary.Read != 1 {
		t.Fatalf("read-only gates missing: validation=%#v graph=%#v plan=%#v\n%s", result.Validation, result.Graph, result.Plan, output)
	}
	projectPath := filepath.Join(outDir, project.DefaultFile)
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		projectPath = filepath.Join(outDir, "project.uws.hcl")
	}
	projectText, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatalf("read-only azure project read failed: %v", err)
	}
	if !strings.Contains(string(projectText), "../azure/resources.json") || strings.Contains(string(projectText), sourcePath) {
		t.Fatalf("project did not preserve ../ source path semantics: %s", projectText)
	}
	doc, err := project.Load(projectPath)
	if err != nil {
		t.Fatalf("read-only azure project does not load: %v", err)
	}
	resource := doc.Profile.Resources[0]
	if len(resource.RequiredOperations) != 1 || resource.RequiredOperations[0] != "read" {
		t.Fatalf("read-only required operations = %#v", resource.RequiredOperations)
	}
	role, ok := resource.Operations["read"]
	if !ok {
		t.Fatalf("read operation role missing: %#v", resource.Operations)
	}
	if role.Method != "GET" || role.OperationID != "Resources_List" {
		t.Fatalf("read operation role = %#v", role)
	}
}

func TestCLIIcotDeleteOperationUsesDeleteRoleAndPlansDelete(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	mustWriteCLIFile(t, sourcePath, []byte(`openapi: 3.0.0
info:
  title: Delete API
  version: v1
paths:
  /widgets/{name}:
    delete:
      operationId: deleteWidget
      parameters:
        - name: name
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
`))
	outDir := filepath.Join(root, "delete")
	answersPath := filepath.Join(root, "answers.txt")
	mustWriteCLIFile(t, answersPath, []byte("deleteWidget\n"))
	cmd := helperCommand("icot", "--agent", "--no-llm", "--answers", answersPath, "--goal", "Delete widget named ramen", "--api-source", "openapi:widgets="+sourcePath, "--out", outDir, "--json", "--validate", "--graph", "--plan")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("delete icot failed: %v\nstdout:\n%s\nstderr:\n%s", err, output, stderr.String())
	}
	var result authorCLIResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("icot JSON is not parseable: %v\n%s", err, output)
	}
	if result.Plan == nil || result.Plan.Plan.Summary.Delete != 1 {
		t.Fatalf("delete plan = %#v\n%s", result.Plan, output)
	}
	doc, err := project.Load(filepath.Join(outDir, project.DefaultFile))
	if err != nil {
		t.Fatalf("delete project does not load: %v", err)
	}
	resource := doc.Profile.Resources[0]
	if len(resource.RequiredOperations) != 1 || resource.RequiredOperations[0] != "delete" {
		t.Fatalf("delete required operations = %#v", resource.RequiredOperations)
	}
	if role := resource.Operations["delete"]; role.Method != "DELETE" || role.OperationID != "deleteWidget" {
		t.Fatalf("delete operation role = %#v", role)
	}
}

func TestCLIIcotUnsupportedAPISourceKindNeedsInput(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	writeICOTOpenAPIForCLITest(t, sourcePath)
	outDir := filepath.Join(root, "unsupported")
	cmd := helperCommand("icot", "--agent", "--no-llm", "--goal", "Create a widget", "--api-source", "unknown:widgets="+sourcePath, "--out", outDir, "--json")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err == nil {
		t.Fatalf("unsupported source unexpectedly succeeded:\nstdout:\n%s\nstderr:\n%s", output, stderr.String())
	}
	var result authorCLIResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("icot JSON is not parseable: %v\n%s", err, output)
	}
	if result.Report.Status != sharedreport.StatusNeedsInput || !strings.Contains(string(output), "ramen.icot.api_source_unsupported") {
		t.Fatalf("unsupported source result = %#v\n%s", result, output)
	}
	if _, err := os.Stat(filepath.Join(outDir, project.DefaultFile)); !os.IsNotExist(err) {
		t.Fatalf("unsupported source wrote project: %v", err)
	}
}

type fakeICOTAssistant struct {
	called *bool
}

func (assistant fakeICOTAssistant) SuggestOperation(context.Context, icotAssistantRequest) (icotAssistantSuggestion, error) {
	*assistant.called = true
	return icotAssistantSuggestion{OperationID: "createWidget"}, nil
}

func TestRunICOTDraftUsesOptionalLLMAssistant(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	writeCreateOnlyICOTOpenAPIForCLITest(t, sourcePath)
	outDir := filepath.Join(root, "llm")
	called := false
	oldFactory := newICOTAssistant
	newICOTAssistant = func(flags sharedicotcli.Flags, env func(string) string) (icotAssistant, icotModelConfig, error) {
		return fakeICOTAssistant{called: &called}, icotModelConfig{Provider: "openai", Model: "gpt-test", Temperature: flags.Temperature}, nil
	}
	defer func() { newICOTAssistant = oldFactory }()
	result := runICOTDraft(context.Background(), "Create a widget", "", outDir, "", []string{"openapi:widgets=" + sourcePath}, false, false, false, sharedicotcli.Flags{Agent: true, Provider: "openai", Model: "gpt-test", Temperature: 0.2}, nil, io.Discard)
	if result.Report.Status != sharedreport.StatusComplete || !called {
		t.Fatalf("icot llm result = %#v called=%t", result, called)
	}
}

func TestRamenICOTProviderEnvDefaultsDoNotAutoSelectAPIKeys(t *testing.T) {
	env := func(key string) string {
		switch key {
		case "OPENAI_API_KEY":
			return "present"
		default:
			return ""
		}
	}
	if got := ramenICOTProviderName(sharedicotcli.Flags{}, env); got != "copilot-api" {
		t.Fatalf("provider with only OPENAI_API_KEY = %q, want copilot-api", got)
	}
	env = func(key string) string {
		switch key {
		case "RAMEN_LLM_PROVIDER":
			return "openai"
		case "RAMEN_LLM_MODEL":
			return "gpt-env"
		default:
			return ""
		}
	}
	if provider := ramenICOTProviderName(sharedicotcli.Flags{}, env); provider != "openai" {
		t.Fatalf("RAMEN_LLM_PROVIDER = %q", provider)
	}
	if model := ramenICOTModelName(sharedicotcli.Flags{}, env); model != "gpt-env" {
		t.Fatalf("RAMEN_LLM_MODEL = %q", model)
	}
}

func TestCLIIcotMissingInputsWritesNoProject(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "missing")
	cmd := helperCommand("icot", "--agent", "--no-llm", "--out", outDir, "--json")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("icot missing input unexpectedly succeeded:\n%s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("icot exit = %v, output:\n%s", err, output)
	}
	if !strings.Contains(string(output), "ramen.icot.missing_goal") {
		t.Fatalf("missing goal diagnostic absent:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(outDir, project.DefaultFile)); !os.IsNotExist(err) {
		t.Fatalf("icot wrote project despite missing input: %v", err)
	}
}

func TestPositionalFirstLast(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "empty", args: nil, want: nil},
		{name: "flags only", args: []string{"--state", "state.db"}, want: []string{"--state", "state.db"}},
		{name: "single positional", args: []string{"addr"}, want: []string{"addr"}},
		{name: "positional before flags", args: []string{"addr", "--json"}, want: []string{"--json", "addr"}},
		{name: "flag before positional unchanged", args: []string{"--json", "addr"}, want: []string{"--json", "addr"}},
	}
	for _, tt := range tests {
		got := positionalFirstLast(tt.args)
		if len(got) != len(tt.want) {
			t.Fatalf("%s length = %d, want %d (%#v)", tt.name, len(got), len(tt.want), got)
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Fatalf("%s[%d] = %q, want %q (%#v)", tt.name, i, got[i], tt.want[i], got)
			}
		}
	}
}

func TestCLIInitAndPlanHelpIncludesContracts(t *testing.T) {
	for _, tt := range []struct {
		command  []string
		expected []string
	}{
		{command: []string{"apply", "--help"}, expected: []string{"Usage: ramen apply", "--plan", "--config-dir", "--workspace", "--api-source", "--var-file", "--var", "--auto-approve", "--mock", "--executor", "--udon-output", "--json", "approved plan actions", "trusted executor"}},
		{command: []string{"import", "--help"}, expected: []string{"Usage: ramen import", "--config-dir", "--workspace", "--api-source", "--var-file", "--var", "--identity", "plan-compatible desired hash"}},
		{command: []string{"init", "--help"}, expected: []string{"Usage: ramen init", "--config-dir", "--state", "--workspace", "does not execute Terraform"}},
		{command: []string{"plan", "--help"}, expected: []string{"Usage: ramen plan", "--config-dir", "--workspace", "--api-source", "--var-file", "--var", "--policy-file", "--approved-by", "--approved-at", "--state", "--target", "--exclude", "--replace", "--out", "--hcl-out", "does not execute Terraform"}},
		{command: []string{"run", "--help"}, expected: []string{"Usage: ramen run", "--target", "--policy-file", "--check", "--approval-digest", "--auto-approve", "--mock", "trusted executor"}},
		{command: []string{"show", "--help"}, expected: []string{"Usage: ramen show", "--json", "without reading state"}},
		{command: []string{"state", "--help"}, expected: []string{"Usage: ramen state", "audit", "backup", "export", "list", "restore", "show ADDRESS", "history", "runs", "vacuum"}},
	} {
		cmd := helperCommand(tt.command...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v help failed: %v\n%s", tt.command, err, output)
		}
		text := string(output)
		for _, expected := range tt.expected {
			if !strings.Contains(text, expected) {
				t.Fatalf("%v help missing %q:\n%s", tt.command, expected, text)
			}
		}
		if tt.command[0] == "plan" && strings.Contains(text, "--destroy") {
			t.Fatalf("plan help still advertises deprecated --destroy flag:\n%s", text)
		}
	}
}

func TestCLIDestroyCommandRemoved(t *testing.T) {
	cmd := helperCommand("destroy", "--help")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("destroy command unexpectedly succeeded:\n%s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
		t.Fatalf("destroy exit = %v, output:\n%s", err, output)
	}
	if !strings.Contains(string(output), `unknown command "destroy"`) {
		t.Fatalf("destroy output missing unknown-command diagnostic:\n%s", output)
	}
}

func TestPlanHasChangesIncludesAPIActions(t *testing.T) {
	actions := []struct {
		name    string
		summary tfplan.Summary
	}{
		{name: "read", summary: tfplan.Summary{Read: 1}},
		{name: "post", summary: tfplan.Summary{Post: 1}},
		{name: "put", summary: tfplan.Summary{Put: 1}},
		{name: "patch", summary: tfplan.Summary{Patch: 1}},
	}
	for _, tt := range actions {
		if !planHasChanges(tfplan.Document{Summary: tt.summary}) {
			t.Fatalf("%s action should count as plan work", tt.name)
		}
	}
	if planHasChanges(tfplan.Document{Summary: tfplan.Summary{NoOp: 1}}) {
		t.Fatalf("no-op summary should not count as executable plan work")
	}
}

func TestCLIShowIncludesAPIMethodSummary(t *testing.T) {
	root := t.TempDir()
	planPath := filepath.Join(root, "method-plan.json")
	data, err := json.Marshal(tfplan.Document{
		Version: tfplan.Version,
		Action:  "post",
		Summary: tfplan.Summary{Post: 1, Put: 1, Patch: 1, Read: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustWriteCLIFile(t, planPath, data)
	cmd := helperCommand("show", planPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("show failed: %v\n%s", err, output)
	}
	for _, expected := range []string{"post=1", "put=1", "patch=1", "read=1"} {
		if !strings.Contains(string(output), expected) {
			t.Fatalf("show output missing %q:\n%s", expected, output)
		}
	}
}

func TestCLIImportReportsStableIdentityDiagnostics(t *testing.T) {
	for _, identity := range []string{"{", "null"} {
		cmd := helperCommand("import", "example_resource.test", "--type", "example_resource", "--identity", identity)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("import with identity %q unexpectedly succeeded:\n%s", identity, output)
		}
		if !strings.Contains(string(output), "import.identity_invalid") {
			t.Fatalf("import identity diagnostic missing stable code:\n%s", output)
		}
	}
}

func TestCLIRunCheckAndApprovedExecution(t *testing.T) {
	root := t.TempDir()
	docPath := writeNativeProjectForCLITest(t, root, project.Profile{Version: project.Version})
	statePath := filepath.Join(root, "run-state.db")
	cmd := helperCommand("run", docPath, "--target", "a", "--target", "b", "--state", statePath, "--check", "--json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run check failed: %v\n%s", err, output)
	}
	var preview ramenrun.Result
	if err := json.Unmarshal(output, &preview); err != nil || preview.Version != ramenrun.Version || !preview.Check || preview.Summary.Skipped != 2 || preview.ApprovalDigest == "" {
		t.Fatalf("preview invalid: %#v err=%v\n%s", preview, err, output)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("check mode created state: %v", err)
	}
	cmd = helperCommand("run", docPath, "--target", "a", "--target", "b", "--state", statePath, "--approval-digest", preview.ApprovalDigest, "--mock", "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("approved run failed: %v\n%s", err, output)
	}
	var executed ramenrun.Result
	if err := json.Unmarshal(output, &executed); err != nil || executed.RunID == 0 || executed.Summary.Executed != 2 {
		t.Fatalf("executed invalid: %#v err=%v\n%s", executed, err, output)
	}
}

func TestCLIVersionOutputsPlainTextJSONAndHelp(t *testing.T) {
	cmd := helperCommand("version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version failed: %v\n%s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != version {
		t.Fatalf("version output = %q, want %q", got, version)
	}

	cmd = helperCommand("version", "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version --json failed: %v\n%s", err, output)
	}
	var info versionInfo
	if err := json.Unmarshal(output, &info); err != nil {
		t.Fatalf("version JSON is not parseable: %v\n%s", err, output)
	}
	if info.Version != version {
		t.Fatalf("version JSON version = %q, want %q", info.Version, version)
	}
	if info.Module != "github.com/OpenUdon/ramen" {
		t.Fatalf("version JSON module = %q, want github.com/OpenUdon/ramen", info.Module)
	}

	cmd = helperCommand("version", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version help failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"Usage: ramen version", "--json", "does not check networks"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("version help missing %q:\n%s", expected, text)
		}
	}
}

func TestCLIValidateOutputsHumanJSONAndHelp(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	mustWriteCLIFile(t, sourcePath, []byte(`openapi: 3.0.0
info:
  title: Validate CLI
  version: v1
paths:
  /examples:
    post:
      operationId: createExample
      responses:
        "200":
          description: ok
`))
	projectPath := writeNativeProjectForCLITest(t, root, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: sourcePath}},
		Resources: []project.Resource{{
			Address:    "example_resource.test",
			Kind:       "resource",
			Type:       "example_resource",
			Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"}},
		}},
	})

	cmd := helperCommand("validate", "--project", projectPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validate failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "valid=true") {
		t.Fatalf("validate output missing success:\n%s", output)
	}

	cmd = helperCommand("validate", "--project", projectPath, "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validate --json failed: %v\n%s", err, output)
	}
	var result struct {
		Version string `json:"version"`
		Valid   bool   `json:"valid"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("validate JSON is not parseable: %v\n%s", err, output)
	}
	if result.Version != "ramen.validate.v1" || !result.Valid {
		t.Fatalf("validate JSON result = %#v\n%s", result, output)
	}

	cmd = helperCommand("validate", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validate help failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"Usage: ramen validate", "--project", "--api-source", "--json", "--strict", "without planning"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("validate help missing %q:\n%s", expected, text)
		}
	}

	missingPath := filepath.Join(root, "missing.yaml")
	badProjectPath := writeNativeProjectForCLITest(t, filepath.Join(root, "bad"), project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: missingPath}},
		Resources: []project.Resource{{
			Address:    "example_resource.test",
			Kind:       "resource",
			Type:       "example_resource",
			Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"}},
		}},
	})
	cmd = helperCommand("validate", "--project", badProjectPath, "--json")
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("validate unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), `"valid": false`) || !strings.Contains(string(output), "validate.api_source_document_read") {
		t.Fatalf("validate failure JSON missing diagnostics:\n%s", output)
	}
}

func TestCLIGraphOutputsDOTJSONAndOutFile(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	mustWriteCLIFile(t, sourcePath, []byte(`openapi: 3.0.0
info:
  title: Graph CLI
  version: v1
paths:
  /db:
    post:
      operationId: createDB
      responses:
        "200":
          description: ok
  /app:
    post:
      operationId: createApp
      responses:
        "200":
          description: ok
`))
	projectPath := writeNativeProjectForCLITest(t, root, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: sourcePath}},
		Resources: []project.Resource{
			{
				Address:      "example_resource.app",
				Kind:         "resource",
				Type:         "example_resource",
				Dependencies: []string{"example_resource.db"},
				Operations:   map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createApp"}},
			},
			{
				Address:    "example_resource.db",
				Kind:       "resource",
				Type:       "example_resource",
				Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createDB"}},
			},
		},
	})

	cmd := helperCommand("graph", "--project", projectPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("graph failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `digraph ramen`) || !strings.Contains(string(output), `"example_resource.db" -> "example_resource.app"`) {
		t.Fatalf("graph DOT missing expected edge:\n%s", output)
	}

	cmd = helperCommand("graph", "--project", projectPath, "--format", "json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("graph json failed: %v\n%s", err, output)
	}
	var result struct {
		Version string `json:"version"`
		Nodes   []struct {
			Address string `json:"address"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("graph JSON is not parseable: %v\n%s", err, output)
	}
	if result.Version != "ramen.graph.v1" || len(result.Nodes) != 2 || len(result.Edges) != 1 {
		t.Fatalf("graph JSON result = %#v\n%s", result, output)
	}

	outPath := filepath.Join(root, "graph.dot")
	cmd = helperCommand("graph", "--project", projectPath, "--out", outPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("graph --out failed: %v\n%s", err, output)
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("graph out file missing: %v", err)
	}
	if !strings.Contains(string(out), `"example_resource.db" -> "example_resource.app"`) {
		t.Fatalf("graph out file missing edge:\n%s", out)
	}
}

func TestCLIGraphReportsValidationDiagnostics(t *testing.T) {
	root := t.TempDir()
	projectPath := writeNativeProjectForCLITest(t, root, project.Profile{
		Version: project.Version,
		Resources: []project.Resource{
			{Address: "a", Kind: "resource", Type: "example", Dependencies: []string{"b"}, Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createA"}}},
			{Address: "b", Kind: "resource", Type: "example", Dependencies: []string{"a"}, Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createB"}}},
		},
	})
	cmd := helperCommand("graph", "--project", projectPath, "--format", "json")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("graph unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), `"diagnostics"`) || !strings.Contains(string(output), "validate.dependency_cycle") {
		t.Fatalf("graph JSON missing diagnostics:\n%s", output)
	}
}

func TestCLIForceUnlockRequiresExactHolder(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.db")
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.AcquireLock(context.Background(), "state", "holder-1", time.Minute); err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	_ = store.Close()

	cmd := helperCommand("force-unlock", "wrong-holder", "--state", statePath)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("force-unlock unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), "holder-1") || !strings.Contains(string(output), "wrong-holder") {
		t.Fatalf("force-unlock mismatch output missing holder detail:\n%s", output)
	}

	cmd = helperCommand("force-unlock", "holder-1", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("force-unlock failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "force-unlocked state held by holder-1") {
		t.Fatalf("force-unlock output missing summary:\n%s", output)
	}
	verify, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("reopen state: %v", err)
	}
	if lock, err := verify.CurrentLock(context.Background(), "state"); err != nil || lock != nil {
		t.Fatalf("lock after force unlock = %#v err=%v", lock, err)
	}
	_ = verify.Close()

	cmd = helperCommand("force-unlock", "holder-1", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("force-unlock missing lock unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), `state lock "state" is not held`) {
		t.Fatalf("force-unlock missing output:\n%s", output)
	}

	expired, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open expired state: %v", err)
	}
	if err := expired.AcquireLock(context.Background(), "state", "expired-holder", time.Nanosecond); err != nil {
		t.Fatalf("acquire expired lock: %v", err)
	}
	_ = expired.Close()
	time.Sleep(time.Millisecond)
	cmd = helperCommand("force-unlock", "expired-holder", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("force-unlock expired unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), `state lock "state" is not held`) {
		t.Fatalf("force-unlock expired output:\n%s", output)
	}

	cmd = helperCommand("force-unlock", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("force-unlock malformed args unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), "Usage: ramen force-unlock") {
		t.Fatalf("force-unlock malformed output missing usage:\n%s", output)
	}
}

func TestCLIApplyMockWritesState(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, "state.db")
	outDir := filepath.Join(root, "apply")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_iam_role" "role" {
  name = "cli-apply-role"
  assume_role_policy = "{}"
}
`))
	mustWriteCLIFile(t, sourcePath, []byte(`{
  "smithy": "2.0",
  "shapes": {
    "com.amazonaws.iam#IAM": {
      "type": "service",
      "version": "2010-05-08",
      "operations": [{"target": "com.amazonaws.iam#CreateRole"}],
      "traits": {
        "aws.api#service": {"sdkId": "IAM", "endpointPrefix": "iam"},
        "aws.auth#sigv4": {"name": "iam"},
        "aws.protocols#awsQuery": {}
      }
    },
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`))
	cmd := helperCommand("apply", "--config-dir", configDir, "--state", statePath, "--api-source", "aws-smithy:iam="+sourcePath, "--auto-approve", "--mock", "--out", outDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("apply failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "executed=1") {
		t.Fatalf("apply output missing summary:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(outDir, "actions", "aws_iam_role_role.uws.json")); err != nil {
		t.Fatalf("apply UWS not written: %v", err)
	}
	cmd = helperCommand("apply", "--config-dir", configDir, "--state", statePath, "--api-source", "aws-smithy:iam="+sourcePath, "--auto-approve", "--mock")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("second apply failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "no-op=1") || !strings.Contains(string(output), "executed=0") {
		t.Fatalf("second apply output missing no-op summary:\n%s", output)
	}
}

func TestCLIApplyPlanReplayUsesArtifactStatePath(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	planPath := filepath.Join(root, "read-plan.json")
	outDir := filepath.Join(root, "apply")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_iam_role" "role" {
  name = "cli-apply-replay-role"
  assume_role_policy = "{}"
}
`))
	mustWriteCLIFile(t, sourcePath, []byte(`{
  "smithy": "2.0",
  "shapes": {
    "com.amazonaws.iam#IAM": {
      "type": "service",
      "version": "2010-05-08",
      "operations": [{"target": "com.amazonaws.iam#CreateRole"}],
      "traits": {
        "aws.api#service": {"sdkId": "IAM", "endpointPrefix": "iam"},
        "aws.auth#sigv4": {"name": "iam"},
        "aws.protocols#awsQuery": {}
      }
    },
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}, "traits": {"aws.api#input": {}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`))
	cmd := helperCommand("plan", "--config-dir", configDir, "--state", filepath.Join(root, "plan-state.db"), "--api-source", "aws-smithy:iam="+sourcePath, "--out", planPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("plan for replay test failed: %v\n%s", err, output)
	}
	cmd = helperCommand("apply", "--plan", planPath, "--auto-approve", "--mock", "--out", outDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("apply plan replay failed: %v\n%s", err, output)
	}
	if strings.Contains(string(output), "apply.approval_mismatch") {
		t.Fatalf("apply plan replay used wrong default state path:\n%s", output)
	}
	if !strings.Contains(string(output), "executed=1") {
		t.Fatalf("apply output missing execution summary:\n%s", output)
	}
}

func TestCLIConvertWritesDraftArtifacts(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "openapi.yaml")
	outDir := filepath.Join(root, "out")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_instance" "web" {
  name = "web"
}
`))
	mustWriteCLIFile(t, openAPIPath, []byte(`openapi: 3.0.0
info:
  title: AWS Test
  version: v1
paths:
  /instances:
    post:
      operationId: createAwsInstance
      responses:
        "200":
          description: ok
`))
	cmd := helperCommand("convert", "--config-dir", configDir, "--openapi", "aws="+openAPIPath, "--action", "create", "--out", outDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "ramen: convert wrote") {
		t.Fatalf("convert output missing summary:\n%s", output)
	}
	for _, rel := range []string{"project.md", "project.uws.yaml", "workflows/workflow.uws.yaml", "expected/conversion.json", "expected/mappings.json", "expected/diagnostics.json", "expected/diagnostics.md", "expected/review.md"} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	planPath := filepath.Join(root, "native-plan.json")
	cmd = helperCommand("plan", "--project", outDir, "--state", filepath.Join(root, "state.db"), "--out", planPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native project plan failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "create=1") {
		t.Fatalf("native project plan output missing summary:\n%s", output)
	}
	cmd = helperCommand("convert", "tf", "--config-dir", configDir, "--openapi", "aws="+openAPIPath, "--action", "create", "--out", filepath.Join(root, "subcommand-out"))
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert tf subcommand failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(root, "subcommand-out", "workflows", "workflow.uws.yaml")); err != nil {
		t.Fatalf("convert tf subcommand missing UWS artifact: %v", err)
	}
}

func TestCLIConvertAnsibleWritesReviewArtifacts(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "ansible")
	playbookPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "nginx", "playbook.yml")
	argspecPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "argspec", "ansible-builtin.argspec.json")
	cmd := helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "builtin="+argspecPath,
		"--project-dir", root,
		"--roles-path", filepath.Join(root, "roles"),
		"--collections-path", filepath.Join(root, "collections"),
		"--inventory", filepath.Join(root, "inventory.ini"),
		"--extra-var", "env=test",
		"--ignore-unsupported",
		"--out", outDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert ansible failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"Converted playbook:", "UWS document:", "Diagnostics:", "Review:"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("convert ansible output missing %q:\n%s", expected, output)
		}
	}
	for _, rel := range []string{"workflows/workflow.uws.yaml", "workflows/workflow.hcl", "expected/diagnostics.json", "expected/diagnostics.md", "expected/review.md"} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	review, err := os.ReadFile(filepath.Join(outDir, "expected", "review.md"))
	if err != nil {
		t.Fatalf("read review: %v", err)
	}
	reviewText := string(review)
	for _, expected := range []string{"## Unsupported Gate", "Generated artifacts are static review scaffolding", "Project directory:", "Roles paths:", "Collections paths:", "Inventory inputs:", "Extra vars:", "env=test"} {
		if !strings.Contains(reviewText, expected) {
			t.Fatalf("review missing %q:\n%s", expected, review)
		}
	}
	if !strings.Contains(reviewText, filepath.Join(root, "roles")) {
		t.Fatalf("review missing expected sections:\n%s", review)
	}
}

func TestCLIConvertAnsibleTargetUWS15WritesCompatibilityDocument(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "ansible")
	playbookPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "nginx", "playbook.yml")
	argspecPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "argspec", "ansible-builtin.argspec.json")
	cmd := helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "builtin="+argspecPath,
		"--target-uws", "1.5",
		"--ignore-unsupported",
		"--out", outDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert ansible --target-uws 1.5 failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(filepath.Join(outDir, "workflows", "workflow.uws.yaml"))
	if err != nil {
		t.Fatalf("read UWS output: %v", err)
	}
	var doc uws1.Document
	if err := uwsconvert.UnmarshalYAML(data, &doc); err != nil {
		t.Fatalf("parse UWS output: %v", err)
	}
	if doc.UWS != "1.5.0" {
		t.Fatalf("uws = %q, want 1.5.0", doc.UWS)
	}
	if len(doc.SourceDescriptions) != 0 {
		t.Fatalf("compatibility document should not emit ansible source descriptions: %#v", doc.SourceDescriptions)
	}
	op := findOperationInDoc(&doc, "install_nginx")
	if op == nil || op.Extensions[uws1.ExtensionOperationProfile] != "uws.ansible-module-call.1.0" {
		t.Fatalf("install operation missing compatibility profile: %#v", op)
	}
}

func TestCLIConvertAnsibleRejectsInvalidTargetUWS(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "ansible")
	playbookPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "nginx", "playbook.yml")
	argspecPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "argspec", "ansible-builtin.argspec.json")
	cmd := helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "builtin="+argspecPath,
		"--target-uws", "1.4",
		"--out", outDir)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("convert ansible accepted invalid target:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 2 {
		t.Fatalf("convert ansible exit = %v, want code 2\n%s", err, output)
	}
	if !strings.Contains(string(output), "unsupported --target-uws") {
		t.Fatalf("invalid target output missing diagnostic:\n%s", output)
	}
}

func findOperationInDoc(doc *uws1.Document, operationID string) *uws1.Operation {
	for _, op := range doc.Operations {
		if op != nil && op.OperationID == operationID {
			return op
		}
	}
	return nil
}

func TestCLIConvertAnsibleUnsupportedExitsByDefault(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "ansible")
	playbookPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "tier3", "playbook.yml")
	argspecPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "argspec", "ansible-builtin.argspec.json")
	cmd := helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "builtin="+argspecPath,
		"--out", outDir)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("convert ansible unsupported unexpectedly succeeded:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 3 {
		t.Fatalf("convert ansible exit = %v, want code 3\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"unsupported Ansible features found", "ansible.jinja_unsupported", "rerun with --ignore-unsupported"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("unsupported output missing %q:\n%s", expected, output)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "workflows", "workflow.uws.yaml")); !os.IsNotExist(err) {
		t.Fatalf("unsupported conversion should not write workflow, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "expected", "diagnostics.json")); err != nil {
		t.Fatalf("unsupported conversion should write diagnostics: %v", err)
	}
}

func TestCLIConvertAnsibleArgspecIngestionFailureExitsOneWithoutArtifacts(t *testing.T) {
	root := t.TempDir()
	playbookPath := filepath.Join(root, "playbook.yml")
	argspecPath := filepath.Join(root, "argspec.json")
	outDir := filepath.Join(root, "out")
	if err := os.WriteFile(playbookPath, []byte(`- name: invalid argspec
  hosts: localhost
  tasks:
    - name: Safe task
      acme.tools.file:
        path: /tmp/safe
`), 0o644); err != nil {
		t.Fatalf("write playbook: %v", err)
	}
	if err := os.WriteFile(argspecPath, []byte(`{
  "argspec": "uws.ansible.1.0",
  "collection": "acme.tools",
  "modules": {
    "acme.tools.file": {
      "parameters": {"path": {"type": "path"}},
      "unknown": true
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("write argspec: %v", err)
	}

	cmd := helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "tools="+argspecPath,
		"--out", outDir)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("invalid argspec unexpectedly succeeded:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("convert ansible exit = %v, want code 1\n%s", err, output)
	}
	if !strings.Contains(string(output), "schema validation failed") {
		t.Fatalf("invalid argspec output missing schema failure:\n%s", output)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("argspec ingestion failure wrote conversion artifacts, stat err=%v", err)
	}
}

func TestCLIConvertAnsibleArgspecConflictExitsThreeAndPartialOutputOmitsTask(t *testing.T) {
	root := t.TempDir()
	playbookPath := filepath.Join(root, "playbook.yml")
	argspecPath := filepath.Join(root, "argspec.json")
	if err := os.WriteFile(playbookPath, []byte(`- name: aliases
  hosts: localhost
  tasks:
    - name: Conflicting task
      acme.tools.file:
        path: /tmp/one
        dest: /tmp/two
    - name: Safe task
      acme.tools.file:
        path: /tmp/safe
`), 0o644); err != nil {
		t.Fatalf("write playbook: %v", err)
	}
	if err := os.WriteFile(argspecPath, []byte(`{
  "argspec": "uws.ansible.1.0",
  "collection": "acme.tools",
  "modules": {
    "acme.tools.file": {
      "parameters": {
        "path": {"type": "path", "required": true, "aliases": ["dest"]}
      }
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("write argspec: %v", err)
	}

	strictOut := filepath.Join(root, "strict")
	cmd := helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "tools="+argspecPath,
		"--out", strictOut)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("conflicting aliases unexpectedly succeeded:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 3 {
		t.Fatalf("convert ansible exit = %v, want code 3\n%s", err, output)
	}
	if !strings.Contains(string(output), "ansible.argspec_violation") {
		t.Fatalf("strict output missing argspec diagnostic:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(strictOut, "workflows", "workflow.uws.yaml")); !os.IsNotExist(err) {
		t.Fatalf("strict conversion wrote workflow, stat err=%v", err)
	}

	partialOut := filepath.Join(root, "partial")
	cmd = helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "tools="+argspecPath,
		"--ignore-unsupported",
		"--out", partialOut)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("partial conversion failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(filepath.Join(partialOut, "workflows", "workflow.uws.yaml"))
	if err != nil {
		t.Fatalf("read partial workflow: %v", err)
	}
	var doc uws1.Document
	if err := uwsconvert.UnmarshalYAML(data, &doc); err != nil {
		t.Fatalf("parse partial workflow: %v", err)
	}
	if findOperationInDoc(&doc, "conflicting_task") != nil {
		t.Fatalf("conflicting task leaked into partial workflow: %#v", doc.Operations)
	}
	if findOperationInDoc(&doc, "safe_task") == nil {
		t.Fatalf("safe task missing from partial workflow: %#v", doc.Operations)
	}
}

func TestCLIInitAndPlanWritesStaticPlan(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	statePath := filepath.Join(root, "state.db")
	planPath := filepath.Join(root, "plan.json")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_iam_role" "role" {
  name = "cli-role"
  assume_role_policy = "{}"
}
`))
	mustWriteCLIFile(t, sourcePath, []byte(`{
  "smithy": "2.0",
  "shapes": {
    "com.amazonaws.iam#IAM": {
      "type": "service",
      "version": "2010-05-08",
      "operations": [{"target": "com.amazonaws.iam#CreateRole"}],
      "traits": {
        "aws.api#service": {"sdkId": "IAM", "endpointPrefix": "iam"},
        "aws.auth#sigv4": {"name": "iam"},
        "aws.protocols#awsQuery": {}
      }
    },
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`))
	cmd := helperCommand("init", "--config-dir", configDir, "--state", statePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state was not created: %v", err)
	}
	cmd = helperCommand("plan", "--config-dir", configDir, "--state", statePath, "--api-source", "aws-smithy:iam="+sourcePath, "--out", planPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plan failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "create=1") {
		t.Fatalf("plan output missing summary:\n%s", output)
	}
	if !strings.Contains(string(output), "plan-hcl:") {
		t.Fatalf("plan output missing HCL path:\n%s", output)
	}
	planText, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("plan JSON missing: %v", err)
	}
	for _, expected := range []string{`"version": "ramen.plan.v1"`, `"address": "aws_iam_role.role"`, `"operation_id": "CreateRole"`} {
		if !strings.Contains(string(planText), expected) {
			t.Fatalf("plan JSON missing %q:\n%s", expected, planText)
		}
	}
	planHCLPath := strings.TrimSuffix(planPath, filepath.Ext(planPath)) + ".hcl"
	planHCL, err := os.ReadFile(planHCLPath)
	if err != nil {
		t.Fatalf("plan HCL missing: %v", err)
	}
	for _, expected := range []string{`"ramen.plan.v1"`, `resource "aws_iam_role.role"`, `operation_id = "CreateRole"`} {
		if !strings.Contains(string(planHCL), expected) {
			t.Fatalf("plan HCL missing %q:\n%s", expected, planHCL)
		}
	}
}

func TestCLIShowAndStateInspectReadOnly(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.yaml")
	statePath := filepath.Join(root, "state.db")
	planPath := filepath.Join(root, "plan.json")
	mustWriteCLIFile(t, sourcePath, []byte(`openapi: 3.0.0
info:
  title: Show CLI
  version: v1
paths:
  /examples:
    post:
      operationId: createExample
      responses:
        "200":
          description: ok
`))
	projectPath := writeNativeProjectForCLITest(t, root, project.Profile{
		Version:    project.Version,
		APISources: []project.APISource{{Kind: "openapi", ID: "api", Path: sourcePath}},
		Resources: []project.Resource{{
			Address:    "example_resource.test",
			Kind:       "resource",
			Type:       "example_resource",
			Operations: map[string]project.OperationRole{"create": {SourceKind: "openapi", SourceID: "api", OperationID: "createExample"}},
		}},
	})
	cmd := helperCommand("plan", "--project", projectPath, "--state", statePath, "--out", planPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plan failed: %v\n%s", err, output)
	}
	cmd = helperCommand("show", planPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("show failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "ramen: show") || !strings.Contains(string(output), "approval:") {
		t.Fatalf("show output missing summary:\n%s", output)
	}
	cmd = helperCommand("show", planPath, "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("show --json failed: %v\n%s", err, output)
	}
	var shown tfplan.Document
	if err := json.Unmarshal(output, &shown); err != nil || shown.Version != tfplan.Version {
		t.Fatalf("show JSON invalid doc=%#v err=%v\n%s", shown, err, output)
	}
	badPath := filepath.Join(root, "bad.json")
	mustWriteCLIFile(t, badPath, []byte(`{"version":"bad"}`))
	cmd = helperCommand("show", badPath)
	output, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "show.plan_version_invalid") {
		t.Fatalf("bad show output: err=%v\n%s", err, output)
	}

	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	if err := store.RecordResource(context.Background(), state.ResourceSnapshot{Address: "example_resource.test", Type: "example_resource", DesiredHash: "hash", IdentityJSON: `{"id":"test"}`, Status: "managed"}); err != nil {
		t.Fatalf("record resource: %v", err)
	}
	if err := store.RecordRevision(context.Background(), state.Revision{ResourceAddress: "example_resource.test", Action: "import", AfterJSON: `{"status":"managed"}`}); err != nil {
		t.Fatalf("record revision: %v", err)
	}
	runID, err := store.StartRun(context.Background(), "apply")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := store.FinishRun(context.Background(), runID, "completed", `{"ok":true}`); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	_ = store.Close()
	cmd = helperCommand("state", "list", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state list failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "example_resource.test") {
		t.Fatalf("state list missing address:\n%s", output)
	}
	cmd = helperCommand("state", "runs", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state runs failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "apply") || !strings.Contains(string(output), "completed") {
		t.Fatalf("state runs missing run summary:\n%s", output)
	}
	cmd = helperCommand("state", "export", "--state", statePath, "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state export failed: %v\n%s", err, output)
	}
	var exported state.ExportDocument
	if err := json.Unmarshal(output, &exported); err != nil || exported.Version != state.ExportVersion || len(exported.Resources) != 1 || len(exported.Revisions) != 1 || len(exported.Runs) != 1 {
		t.Fatalf("state export JSON invalid export=%#v err=%v\n%s", exported, err, output)
	}
	cmd = helperCommand("state", "audit", "--state", statePath, "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state audit failed: %v\n%s", err, output)
	}
	var audit state.AuditDocument
	if err := json.Unmarshal(output, &audit); err != nil || audit.Version != state.AuditVersion || !strings.HasPrefix(audit.Digest, "sha256:") || audit.Counts["resources"] != 1 {
		t.Fatalf("state audit JSON invalid audit=%#v err=%v\n%s", audit, err, output)
	}
	backupPath := filepath.Join(root, "backup.db")
	cmd = helperCommand("state", "backup", "--state", statePath, "--out", backupPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state backup failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "state backup written") {
		t.Fatalf("state backup output missing summary:\n%s", output)
	}
	restorePath := filepath.Join(root, "restored.db")
	cmd = helperCommand("state", "restore", "--state", restorePath, "--from", backupPath)
	output, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "--force") {
		t.Fatalf("state restore without force output: err=%v\n%s", err, output)
	}
	cmd = helperCommand("state", "restore", "--state", restorePath, "--from", backupPath, "--force")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state restore failed: %v\n%s", err, output)
	}
	cmd = helperCommand("state", "vacuum", "--state", restorePath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state vacuum failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "state vacuum completed") {
		t.Fatalf("state vacuum output missing summary:\n%s", output)
	}
	cmd = helperCommand("state", "show", "example_resource.test", "--state", statePath, "--json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state show failed: %v\n%s", err, output)
	}
	var resource state.ResourceSnapshot
	if err := json.Unmarshal(output, &resource); err != nil || resource.Address != "example_resource.test" {
		t.Fatalf("state show JSON invalid resource=%#v err=%v\n%s", resource, err, output)
	}
	cmd = helperCommand("state", "history", "example_resource.test", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state history failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "action=import") {
		t.Fatalf("state history missing revision:\n%s", output)
	}
	cmd = helperCommand("state", "show", "missing.address", "--state", statePath)
	output, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "state.resource_not_found") {
		t.Fatalf("missing state show output: err=%v\n%s", err, output)
	}
}

func TestCLIInitValidatesStaticConfigBeforeCreatingState(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	statePath := filepath.Join(root, "state.db")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_iam_role" "role" {
  name = "broken"
`))
	cmd := helperCommand("init", "--config-dir", configDir, "--state", statePath)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("init succeeded unexpectedly:\n%s", output)
	}
	if _, statErr := os.Stat(statePath); !os.IsNotExist(statErr) {
		t.Fatalf("state should not be created after invalid init, stat err=%v", statErr)
	}
}

func TestCLIPlanErroredOutAndDetailedExitCode(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	missingSource := filepath.Join(root, "missing.json")
	planPath := filepath.Join(root, "plan.json")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_iam_role" "role" {
  name = "cli-role"
  assume_role_policy = "{}"
}
`))
	cmd := helperCommand("plan", "--config-dir", configDir, "--state", filepath.Join(root, "state.db"), "--api-source", "aws-smithy:iam="+missingSource, "--out", planPath, "--detailed-exitcode")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("plan succeeded unexpectedly:\n%s", output)
	}
	planText, readErr := os.ReadFile(planPath)
	if readErr != nil {
		t.Fatalf("errored plan JSON missing: %v\n%s", readErr, output)
	}
	if !strings.Contains(string(planText), `"errored": true`) || !strings.Contains(string(planText), `"resources": null`) {
		t.Fatalf("plan output should be non-actionable:\n%s", planText)
	}
	if !strings.Contains(string(output), "api_source.load_error") {
		t.Fatalf("plan output missing diagnostic:\n%s", output)
	}
}

func TestCLIPlanDetailedExitCodeReturnsTwoForChanges(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	sourcePath := filepath.Join(root, "iam.json")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_iam_role" "role" {
  name = "cli-role"
  assume_role_policy = "{}"
}
`))
	mustWriteCLIFile(t, sourcePath, []byte(`{
  "smithy": "2.0",
  "shapes": {
    "com.amazonaws.iam#IAM": {
      "type": "service",
      "version": "2010-05-08",
      "operations": [{"target": "com.amazonaws.iam#CreateRole"}],
      "traits": {
        "aws.api#service": {"sdkId": "IAM", "endpointPrefix": "iam"},
        "aws.auth#sigv4": {"name": "iam"},
        "aws.protocols#awsQuery": {}
      }
    },
    "com.amazonaws.iam#CreateRole": {"type": "operation", "input": {"target": "com.amazonaws.iam#CreateRoleRequest"}, "output": {"target": "com.amazonaws.iam#CreateRoleResponse"}},
    "com.amazonaws.iam#CreateRoleRequest": {"type": "structure", "members": {"RoleName": {"target": "com.amazonaws.iam#roleNameType"}, "AssumeRolePolicyDocument": {"target": "com.amazonaws.iam#policyDocumentType"}}, "traits": {"smithy.api#input": {}}},
    "com.amazonaws.iam#CreateRoleResponse": {"type": "structure", "members": {}},
    "com.amazonaws.iam#roleNameType": {"type": "string"},
    "com.amazonaws.iam#policyDocumentType": {"type": "string"}
  }
}`))
	cmd := helperCommand("plan", "--config-dir", configDir, "--state", filepath.Join(root, "state.db"), "--api-source", "aws-smithy:iam="+sourcePath, "--detailed-exitcode")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("plan should exit 2 for changes but succeeded:\n%s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
		t.Fatalf("plan exit = %v, output:\n%s", err, output)
	}
}

func TestCLIStateAsyncEvidenceJSON(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.db")
	store, err := state.Open(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	runID, err := store.StartRun(context.Background(), "apply")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := store.RecordAsyncEvidence(context.Background(), state.AsyncEvidenceRecord{
		RunID:           runID,
		ResourceAddress: "example.one",
		Action:          "create",
		OperationID:     "createOne",
		RecordKind:      "execution_request",
		Phase:           "submitted",
		EvidenceID:      "ev-cli",
		AttemptID:       "attempt-cli",
		Sequence:        1,
		RecordJSON:      `{"version":"evidence.async.execution-request.v1"}`,
	}); err != nil {
		t.Fatalf("record async evidence: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close state: %v", err)
	}

	cmd := helperCommand("state", "async-evidence", "--state", statePath, "--run", fmt.Sprint(runID), "--json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state async-evidence failed: %v\n%s", err, output)
	}
	var records []state.AsyncEvidenceRecord
	if err := json.Unmarshal(output, &records); err != nil {
		t.Fatalf("async evidence JSON parse: %v\n%s", err, output)
	}
	if len(records) != 1 || records[0].EvidenceID != "ev-cli" || records[0].RecordKind != "execution_request" {
		t.Fatalf("records = %#v", records)
	}
}

func TestCLIStateAsyncEvidenceDoesNotExposeResumeOrResubmit(t *testing.T) {
	for _, flag := range []string{"--resume", "--resubmit"} {
		cmd := helperCommand("state", "async-evidence", flag)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("state async-evidence %s unexpectedly succeeded:\n%s", flag, output)
		}
		if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
			t.Fatalf("state async-evidence %s exit = %v, output:\n%s", flag, err, output)
		}
		if !strings.Contains(string(output), strings.TrimPrefix(flag, "--")) {
			t.Fatalf("state async-evidence %s output missing rejected flag:\n%s", flag, output)
		}
	}

	cmd := helperCommand("state", "async-evidence", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("state async-evidence help failed: %v\n%s", err, output)
	}
	for _, forbidden := range []string{"--resume", "--resubmit"} {
		if strings.Contains(string(output), forbidden) {
			t.Fatalf("state async-evidence help advertises %s:\n%s", forbidden, output)
		}
	}
}

func helperCommand(args ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=TestHelperProcess", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	os.Args = append([]string{"ramen"}, args[1:]...)
	main()
	os.Exit(0)
}

func mustWriteCLIFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeAuthorContextForCLITest(t *testing.T, dir string, ctx sharedpromptcontext.Context) string {
	t.Helper()
	data, err := sharedpromptcontext.CanonicalJSON(ctx)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "context.json")
	mustWriteCLIFile(t, path, data)
	return path
}

func writeAuthorOpenAPIForCLITest(t *testing.T, path string) {
	t.Helper()
	mustWriteCLIFile(t, path, []byte(`openapi: 3.0.0
info:
  title: Author CLI Test
  version: v1
paths:
  /widgets:
    post:
      operationId: createWidget
      responses:
        "200":
          description: ok
`))
}

func writeICOTOpenAPIForCLITest(t *testing.T, path string) {
	t.Helper()
	mustWriteCLIFile(t, path, []byte(`openapi: 3.0.0
info:
  title: iCoT CLI Test
  version: v1
paths:
  /widgets:
    get:
      operationId: listWidgets
      responses:
        "200":
          description: ok
    post:
      operationId: createWidget
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name]
              properties:
                name:
                  type: string
      responses:
        "200":
          description: ok
`))
}

func writeCreateOnlyICOTOpenAPIForCLITest(t *testing.T, path string) {
	t.Helper()
	mustWriteCLIFile(t, path, []byte(`openapi: 3.0.0
info:
  title: iCoT CLI Test
  version: v1
paths:
  /widgets:
    post:
      operationId: createWidget
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name]
              properties:
                name:
                  type: string
      responses:
        "200":
          description: ok
`))
}

func writeLifecycleICOTOpenAPIForCLITest(t *testing.T, path string) {
	t.Helper()
	mustWriteCLIFile(t, path, []byte(`openapi: 3.0.0
info:
  title: iCoT Lifecycle CLI Test
  version: v1
paths:
  /widgets:
    post:
      operationId: createWidget
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name]
              properties:
                name:
                  type: string
      responses:
        "200":
          description: ok
  /widgets/{id}:
    get:
      operationId: getWidget
      parameters:
        - name: id
          in: path
          required: true
          schema: {type: string}
      responses:
        "200":
          description: ok
    patch:
      operationId: patchWidget
      parameters:
        - name: id
          in: path
          required: true
          schema: {type: string}
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
      responses:
        "200":
          description: ok
    delete:
      operationId: deleteWidget
      parameters:
        - name: id
          in: path
          required: true
          schema: {type: string}
      responses:
        "204":
          description: deleted
`))
}

func writeNativeProjectForCLITest(t *testing.T, dir string, profile project.Profile) string {
	t.Helper()
	doc := &uws1.Document{
		UWS: "1.4.0",
		Info: &uws1.Info{
			Title:   "cli_validate_fixture",
			Version: "1.0.0",
		},
		Operations: []*uws1.Operation{{
			OperationID: "review",
			Request:     map[string]any{"x-test": true},
			Extensions:  map[string]any{uws1.ExtensionOperationProfile: "ramen-cli-test"},
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID: "main",
			Type:       uws1.WorkflowTypeSequence,
			Steps: []*uws1.Step{{
				StepID:       "review",
				OperationRef: "review",
			}},
		}},
		Extensions: map[string]any{project.ExtensionKey: profile},
	}
	data, err := uwsconvert.MarshalJSONIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, project.DefaultJSON)
	mustWriteCLIFile(t, path, data)
	return path
}
