package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"
	"time"

	sharedicotcli "github.com/OpenUdon/authoring/icotcli"
	sharedpromptcontext "github.com/OpenUdon/authoring/promptcontext"
	sharedreadiness "github.com/OpenUdon/authoring/readiness"
	sharedreport "github.com/OpenUdon/authoring/report"
	"github.com/OpenUdon/authoring/trust"
	tfapply "github.com/OpenUdon/ramen/apply"
	ramenauthoring "github.com/OpenUdon/ramen/authoring"
	"github.com/OpenUdon/ramen/executor"
	"github.com/OpenUdon/ramen/governance"
	"github.com/OpenUdon/ramen/graph"
	"github.com/OpenUdon/ramen/internal/ansibleconvert"
	"github.com/OpenUdon/ramen/internal/tfconvert"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/reconcile"
	ramenrun "github.com/OpenUdon/ramen/run"
	"github.com/OpenUdon/ramen/state"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
	"github.com/OpenUdon/tfconfig"
)

// version is replaced in release archives with -ldflags. Module-installed
// binaries fall back to debug.BuildInfo's main module version.
var version = "devel"

const (
	defaultICOTLLMProvider  = "copilot-api"
	defaultICOTCopilotModel = "gpt-5.4-mini"
)

type repeatedStringFlag []string

func (f *repeatedStringFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *repeatedStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: ramen <command>\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Commands:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  apply     execute approved plans through a trusted executor\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  author    draft a native UWS/Ramen project from prompt-safe API context\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  convert   generate Ramen review scaffolding from supported source formats\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  force-unlock release a local Ramen state lock by exact holder token\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  graph     emit the native resource dependency graph\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  icot      interactively draft a native UWS/Ramen project from local API metadata\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  import    attach an existing resource identity to state\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  init      create or migrate local Ramen state\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  plan      emit a static desired-state plan without mutation\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  refresh   read tracked resources and update state through a trusted executor\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  run       execute approved imperative UWS runbooks through a trusted executor\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  show      inspect Ramen plan and approval artifacts\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  state     inspect local Ramen state\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  validate  validate a native UWS/Ramen project without mutation\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  version   print version\n")
	}
	flag.Parse()

	command := "help"
	if flag.NArg() > 0 {
		command = flag.Arg(0)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch command {
	case "apply":
		runApplyCommand(ctx, flag.Args()[1:])
	case "author":
		runAuthorCommand(ctx, flag.Args()[1:])
	case "convert":
		runConvertCommand(ctx, flag.Args()[1:])
	case "force-unlock":
		runForceUnlockCommand(ctx, flag.Args()[1:])
	case "graph":
		runGraphCommand(ctx, flag.Args()[1:])
	case "icot":
		runICOTCommand(ctx, flag.Args()[1:])
	case "import":
		runImportCommand(ctx, flag.Args()[1:])
	case "init":
		runInitCommand(ctx, flag.Args()[1:])
	case "plan":
		runPlanCommand(ctx, flag.Args()[1:])
	case "refresh":
		runRefreshCommand(ctx, flag.Args()[1:])
	case "run":
		runRunCommand(ctx, flag.Args()[1:])
	case "show":
		runShowCommand(flag.Args()[1:])
	case "state":
		runStateCommand(ctx, flag.Args()[1:])
	case "validate":
		runValidateCommand(ctx, flag.Args()[1:])
	case "version":
		runVersionCommand(flag.Args()[1:])
	case "-h", "--help", "help":
		flag.Usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", command)
		flag.Usage()
		os.Exit(2)
	}
}

type authorCLIResult struct {
	Report         sharedreport.Result   `json:"report"`
	ProjectPath    string                `json:"project_path,omitempty"`
	ProjectHCLPath string                `json:"project_hcl_path,omitempty"`
	Validation     *ramenvalidate.Result `json:"validation,omitempty"`
	Graph          *graph.Document       `json:"graph,omitempty"`
	Plan           *tfplan.Result        `json:"plan,omitempty"`
}

func runAuthorCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("author", flag.ExitOnError)
	contextPath := fs.String("context", "", "authoring.prompt-context.v1 JSON input path")
	goal := fs.String("goal", "", "Desired-state project goal; omitted goals return needs_input")
	projectName := fs.String("project-name", "", "Optional generated project name")
	outDir := fs.String("out", "./.ramen/author", "Output directory for project.uws.yaml")
	validateGate := fs.Bool("validate", false, "Run native project validation after drafting")
	graphGate := fs.Bool("graph", false, "Build the native dependency graph after drafting")
	planGate := fs.Bool("plan", false, "Build a non-mutating desired-state plan after drafting")
	statePath := fs.String("state", "", "Optional SQLite state path for --plan")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen author --context context.json [--goal TEXT] [--project-name NAME] [--out DIR] [--validate] [--graph] [--plan] [--state PATH] [--json]\n")
		fmt.Fprintf(fs.Output(), "\nDrafts a native UWS/Ramen project from prompt-safe API operation context. It is noninteractive, provider-free, and does not execute Terraform/OpenTofu, providers, API source operations, refresh, apply, or UWS workflows.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 || strings.TrimSpace(*contextPath) == "" {
		fs.Usage()
		os.Exit(2)
	}
	promptContext, err := loadPromptContextFile(*contextPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := ramenauthoring.DraftProject(ctx, ramenauthoring.Options{
		Goal:        *goal,
		ProjectName: *projectName,
		OutDir:      *outDir,
		Context:     promptContext,
		Validate:    *validateGate,
		Graph:       *graphGate,
		Plan:        *planGate,
		StatePath:   *statePath,
	})
	cliResult := authorCLIResult{
		Report:         result.Report,
		ProjectPath:    result.ProjectPath,
		ProjectHCLPath: result.ProjectHCLPath,
		Validation:     result.Validation,
		Graph:          result.Graph,
		Plan:           result.Plan,
	}
	if *jsonOut {
		writeJSONOutput(cliResult)
		printAuthorDiagnostics(cliResult.Report)
	} else {
		printAuthorHuman(cliResult)
		printAuthorDiagnostics(cliResult.Report)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cliResult.Report.Status != sharedreport.StatusComplete {
		os.Exit(1)
	}
}

func loadPromptContextFile(path string) (sharedpromptcontext.Context, error) {
	path = strings.TrimSpace(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return sharedpromptcontext.Context{}, fmt.Errorf("author.context_read_error: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	var ctx sharedpromptcontext.Context
	if err := decoder.Decode(&ctx); err != nil {
		return sharedpromptcontext.Context{}, fmt.Errorf("author.context_parse_error: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return sharedpromptcontext.Context{}, fmt.Errorf("author.context_parse_error: context file must contain one JSON document")
		}
		return sharedpromptcontext.Context{}, fmt.Errorf("author.context_parse_error: %w", err)
	}
	if ctx.Version != "" && ctx.Version != sharedpromptcontext.Version {
		return sharedpromptcontext.Context{}, fmt.Errorf("author.context_version_invalid: got %q, want %q", ctx.Version, sharedpromptcontext.Version)
	}
	return sharedpromptcontext.Normalize(ctx), nil
}

func printAuthorHuman(result authorCLIResult) {
	fmt.Printf("ramen: author status=%s\n", result.Report.Status)
	if result.ProjectPath != "" {
		fmt.Printf("  project: %s\n", result.ProjectPath)
	}
	if result.ProjectHCLPath != "" {
		fmt.Printf("  project-hcl: %s\n", result.ProjectHCLPath)
	}
	if result.Validation != nil {
		fmt.Printf("  validate: valid=%t errors=%d warnings=%d diagnostics=%d\n", result.Validation.Valid, result.Validation.Summary.Errors, result.Validation.Summary.Warnings, result.Validation.Summary.Diagnostics)
	}
	if result.Graph != nil {
		fmt.Printf("  graph: nodes=%d edges=%d diagnostics=%d\n", len(result.Graph.Nodes), len(result.Graph.Edges), len(result.Graph.Diagnostics))
	}
	if result.Plan != nil {
		summary := result.Plan.Plan.Summary
		fmt.Printf("  plan: create=%d update=%d delete=%d post=%d put=%d patch=%d replace=%d no-op=%d diagnostics=%d\n", summary.Create, summary.Update, summary.Delete, summary.Post, summary.Put, summary.Patch, summary.Replace, summary.NoOp, summary.Diagnostics)
	}
}

func printAuthorDiagnostics(report sharedreport.Result) {
	if report.TopIssue != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", report.TopIssue.Code, report.TopIssue.Message)
	}
	for _, diag := range report.Diagnostics {
		fmt.Fprintf(os.Stderr, "%s: %s\n", diag.Code, diag.Message)
	}
}

func runICOTCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("icot", flag.ExitOnError)
	goal := fs.String("goal", "", "Desired-state project goal")
	projectName := fs.String("project-name", "", "Optional generated project name")
	outDir := fs.String("out", "./.ramen/icot", "Output directory for project.uws.yaml")
	validateGate := fs.Bool("validate", false, "Run native project validation after drafting")
	graphGate := fs.Bool("graph", false, "Build the native dependency graph after drafting")
	planGate := fs.Bool("plan", false, "Build a non-mutating desired-state plan after drafting")
	statePath := fs.String("state", "", "Optional SQLite state path for --plan")
	var apiSources repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH")
	common := sharedicotcli.Flags{NoLLM: true, PromptMode: "normal"}
	sharedicotcli.AddFlags(fs, &common)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen icot [--goal TEXT] [--api-source KIND:ID=PATH ...] [--project-name NAME] [--out DIR] [--validate] [--graph] [--plan] [--state PATH] [--prompt-mode full|normal|fast] [--no-llm] [--provider NAME] [--model NAME] [--temperature FLOAT] [--agent] [--json] [--answers PATH] [--no-transcript] [--report PATH]\n")
		fmt.Fprintf(fs.Output(), "\nInteractively drafts a native UWS/Ramen project from local API source metadata. It never executes API calls, Terraform/OpenTofu, providers, refresh, apply, or UWS workflows.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	if _, err := sharedicotcli.PromptDefaultMode(common.PromptMode); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	answerInput, err := loadICOTAnswers(common.Answers)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	promptOut := io.Writer(os.Stdout)
	if common.JSON {
		promptOut = os.Stderr
	}
	var reader io.Reader
	if answerInput != "" {
		reader = strings.NewReader(answerInput)
	} else if !common.Agent {
		reader = os.Stdin
	}
	result := runICOTDraft(ctx, *goal, *projectName, *outDir, *statePath, []string(apiSources), *validateGate, *graphGate, *planGate, common, reader, promptOut)
	if common.Report != "" {
		if err := writeJSONFile(common.Report, result); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if common.JSON {
		writeJSONOutput(result)
	} else {
		printICOTHuman(result)
		printAuthorDiagnostics(result.Report)
	}
	if result.Report.Status != sharedreport.StatusComplete {
		os.Exit(1)
	}
}

func runICOTDraft(ctx context.Context, goal, projectName, outDir, statePath string, sourceFlags []string, validateGate, graphGate, planGate bool, common sharedicotcli.Flags, answerInput io.Reader, promptOut io.Writer) authorCLIResult {
	prompts := newCLIAnswers(answerInput, promptOut)
	goal = strings.TrimSpace(goal)
	if goal == "" && !common.Agent {
		answer, ok := prompts.ask("Ramen goal")
		if !ok {
			return icotNeedsInput("ramen.icot.missing_goal", "Describe the Ramen desired-state project goal.", "goal")
		}
		goal = strings.TrimSpace(answer)
	}
	if goal == "" {
		return icotNeedsInput("ramen.icot.missing_goal", "Describe the Ramen desired-state project goal.", "goal")
	}
	if len(sourceFlags) == 0 && !common.Agent {
		answer, ok := prompts.ask("API source (KIND:ID=PATH)")
		if !ok {
			return icotNeedsInput("ramen.icot.missing_api_source", "Provide a local API source as KIND:ID=PATH.", "api_source")
		}
		sourceFlags = append(sourceFlags, answer)
	}
	if len(sourceFlags) == 0 {
		return icotNeedsInput("ramen.icot.missing_api_source", "Provide a local API source as KIND:ID=PATH.", "api_source")
	}
	tfInputs, err := parseAPISourceFlags(sourceFlags)
	if err != nil {
		return icotFailed("ramen.icot.api_source_invalid", err.Error())
	}
	apiInputs := make([]ramenauthoring.APISourceInput, len(tfInputs))
	for i, input := range tfInputs {
		apiInputs[i] = ramenauthoring.APISourceInput{Kind: input.Kind, ID: input.ID, Path: input.Path, DownloadDir: outDir}
	}
	promptContext, err := ramenauthoring.PromptContextFromAPISources(ctx, goal, apiInputs)
	if err != nil {
		var inputErr ramenauthoring.APISourceInputError
		if errors.As(err, &inputErr) {
			return icotNeedsInputWithDetail(firstNonEmpty(inputErr.Code, "ramen.icot.api_source_invalid"), err.Error(), "api_source", map[string]any{
				"kind": inputErr.Kind,
				"id":   inputErr.ID,
				"path": inputErr.Path,
			})
		}
		return icotFailed("ramen.icot.api_source_load_error", err.Error())
	}
	if len(promptContext.Operations) == 0 {
		return icotNeedsInput("ramen.icot.missing_operation", "No operation candidates were found in the local API source metadata.", "operation")
	}
	suggestedOperationID := ""
	if !common.NoLLM {
		advisor, config, err := newICOTAssistant(common, os.Getenv)
		if err != nil {
			return icotNeedsInputWithDetail("ramen.icot.llm_unavailable", err.Error(), "llm", map[string]any{
				"provider": config.Provider,
				"model":    config.Model,
			})
		}
		if advisor != nil {
			suggestion, err := advisor.SuggestOperation(ctx, icotAssistantRequest{
				Goal:        goal,
				Context:     promptContext,
				Provider:    config.Provider,
				Model:       config.Model,
				Temperature: config.Temperature,
			})
			if err != nil {
				return icotNeedsInputWithDetail("ramen.icot.llm_unavailable", err.Error(), "llm", map[string]any{
					"provider": config.Provider,
					"model":    config.Model,
				})
			}
			suggestedOperationID = suggestion.OperationID
			if promptContext.Metadata == nil {
				promptContext.Metadata = map[string]string{}
			}
			promptContext.Metadata["llm_provider"] = config.Provider
			promptContext.Metadata["llm_model"] = config.Model
			if suggestedOperationID != "" {
				promptContext.Metadata["llm_suggested_operation_id"] = suggestedOperationID
			}
		}
	}
	selection := chooseICOTOperation(goal, promptContext, prompts, common.Agent, suggestedOperationID)
	if !selection.OK {
		if selection.Ambiguous {
			return icotNeedsInputWithDetail("ramen.icot.operation_ambiguous", "Choose one listed operation ID from the local API source metadata.", "operation", map[string]any{
				"candidates": selection.Candidates,
				"suggested":  suggestedOperationID,
			})
		}
		return icotNeedsInputWithDetail("ramen.icot.missing_operation", "Choose one listed operation ID from the local API source metadata.", "operation", map[string]any{
			"candidates": selection.Candidates,
			"suggested":  suggestedOperationID,
		})
	}
	operation := selection.Operation
	resources := []project.Resource{ramenauthoring.APILifecycleResource(promptContext, operation, goal, projectName)}
	result, err := ramenauthoring.DraftProject(ctx, ramenauthoring.Options{
		Goal:        goal,
		ProjectName: projectName,
		OutDir:      outDir,
		Context:     promptContext,
		Resources:   resources,
		Validate:    validateGate,
		Graph:       graphGate,
		Plan:        planGate,
		StatePath:   statePath,
	})
	if err != nil {
		return icotFailed("ramen.icot.draft_failed", err.Error())
	}
	cliResult := authorCLIResult{Report: result.Report, ProjectPath: result.ProjectPath, ProjectHCLPath: result.ProjectHCLPath, Validation: result.Validation, Graph: result.Graph, Plan: result.Plan}
	return cliResult
}

type cliAnswers struct {
	reader *bufio.Reader
	out    io.Writer
}

func newCLIAnswers(in io.Reader, out io.Writer) *cliAnswers {
	if out == nil {
		out = io.Discard
	}
	if in == nil {
		return &cliAnswers{out: out}
	}
	return &cliAnswers{reader: bufio.NewReader(in), out: out}
}

func (answers *cliAnswers) ask(label string) (string, bool) {
	if answers == nil || answers.reader == nil {
		return "", false
	}
	fmt.Fprintf(answers.out, "%s: ", label)
	line, err := answers.reader.ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	return strings.TrimRight(line, "\r\n"), true
}

func (answers *cliAnswers) askDefault(label, current string) (string, bool) {
	if answers == nil || answers.reader == nil {
		return "", false
	}
	current = strings.TrimSpace(current)
	if current != "" {
		fmt.Fprintf(answers.out, "%s [%s]: ", label, current)
	} else {
		fmt.Fprintf(answers.out, "%s: ", label)
	}
	line, err := answers.reader.ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	answer := strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(answer) == "" {
		return current, current != ""
	}
	return answer, true
}

type icotOperationSelection struct {
	Operation  sharedpromptcontext.OperationCandidate
	OK         bool
	Ambiguous  bool
	Candidates []string
}

func chooseICOTOperation(goal string, ctx sharedpromptcontext.Context, prompts *cliAnswers, agent bool, suggestedOperationID string) icotOperationSelection {
	ctx = sharedpromptcontext.Normalize(ctx)
	if len(ctx.Operations) == 1 {
		return icotOperationSelection{Operation: ctx.Operations[0], OK: true}
	}
	if op, ok := exactICOTOperationMatch(goal, ctx.Operations); ok {
		return icotOperationSelection{Operation: op, OK: true}
	}
	ranked := rankICOTOperations(goal, ctx.Operations)
	candidates := icotOperationIDs(ranked)
	if len(ranked) == 0 {
		return icotOperationSelection{}
	}
	if len(ranked) == 1 {
		return icotOperationSelection{Operation: ranked[0], OK: true, Candidates: candidates}
	}
	fmt.Fprintln(prompts.out, "Operation candidates:")
	for _, op := range ranked {
		fmt.Fprintf(prompts.out, "  %s %s %s\n", firstNonEmpty(op.OperationID, op.ID), strings.ToUpper(op.Verb), op.Path)
	}
	if agent && (prompts == nil || prompts.reader == nil) {
		return icotOperationSelection{Ambiguous: true, Candidates: candidates}
	}
	if suggestedOperationID != "" {
		if _, ok := operationByID(ranked, suggestedOperationID); !ok {
			suggestedOperationID = ""
		}
	}
	answer, ok := prompts.askDefault("Operation ID", suggestedOperationID)
	if !ok || strings.TrimSpace(answer) == "" {
		return icotOperationSelection{Ambiguous: true, Candidates: candidates}
	}
	answer = strings.TrimSpace(answer)
	if op, ok := operationByID(ctx.Operations, answer); ok {
		return icotOperationSelection{Operation: op, OK: true, Candidates: candidates}
	}
	return icotOperationSelection{Candidates: candidates}
}

func rankICOTOperations(goal string, operations []sharedpromptcontext.OperationCandidate) []sharedpromptcontext.OperationCandidate {
	goal = strings.ToLower(goal)
	out := append([]sharedpromptcontext.OperationCandidate(nil), operations...)
	slices.SortStableFunc(out, func(a, b sharedpromptcontext.OperationCandidate) int {
		left := icotOperationScore(goal, a)
		right := icotOperationScore(goal, b)
		if left != right {
			return right - left
		}
		return strings.Compare(firstNonEmpty(a.OperationID, a.ID), firstNonEmpty(b.OperationID, b.ID))
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

func icotOperationScore(goal string, op sharedpromptcontext.OperationCandidate) int {
	text := strings.ToLower(strings.Join([]string{op.ID, op.OperationID, op.Name, op.Verb, op.Path, op.Summary, strings.Join(op.Tags, " ")}, " "))
	score := 0
	for _, token := range strings.FieldsFunc(goal, func(r rune) bool { return r < 'a' || r > 'z' }) {
		if len(token) > 2 && strings.Contains(text, token) {
			score += 5
		}
	}
	if strings.Contains(goal, "create") || strings.Contains(goal, "add") || strings.Contains(goal, "manage") {
		if strings.EqualFold(op.Verb, "POST") || strings.Contains(text, "create") {
			score += 20
		}
	}
	if strings.Contains(goal, "update") || strings.Contains(goal, "change") {
		if strings.EqualFold(op.Verb, "PUT") || strings.EqualFold(op.Verb, "PATCH") || strings.Contains(text, "update") {
			score += 20
		}
	}
	if strings.Contains(goal, "delete") || strings.Contains(goal, "remove") {
		if strings.EqualFold(op.Verb, "DELETE") || strings.Contains(text, "delete") {
			score += 20
		}
	}
	if icotReadOnlyText(goal) && icotReadOnlyGoal(goal, op) {
		score += 10
	}
	return score
}

func exactICOTOperationMatch(goal string, operations []sharedpromptcontext.OperationCandidate) (sharedpromptcontext.OperationCandidate, bool) {
	needle := normalizeOperationID(goal)
	if needle == "" {
		return sharedpromptcontext.OperationCandidate{}, false
	}
	var match sharedpromptcontext.OperationCandidate
	count := 0
	for _, op := range operations {
		for _, candidate := range []string{op.OperationID, op.ID} {
			if normalizeOperationID(candidate) == needle {
				match = op
				count++
				break
			}
		}
	}
	return match, count == 1
}

func operationByID(operations []sharedpromptcontext.OperationCandidate, id string) (sharedpromptcontext.OperationCandidate, bool) {
	id = strings.TrimSpace(id)
	for _, op := range operations {
		if id != "" && (id == op.ID || id == op.OperationID) {
			return op, true
		}
	}
	return sharedpromptcontext.OperationCandidate{}, false
}

func icotOperationIDs(operations []sharedpromptcontext.OperationCandidate) []string {
	ids := make([]string, 0, len(operations))
	for _, op := range operations {
		if id := firstNonEmpty(op.OperationID, op.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func normalizeOperationID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type icotModelConfig struct {
	Provider    string
	Model       string
	Temperature float64
}

type icotAssistantRequest struct {
	Goal        string
	Context     sharedpromptcontext.Context
	Provider    string
	Model       string
	Temperature float64
}

type icotAssistantSuggestion struct {
	OperationID string
}

type icotAssistant interface {
	SuggestOperation(context.Context, icotAssistantRequest) (icotAssistantSuggestion, error)
}

var newICOTAssistant = func(flags sharedicotcli.Flags, env func(string) string) (icotAssistant, icotModelConfig, error) {
	config := icotModelConfig{
		Provider:    ramenICOTProviderName(flags, env),
		Model:       ramenICOTModelName(flags, env),
		Temperature: flags.Temperature,
	}
	switch config.Provider {
	case "copilot-api":
		if config.Model == "" {
			config.Model = defaultICOTCopilotModel
		}
		return &httpICOTAssistant{
			provider:    config.Provider,
			model:       config.Model,
			apiKey:      firstNonEmpty(envValue(env, "COPILOT_API_KEY"), "copilot-api"),
			baseURL:     firstNonEmpty(envValue(env, "COPILOT_API_BASE_URL"), "http://localhost:4141"),
			temperature: config.Temperature,
			client:      &http.Client{Timeout: 45 * time.Second},
		}, config, nil
	case "openai":
		if config.Model == "" {
			config.Model = "gpt-4-turbo"
		}
		apiKey := envValue(env, "OPENAI_API_KEY")
		if apiKey == "" {
			return nil, config, fmt.Errorf("OPENAI_API_KEY environment variable not set")
		}
		return &httpICOTAssistant{
			provider:    config.Provider,
			model:       config.Model,
			apiKey:      apiKey,
			baseURL:     firstNonEmpty(envValue(env, "OPENAI_BASE_URL"), "https://api.openai.com"),
			temperature: config.Temperature,
			client:      &http.Client{Timeout: 45 * time.Second},
		}, config, nil
	case "anthropic":
		if config.Model == "" {
			config.Model = "claude-3-opus-20240229"
		}
		apiKey := envValue(env, "ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, config, fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
		}
		return &httpICOTAssistant{
			provider:    config.Provider,
			model:       config.Model,
			apiKey:      apiKey,
			baseURL:     firstNonEmpty(envValue(env, "ANTHROPIC_BASE_URL"), "https://api.anthropic.com"),
			temperature: config.Temperature,
			client:      &http.Client{Timeout: 45 * time.Second},
		}, config, nil
	case "gemini":
		if config.Model == "" {
			config.Model = "gemini-1.5-pro"
		}
		apiKey := envValue(env, "GEMINI_API_KEY")
		if apiKey == "" {
			return nil, config, fmt.Errorf("GEMINI_API_KEY environment variable not set")
		}
		return &httpICOTAssistant{
			provider:    config.Provider,
			model:       config.Model,
			apiKey:      apiKey,
			baseURL:     firstNonEmpty(envValue(env, "GEMINI_BASE_URL"), "https://generativelanguage.googleapis.com"),
			temperature: config.Temperature,
			client:      &http.Client{Timeout: 45 * time.Second},
		}, config, nil
	default:
		return nil, config, fmt.Errorf("unknown LLM provider %q", config.Provider)
	}
}

func ramenICOTProviderName(flags sharedicotcli.Flags, env func(string) string) string {
	if value := strings.ToLower(strings.TrimSpace(flags.Provider)); value != "" {
		return value
	}
	if value := strings.ToLower(strings.TrimSpace(envValue(env, "RAMEN_LLM_PROVIDER"))); value != "" {
		return value
	}
	return defaultICOTLLMProvider
}

func ramenICOTModelName(flags sharedicotcli.Flags, env func(string) string) string {
	if value := strings.TrimSpace(flags.Model); value != "" {
		return value
	}
	if value := strings.TrimSpace(envValue(env, "RAMEN_LLM_MODEL")); value != "" {
		return value
	}
	return ""
}

func envValue(env func(string) string, key string) string {
	if env == nil {
		env = os.Getenv
	}
	return strings.TrimSpace(env(key))
}

type httpICOTAssistant struct {
	provider    string
	model       string
	apiKey      string
	baseURL     string
	temperature float64
	client      *http.Client
}

func (assistant *httpICOTAssistant) SuggestOperation(ctx context.Context, request icotAssistantRequest) (icotAssistantSuggestion, error) {
	if assistant == nil {
		return icotAssistantSuggestion{}, nil
	}
	content, err := assistant.generate(ctx, icotOperationSuggestionPrompt(request))
	if err != nil {
		return icotAssistantSuggestion{}, err
	}
	var parsed struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &parsed); err != nil {
		return icotAssistantSuggestion{}, fmt.Errorf("model did not return operation suggestion JSON: %w", err)
	}
	operationID := strings.TrimSpace(parsed.OperationID)
	if operationID == "" {
		return icotAssistantSuggestion{}, nil
	}
	if _, ok := operationByID(request.Context.Operations, operationID); !ok {
		return icotAssistantSuggestion{}, fmt.Errorf("model suggested unlisted operation ID %q", operationID)
	}
	return icotAssistantSuggestion{OperationID: operationID}, nil
}

func icotOperationSuggestionPrompt(request icotAssistantRequest) string {
	type operation struct {
		ID          string   `json:"id"`
		OperationID string   `json:"operation_id"`
		Verb        string   `json:"verb"`
		Path        string   `json:"path"`
		Summary     string   `json:"summary,omitempty"`
		Tags        []string `json:"tags,omitempty"`
	}
	ops := make([]operation, 0, len(request.Context.Operations))
	for _, op := range request.Context.Operations {
		ops = append(ops, operation{
			ID:          op.ID,
			OperationID: op.OperationID,
			Verb:        op.Verb,
			Path:        op.Path,
			Summary:     op.Summary,
			Tags:        append([]string(nil), op.Tags...),
		})
	}
	payload, _ := json.Marshal(struct {
		Goal       string      `json:"goal"`
		Operations []operation `json:"operations"`
	}{
		Goal:       strings.TrimSpace(request.Goal),
		Operations: ops,
	})
	return "Choose the best API operation for this Ramen iCoT draft. Use only the listed operation IDs. Return JSON only in the form {\"operation_id\":\"...\"}; return {\"operation_id\":\"\"} if unsure.\n" + string(payload)
}

func (assistant *httpICOTAssistant) generate(ctx context.Context, prompt string) (string, error) {
	switch assistant.provider {
	case "openai", "copilot-api":
		if assistant.provider == "copilot-api" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(assistant.model)), "gpt-5") {
			return assistant.generateOpenAIResponses(ctx, prompt)
		}
		return assistant.generateOpenAIChat(ctx, prompt)
	case "anthropic":
		return assistant.generateAnthropic(ctx, prompt)
	case "gemini":
		return assistant.generateGemini(ctx, prompt)
	default:
		return "", fmt.Errorf("unknown LLM provider %q", assistant.provider)
	}
}

func (assistant *httpICOTAssistant) generateOpenAIChat(ctx context.Context, prompt string) (string, error) {
	payload := map[string]any{
		"model": assistant.model,
		"messages": []map[string]string{
			{"role": "system", "content": "You suggest one operation from local metadata. Never invent operation IDs."},
			{"role": "user", "content": prompt},
		},
	}
	if assistant.temperature != 0 && requestTemperatureAllowed(assistant.provider, assistant.model) {
		payload["temperature"] = assistant.temperature
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := assistant.postJSON(ctx, strings.TrimRight(assistant.baseURL, "/")+"/v1/chat/completions", map[string]string{"Authorization": "Bearer " + assistant.apiKey}, payload, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("model response did not include choices")
	}
	return out.Choices[0].Message.Content, nil
}

func (assistant *httpICOTAssistant) generateOpenAIResponses(ctx context.Context, prompt string) (string, error) {
	payload := map[string]any{
		"model": assistant.model,
		"input": []map[string]string{
			{"role": "system", "content": "You suggest one operation from local metadata. Never invent operation IDs."},
			{"role": "user", "content": prompt},
		},
	}
	var out struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := assistant.postJSON(ctx, strings.TrimRight(assistant.baseURL, "/")+"/v1/responses", map[string]string{"Authorization": "Bearer " + assistant.apiKey}, payload, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.OutputText) != "" {
		return out.OutputText, nil
	}
	for _, item := range out.Output {
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) != "" {
				return content.Text, nil
			}
		}
	}
	return "", fmt.Errorf("model response did not include output text")
}

func (assistant *httpICOTAssistant) generateAnthropic(ctx context.Context, prompt string) (string, error) {
	payload := map[string]any{
		"model":      assistant.model,
		"max_tokens": 128,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	var out struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	headers := map[string]string{
		"x-api-key":         assistant.apiKey,
		"anthropic-version": "2023-06-01",
	}
	if err := assistant.postJSON(ctx, strings.TrimRight(assistant.baseURL, "/")+"/v1/messages", headers, payload, &out); err != nil {
		return "", err
	}
	if len(out.Content) == 0 {
		return "", fmt.Errorf("model response did not include content")
	}
	return out.Content[0].Text, nil
}

func (assistant *httpICOTAssistant) generateGemini(ctx context.Context, prompt string) (string, error) {
	model := strings.TrimPrefix(strings.TrimSpace(assistant.model), "models/")
	endpoint := strings.TrimRight(assistant.baseURL, "/") + "/v1beta/models/" + url.PathEscape(model) + ":generateContent?key=" + url.QueryEscape(assistant.apiKey)
	payload := map[string]any{
		"contents": []map[string]any{{
			"parts": []map[string]string{{"text": prompt}},
		}},
	}
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := assistant.postJSON(ctx, endpoint, nil, payload, &out); err != nil {
		return "", err
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("model response did not include candidates")
	}
	return out.Candidates[0].Content.Parts[0].Text, nil
}

func (assistant *httpICOTAssistant) postJSON(ctx context.Context, endpoint string, headers map[string]string, payload any, out any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := assistant.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("model provider returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return err
	}
	return nil
}

func requestTemperatureAllowed(provider, model string) bool {
	return !(provider == "copilot-api" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "gpt-5"))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func icotReadOnlyGoal(goal string, op sharedpromptcontext.OperationCandidate) bool {
	text := strings.ToLower(strings.Join([]string{goal, op.ID, op.OperationID, op.Name, op.Summary}, " "))
	verb := strings.ToUpper(strings.TrimSpace(op.Verb))
	if verb == "GET" || verb == "HEAD" {
		return true
	}
	return icotReadOnlyText(text)
}

func icotReadOnlyText(text string) bool {
	text = strings.ToLower(text)
	words := strings.FieldsFunc(text, func(r rune) bool { return r < 'a' || r > 'z' })
	for _, word := range words {
		switch word {
		case "list", "read", "get", "show", "fetch", "enumerate", "all":
			return true
		}
	}
	return false
}

func icotNeedsInput(code, message, slot string) authorCLIResult {
	return icotNeedsInputWithDetail(code, message, slot, nil)
}

func icotNeedsInputWithDetail(code, message, slot string, detail map[string]any) authorCLIResult {
	issue := sharedreadiness.Issue{Code: code, Severity: "blocking", Message: message, Slot: slot}
	diagnostic := trust.DiagnosticRecord{Code: code, Severity: "blocking", Message: message}
	if len(detail) > 0 {
		diagnostic.Detail = detail
	}
	return authorCLIResult{Report: sharedreport.Normalize(sharedreport.Result{
		Status:   sharedreport.StatusNeedsInput,
		Summary:  message,
		TopIssue: &issue,
		Readiness: &sharedreadiness.Result{
			Ready:    false,
			Issues:   []sharedreadiness.Issue{issue},
			Blocking: []sharedreadiness.Issue{issue},
			TopIssue: &issue,
		},
		Diagnostics: []trust.DiagnosticRecord{diagnostic},
		Metadata:    map[string]string{"adapter": "ramen.icot"},
	})}
}

func icotFailed(code, message string) authorCLIResult {
	return authorCLIResult{Report: sharedreport.Normalize(sharedreport.Result{
		Status:      sharedreport.StatusFailed,
		Summary:     message,
		Diagnostics: []trust.DiagnosticRecord{{Code: code, Severity: "error", Message: message}},
		Metadata:    map[string]string{"adapter": "ramen.icot"},
	})}
}

func loadICOTAnswers(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("icot.answers_read_error: %w", err)
	}
	var replay struct {
		Input string `json:"input"`
		Turns []struct {
			Answer string `json:"answer"`
		} `json:"turns"`
	}
	if json.Unmarshal(data, &replay) == nil {
		if strings.TrimSpace(replay.Input) != "" {
			return replay.Input, nil
		}
		if len(replay.Turns) > 0 {
			var lines []string
			for _, turn := range replay.Turns {
				lines = append(lines, turn.Answer)
			}
			return strings.Join(lines, "\n") + "\n", nil
		}
	}
	return string(data), nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func printICOTHuman(result authorCLIResult) {
	fmt.Printf("ramen: icot status=%s\n", result.Report.Status)
	if result.ProjectPath != "" {
		fmt.Printf("  project: %s\n", result.ProjectPath)
	}
	if result.ProjectHCLPath != "" {
		fmt.Printf("  project-hcl: %s\n", result.ProjectHCLPath)
	}
	if result.Validation != nil {
		fmt.Printf("  validate: valid=%t errors=%d warnings=%d diagnostics=%d\n", result.Validation.Valid, result.Validation.Summary.Errors, result.Validation.Summary.Warnings, result.Validation.Summary.Diagnostics)
	}
	if result.Graph != nil {
		fmt.Printf("  graph: nodes=%d edges=%d diagnostics=%d\n", len(result.Graph.Nodes), len(result.Graph.Edges), len(result.Graph.Diagnostics))
	}
	if result.Plan != nil {
		summary := result.Plan.Plan.Summary
		fmt.Printf("  plan: create=%d update=%d delete=%d post=%d put=%d patch=%d replace=%d no-op=%d diagnostics=%d\n", summary.Create, summary.Update, summary.Delete, summary.Post, summary.Put, summary.Patch, summary.Replace, summary.NoOp, summary.Diagnostics)
	}
}

func runForceUnlockCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("force-unlock", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	lockName := fs.String("name", "state", "Lock name to release")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen force-unlock LOCK_HOLDER [--state PATH] [--name state]\n")
		fmt.Fprintf(fs.Output(), "\nReleases a local SQLite state lock only when LOCK_HOLDER exactly matches the stored holder. It does not modify resources, revisions, runs, project files, API source documents, or remote systems.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(positionalFirstLast(args)); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		fs.Usage()
		os.Exit(2)
	}
	path := strings.TrimSpace(*statePath)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "state path %s does not exist\n", path)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	store, err := state.Open(ctx, path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	lock, err := store.ForceUnlock(ctx, *lockName, fs.Arg(0))
	if err != nil {
		var missing state.LockNotFoundError
		var mismatch state.LockHolderMismatchError
		switch {
		case errors.As(err, &missing):
			fmt.Fprintln(os.Stderr, missing.Error())
		case errors.As(err, &mismatch):
			fmt.Fprintln(os.Stderr, mismatch.Error())
		default:
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
	fmt.Printf("ramen: force-unlocked %s held by %s since %s\n", lock.Name, lock.Holder, lock.AcquiredAt.Format(time.RFC3339Nano))
}

func runShowCommand(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "Emit JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen show PLAN_OR_APPROVAL [--json]\n")
		fmt.Fprintf(fs.Output(), "\nInspects a Ramen plan/approval artifact without reading state, executing workflows, or mutating files.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(positionalFirstLast(args)); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	doc, err := loadPlanForShow(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(doc)
		return
	}
	fmt.Printf("ramen: show version=%s action=%s errored=%t resources=%d diagnostics=%d\n", doc.Version, doc.Action, doc.Errored, len(doc.Resources), len(doc.Diagnostics))
	fmt.Printf("  summary: create=%d update=%d delete=%d post=%d put=%d patch=%d replace=%d no-op=%d read=%d diagnostics=%d\n", doc.Summary.Create, doc.Summary.Update, doc.Summary.Delete, doc.Summary.Post, doc.Summary.Put, doc.Summary.Patch, doc.Summary.Replace, doc.Summary.NoOp, doc.Summary.Read, doc.Summary.Diagnostics)
	if doc.Approval != nil {
		fmt.Printf("  approval: version=%s digest=%s project=%s state=%s\n", doc.Approval.Version, doc.Approval.Digest, doc.Approval.ProjectDigest, doc.Approval.StateDigest)
	}
	if len(doc.Controls.Targets) > 0 || len(doc.Controls.Excludes) > 0 || len(doc.Controls.Replaces) > 0 || doc.Controls.Destroy {
		fmt.Printf("  controls: targets=%d excludes=%d replaces=%d destroy=%t\n", len(doc.Controls.Targets), len(doc.Controls.Excludes), len(doc.Controls.Replaces), doc.Controls.Destroy)
	}
}

func loadPlanForShow(path string) (tfplan.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return tfplan.Document{}, fmt.Errorf("show.plan_read_error: %w", err)
	}
	var doc tfplan.Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return tfplan.Document{}, fmt.Errorf("show.plan_parse_error: %w", err)
	}
	if doc.Version != tfplan.Version {
		return tfplan.Document{}, fmt.Errorf("show.plan_version_invalid: got %q, want %q", doc.Version, tfplan.Version)
	}
	return doc, nil
}

func printFlagDefaultsExcluding(fs *flag.FlagSet, excluded map[string]bool) {
	fs.VisitAll(func(f *flag.Flag) {
		if excluded[f.Name] {
			return
		}
		name, usage := flag.UnquoteUsage(f)
		if name != "" {
			fmt.Fprintf(fs.Output(), "  -%s %s\n    \t%s", f.Name, name, usage)
		} else {
			fmt.Fprintf(fs.Output(), "  -%s\n    \t%s", f.Name, usage)
		}
		if f.DefValue != "" && f.DefValue != "false" {
			fmt.Fprintf(fs.Output(), " (default %q)", f.DefValue)
		}
		fmt.Fprint(fs.Output(), "\n")
	})
}

func positionalFirstLast(args []string) []string {
	if len(args) > 1 && !strings.HasPrefix(args[0], "-") {
		return append(slices.Clone(args[1:]), args[0])
	}
	return args
}

func maintenanceLockHolder(command string) string {
	return fmt.Sprintf("state-%s-%d-%d", command, os.Getpid(), time.Now().UTC().UnixNano())
}

func runStateCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		stateUsage(os.Stderr)
		os.Exit(2)
	}
	switch args[0] {
	case "async-evidence":
		runStateAsyncEvidenceCommand(ctx, args[1:])
	case "audit":
		runStateAuditCommand(ctx, args[1:])
	case "backup":
		runStateBackupCommand(ctx, args[1:])
	case "export":
		runStateExportCommand(ctx, args[1:])
	case "list":
		runStateListCommand(ctx, args[1:])
	case "restore":
		runStateRestoreCommand(ctx, args[1:])
	case "show":
		runStateShowCommand(ctx, args[1:])
	case "history":
		runStateHistoryCommand(ctx, args[1:])
	case "runs":
		runStateRunsCommand(ctx, args[1:])
	case "vacuum":
		runStateVacuumCommand(ctx, args[1:])
	case "-h", "--help", "help":
		stateUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown state subcommand %q\n", args[0])
		stateUsage(os.Stderr)
		os.Exit(2)
	}
}

func stateUsage(out *os.File) {
	fmt.Fprintf(out, "Usage: ramen state <async-evidence|audit|backup|export|list|restore|show|history|runs|vacuum> [args]\n\n")
	fmt.Fprintf(out, "Local SQLite state inspection and maintenance. No backend access, provider execution, or Terraform/OpenTofu state compatibility is performed.\n\n")
	fmt.Fprintf(out, "Subcommands:\n")
	fmt.Fprintf(out, "  async-evidence    list neutral async execution evidence records\n")
	fmt.Fprintf(out, "  audit             export tamper-evident audit summary\n")
	fmt.Fprintf(out, "  backup            write a consistent SQLite backup snapshot\n")
	fmt.Fprintf(out, "  export            export redacted state metadata as JSON\n")
	fmt.Fprintf(out, "  list              list current resource addresses\n")
	fmt.Fprintf(out, "  restore           replace local state from a backup with --force\n")
	fmt.Fprintf(out, "  show ADDRESS      show one current resource\n")
	fmt.Fprintf(out, "  history [ADDRESS] show revision history\n")
	fmt.Fprintf(out, "  runs              show command run history\n")
	fmt.Fprintf(out, "  vacuum            compact local SQLite state\n")
}

func runStateAsyncEvidenceCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state async-evidence", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	runID := fs.Int64("run", 0, "Optional run ID filter")
	address := fs.String("address", "", "Optional resource address filter")
	kind := fs.String("kind", "", "Optional record kind filter")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state async-evidence [--state PATH] [--run ID] [--address ADDRESS] [--kind KIND] [--json]\n")
		fmt.Fprintf(fs.Output(), "\nLists durable neutral async evidence records attached to local runs. These records are execution observations only; accepted executor responses do not imply convergence or state success.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if store == nil {
		if *jsonOut {
			writeJSONOutput([]state.AsyncEvidenceRecord{})
		} else {
			fmt.Println("ramen: state async-evidence=0")
		}
		return
	}
	defer func() { _ = store.Close() }()
	records, err := store.ListAsyncEvidence(ctx, state.AsyncEvidenceFilter{RunID: *runID, ResourceAddress: *address, RecordKind: *kind})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(records)
		return
	}
	fmt.Printf("ramen: state async-evidence=%d\n", len(records))
	for _, record := range records {
		fmt.Printf("  #%d run=%d %s %s kind=%s phase=%s evidence=%s\n", record.ID, record.RunID, record.ResourceAddress, record.Action, record.RecordKind, record.Phase, record.EvidenceID)
	}
}

func runStateAuditCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state audit", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state audit [--state PATH] [--json]\n")
		fmt.Fprintf(fs.Output(), "\nExports a tamper-evident local audit summary derived from runs, revisions, resources, locks, and migration records without remote backend access.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "state path %s does not exist\n", strings.TrimSpace(*statePath))
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	doc, err := store.Audit(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(doc)
		return
	}
	fmt.Printf("ramen: state audit version=%s digest=%s resources=%d revisions=%d runs=%d events=%d async_evidence=%d locks=%d\n", doc.Version, doc.Digest, doc.Counts["resources"], doc.Counts["revisions"], doc.Counts["runs"], doc.Counts["run_events"], doc.Counts["async_evidence"], doc.Counts["locks"])
}

func runStateBackupCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state backup", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	outPath := fs.String("out", "", "Backup output path")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state backup --state PATH --out PATH\n")
		fmt.Fprintf(fs.Output(), "\nWrites a consistent local SQLite backup snapshot. It does not read or write remote backends or Terraform/OpenTofu state.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 || strings.TrimSpace(*outPath) == "" {
		fs.Usage()
		os.Exit(2)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if store == nil {
		fmt.Fprintf(os.Stderr, "state path %s does not exist\n", strings.TrimSpace(*statePath))
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	if err := store.Backup(ctx, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: state backup written %s\n", strings.TrimSpace(*outPath))
}

func runStateExportCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state export", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state export --state PATH --json\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if store == nil {
		fmt.Fprintf(os.Stderr, "state path %s does not exist\n", strings.TrimSpace(*statePath))
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	doc, err := store.Export(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(doc)
		return
	}
	fmt.Printf("ramen: state export version=%s resources=%d revisions=%d runs=%d async_evidence=%d locks=%d\n", doc.Version, len(doc.Resources), len(doc.Revisions), len(doc.Runs), len(doc.AsyncEvidence), len(doc.Locks))
}

func runStateRestoreCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state restore", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	fromPath := fs.String("from", "", "Backup input path")
	force := fs.Bool("force", false, "Required confirmation to overwrite local state")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state restore --state PATH --from PATH --force\n")
		fmt.Fprintf(fs.Output(), "\nReplaces the local SQLite state file from a validated Ramen backup. This is local state surgery only and never reads remote backends or Terraform/OpenTofu state.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 || strings.TrimSpace(*fromPath) == "" {
		fs.Usage()
		os.Exit(2)
	}
	if err := state.Restore(ctx, *statePath, *fromPath, *force); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: state restored %s from %s\n", strings.TrimSpace(*statePath), strings.TrimSpace(*fromPath))
}

func runStateVacuumCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state vacuum", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state vacuum [--state PATH]\n")
		fmt.Fprintf(fs.Output(), "\nCompacts local SQLite state after taking the cooperative state lock. It does not read or write remote backends or Terraform/OpenTofu state.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	store, err := state.Open(ctx, strings.TrimSpace(*statePath))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	holder := maintenanceLockHolder("vacuum")
	if err := store.AcquireLock(ctx, "state", holder, 30*time.Minute); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = store.ReleaseLock(context.Background(), "state", holder) }()
	if err := store.Vacuum(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: state vacuum completed %s\n", strings.TrimSpace(*statePath))
}

func runStateListCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state list", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state list [--state PATH] [--json]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if store == nil {
		if *jsonOut {
			writeJSONOutput([]state.ResourceSnapshot{})
		} else {
			fmt.Println("ramen: state resources=0")
		}
		return
	}
	defer func() { _ = store.Close() }()
	resources, err := store.ListCurrentResources(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(resources)
		return
	}
	fmt.Printf("ramen: state resources=%d\n", len(resources))
	for _, resource := range resources {
		fmt.Printf("  %s %s status=%s\n", resource.Address, resource.Type, resource.Status)
	}
}

func runStateShowCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state show", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state show ADDRESS [--state PATH] [--json]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(positionalFirstLast(args)); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		fs.Usage()
		os.Exit(2)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if store == nil {
		fmt.Fprintf(os.Stderr, "state.resource_not_found: %s\n", fs.Arg(0))
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	resource, err := store.CurrentResource(ctx, fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if resource == nil {
		fmt.Fprintf(os.Stderr, "state.resource_not_found: %s\n", fs.Arg(0))
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(resource)
		return
	}
	fmt.Printf("ramen: state address=%s type=%s status=%s\n", resource.Address, resource.Type, resource.Status)
	fmt.Printf("  provider=%s source=%s:%s operation=%s run=%d updated=%s\n", resource.Provider, resource.SourceKind, resource.SourceID, resource.OperationID, resource.UpdatedRunID, resource.UpdatedAt.Format(time.RFC3339Nano))
}

func runStateHistoryCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state history", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state history [ADDRESS] [--state PATH] [--json]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(positionalFirstLast(args)); err != nil {
		os.Exit(2)
	}
	if fs.NArg() > 1 {
		fs.Usage()
		os.Exit(2)
	}
	address := ""
	if fs.NArg() == 1 {
		address = fs.Arg(0)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if store == nil {
		if *jsonOut {
			writeJSONOutput([]state.Revision{})
		} else {
			fmt.Println("ramen: state revisions=0")
		}
		return
	}
	defer func() { _ = store.Close() }()
	revisions, err := store.ListRevisions(ctx, address)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(revisions)
		return
	}
	fmt.Printf("ramen: state revisions=%d\n", len(revisions))
	for _, rev := range revisions {
		fmt.Printf("  #%d %s action=%s run=%d at=%s\n", rev.ID, rev.ResourceAddress, rev.Action, rev.RunID, rev.CreatedAt.Format(time.RFC3339Nano))
	}
}

func runStateRunsCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("state runs", flag.ExitOnError)
	statePath := fs.String("state", state.DefaultPath("."), "SQLite state path")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	status := fs.String("status", "", "Optional run status filter")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen state runs [--state PATH] [--status STATUS] [--json]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	store, err := openStateReadOnly(ctx, *statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if store == nil {
		if *jsonOut {
			writeJSONOutput([]state.Run{})
		} else {
			fmt.Println("ramen: state runs=0")
		}
		return
	}
	defer func() { _ = store.Close() }()
	runs, err := store.ListRuns(ctx, *status)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(runs)
		return
	}
	fmt.Printf("ramen: state runs=%d\n", len(runs))
	for _, run := range runs {
		finished := ""
		if !run.FinishedAt.IsZero() {
			finished = " finished=" + run.FinishedAt.Format(time.RFC3339Nano)
		}
		fmt.Printf("  #%d %s status=%s started=%s%s\n", run.ID, run.Command, run.Status, run.StartedAt.Format(time.RFC3339Nano), finished)
	}
}

func openStateReadOnly(ctx context.Context, path string) (*state.Store, error) {
	store, err := state.OpenReadOnly(ctx, strings.TrimSpace(path))
	if err != nil {
		return nil, fmt.Errorf("state.open_read_error: %w", err)
	}
	return store, nil
}

func writeJSONOutput(value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(append(data, '\n'))
}

func runGraphCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("graph", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	format := fs.String("format", "dot", "Output format: dot or json")
	outPath := fs.String("out", "", "Optional output path")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen graph --project DIR|FILE [--format dot|json] [--out PATH]\n")
		fmt.Fprintf(fs.Output(), "\nEmits the native UWS/Ramen resource dependency graph, including operation-role references, without Terraform/OpenTofu graph compatibility, provider execution, state access, or mutation.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	normalizedFormat := strings.ToLower(strings.TrimSpace(*format))
	if normalizedFormat != "dot" && normalizedFormat != "json" {
		fmt.Fprintf(os.Stderr, "--format must be dot or json, got %q\n", *format)
		os.Exit(2)
	}
	validation, err := ramenvalidate.Run(ctx, ramenvalidate.Options{ProjectPath: *projectPath})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var graphDoc graph.Document
	if validation.Valid {
		proj, err := project.Load(*projectPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		graphDoc = graph.BuildProject(proj)
	} else {
		graphDoc = graph.Document{Version: graph.DocumentVersion, ProjectPath: validation.ProjectPath}
	}
	graphDoc.Diagnostics = append(graphDoc.Diagnostics, graphDiagnostics(validation.Diagnostics)...)
	if normalizedFormat == "json" {
		writeGraphOutput(*outPath, mustGraphJSON(graphDoc))
	} else {
		writeGraphOutput(*outPath, graph.DOT(graphDoc))
	}
	for _, diag := range graphDoc.Diagnostics {
		fmt.Fprintf(os.Stderr, "%s: %s\n", diag.Code, diag.Message)
	}
	if hasGraphErrors(graphDoc.Diagnostics) {
		os.Exit(1)
	}
}

func graphDiagnostics(diagnostics []ramenvalidate.Diagnostic) []graph.Diagnostic {
	out := make([]graph.Diagnostic, 0, len(diagnostics))
	for _, diag := range diagnostics {
		out = append(out, graph.Diagnostic{
			Code:          diag.Code,
			Severity:      diag.Severity,
			Message:       diag.Message,
			Address:       diag.Address,
			APISourceKind: diag.APISourceKind,
			APISourceID:   diag.APISourceID,
			OperationID:   diag.OperationID,
		})
	}
	return out
}

func mustGraphJSON(doc graph.Document) string {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return string(data) + "\n"
}

func writeGraphOutput(path, content string) {
	if strings.TrimSpace(path) == "" {
		fmt.Print(content)
		return
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func hasGraphErrors(diagnostics []graph.Diagnostic) bool {
	for _, diag := range diagnostics {
		if diag.Severity == "error" {
			return true
		}
	}
	return false
}

func runValidateCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	jsonOutput := fs.Bool("json", false, "Print machine-readable validation diagnostics")
	strict := fs.Bool("strict", false, "Treat validation warnings as errors")
	var apiSources repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen validate --project DIR|FILE [--api-source KIND:ID=PATH] [--json] [--strict]\n")
		fmt.Fprintf(fs.Output(), "\nValidates a native UWS/Ramen project, optional local API source operation references, and diagnostics without planning, executing, touching state, reading Terraform/OpenTofu HCL, or performing network access.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	sources, err := parseValidateAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := ramenvalidate.Run(ctx, ramenvalidate.Options{ProjectPath: *projectPath, APISources: sources, Strict: *strict})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOutput {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		fmt.Printf("ramen: validate valid=%t errors=%d warnings=%d diagnostics=%d\n", result.Valid, result.Summary.Errors, result.Summary.Warnings, result.Summary.Diagnostics)
		for _, diag := range result.Diagnostics {
			fmt.Fprintf(os.Stderr, "%s: %s\n", diag.Code, diag.Message)
		}
	}
	if !result.Valid {
		os.Exit(1)
	}
}

type versionInfo struct {
	Version   string            `json:"version"`
	Module    string            `json:"module"`
	MainPath  string            `json:"main_path,omitempty"`
	GoVersion string            `json:"go_version,omitempty"`
	Revision  string            `json:"revision,omitempty"`
	BuildTags []string          `json:"build_tags,omitempty"`
	Settings  map[string]string `json:"settings,omitempty"`
}

func runVersionCommand(args []string) {
	fs := flag.NewFlagSet("version", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "Print version and local build metadata as JSON")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen version [--json]\n")
		fmt.Fprintf(fs.Output(), "\nPrints the Ramen CLI version. With --json, prints local build metadata only; it does not check networks, releases, updates, or telemetry.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	info := collectVersionInfo()
	if *jsonOutput {
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}
	fmt.Println(info.Version)
}

func collectVersionInfo() versionInfo {
	info := versionInfo{
		Version: strings.TrimSpace(version),
		Module:  "github.com/OpenUdon/ramen",
	}
	if build, ok := debug.ReadBuildInfo(); ok {
		info.GoVersion = build.GoVersion
		if build.Main.Path != "" {
			info.MainPath = build.Main.Path
		}
		if (info.Version == "" || info.Version == "devel") &&
			build.Main.Version != "" && build.Main.Version != "(devel)" {
			info.Version = strings.TrimPrefix(build.Main.Version, "v")
		}
		settings := make(map[string]string)
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				info.Revision = setting.Value
			case "-tags":
				if strings.TrimSpace(setting.Value) != "" {
					info.BuildTags = splitBuildTags(setting.Value)
				}
			case "vcs.modified", "vcs.time", "vcs":
				settings[setting.Key] = setting.Value
			}
		}
		if len(settings) > 0 {
			info.Settings = settings
		}
	}
	if info.Version == "" {
		info.Version = "devel"
	}
	return info
}

func splitBuildTags(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' '
	})
	tags := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags
}

func runRunCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	statePath := fs.String("state", "", "SQLite state path; defaults to the UWS document directory .ramen/state.db")
	workspace := fs.String("workspace", "", "Workspace name; defaults to the base local state path")
	checkMode := fs.Bool("check", false, "Validate and preview without executor calls or state writes")
	autoApprove := fs.Bool("auto-approve", false, "Approve imperative execution after reviewing the approval digest")
	approvalDigest := fs.String("approval-digest", "", "Digest-bound approval for this UWS document and target set")
	mock := fs.Bool("mock", false, "Use the public mock executor instead of a live trusted executor")
	outDir := fs.String("out", "", "Optional executor output directory")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	var targets repeatedStringFlag
	var policyFiles repeatedStringFlag
	fs.Var(&targets, "target", "Repeatable run target; defaults to one target named default")
	fs.Var(&policyFiles, "policy-file", "Repeatable Ramen policy file for run governance")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen run UWS_FILE [--target NAME] [--policy-file PATH] [--state PATH] [--workspace NAME] [--check | --auto-approve | --approval-digest DIGEST] --mock [--out DIR] [--json]\n")
		fmt.Fprintf(fs.Output(), "\nExecutes an approved imperative UWS runbook through the trusted executor boundary without treating outputs as desired-state resources. --check validates and previews without executor calls or state writes.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(positionalFirstLast(args)); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	docPath := strings.TrimSpace(fs.Arg(0))
	path := *statePath
	if strings.TrimSpace(path) == "" {
		path = statePathOrDefault("", docPath, filepath.Dir(docPath), *workspace)
	}
	var exec executor.Executor
	if *mock {
		exec = &executor.MockExecutor{}
	}
	result, err := ramenrun.Execute(ctx, ramenrun.Options{
		DocumentPath:   docPath,
		StatePath:      path,
		Workspace:      *workspace,
		Targets:        []string(targets),
		PolicyFiles:    []string(policyFiles),
		Check:          *checkMode,
		AutoApprove:    *autoApprove,
		ApprovalDigest: *approvalDigest,
		OutDir:         *outDir,
		Executor:       exec,
	})
	if err != nil {
		if result != nil && *jsonOut {
			writeJSONOutput(result)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(result)
		return
	}
	fmt.Printf("ramen: run targets=%d executed=%d skipped=%d failed=%d\n", result.Summary.Targets, result.Summary.Executed, result.Summary.Skipped, result.Summary.Failed)
	fmt.Printf("  approval_digest: %s\n", result.ApprovalDigest)
	if result.RunID != 0 {
		fmt.Printf("  run: %d\n", result.RunID)
	}
}

func runRefreshCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("refresh", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	workspace := fs.String("workspace", "", "Workspace name; defaults to the base local state path")
	mock := fs.Bool("mock", false, "Use the public mock executor instead of a live trusted executor")
	executorMode := fs.String("executor", "", "Trusted executor to use: mock or udon")
	udonOutputDir := fs.String("udon-output", "", "Optional root directory for udon runtime artifacts when --executor udon is selected")
	outDir := fs.String("out", "", "Optional directory for generated read UWS action documents")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	var apiSources repeatedStringFlag
	var varFiles repeatedStringFlag
	var cliVars repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Var(&varFiles, "var-file", "Repeatable native Ramen values file; later files override earlier files")
	fs.Var(&cliVars, "var", "Repeatable native Ramen variable assignment as name=value; overrides defaults and files")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen refresh [--project DIR|FILE | --config-dir DIR] [--state PATH] [--workspace NAME] [--api-source KIND:ID=PATH] [--var-file PATH] [--var name=value] [--mock | --executor udon] [--udon-output DIR] [--out DIR] [--json]\n")
		fmt.Fprintf(fs.Output(), "\nReads tracked resources through a trusted executor and records redacted refresh revisions. Public builds only include the mock executor; live udon execution requires an opt-in adapter build.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	sources, err := parseReconcileAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	exec, err := selectTrustedExecutor(*executorMode, *mock, *udonOutputDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := reconcile.Refresh(ctx, reconcile.Options{ConfigDir: *configDir, ProjectPath: *projectPath, StatePath: statePathOrDefault(*statePath, *projectPath, *configDir, *workspace), APISources: sources, VarFiles: []string(varFiles), Vars: []string(cliVars), Workspace: *workspace, OutDir: *outDir, Executor: exec})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(result)
		return
	}
	fmt.Printf("ramen: refresh read=%d changed=%d unchanged=%d missing=%d skipped=%d failed=%d\n", result.Summary.Read, result.Summary.Changed, result.Summary.Unchanged, result.Summary.Missing, result.Summary.Skipped, result.Summary.Failed)
}

func runImportCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory used to compute plan-compatible desired hashes")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory used to compute plan-compatible desired hashes")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	workspace := fs.String("workspace", "", "Workspace name; defaults to the base local state path")
	typeName := fs.String("type", "", "Terraform/OpenTofu resource type")
	provider := fs.String("provider", "", "Provider address")
	identity := fs.String("identity", "{}", "Identity JSON object")
	sourceKind := fs.String("source-kind", "", "Optional API source kind")
	sourceID := fs.String("source-id", "", "Optional API source ID")
	operationID := fs.String("operation-id", "", "Optional read/import operation ID")
	var apiSources repeatedStringFlag
	var varFiles repeatedStringFlag
	var cliVars repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; when ADDRESS exists in config, import records the plan-compatible desired hash")
	fs.Var(&varFiles, "var-file", "Repeatable native Ramen values file; later files override earlier files")
	fs.Var(&cliVars, "var", "Repeatable native Ramen variable assignment as name=value; overrides defaults and files")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen import ADDRESS --type TYPE --identity JSON [--project DIR|FILE | --config-dir DIR] [--state PATH] [--workspace NAME] [--api-source KIND:ID=PATH] [--var-file PATH] [--var name=value]\n")
		fmt.Fprintf(fs.Output(), "\nAttaches an existing resource identity to local Ramen state without executing Terraform, providers, or API source operations.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(positionalFirstLast(args)); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	var identityMap map[string]any
	if err := json.Unmarshal([]byte(*identity), &identityMap); err != nil {
		fmt.Fprintf(os.Stderr, "import.identity_invalid: identity must be a JSON object: %v\n", err)
		os.Exit(2)
	}
	if identityMap == nil {
		fmt.Fprintln(os.Stderr, "import.identity_invalid: identity must be a JSON object")
		os.Exit(2)
	}
	sources, err := parseReconcileAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := reconcile.Import(ctx, reconcile.ImportOptions{
		ConfigDir:   *configDir,
		ProjectPath: *projectPath,
		StatePath:   statePathOrDefault(*statePath, *projectPath, *configDir, *workspace),
		APISources:  sources,
		VarFiles:    []string(varFiles),
		Vars:        []string(cliVars),
		Workspace:   *workspace,
		Address:     fs.Arg(0),
		Type:        *typeName,
		Provider:    *provider,
		Identity:    identityMap,
		SourceKind:  *sourceKind,
		SourceID:    *sourceID,
		OperationID: *operationID,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: import imported=%d run=%d\n", result.Summary.Imported, result.RunID)
}

func statePathOrDefault(path, projectPath, configDir, workspace string) string {
	if strings.TrimSpace(path) != "" {
		return path
	}
	resolved, err := state.WorkspacePath(stateBaseDir(projectPath, configDir), workspace)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	return resolved
}

func approvalInputsOrExit(identity, role, context, approvedAt string) []governance.Approver {
	if strings.TrimSpace(identity) == "" && strings.TrimSpace(role) == "" && strings.TrimSpace(context) == "" && strings.TrimSpace(approvedAt) == "" {
		return nil
	}
	if strings.TrimSpace(identity) == "" {
		fmt.Fprintln(os.Stderr, "--approved-by is required when approval metadata is supplied")
		os.Exit(2)
	}
	if strings.TrimSpace(approvedAt) == "" {
		fmt.Fprintln(os.Stderr, "--approved-at is required when --approved-by is supplied")
		os.Exit(2)
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(approvedAt))
	if err != nil {
		fmt.Fprintf(os.Stderr, "--approved-at must be RFC3339: %v\n", err)
		os.Exit(2)
	}
	return []governance.Approver{{Identity: identity, Role: role, Context: context, ApprovedAt: parsed}}
}

func stateBaseDir(projectPath, configDir string) string {
	projectPath = strings.TrimSpace(projectPath)
	if projectPath == "" {
		return configDir
	}
	info, err := os.Stat(projectPath)
	if err == nil && info.IsDir() {
		return projectPath
	}
	return filepath.Dir(projectPath)
}

func validateStaticConfig(configDir string) error {
	doc, err := tfconfig.LoadDir(configDir)
	if err != nil {
		return err
	}
	for _, diag := range doc.Diagnostics {
		if strings.EqualFold(string(diag.Severity), "error") {
			return fmt.Errorf("%s", diagnosticText(diag))
		}
	}
	for _, mod := range doc.Modules {
		for _, diag := range mod.Diagnostics {
			if strings.EqualFold(string(diag.Severity), "error") {
				return fmt.Errorf("%s", diagnosticText(diag))
			}
		}
	}
	return nil
}

func validateNativeProject(projectPath string) error {
	_, err := project.Load(projectPath)
	return err
}

func diagnosticText(diag tfconfig.Diagnostic) string {
	if strings.TrimSpace(diag.Detail) == "" {
		return diag.Summary
	}
	return diag.Summary + ": " + diag.Detail
}

func planHasChanges(doc tfplan.Document) bool {
	return doc.Summary.Create != 0 || doc.Summary.Update != 0 || doc.Summary.Delete != 0 || doc.Summary.Post != 0 || doc.Summary.Put != 0 || doc.Summary.Patch != 0 || doc.Summary.Replace != 0 || doc.Summary.Read != 0
}

func runApplyCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	workspace := fs.String("workspace", "", "Workspace name; defaults to the base local state path")
	planPath := fs.String("plan", "", "Digest-bound Ramen plan artifact to verify and execute")
	autoApprove := fs.Bool("auto-approve", false, "Approve planned actions without an interactive prompt")
	mock := fs.Bool("mock", false, "Use the public mock executor instead of a live trusted executor")
	executorMode := fs.String("executor", "", "Trusted executor to use: mock or udon")
	udonOutputDir := fs.String("udon-output", "", "Optional root directory for udon runtime artifacts when --executor udon is selected")
	outDir := fs.String("out", "", "Optional directory for generated executor-ready UWS action documents")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	var apiSources repeatedStringFlag
	var varFiles repeatedStringFlag
	var cliVars repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Var(&varFiles, "var-file", "Repeatable native Ramen values file; later files override earlier files")
	fs.Var(&cliVars, "var", "Repeatable native Ramen variable assignment as name=value; overrides defaults and files")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen apply [--plan PLAN.json | --project DIR|FILE | --config-dir DIR] [--state PATH] [--workspace NAME] [--api-source KIND:ID=PATH] [--var-file PATH] [--var name=value] --auto-approve [--mock | --executor udon] [--udon-output DIR] [--out DIR] [--json]\n")
		fmt.Fprintf(fs.Output(), "\nVerifies a digest-bound plan artifact or builds the same approval contract from project inputs, requires explicit approval, generates executor-ready UWS action documents, and hands approved plan actions to a trusted executor. Public builds only include the mock executor; live udon execution requires an opt-in adapter build.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	configDirSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "config-dir" {
			configDirSet = true
		}
	})
	sources, err := parseApplyAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	path := *statePath
	if strings.TrimSpace(*planPath) != "" {
		// Keep plan artifacts self-contained: when replaying an approved plan, let
		// it supply the state path so this command remains valid from alternate
		// working directories.
		path = ""
	} else if strings.TrimSpace(path) == "" {
		path = statePathOrDefault(*statePath, *projectPath, *configDir, *workspace)
	}
	configDirValue := *configDir
	if strings.TrimSpace(*planPath) != "" && !configDirSet {
		configDirValue = ""
	}
	exec, err := selectTrustedExecutor(*executorMode, *mock, *udonOutputDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := tfapply.Apply(ctx, tfapply.Options{
		ConfigDir:   configDirValue,
		ProjectPath: *projectPath,
		StatePath:   path,
		APISources:  sources,
		VarFiles:    []string(varFiles),
		Vars:        []string(cliVars),
		Workspace:   *workspace,
		PlanPath:    *planPath,
		AutoApprove: *autoApprove,
		OutDir:      *outDir,
		Executor:    exec,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if result != nil {
			for _, msg := range result.Errors {
				fmt.Fprintln(os.Stderr, msg)
			}
		}
		os.Exit(1)
	}
	if *jsonOut {
		writeJSONOutput(result)
		return
	}
	fmt.Printf("ramen: apply create=%d update=%d delete=%d post=%d put=%d patch=%d read=%d no-op=%d skipped=%d failed=%d blocked=%d executed=%d\n", result.Summary.Create, result.Summary.Update, result.Summary.Delete, result.Summary.Post, result.Summary.Put, result.Summary.Patch, result.Summary.Read, result.Summary.NoOp, result.Summary.Skipped, result.Summary.Failed, result.Summary.Blocked, len(result.Executed))
	if result.RunID != 0 {
		fmt.Printf("  run:   %d\n", result.RunID)
	}
	if len(result.GeneratedDocuments) > 0 {
		fmt.Printf("  uws:   %s\n", *outDir)
	}
}

func runInitCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	workspace := fs.String("workspace", "", "Workspace name; defaults to the base local state path")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen init [--project DIR|FILE | --config-dir DIR] [--state PATH] [--workspace NAME]\n")
		fmt.Fprintf(fs.Output(), "\nValidates a native UWS/Ramen project when --project is supplied, otherwise validates Terraform/OpenTofu configuration with tfconfig, then creates or migrates local Ramen SQLite state. It does not execute Terraform, providers, API source operations, module downloads, backend initialization, or UWS workflows.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	path := *statePath
	if strings.TrimSpace(path) == "" {
		path = statePathOrDefault(*statePath, *projectPath, *configDir, *workspace)
	}
	var validationErr error
	if strings.TrimSpace(*projectPath) != "" {
		validationErr = validateNativeProject(*projectPath)
	} else {
		validationErr = validateStaticConfig(*configDir)
	}
	if validationErr != nil {
		fmt.Fprintln(os.Stderr, validationErr)
		os.Exit(1)
	}
	if err := state.Init(ctx, path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ramen: initialized state %s\n", path)
}

func runPlanCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	projectPath := fs.String("project", "", "Native UWS/Ramen project file or directory")
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	statePath := fs.String("state", "", "SQLite state path; defaults to CONFIG_DIR/.ramen/state.db")
	workspace := fs.String("workspace", "", "Workspace name; defaults to the base local state path")
	action := fs.String("action", "", "Desired action; native API-first projects default to their declared API operation, Terraform/OpenTofu config defaults to create")
	destroy := fs.Bool("destroy", false, "Deprecated compatibility flag for Terraform/OpenTofu-shaped delete plans; prefer native API DELETE operation roles")
	outPath := fs.String("out", "", "Optional JSON plan output path; also writes a sibling .hcl plan view")
	hclOutPath := fs.String("hcl-out", "", "Optional HCL plan output path; defaults to sibling .hcl when --out is set")
	detailedExitCode := fs.Bool("detailed-exitcode", false, "Return 2 when the plan has changes, 1 on errors, and 0 when empty")
	var apiSources repeatedStringFlag
	var targets repeatedStringFlag
	var excludes repeatedStringFlag
	var replaces repeatedStringFlag
	var varFiles repeatedStringFlag
	var cliVars repeatedStringFlag
	var policyFiles repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Var(&targets, "target", "Repeatable native resource address to include with dependency closure")
	fs.Var(&excludes, "exclude", "Repeatable native resource address to exclude with dependent closure")
	fs.Var(&replaces, "replace", "Repeatable native resource address to force replacement in the plan")
	fs.Var(&varFiles, "var-file", "Repeatable native Ramen values file; later files override earlier files")
	fs.Var(&cliVars, "var", "Repeatable native Ramen variable assignment as name=value; overrides defaults and files")
	fs.Var(&policyFiles, "policy-file", "Repeatable Ramen policy file for plan-time governance")
	approvedBy := fs.String("approved-by", "", "Approver identity to bind into the plan approval artifact")
	approvalRole := fs.String("approval-role", "", "Approver role for policy approval routing")
	approvalContext := fs.String("approval-context", "", "Free-form approval context")
	approvedAt := fs.String("approved-at", "", "Approval timestamp in RFC3339 format")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen plan [--project DIR|FILE | --config-dir DIR] [--state PATH] [--workspace NAME] [--api-source KIND:ID=PATH] [--var-file PATH] [--var name=value] [--policy-file PATH] [--approved-by ID --approved-at RFC3339] [--target ADDRESS] [--exclude ADDRESS] [--replace ADDRESS] [--out PATH] [--hcl-out PATH]\n")
		fmt.Fprintf(fs.Output(), "\nBuilds a deterministic desired-state plan from native UWS/Ramen project artifacts or transitional Terraform/OpenTofu facts, API source metadata, and recorded SQLite state. It does not execute Terraform, providers, API source operations, refresh, apply, or UWS workflows.\n\n")
		printFlagDefaultsExcluding(fs, map[string]bool{"destroy": true})
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	destroySet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "destroy" {
			destroySet = true
		}
	})
	if destroySet {
		fmt.Fprintln(os.Stderr, "ramen plan --destroy is deprecated; model DELETE operations in the native project and use ramen apply --plan")
	}
	sources, err := parsePlanAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	path := *statePath
	if strings.TrimSpace(path) == "" {
		path = statePathOrDefault(*statePath, *projectPath, *configDir, *workspace)
	}
	approvers := approvalInputsOrExit(*approvedBy, *approvalRole, *approvalContext, *approvedAt)
	result, err := tfplan.Build(ctx, tfplan.Options{
		ConfigDir:   *configDir,
		ProjectPath: *projectPath,
		StatePath:   path,
		APISources:  sources,
		VarFiles:    []string(varFiles),
		Vars:        []string(cliVars),
		PolicyFiles: []string(policyFiles),
		Approvers:   approvers,
		Workspace:   *workspace,
		Action:      *action,
		OutPath:     *outPath,
		HCLPath:     *hclOutPath,
		Targets:     []string(targets),
		Excludes:    []string(excludes),
		Replaces:    []string(replaces),
		Destroy:     *destroy,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	summary := result.Plan.Summary
	fmt.Printf("ramen: plan create=%d update=%d delete=%d post=%d put=%d patch=%d replace=%d no-op=%d read=%d diagnostics=%d\n", summary.Create, summary.Update, summary.Delete, summary.Post, summary.Put, summary.Patch, summary.Replace, summary.NoOp, summary.Read, summary.Diagnostics)
	if result.OutPath != "" {
		fmt.Printf("  plan: %s\n", result.OutPath)
	}
	if result.HCLPath != "" {
		fmt.Printf("  plan-hcl: %s\n", result.HCLPath)
	}
	for _, diag := range result.Diagnostics {
		if diag.Severity == "error" {
			fmt.Fprintf(os.Stderr, "%s: %s\n", diag.Code, diag.Message)
		}
	}
	for _, diag := range result.Diagnostics {
		if diag.Severity == "error" {
			os.Exit(1)
		}
	}
	if *detailedExitCode && planHasChanges(result.Plan) {
		os.Exit(2)
	}
}

func runConvertCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		convertUsage(os.Stderr, "ramen convert")
		os.Exit(2)
	}
	switch args[0] {
	case "-h", "--help", "help":
		convertUsage(os.Stdout, "ramen convert")
	case "tf":
		runConvertTFCommand(ctx, args[1:])
	case "ansible":
		runConvertAnsibleCommand(ctx, args[1:])
	default:
		// Backward compatible: bare flags keep converting Terraform/OpenTofu.
		runConvertTFCommand(ctx, args)
	}
}

