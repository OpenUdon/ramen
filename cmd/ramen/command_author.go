package main

import (
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
	"strings"
	"time"

	"github.com/OpenUdon/apitools"
	sharedicot "github.com/OpenUdon/authoring/icot"
	sharedicotcli "github.com/OpenUdon/authoring/icotcli"
	sharedpromptcontext "github.com/OpenUdon/authoring/promptcontext"
	sharedreadiness "github.com/OpenUdon/authoring/readiness"
	sharedreport "github.com/OpenUdon/authoring/report"
	"github.com/OpenUdon/authoring/trust"
	ramenauthoring "github.com/OpenUdon/ramen/authoring"
	"github.com/OpenUdon/ramen/graph"
	ramenicot "github.com/OpenUdon/ramen/internal/icot"
	tfplan "github.com/OpenUdon/ramen/plan"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
)

const (
	defaultICOTLLMProvider  = "copilot-api"
	defaultICOTCopilotModel = "gpt-5.4-mini"
)

var discoverICOTRemoteSources = ramenicot.DiscoverRemoteSources

type authorCLIResult struct {
	Version            string                        `json:"version,omitempty"`
	Status             string                        `json:"status,omitempty"`
	Report             sharedreport.Result           `json:"report"`
	ProjectPath        string                        `json:"project_path,omitempty"`
	ProjectHCLPath     string                        `json:"project_hcl_path,omitempty"`
	Validation         *ramenvalidate.Result         `json:"validation,omitempty"`
	Graph              *graph.Document               `json:"graph,omitempty"`
	Plan               *tfplan.Result                `json:"plan,omitempty"`
	Session            *ramenicot.Session            `json:"session,omitempty"`
	Frontier           []sharedreadiness.Question    `json:"frontier,omitempty"`
	Proposal           *ramenicot.Proposal           `json:"proposal,omitempty"`
	CandidateWorkflows []ramenicot.CandidateWorkflow `json:"candidate_workflows,omitempty"`
	SourceCandidates   []ramenicot.SourcePlan        `json:"source_candidates,omitempty"`
	Blockers           []ramenicot.Blocker           `json:"blockers,omitempty"`
	Backups            []string                      `json:"backups,omitempty"`
}

func runAuthorCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("author", flag.ExitOnError)
	contextPath := fs.String("context", "", "authoring.prompt-context.v2 JSON input path")
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
	var openAPIs repeatedStringFlag
	var sourceRoots repeatedStringFlag
	fs.Var(&apiSources, "api-source", "Repeatable API source input as KIND:ID=PATH")
	fs.Var(&openAPIs, "openapi", "Repeatable OpenAPI shorthand as ID=PATH")
	fs.Var(&sourceRoots, "source-root", "Repeatable explicit local file or directory root for bounded discovery")
	network := fs.String("network", "", "Remote lookup policy: never, ask, or allow")
	resumePath := fs.String("resume", "", "Resume a ramen.icot-session.v2 JSON file")
	sessionPath := fs.String("session", "", "Autosave path for ramen.icot-session.v2 JSON")
	transcriptPath := fs.String("transcript", "", "Transcript path for ramen.icot-transcript.v2 JSON")
	force := fs.Bool("force", false, "Replace differing approved targets while retaining backups")
	printOnly := fs.Bool("print", false, "Print interview/proposal state without writing deliverables")
	maxEntries := fs.Int("source-max-entries", 0, "Optional local discovery visit limit")
	maxCandidates := fs.Int("source-max-candidates", 0, "Optional local discovery candidate limit")
	maxBytes := fs.Int64("source-max-bytes", 0, "Optional local discovery per-file byte limit")
	common := sharedicotcli.Flags{NoLLM: true, PromptMode: "normal"}
	sharedicotcli.AddFlags(fs, &common)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ramen icot [--goal TEXT] [--api-source KIND:ID=PATH ...] [--openapi ID=PATH ...] [--source-root PATH ...] [--network never|ask|allow] [--resume SESSION] [--project-name NAME] [--out DIR] [--validate] [--graph] [--plan] [--state PATH] [--prompt-mode full|normal|fast] [--no-llm] [--agent] [--print] [--json] [--answers PATH] [--no-transcript] [--report PATH]\n")
		fmt.Fprintf(fs.Output(), "\nRuns a dependency-aware, proposal-gated interview that authors a native UWS/Ramen project from validated API source evidence. It never executes API operations, Terraform/OpenTofu, providers, refresh, apply, or UWS workflows.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	defaultMode, err := sharedicotcli.PromptDefaultMode(common.PromptMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	policy, err := resolveRamenICOTNetworkPolicy(*network, common.Agent)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	answerInput, err := loadICOTV2Answers(common.Answers)
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
	var session ramenicot.Session
	if strings.TrimSpace(*resumePath) != "" {
		session, err = ramenicot.LoadSession(*resumePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		ramenicot.PrepareResume(&session)
	} else {
		session = ramenicot.SeedSession(*goal, *projectName, *outDir, policy, nil, nil, sharedpromptcontext.Context{})
	}
	session.NetworkPolicy = policy
	session.Force = session.Force || *force
	if flagWasSet(fs, "out") || strings.TrimSpace(session.OutDir) == "" {
		session.OutDir = *outDir
	}
	localSources, parseErr := parseICOTLocalSources([]string(apiSources), []string(openAPIs))
	if parseErr != nil {
		fmt.Fprintln(os.Stderr, parseErr)
		os.Exit(2)
	}
	if len(localSources) > 0 || len(sourceRoots) > 0 {
		discovered, discoverErr := ramenicot.DiscoverLocalSources(ctx, ramenicot.DiscoveryOptions{
			Goal: firstNonEmpty(*goal, session.Boundary.Outcome), Roots: []string(sourceRoots), Sources: localSources,
			MaxVisitedEntries: *maxEntries, MaxCandidates: *maxCandidates, MaxBytes: *maxBytes,
		})
		if discoverErr != nil {
			session.Blockers = []ramenicot.Blocker{{Code: "ramen.icot.api_source_invalid", Message: discoverErr.Error(), Remediation: "Correct the explicit source declaration or narrow the source root.", Deferrable: true}}
		} else {
			ramenicot.ReplaceDiscovery(&session, discovered)
		}
	}
	if len(session.SourcePlans) == 0 && len(session.Blockers) == 0 && policy == "allow" {
		discovered, discoverErr := discoverICOTRemoteSources(ctx, ramenicot.RemoteLookupOptions{Query: firstNonEmpty(*goal, session.Boundary.Outcome)})
		if discoverErr != nil {
			if errors.Is(discoverErr, context.Canceled) {
				fmt.Fprintln(os.Stderr, discoverErr)
				os.Exit(1)
			}
			session.Blockers = []ramenicot.Blocker{{Code: "ramen.icot.remote_lookup_failed", Message: discoverErr.Error(), Remediation: "Provide --api-source or --source-root and retry.", Deferrable: true}}
		} else {
			ramenicot.ReplaceDiscovery(&session, discovered)
		}
	}
	if common.Agent && len(session.SourcePlans) == 0 && len(session.Blockers) == 0 {
		session.Blockers = []ramenicot.Blocker{{Code: "ramen.icot.missing_api_source", Message: "No explicit local API source or source root was provided.", Remediation: "Provide --api-source or --source-root.", Deferrable: true}}
	}
	if err := applyICOTLLMSuggestion(ctx, &session, common); err != nil {
		session.Blockers = append(session.Blockers, ramenicot.Blocker{Code: "ramen.icot.llm_unavailable", Message: err.Error(), Remediation: "Retry without model assistance using --no-llm or correct the model configuration.", Deferrable: true})
	}
	if strings.TrimSpace(*sessionPath) == "" {
		if strings.TrimSpace(*resumePath) != "" {
			*sessionPath = *resumePath
		} else {
			*sessionPath = filepath.Join(session.OutDir, ".icot", "session.json")
		}
	}
	if strings.TrimSpace(*transcriptPath) == "" {
		*transcriptPath = filepath.Join(session.OutDir, ".icot", "transcript.json")
	}
	v2Result, runErr := ramenicot.Run(ctx, ramenicot.RunOptions{
		Session: session, Input: reader, Output: promptOut, DefaultMode: defaultMode,
		Agent: common.Agent, PrintOnly: *printOnly, AutosavePath: *sessionPath,
		TranscriptPath: *transcriptPath, NoTranscript: common.NoTranscript,
		Validate: *validateGate, Graph: *graphGate, Plan: *planGate, StatePath: *statePath,
		RemoteLookup: discoverICOTRemoteSources,
	})
	result := authorCLIResultForICOTV2(v2Result, runErr)
	if v2Result.Status == "complete" && !common.Agent && !*printOnly && strings.TrimSpace(*sessionPath) != "" {
		if err := os.Remove(*sessionPath); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "ramen.icot.session_cleanup_failed: %v\n", err)
		}
	}
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
	if runErr != nil && !errors.Is(runErr, sharedicot.ErrNeedsInput) && !errors.Is(runErr, sharedicot.ErrCanceled) {
		fmt.Fprintln(os.Stderr, runErr)
	}
	if v2Result.Status != "complete" && !(*printOnly && v2Result.Status == "proposal") {
		os.Exit(1)
	}
}

