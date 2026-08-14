package icot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenUdon/apitools"
	sharedicot "github.com/OpenUdon/authoring/icot"
	"github.com/OpenUdon/authoring/prompt"
	"github.com/OpenUdon/authoring/promptcontext"
	"github.com/OpenUdon/authoring/readiness"
	sharedsession "github.com/OpenUdon/authoring/session"
	"github.com/OpenUdon/authoring/transcript"
	ramenauthoring "github.com/OpenUdon/ramen/authoring"
	"github.com/OpenUdon/ramen/graph"
	"github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	ramvalidate "github.com/OpenUdon/ramen/validate"
)

type RunOptions struct {
	Session        Session
	Input          io.Reader
	Output         io.Writer
	DefaultMode    prompt.DefaultMode
	Agent          bool
	PrintOnly      bool
	AutosavePath   string
	TranscriptPath string
	NoTranscript   bool
	Validate       bool
	Graph          bool
	Plan           bool
	StatePath      string
	RemoteLookup   func(context.Context, RemoteLookupOptions) (DiscoveryResult, error)
}

type Artifact struct {
	Status         string              `json:"status"`
	ProjectPath    string              `json:"project_path,omitempty"`
	ProjectHCLPath string              `json:"project_hcl_path,omitempty"`
	SessionPath    string              `json:"session_path,omitempty"`
	Validation     *ramvalidate.Result `json:"validation,omitempty"`
	Graph          *graph.Document     `json:"graph,omitempty"`
	Plan           *plan.Result        `json:"plan,omitempty"`
	Backups        []string            `json:"backups,omitempty"`
}

type RunResult struct {
	Version            string                     `json:"version"`
	Status             string                     `json:"status"`
	Session            Session                    `json:"session"`
	Frontier           []readiness.Question       `json:"frontier,omitempty"`
	Proposal           Proposal                   `json:"proposal"`
	CandidateWorkflows []CandidateWorkflow        `json:"candidate_workflows,omitempty"`
	SourceCandidates   []SourcePlan               `json:"source_candidates,omitempty"`
	Blockers           []Blocker                  `json:"blockers,omitempty"`
	Artifact           Artifact                   `json:"artifact,omitempty"`
	Events             []transcript.Event         `json:"events,omitempty"`
	Turns              []sharedsession.PromptTurn `json:"turns,omitempty"`
	Rounds             int                        `json:"rounds,omitempty"`
	NoProgressRounds   int                        `json:"no_progress_rounds,omitempty"`
	Completed          bool                       `json:"completed"`
}

type Transcript struct {
	Version string                     `json:"version"`
	TimeUTC string                     `json:"time_utc"`
	Session Session                    `json:"session"`
	Turns   []sharedsession.PromptTurn `json:"turns,omitempty"`
	Events  []transcript.Event         `json:"events,omitempty"`
}

