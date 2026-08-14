package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	sharedicotcli "github.com/OpenUdon/authoring/icotcli"
	sharedpromptcontext "github.com/OpenUdon/authoring/promptcontext"
	sharedreport "github.com/OpenUdon/authoring/report"
	ramenicot "github.com/OpenUdon/ramen/internal/icot"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	uwsconvert "github.com/OpenUdon/uws/convert"
)

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
	answersPath := filepath.Join(root, "answers.json")
	writeICOTV2Answers(t, answersPath, "createWidget", "approve", "approve")
	cmd := helperCommand("icot", "--no-llm", "--answers", answersPath, "--goal", "Create a widget", "--api-source", "openapi:widgets="+sourcePath, "--out", outDir, "--json")
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
	wantSourcePath := filepath.Join(outDir, "sources", "openapi", "widgets.yaml")
	if got := doc.Profile.APISources[0].Path; got != wantSourcePath {
		t.Fatalf("api source path = %q, want %q", got, wantSourcePath)
	}
	projectText, err := os.ReadFile(filepath.Join(outDir, project.DefaultFile))
	if err != nil {
		t.Fatalf("read generated icot project: %v", err)
	}
	if !strings.Contains(string(projectText), "sources/openapi/widgets.yaml") || strings.Contains(string(projectText), sourcePath) {
		t.Fatalf("generated project did not materialize its digest-bound API source:\n%s", projectText)
	}
	if _, err := os.Stat(filepath.Join(outDir, ".icot", "transcript.json")); err != nil {
		t.Fatalf("default transcript is not under .icot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, ".icot", "session.json")); !os.IsNotExist(err) {
		t.Fatalf("completed run retained obsolete default session: %v", err)
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
	if got := doc.Profile.Resources[0].Metadata["fallback_behavior"]; got != "fail closed and retain the last confirmed state" {
		t.Fatalf("fallback behavior = %#v", got)
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
	if result.Report.Status != sharedreport.StatusNeedsInput || !strings.Contains(string(output), "operation.seed") || len(result.Frontier) != 1 {
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
	answersPath := filepath.Join(root, "answers.json")
	writeICOTV2Answers(t, answersPath, "createWidget", "approve", "approve")
	cmd := helperCommand("icot", "--no-llm", "--answers", answersPath, "--goal", "Create, read, update, and delete a widget named ramen", "--api-source", "openapi:widgets="+sourcePath, "--out", outDir, "--json", "--validate")
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
	answersPath := filepath.Join(root, "answers.json")
	writeICOTV2Answers(t, answersPath, "approve")
	cmd := helperCommand("icot", "--no-llm", "--answers", answersPath, "--goal", "List all widgets", "--api-source", "openapi:widgets="+sourcePath, "--out", outDir, "--json", "--validate", "--graph", "--plan")
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
	answersPath := filepath.Join(root, "answers.json")
	writeICOTV2Answers(t, answersPath, "approve")
	cmd := helperCommand("icot", "--no-llm", "--answers", answersPath, "--goal", "List Azure resources in the selected subscription", "--api-source", "openapi:azure-resources="+sourcePath, "--out", outDir, "--json", "--validate", "--graph", "--plan")
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
	if !strings.Contains(string(projectText), "sources/openapi/azure-resources.json") || strings.Contains(string(projectText), sourcePath) {
		t.Fatalf("project did not use the materialized source path: %s", projectText)
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
	answersPath := filepath.Join(root, "answers.json")
	writeICOTV2Answers(t, answersPath, "approve", "approve")
	cmd := helperCommand("icot", "--no-llm", "--answers", answersPath, "--goal", "Delete widget named ramen", "--api-source", "openapi:widgets="+sourcePath, "--out", outDir, "--json", "--validate", "--graph", "--plan")
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
	if result.Report.Status != sharedreport.StatusNeedsInput || !strings.Contains(string(output), "ramen.icot.api_source_invalid") {
		t.Fatalf("unsupported source result = %#v\n%s", result, output)
	}
	if _, err := os.Stat(filepath.Join(outDir, project.DefaultFile)); !os.IsNotExist(err) {
		t.Fatalf("unsupported source wrote project: %v", err)
	}
}

func TestLoadICOTV2AnswersRejectsV1AndUnversionedInputs(t *testing.T) {
	for _, content := range []string{`{"version":"ramen.icot-answers.v1","input":"approve\n"}`, `{"input":"approve\n"}`} {
		path := filepath.Join(t.TempDir(), "answers.json")
		mustWriteCLIFile(t, path, []byte(content))
		_, err := loadICOTV2Answers(path)
		if err == nil || !strings.Contains(err.Error(), "v1 and unversioned inputs are not compatible") {
			t.Fatalf("answers error = %v", err)
		}
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
	if result.Report.Status != sharedreport.StatusNeedsInput || !called || result.Session == nil || result.Session.Metadata["llm_suggested_operation_id"] != "createWidget" {
		t.Fatalf("icot llm result = %#v called=%t", result, called)
	}
	if result.ProjectPath != "" {
		t.Fatalf("agent mode wrote project %q", result.ProjectPath)
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
	if !strings.Contains(string(output), "boundary.outcome") {
		t.Fatalf("missing goal diagnostic absent:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(outDir, project.DefaultFile)); !os.IsNotExist(err) {
		t.Fatalf("icot wrote project despite missing input: %v", err)
	}
}

func writeICOTV2Answers(t *testing.T, path string, answers ...string) {
	t.Helper()
	data, err := json.Marshal(ramenicot.AnswersFile{Version: ramenicot.AnswersVersion, Input: strings.Join(answers, "\n") + "\n"})
	if err != nil {
		t.Fatal(err)
	}
	mustWriteCLIFile(t, path, data)
}
