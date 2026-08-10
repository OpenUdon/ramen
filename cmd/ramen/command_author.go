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
	"path/filepath"
	"slices"
	"strings"
	"time"

	sharedicotcli "github.com/OpenUdon/authoring/icotcli"
	sharedpromptcontext "github.com/OpenUdon/authoring/promptcontext"
	sharedreadiness "github.com/OpenUdon/authoring/readiness"
	sharedreport "github.com/OpenUdon/authoring/report"
	"github.com/OpenUdon/authoring/trust"
	ramenauthoring "github.com/OpenUdon/ramen/authoring"
	"github.com/OpenUdon/ramen/graph"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
)

const (
	defaultICOTLLMProvider  = "copilot-api"
	defaultICOTCopilotModel = "gpt-5.4-mini"
)

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