func Run(ctx context.Context, opts RunOptions) (RunResult, error) {
	state := opts.Session
	Normalize(&state)
	if err := ValidateSession(state); err != nil {
		return resultFor(state), err
	}
	out := opts.Output
	if out == nil {
		out = io.Discard
	}
	in := opts.Input
	if (opts.Agent || opts.PrintOnly) && in == nil {
		in = strings.NewReader("")
	}
	autosave := func(current Session) error { return nil }
	if !opts.Agent && !opts.PrintOnly && strings.TrimSpace(opts.AutosavePath) != "" {
		autosave = func(current Session) error { return SaveSession(opts.AutosavePath, current) }
	}
	sharedResult, runErr := sharedicot.Run[Session, promptcontext.Context, Artifact](ctx, in, out, sharedicot.Options[Session, promptcontext.Context, Artifact]{
		Session: state, Documents: []promptcontext.Context{state.Context}, DefaultMode: opts.DefaultMode,
		Normalize: func(current *Session) { Normalize(current) },
		CheckReadiness: func(current Session, _ []promptcontext.Context) []sharedsession.ReadinessIssue {
			return CheckReadiness(current)
		},
		Ready: func(current Session, issues []sharedsession.ReadinessIssue) bool { return Ready(current, issues) },
		PlanFrontier: func(current Session, _ []promptcontext.Context, _ []sharedsession.ReadinessIssue) []readiness.Question {
			frontier, err := PlanFrontier(current)
			if err != nil {
				return []readiness.Question{{ID: "interview.invalid", Prompt: err.Error(), Required: true, Forced: true, Priority: 1000}}
			}
			if containsQuestion(frontier, nodeProposal) {
				writeProposal(out, BuildProposal(current))
			}
			return frontier
		},
		ApplyRound: func(current *Session, answers []sharedicot.RoundAnswer, _ []promptcontext.Context) error {
			if err := ApplyRound(current, answers); err != nil {
				return err
			}
			if spec := strings.TrimSpace(current.Metadata["pending_source_input"]); spec != "" {
				source, err := parseLocalSourceSpec(spec)
				if err != nil {
					return err
				}
				discovered, err := DiscoverLocalSources(ctx, DiscoveryOptions{Goal: current.Boundary.Outcome, Sources: []apitools.LocalSource{source}})
				if err != nil {
					return err
				}
				ReplaceDiscovery(current, discovered)
				delete(current.Metadata, "pending_source_input")
				Normalize(current)
			}
			if current.Metadata["pending_remote_lookup"] == "true" {
				lookup := opts.RemoteLookup
				if lookup == nil {
					lookup = DiscoverRemoteSources
				}
				discovered, err := lookup(ctx, RemoteLookupOptions{Query: current.Boundary.Outcome})
				if err != nil {
					return err
				}
				ReplaceDiscovery(current, discovered)
				delete(current.Metadata, "pending_remote_lookup")
				Normalize(current)
			}
			return nil
		},
		Autosave: autosave,
		FinalConfirm: func(ctx context.Context, current *Session, _ []promptcontext.Context, _ *[]transcript.Event) (Artifact, error) {
			return finalize(ctx, current, opts)
		},
	})
	result := resultFor(sharedResult.Session)
	result.Frontier = append([]readiness.Question(nil), sharedResult.Frontier...)
	result.Artifact = sharedResult.Artifact
	result.Events = append([]transcript.Event(nil), sharedResult.Events...)
	result.Turns = append([]sharedsession.PromptTurn(nil), sharedResult.Turns...)
	result.Rounds = sharedResult.Rounds
	result.NoProgressRounds = sharedResult.NoProgressRounds
	result.Completed = sharedResult.Completed
	switch {
	case runErr == nil && result.Artifact.Status == "complete":
		result.Status = "complete"
	case runErr == nil && result.Artifact.Status == "draft":
		result.Status = "draft"
	case runErr == nil && result.Artifact.Status == "proposal":
		result.Status = "proposal"
	case errors.Is(runErr, sharedicot.ErrCanceled):
		result.Status = "canceled"
	case errors.Is(runErr, sharedicot.ErrNeedsInput):
		result.Status = "needs_input"
	default:
		result.Status = "failed"
	}
	if !opts.NoTranscript && !opts.Agent && !opts.PrintOnly && strings.TrimSpace(opts.TranscriptPath) != "" {
		if err := saveTranscript(opts.TranscriptPath, Transcript{
			Version: TranscriptVersion, TimeUTC: time.Now().UTC().Format(time.RFC3339), Session: result.Session,
			Turns: result.Turns, Events: result.Events,
		}); err != nil && runErr == nil {
			runErr = err
			result.Status = "failed"
		}
	}
	return result, runErr
}

func resultFor(session Session) RunResult {
	Normalize(&session)
	return RunResult{
		Version: ReportVersion, Status: "needs_input", Session: session, Proposal: BuildProposal(session),
		CandidateWorkflows: append([]CandidateWorkflow(nil), session.CandidateWorkflows...),
		SourceCandidates:   publicSourcePlans(session.SourcePlans), Blockers: append([]Blocker(nil), session.Blockers...),
	}
}