func convertUsage(out *os.File, command string) {
	fmt.Fprintf(out, "Usage: %s [tf] [--config-dir DIR] --api-source KIND:ID=PATH [--openapi ID=PATH] [--action create|update|delete|replace] [--target ADDRESS] [--out DIR] [--strict]\n", command)
	fmt.Fprintf(out, "       %s ansible --playbook FILE [--argspec ID=PATH] [--project-dir DIR] [--roles-path DIR] [--collections-path DIR] [--inventory FILE] [--extra-var NAME=VALUE] [--target-uws 1.5|1.6|1.7] [--out DIR] [--ignore-unsupported]\n\n", command)
	fmt.Fprintf(out, "Converts Terraform/OpenTofu configuration (default or `tf`) or an Ansible playbook (`ansible`) into native Ramen/UWS project artifacts. It does not execute Terraform, providers, Ansible modules, API source operations, or UWS workflows.\n\n")
}

func runConvertAnsibleCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("ramen convert ansible", flag.ExitOnError)
	playbook := fs.String("playbook", "", "Ansible playbook YAML file")
	outDir := fs.String("out", ".ramen/convert-ansible", "Output directory for converted artifacts")
	projectDir := fs.String("project-dir", "", "Static Ansible project root (defaults to the playbook directory)")
	strict := fs.Bool("strict", false, "Deprecated for Ansible conversion; unsupported constructs fail by default")
	ignoreUnsupported := fs.Bool("ignore-unsupported", false, "Write a partial workflow that omits unsupported Ansible constructs")
	targetUWS := fs.String("target-uws", "1.5", "UWS version declared by the emitted document: 1.5, 1.6, or 1.7. Module leaves are extension-owned at every version, so the shape does not change")
	var argspecs repeatedStringFlag
	var rolesPaths repeatedStringFlag
	var collectionsPaths repeatedStringFlag
	var inventoryPaths repeatedStringFlag
	var extraVars repeatedStringFlag
	fs.Var(&argspecs, "argspec", "Collection argspec document as ID=PATH (repeatable; uws.ansible.1.0 shape)")
	fs.Var(&rolesPaths, "roles-path", "Static Ansible roles search path for resolving play roles/import_role (repeatable)")
	fs.Var(&collectionsPaths, "collections-path", "Static Ansible collections search path for resolving FQCN collection roles (repeatable)")
	fs.Var(&inventoryPaths, "inventory", "Inventory file or directory input; when supplied, non-local plays lower as host fan-out over $inputs.hosts (repeatable)")
	fs.Var(&extraVars, "extra-var", "Static extra variable NAME=VALUE or @file (repeatable; recorded for review, not used for static expression lowering)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen convert ansible --playbook FILE [--argspec ID=PATH] [--project-dir DIR] [--roles-path DIR] [--collections-path DIR] [--inventory FILE] [--extra-var NAME=VALUE] [--target-uws 1.5|1.6|1.7] [--out DIR] [--ignore-unsupported]\n\n")
		fmt.Fprintf(fs.Output(), "Converts an Ansible playbook into a reviewable UWS workflow. Ansible module leaves are emitted as extension-owned operations carrying uws.ansible-module-call.1.0; --target-uws only selects the uws version the document declares, defaulting to 1.5. Unsupported constructs are reported explicitly and fail the command unless --ignore-unsupported is set.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if strings.TrimSpace(*playbook) == "" {
		fmt.Fprintln(os.Stderr, "ramen convert ansible: --playbook is required")
		os.Exit(2)
	}
	target := strings.TrimSpace(*targetUWS)
	switch target {
	case "",
		ansibleconvert.TargetUWS15, ansibleconvert.TargetUWS15 + ".0",
		ansibleconvert.TargetUWS16, ansibleconvert.TargetUWS16 + ".0",
		ansibleconvert.TargetUWS17, ansibleconvert.TargetUWS17 + ".0":
	default:
		fmt.Fprintf(os.Stderr, "ramen convert ansible: unsupported --target-uws %q (want 1.5, 1.6, or 1.7)\n", *targetUWS)
		os.Exit(2)
	}
	specs := make([]ansibleconvert.ArgspecInput, 0, len(argspecs))
	for _, value := range argspecs {
		id, path, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(path) == "" {
			fmt.Fprintf(os.Stderr, "ramen convert ansible: --argspec must be ID=PATH, got %q\n", value)
			os.Exit(2)
		}
		specs = append(specs, ansibleconvert.ArgspecInput{ID: strings.TrimSpace(id), Path: strings.TrimSpace(path)})
	}
	result, err := ansibleconvert.Convert(ctx, ansibleconvert.Options{
		PlaybookPath:      *playbook,
		Argspecs:          specs,
		OutDir:            *outDir,
		Strict:            *strict,
		ProjectDir:        *projectDir,
		RolesPaths:        []string(rolesPaths),
		CollectionsPaths:  []string(collectionsPaths),
		InventoryPaths:    []string(inventoryPaths),
		ExtraVars:         []string(extraVars),
		TargetUWS:         target,
		IgnoreUnsupported: *ignoreUnsupported,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Converted playbook: %s\n", *playbook)
	if result.UWSPath != "" {
		fmt.Printf("UWS document: %s\n", result.UWSPath)
	} else {
		fmt.Println("UWS document: not written (unsupported features or no lowerable tasks; see diagnostics)")
	}
	fmt.Printf("Diagnostics: %s (%d total, %d strict)\n", result.DiagnosticsJSON, len(result.Diagnostics), result.StrictFailures)
	fmt.Printf("Review: %s\n", result.ReviewMD)
	if result.StrictFailures > 0 && !*ignoreUnsupported {
		fmt.Fprintf(os.Stderr, "ramen convert ansible: unsupported Ansible features found (%d strict diagnostics); workflow artifacts were not written\n", result.StrictFailures)
		for _, diag := range result.Diagnostics {
			if !diag.StrictFailure {
				continue
			}
			task := diag.Task
			if task == "" {
				task = "-"
			}
			fmt.Fprintf(os.Stderr, "- %s task=%q: %s\n", diag.Code, task, diag.Message)
		}
		fmt.Fprintln(os.Stderr, "rerun with --ignore-unsupported to write a partial workflow that omits unsupported constructs")
		os.Exit(3)
	}
}

func runConvertTFCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("ramen convert", flag.ExitOnError)
	configDir := fs.String("config-dir", ".", "Terraform/OpenTofu configuration directory")
	action := fs.String("action", "", "Managed resource action: create, update, delete, or replace")
	outDir := fs.String("out", "./.ramen/convert", "Output directory for draft review artifacts")
	strict := fs.Bool("strict", false, "Fail when strict-failure diagnostics remain")
	var openAPIs repeatedStringFlag
	var apiSources repeatedStringFlag
	var targets repeatedStringFlag
	fs.Var(&openAPIs, "openapi", "Repeatable OpenAPI input as ID=PATH")
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH; kind is openapi, aws-smithy, or google-discovery")
	fs.Var(&targets, "target", "Repeatable Terraform address target")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen convert [--config-dir DIR] --api-source KIND:ID=PATH [--openapi ID=PATH] [--action create|update|delete|replace] [--target ADDRESS] [--out DIR] [--strict]\n")
		fmt.Fprintf(fs.Output(), "\nGenerates draft Ramen review scaffolding from static Terraform/OpenTofu configuration and local API source documents. It does not execute Terraform, providers, API source operations, or UWS workflows.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	inputs, err := parseOpenAPIFlags(openAPIs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	sources, err := parseAPISourceFlags(apiSources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := tfconvert.Convert(ctx, tfconvert.Options{
		ConfigDir:  *configDir,
		OpenAPIs:   inputs,
		APISources: sources,
		Action:     *action,
		Targets:    []string(targets),
		OutDir:     *outDir,
		Strict:     *strict,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if result != nil {
			fmt.Fprintf(os.Stderr, "diagnostics: %s\n", result.DiagnosticsJSON)
		}
		os.Exit(1)
	}
	fmt.Printf("ramen: convert wrote %s\n", result.OutDir)
	fmt.Printf("  project:     %s\n", result.ProjectPath)
	fmt.Printf("  native:      %s\n", result.NativeProjectPath)
	fmt.Printf("  native-hcl:  %s\n", result.NativeProjectHCLPath)
	fmt.Printf("  uws:         %s\n", result.UWSPath)
	fmt.Printf("  uws-hcl:     %s\n", result.UWSHCLPath)
	fmt.Printf("  conversion:  %s\n", result.ConversionPath)
	fmt.Printf("  mappings:    %s\n", result.MappingsPath)
	fmt.Printf("  plan:        %s\n", result.PlanJSONPath)
	fmt.Printf("  diagnostics: %s\n", result.DiagnosticsJSON)
	fmt.Printf("  review:      %s\n", result.ReviewPath)
}