func runICOTDraft(ctx context.Context, goal, projectName, outDir, statePath string, sourceFlags []string, validateGate, graphGate, planGate bool, common sharedicotcli.Flags, answerInput io.Reader, promptOut io.Writer) authorCLIResult {
	localSources, err := parseICOTLocalSources(sourceFlags, nil)
	if err != nil {
		return icotFailed("ramen.icot.api_source_invalid", err.Error())
	}
	discovered := ramenicot.DiscoveryResult{}
	if len(localSources) > 0 {
		discovered, err = ramenicot.DiscoverLocalSources(ctx, ramenicot.DiscoveryOptions{Goal: goal, Sources: localSources})
		if err != nil {
			return icotFailed("ramen.icot.api_source_invalid", err.Error())
		}
	}
	session := ramenicot.SeedSession(goal, projectName, outDir, "never", discovered.Plans, discovered.Blockers, discovered.Context)
	session.Discovery = discovered.Report
	if err := applyICOTLLMSuggestion(ctx, &session, common); err != nil {
		return icotFailed("ramen.icot.llm_unavailable", err.Error())
	}
	mode, err := sharedicotcli.PromptDefaultMode(common.PromptMode)
	if err != nil {
		return icotFailed("ramen.icot.prompt_mode_invalid", err.Error())
	}
	result, runErr := ramenicot.Run(ctx, ramenicot.RunOptions{
		Session: session, Input: answerInput, Output: promptOut, DefaultMode: mode, Agent: common.Agent,
		NoTranscript: true, Validate: validateGate, Graph: graphGate, Plan: planGate, StatePath: statePath,
	})
	return authorCLIResultForICOTV2(result, runErr)
}

func applyICOTLLMSuggestion(ctx context.Context, session *ramenicot.Session, common sharedicotcli.Flags) error {
	if common.NoLLM || session == nil || len(session.Context.Operations) == 0 {
		return nil
	}
	advisor, config, err := newICOTAssistant(common, os.Getenv)
	if err != nil {
		return err
	}
	if advisor == nil {
		return nil
	}
	suggestion, err := advisor.SuggestOperation(ctx, icotAssistantRequest{
		Goal: session.Boundary.Outcome, Context: session.Context,
		Provider: config.Provider, Model: config.Model, Temperature: config.Temperature,
	})
	if err != nil {
		return err
	}
	if session.Metadata == nil {
		session.Metadata = map[string]string{}
	}
	session.Metadata["llm_provider"] = config.Provider
	session.Metadata["llm_model"] = config.Model
	if strings.TrimSpace(suggestion.OperationID) != "" {
		session.Metadata["llm_suggested_operation_id"] = strings.TrimSpace(suggestion.OperationID)
	}
	return nil
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

func icotFailed(code, message string) authorCLIResult {
	return authorCLIResult{Report: sharedreport.Normalize(sharedreport.Result{
		Status:      sharedreport.StatusFailed,
		Summary:     message,
		Diagnostics: []trust.DiagnosticRecord{{Code: code, Severity: "error", Message: message}},
		Metadata:    map[string]string{"adapter": "ramen.icot"},
	})}
}

func loadICOTV2Answers(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("icot.answers_read_error: %w", err)
	}
	var answers ramenicot.AnswersFile
	if err := json.Unmarshal(data, &answers); err != nil {
		return "", fmt.Errorf("icot.answers_parse_error: v2 answers must be JSON: %w", err)
	}
	if answers.Version != ramenicot.AnswersVersion {
		return "", fmt.Errorf("icot.answers_version_invalid: got %q, want %q; v1 and unversioned inputs are not compatible", answers.Version, ramenicot.AnswersVersion)
	}
	return answers.Input, nil
}