func finalize(ctx context.Context, session *Session, opts RunOptions) (Artifact, error) {
	if session == nil {
		return Artifact{}, fmt.Errorf("Ramen iCoT session is required")
	}
	Normalize(session)
	if session.Approval != "approve" && session.Approval != "save-draft" {
		return Artifact{}, fmt.Errorf("proposal approval is required")
	}
	if !approvalConfirmed(*session) {
		return Artifact{}, fmt.Errorf("proposal approval lacks linked durable user-decision evidence")
	}
	if session.Approval == "approve" && len(session.Interview.Deferrals) > 0 {
		return Artifact{}, fmt.Errorf("incomplete proposal cannot write a runnable Ramen project")
	}
	if len(session.Intent.Resources) > 0 && resourceMutates(session.Intent.Resources[0]) && !mutationConfirmed(*session) {
		return Artifact{}, fmt.Errorf("unconfirmed side-effect commitment cannot write a Ramen project")
	}
	if opts.PrintOnly || opts.Agent {
		return Artifact{Status: "proposal"}, nil
	}
	if session.Approval == "save-draft" && strings.TrimSpace(opts.AutosavePath) != "" {
		if err := SaveSession(opts.AutosavePath, *session); err != nil {
			return Artifact{}, err
		}
	}
	if (len(session.Intent.Resources) == 0 || len(session.Intent.SelectedSourceIDs) == 0) && session.Approval != "save-draft" {
		return Artifact{}, fmt.Errorf("approved proposal is missing source-derived resources")
	}
	authoringOptions := ramenauthoring.Options{
		Goal: session.Boundary.Outcome, ProjectName: session.Intent.ProjectName, OutDir: session.OutDir,
		Context: session.Context, Resources: session.Intent.Resources, CandidateWorkflows: projectCandidateWorkflows(session.CandidateWorkflows),
	}
	for _, source := range selectedSourcePlans(*session) {
		authoringOptions.APISources = append(authoringOptions.APISources, project.APISource{Kind: source.Kind, ID: source.ID, Path: source.TargetPath})
	}
	document, err := ramenauthoring.BuildProject(authoringOptions)
	if session.Approval == "save-draft" && len(session.Intent.Resources) == 0 {
		document, err = ramenauthoring.BuildIncompleteProject(authoringOptions)
	}
	if err != nil {
		return Artifact{}, err
	}
	sources := make([]ramenauthoring.SourceMaterialization, 0, len(session.Intent.SelectedSourceIDs))
	for _, source := range selectedSourcePlans(*session) {
		sources = append(sources, ramenauthoring.SourceMaterialization{
			Kind: source.Kind, ID: source.ID, SourcePath: source.Path, TargetPath: source.TargetPath, SHA256: source.SHA256,
			Content: append([]byte(nil), source.Content...),
		})
	}
	gates := map[string]bool{}
	for _, gate := range session.Intent.Verification {
		gates[gate] = true
	}
	draft := session.Approval == "save-draft"
	removePaths := []string(nil)
	if !draft {
		removePaths = append(removePaths, project.DraftFile, project.DraftHCL)
		if relative, ok := relativeOutputPath(session.OutDir, opts.AutosavePath); ok {
			removePaths = append(removePaths, relative)
		}
	}
	materialized, err := ramenauthoring.MaterializeProject(ctx, ramenauthoring.MaterializeOptions{
		Document: document, OutDir: session.OutDir, Sources: sources, Force: session.Force,
		Draft: draft, RemovePaths: removePaths,
		Validate: !draft && (opts.Validate || gates["validate"]), Graph: !draft && (opts.Graph || gates["graph"]), Plan: !draft && (opts.Plan || gates["plan"]), StatePath: opts.StatePath,
	})
	if err != nil {
		return Artifact{}, err
	}
	status := "complete"
	if draft {
		status = "draft"
	}
	return Artifact{
		Status: status, ProjectPath: materialized.ProjectPath, ProjectHCLPath: materialized.ProjectHCLPath,
		Validation: materialized.Validation, Graph: materialized.Graph, Plan: materialized.Plan, Backups: materialized.Backups,
		SessionPath: opts.AutosavePath,
	}, nil
}

func relativeOutputPath(outDir, path string) (string, bool) {
	if strings.TrimSpace(outDir) == "" || strings.TrimSpace(path) == "" {
		return "", false
	}
	absoluteOut, err := filepath.Abs(outDir)
	if err != nil {
		return "", false
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(absoluteOut, absolutePath)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

func projectCandidateWorkflows(candidates []CandidateWorkflow) []project.CandidateWorkflow {
	out := make([]project.CandidateWorkflow, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, project.CandidateWorkflow{
			Title: candidate.Title, Outcome: candidate.Outcome,
			DeferralReason: candidate.DeferralReason, PromotionTrigger: candidate.PromotionTrigger,
		})
	}
	return out
}

func containsQuestion(questions []readiness.Question, id string) bool {
	for _, question := range questions {
		if question.ID == id {
			return true
		}
	}
	return false
}

func writeProposal(out io.Writer, proposal Proposal) {
	data, err := json.MarshalIndent(proposal, "", "  ")
	if err != nil {
		fmt.Fprintf(out, "Proposal unavailable: %v\n", err)
		return
	}
	fmt.Fprintln(out, "Complete Ramen proposal:")
	fmt.Fprintln(out, string(data))
}

func saveTranscript(path string, record Transcript) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := preparePrivateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".icot-transcript-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func parseLocalSourceSpec(value string) (apitools.LocalSource, error) {
	left, path, ok := strings.Cut(strings.TrimSpace(value), "=")
	if !ok || strings.TrimSpace(path) == "" {
		return apitools.LocalSource{}, fmt.Errorf("local API source must use KIND:ID=PATH")
	}
	kind, id, ok := strings.Cut(left, ":")
	if !ok || strings.TrimSpace(kind) == "" || strings.TrimSpace(id) == "" {
		return apitools.LocalSource{}, fmt.Errorf("local API source must use KIND:ID=PATH")
	}
	return apitools.LocalSource{Kind: strings.TrimSpace(kind), ID: strings.TrimSpace(id), Path: strings.TrimSpace(path)}, nil
}