func parseOpenAPIFlags(values []string) ([]tfconvert.OpenAPIInput, error) {
	inputs := make([]tfconvert.OpenAPIInput, 0, len(values))
	for _, value := range values {
		id, path, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("--openapi must be ID=PATH, got %q", value)
		}
		inputs = append(inputs, tfconvert.OpenAPIInput{ID: strings.TrimSpace(id), Path: strings.TrimSpace(path)})
	}
	return inputs, nil
}

func parseAPISourceFlags(values []string) ([]tfconvert.APISourceInput, error) {
	inputs := make([]tfconvert.APISourceInput, 0, len(values))
	for _, value := range values {
		left, path, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(left) == "" || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("--api-source must be KIND:ID=PATH, got %q", value)
		}
		kind, id, ok := strings.Cut(left, ":")
		if !ok || strings.TrimSpace(kind) == "" || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("--api-source must be KIND:ID=PATH, got %q", value)
		}
		inputs = append(inputs, tfconvert.APISourceInput{Kind: strings.TrimSpace(kind), ID: strings.TrimSpace(id), Path: strings.TrimSpace(path)})
	}
	return inputs, nil
}

func parsePlanAPISourceFlags(values []string) ([]tfplan.APISourceInput, error) {
	inputs := make([]tfplan.APISourceInput, 0, len(values))
	for _, value := range values {
		left, path, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(left) == "" || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("--api-source must be KIND:ID=PATH, got %q", value)
		}
		kind, id, ok := strings.Cut(left, ":")
		if !ok || strings.TrimSpace(kind) == "" || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("--api-source must be KIND:ID=PATH, got %q", value)
		}
		inputs = append(inputs, tfplan.APISourceInput{Kind: strings.TrimSpace(kind), ID: strings.TrimSpace(id), Path: strings.TrimSpace(path)})
	}
	return inputs, nil
}

func parseApplyAPISourceFlags(values []string) ([]tfapply.APISourceInput, error) {
	planInputs, err := parsePlanAPISourceFlags(values)
	if err != nil {
		return nil, err
	}
	inputs := make([]tfapply.APISourceInput, len(planInputs))
	for i, input := range planInputs {
		inputs[i] = tfapply.APISourceInput(input)
	}
	return inputs, nil
}

func parseValidateAPISourceFlags(values []string) ([]ramenvalidate.APISourceInput, error) {
	planInputs, err := parsePlanAPISourceFlags(values)
	if err != nil {
		return nil, err
	}
	inputs := make([]ramenvalidate.APISourceInput, len(planInputs))
	for i, input := range planInputs {
		inputs[i] = ramenvalidate.APISourceInput(input)
	}
	return inputs, nil
}

func parseReconcileAPISourceFlags(values []string) ([]reconcile.APISourceInput, error) {
	planInputs, err := parsePlanAPISourceFlags(values)
	if err != nil {
		return nil, err
	}
	inputs := make([]reconcile.APISourceInput, len(planInputs))
	for i, input := range planInputs {
		inputs[i] = reconcile.APISourceInput(input)
	}
	return inputs, nil
}