func resolveRamenICOTNetworkPolicy(value string, agent bool) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		if agent {
			return "never", nil
		}
		return "ask", nil
	}
	if agent && value == "ask" {
		return "never", nil
	}
	switch value {
	case "never", "ask", "allow":
		return value, nil
	default:
		return "", fmt.Errorf("--network must be never, ask, or allow")
	}
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(current *flag.Flag) {
		if current.Name == name {
			set = true
		}
	})
	return set
}

func parseICOTLocalSources(apiValues, openAPIValues []string) ([]apitools.LocalSource, error) {
	parsed, err := parseAPISourceFlags(apiValues)
	if err != nil {
		return nil, err
	}
	out := make([]apitools.LocalSource, 0, len(parsed)+len(openAPIValues))
	for _, input := range parsed {
		out = append(out, apitools.LocalSource{Kind: input.Kind, ID: input.ID, Path: input.Path})
	}
	openAPIs, err := parseOpenAPIFlags(openAPIValues)
	if err != nil {
		return nil, err
	}
	for _, input := range openAPIs {
		out = append(out, apitools.LocalSource{Kind: apitools.APISourceKindOpenAPI, ID: input.ID, Path: input.Path})
	}
	return out, nil
}

func authorCLIResultForICOTV2(result ramenicot.RunResult, runErr error) authorCLIResult {
	reportStatus := sharedreport.StatusNeedsInput
	switch result.Status {
	case "complete":
		reportStatus = sharedreport.StatusComplete
	case "canceled":
		reportStatus = sharedreport.StatusCanceled
	case "failed":
		reportStatus = sharedreport.StatusFailed
	}
	issues := make([]sharedreadiness.Issue, 0, len(result.Frontier)+len(result.Blockers))
	for _, question := range result.Frontier {
		issues = append(issues, sharedreadiness.Issue{
			Code: "ramen.icot." + strings.ReplaceAll(question.ID, ".", "_"), Severity: sharedreadiness.SeverityBlocking,
			Slot: firstNonEmpty(question.ID, "interview"), Message: question.Prompt, SuggestedAnswer: question.Recommendation,
		})
	}
	diagnostics := make([]trust.DiagnosticRecord, 0, len(result.Blockers)+1)
	for _, blocker := range result.Blockers {
		diagnostics = append(diagnostics, trust.DiagnosticRecord{Code: blocker.Code, Severity: "blocking", Message: blocker.Message, Detail: map[string]any{"remediation": blocker.Remediation}})
	}
	if runErr != nil && !errors.Is(runErr, sharedicot.ErrNeedsInput) && !errors.Is(runErr, sharedicot.ErrCanceled) {
		diagnostics = append(diagnostics, trust.DiagnosticRecord{Code: "ramen.icot.run_failed", Severity: "error", Message: runErr.Error()})
	}
	readinessResult := sharedreadiness.Evaluate(issues)
	report := sharedreport.Result{
		Status: reportStatus, Summary: result.Proposal.Outcome, Diagnostics: diagnostics,
		Metadata: map[string]string{"adapter": "ramen.icot.v2", "session_version": ramenicot.SessionVersion, "report_version": ramenicot.ReportVersion},
	}
	if len(issues) > 0 {
		report.Readiness = &readinessResult
		report.TopIssue = readinessResult.TopIssue
	}
	if result.Artifact.ProjectPath != "" {
		report.Artifacts = append(report.Artifacts, trust.ArtifactRecord{Path: result.Artifact.ProjectPath, Kind: "ramen.project", Required: true})
	}
	if result.Artifact.ProjectHCLPath != "" {
		report.Artifacts = append(report.Artifacts, trust.ArtifactRecord{Path: result.Artifact.ProjectHCLPath, Kind: "ramen.project.hcl"})
	}
	report = sharedreport.Normalize(report)
	session := result.Session
	proposal := result.Proposal
	return authorCLIResult{
		Version: result.Version, Status: result.Status, Report: report,
		ProjectPath: result.Artifact.ProjectPath, ProjectHCLPath: result.Artifact.ProjectHCLPath,
		Validation: result.Artifact.Validation, Graph: result.Artifact.Graph, Plan: result.Artifact.Plan,
		Session: &session, Frontier: result.Frontier, Proposal: &proposal,
		CandidateWorkflows: result.CandidateWorkflows, SourceCandidates: result.SourceCandidates,
		Blockers: result.Blockers, Backups: result.Artifact.Backups,
	}
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
