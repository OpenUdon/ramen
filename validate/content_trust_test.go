package validate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/project"
	uwstrust "github.com/OpenUdon/uws/contenttrust"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

type contentTrustResolverFunc func(context.Context, *uws1.Document, *uws1.Operation) (bool, uwstrust.OperationContract, error)

func (f contentTrustResolverFunc) ResolveOperation(ctx context.Context, doc *uws1.Document, operation *uws1.Operation) (bool, uwstrust.OperationContract, error) {
	return f(ctx, doc, operation)
}

func TestRunReportsBrowserContentTrustAdvisoryAndStrictPromotion(t *testing.T) {
	root := t.TempDir()
	projectPath := writeValidateBrowserProject(t, root, "read_status")
	enableBrowserContentTrust(t, projectPath)

	result, err := Run(context.Background(), Options{ProjectPath: projectPath})
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := diagnosticWithCode(result, uwstrust.CodeUntrustedControl)
	if diagnostic == nil {
		t.Fatalf("missing content-trust diagnostic: %#v", result.Diagnostics)
	}
	if !result.Valid || result.Summary.Warnings != 1 || diagnostic.Severity != "warning" {
		t.Fatalf("default result = %#v", result)
	}
	if diagnostic.Path != "workflows[0].steps[1].when" || diagnostic.Message != "untrusted content can influence control flow" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}

	strict, err := Run(context.Background(), Options{ProjectPath: projectPath, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	diagnostic = diagnosticWithCode(strict, uwstrust.CodeUntrustedControl)
	if diagnostic == nil || strict.Valid || strict.Summary.Errors != 1 || diagnostic.Severity != "error" {
		t.Fatalf("strict result = %#v", strict)
	}
}

func TestContentTrustBrowserResolverFailureIsFixedAndRedacted(t *testing.T) {
	const privatePath = "private-customer-profile.yaml"
	doc := &project.Document{
		Dir: t.TempDir(),
		UWS: &uws1.Document{
			UWS:  "1.9.1",
			Info: &uws1.Info{Title: "resolver failure", Version: "1.0.0"},
			SourceDescriptions: []*uws1.SourceDescription{{
				Name: "browser", Type: uws1.SourceDescriptionTypeBrowserProfile, URL: privatePath,
			}},
			Operations: []*uws1.Operation{{
				OperationID: "read", SourceDescription: "browser", SourceOperationID: "read_status",
			}},
			ContentTrust: &uws1.ContentTrust{SourceDescriptions: map[string]uws1.ContentTrustLevel{
				"browser": uws1.ContentTrustUntrusted,
			}},
		},
	}
	diagnostics, err := contentTrustDiagnostics(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := diagnosticInSlice(diagnostics, uwstrust.CodeResolverFailure)
	if diagnostic == nil || diagnostic.Message != contentTrustAnalysisUnavailableMessage {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	encoded, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), privatePath) {
		t.Fatalf("diagnostics exposed a contained profile path: %s", encoded)
	}
}

func TestAnalyzeContentTrustResolverBoundariesAreDeterministicAndRedacted(t *testing.T) {
	const privateValue = "secret-${trigger.prompt}"
	doc := contentTrustAnalysisProject(privateValue)
	failure := contentTrustResolverFunc(func(context.Context, *uws1.Document, *uws1.Operation) (bool, uwstrust.OperationContract, error) {
		return true, uwstrust.OperationContract{}, errors.New("private resolver detail")
	})
	report, err := analyzeContentTrust(context.Background(), doc, failure)
	if err != nil {
		t.Fatal(err)
	}
	if !reportHasFinding(report, uwstrust.CodeResolverFailure) {
		t.Fatalf("failure report = %#v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), privateValue) || strings.Contains(string(encoded), "private resolver detail") {
		t.Fatalf("report exposed document or resolver content: %s", encoded)
	}

	claim := contentTrustResolverFunc(func(context.Context, *uws1.Document, *uws1.Operation) (bool, uwstrust.OperationContract, error) {
		return true, uwstrust.OperationContract{
			Inputs: []uwstrust.InputChannel{{Path: "/request/body/prompt", Kind: uwstrust.ChannelInstruction}},
		}, nil
	})
	conflict, err := analyzeContentTrust(context.Background(), doc, claim, claim)
	if err != nil {
		t.Fatal(err)
	}
	if !reportHasFinding(conflict, uwstrust.CodeResolverConflict) {
		t.Fatalf("conflict report = %#v", conflict)
	}

	first, err := analyzeContentTrust(context.Background(), doc, claim)
	if err != nil {
		t.Fatal(err)
	}
	second, err := analyzeContentTrust(context.Background(), doc, claim)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("reports differ:\nfirst: %#v\nsecond: %#v", first, second)
	}
	if !reportHasFinding(first, uwstrust.CodeOpaqueExpression) {
		t.Fatalf("opaque report = %#v", first)
	}
}

func TestAnalyzeContentTrustHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := analyzeContentTrust(ctx, contentTrustAnalysisProject("$trigger.prompt")); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if result, err := Run(ctx, Options{ProjectPath: "unused"}); result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() = %#v, %v; want cancellation", result, err)
	}
}

func enableBrowserContentTrust(t *testing.T, projectPath string) {
	t.Helper()
	data, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc uws1.Document
	if err := uwsconvert.UnmarshalJSON(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc.UWS = "1.9.1"
	doc.Operations[0].Outputs = map[string]string{"count": "$response.body#/count"}
	doc.Workflows[0].Steps[0].Outputs = map[string]string{"count": "$outputs.count"}
	doc.Workflows[0].Steps = append(doc.Workflows[0].Steps, &uws1.Step{
		StepID: "check", OperationRef: "read_status_uws",
		StepExecutionFields: uws1.StepExecutionFields{When: "$steps.read.outputs.count == 1"},
	})
	doc.ContentTrust = &uws1.ContentTrust{SourceDescriptions: map[string]uws1.ContentTrustLevel{
		"browser": uws1.ContentTrustUntrusted,
	}}
	data, err = uwsconvert.MarshalJSONIndent(&doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func contentTrustAnalysisProject(value string) *project.Document {
	return &project.Document{UWS: &uws1.Document{
		UWS:  "1.9.1",
		Info: &uws1.Info{Title: "resolver boundary", Version: "1.0.0"},
		Operations: []*uws1.Operation{{
			OperationID: "extension", Request: map[string]any{"body": map[string]any{"prompt": value}},
			Extensions: map[string]any{uws1.ExtensionOperationProfile: "test.extension.1"},
		}},
	}}
}

func diagnosticWithCode(result *Result, code string) *Diagnostic {
	if result == nil {
		return nil
	}
	return diagnosticInSlice(result.Diagnostics, code)
}

func diagnosticInSlice(diagnostics []Diagnostic, code string) *Diagnostic {
	for i := range diagnostics {
		if diagnostics[i].Code == code {
			return &diagnostics[i]
		}
	}
	return nil
}

func reportHasFinding(report *uwstrust.Report, code string) bool {
	if report == nil {
		return false
	}
	for _, finding := range report.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
