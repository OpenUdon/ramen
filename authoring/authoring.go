package authoring

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	sharedicot "github.com/OpenUdon/authoring/icot"
	"github.com/OpenUdon/authoring/lifecycle"
	"github.com/OpenUdon/authoring/promptcontext"
	sharedreadiness "github.com/OpenUdon/authoring/readiness"
	sharedreport "github.com/OpenUdon/authoring/report"
	"github.com/OpenUdon/authoring/session"
	"github.com/OpenUdon/authoring/transcript"
	"github.com/OpenUdon/authoring/trust"
	"github.com/OpenUdon/ramen/project"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

const (
	missingGoalCode    = "ramen.authoring.missing_goal"
	missingMappingCode = "ramen.authoring.missing_mapping_metadata"
)

// Options configures the Ramen Authoring adapter spike.
type Options struct {
	Goal        string
	ProjectName string
	OutDir      string
	Context     promptcontext.Context
	Validate    bool
}

// Result is the adapter outcome. Report is the product-neutral Authoring
// result; Ramen-specific artifacts stay in ProjectPath and Validation.
type Result struct {
	Report      sharedreport.Result
	ProjectPath string
	Validation  *ramenvalidate.Result
}

type state struct {
	Goal        string
	ProjectName string
	Context     promptcontext.Context
	Draft       *uws1.Document
	ProjectPath string
	Validation  *ramenvalidate.Result
}

type artifact struct {
	Path       string
	Validation *ramenvalidate.Result
}

// DraftProject drafts a native Ramen/UWS project skeleton from an operator goal
// and prompt-safe operation context.
func DraftProject(ctx context.Context, opts Options) (Result, error) {
	runtime := runtime{outDir: firstNonEmpty(opts.OutDir, "."), validate: opts.Validate}
	initial := state{
		Goal:        strings.TrimSpace(opts.Goal),
		ProjectName: strings.TrimSpace(opts.ProjectName),
		Context:     promptcontext.Normalize(opts.Context),
	}
	run, err := sharedicot.RunRuntime[state, promptcontext.Context, artifact](
		ctx,
		strings.NewReader(""),
		io.Discard,
		runtime,
		sharedicot.RuntimeConfig[state, promptcontext.Context]{
			Session:     initial,
			Documents:   []promptcontext.Context{initial.Context},
			MaxAttempts: 3,
		},
	)
	result := Result{ProjectPath: run.Artifact.Path, Validation: run.Artifact.Validation}
	if result.ProjectPath == "" {
		result.ProjectPath = run.Session.ProjectPath
	}
	if result.Validation == nil {
		result.Validation = run.Session.Validation
	}
	result.Report = reportForRun(run, err, result)
	if err != nil {
		if errors.Is(err, sharedicot.ErrNeedsInput) {
			return result, nil
		}
		return result, err
	}
	return result, nil
}

type runtime struct {
	outDir   string
	validate bool
}

func (runtime) Normalize(s *state) {
	s.Goal = strings.TrimSpace(s.Goal)
	s.ProjectName = strings.TrimSpace(s.ProjectName)
	if s.ProjectName == "" {
		s.ProjectName = slug(firstNonEmpty(s.Goal, "ramen project"))
	}
	s.Context = promptcontext.Normalize(s.Context)
}

func (r runtime) Draft(_ context.Context, s state, _ []promptcontext.Context, _ []session.ReadinessIssue, _ int) (state, error) {
	s.Draft = buildDocument(s)
	return s, nil
}

func (runtime) Readiness(s state, _ []promptcontext.Context) []session.ReadinessIssue {
	var issues []session.ReadinessIssue
	if strings.TrimSpace(s.Goal) == "" {
		issues = append(issues, session.ReadinessIssue{
			Code:     missingGoalCode,
			Severity: sharedreadiness.SeverityBlocking,
			Slot:     "goal",
			Message:  "Describe the Ramen desired-state project goal.",
		})
	}
	if firstOperation(s.Context).ID == "" {
		issues = append(issues, session.ReadinessIssue{
			Code:            missingMappingCode,
			Severity:        sharedreadiness.SeverityBlocking,
			Slot:            "prompt_context.operations",
			Message:         "Provide prompt-safe API operation metadata before drafting a Ramen project skeleton.",
			SuggestedAnswer: "Add at least one source document and operation candidate.",
		})
	}
	return issues
}

func (runtime) Ready(s state, issues []session.ReadinessIssue) bool {
	return sharedreadiness.Ready(issues) && s.Draft != nil
}

func (runtime) ShouldDraft(s state, _ []promptcontext.Context, issues []session.ReadinessIssue, _ int) bool {
	return s.Draft == nil && sharedreadiness.Ready(issues)
}

func (runtime) PlanQuestion(_ state, _ []promptcontext.Context, issues []session.ReadinessIssue) sharedicot.Question {
	if top := sharedreadiness.TopIssue(issues); top != nil {
		return sharedicot.Question{
			ID:            top.Code,
			Prompt:        top.Message,
			Slots:         []string{top.Slot},
			Required:      true,
			DefaultAnswer: top.SuggestedAnswer,
		}
	}
	return sharedicot.Question{}
}

func (runtime) ApplyAnswer(s *state, question sharedicot.Question, answer string, _ []promptcontext.Context) error {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return nil
	}
	for _, slot := range question.Slots {
		if slot == "goal" {
			s.Goal = answer
			return nil
		}
	}
	return nil
}

func (r runtime) WriteArtifacts(ctx context.Context, s *state, _ []promptcontext.Context, _ *[]transcript.Event) (artifact, error) {
	if s == nil || s.Draft == nil {
		return artifact{}, fmt.Errorf("draft is required")
	}
	if err := os.MkdirAll(r.outDir, 0o755); err != nil {
		return artifact{}, err
	}
	path := filepath.Join(r.outDir, project.DefaultFile)
	data, err := uwsconvert.MarshalYAML(s.Draft)
	if err != nil {
		return artifact{}, err
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	if err := lifecycle.AtomicWrite(path, data, 0o644); err != nil {
		return artifact{}, err
	}
	out := artifact{Path: path}
	s.ProjectPath = path
	if r.validate {
		validation, err := ramenvalidate.Run(ctx, ramenvalidate.Options{ProjectPath: path})
		if err != nil {
			return artifact{}, err
		}
		out.Validation = validation
		s.Validation = validation
	}
	return out, nil
}

func buildDocument(s state) *uws1.Document {
	source := firstSource(s.Context)
	operation := firstOperation(s.Context)
	sourceOperationID := firstNonEmpty(operation.OperationID, operation.ID)
	sourceID := firstNonEmpty(source.ID, operation.SourceID, "api")
	sourceKind := firstNonEmpty(source.Kind, "openapi")
	resourceName := slug(firstNonEmpty(s.ProjectName, s.Goal, "resource"))
	localOperationID := "create_" + resourceName
	profile := project.Profile{
		Version: project.Version,
		APISources: []project.APISource{{
			Kind: sourceKind,
			ID:   sourceID,
			Path: firstNonEmpty(source.URI, source.ID),
		}},
		Resources: []project.Resource{{
			Address:  "resource." + resourceName,
			Kind:     "resource",
			Type:     resourceName,
			Name:     resourceName,
			Provider: sourceKind,
			Attributes: map[string]any{
				"name": resourceName,
			},
			Operations: map[string]project.OperationRole{
				"create": {
					Purpose:            "create",
					SourceKind:         sourceKind,
					SourceID:           sourceID,
					SourcePath:         firstNonEmpty(source.URI, source.ID),
					OperationID:        sourceOperationID,
					CredentialBindings: append([]string(nil), operation.CredentialBindings...),
				},
			},
			IdentityAttributes: []project.IdentityAttribute{{
				Name:     "name",
				Path:     "name",
				Required: true,
			}},
			Schema: []project.SchemaPath{{
				Path:     "name",
				Type:     "string",
				Required: true,
				Identity: true,
			}},
			RequestBindings: []project.RequestBinding{{
				OperationRole: "create",
				OperationID:   sourceOperationID,
				Path:          "name",
				RequestPath:   "name",
				Required:      true,
				Identity:      true,
			}},
			RequiredOperations: []string{"create"},
			CredentialBindings: append([]string(nil), operation.CredentialBindings...),
		}},
	}
	return &uws1.Document{
		UWS: "1.4.0",
		Info: &uws1.Info{
			Title:   firstNonEmpty(s.ProjectName, "Ramen authored project"),
			Version: "0.1.0",
		},
		SourceDescriptions: []*uws1.SourceDescription{{
			Name: sourceID,
			URL:  firstNonEmpty(source.URI, source.ID),
			Type: uws1.SourceDescriptionType(sourceKind),
		}},
		Operations: []*uws1.Operation{{
			OperationID:       localOperationID,
			SourceDescription: sourceID,
			SourceOperationID: sourceOperationID,
			Description:       firstNonEmpty(operation.Summary, s.Goal),
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID:  "main",
			Type:        uws1.WorkflowTypeSequence,
			Description: s.Goal,
			Steps: []*uws1.Step{{
				StepID:       "create_" + resourceName,
				OperationRef: localOperationID,
			}},
		}},
		Extensions: map[string]any{project.ExtensionKey: profile},
	}
}

func reportForRun(run sharedicot.Result[state, artifact], err error, result Result) sharedreport.Result {
	status := sharedreport.StatusComplete
	if err != nil {
		status = sharedreport.StatusForError(err)
		if errors.Is(err, sharedicot.ErrNeedsInput) {
			status = sharedreport.StatusNeedsInput
		}
	}
	readiness := sharedreadiness.Evaluate(runtime{}.Readiness(run.Session, nil))
	report := sharedreport.Result{
		Status:    status,
		Summary:   strings.TrimSpace(run.Session.Goal),
		Readiness: &readiness,
		Metadata: map[string]string{
			"adapter": "ramen.authoring.spike",
		},
	}
	if len(readiness.Issues) == 0 {
		report.Readiness = nil
	}
	if result.ProjectPath != "" {
		report.Artifacts = []trust.ArtifactRecord{{Path: result.ProjectPath, Kind: "ramen.project", Required: true}}
	}
	if result.Validation != nil && !result.Validation.Valid {
		report.Status = sharedreport.StatusNeedsInput
	}
	return sharedreport.Normalize(report)
}

func firstSource(ctx promptcontext.Context) promptcontext.SourceDocument {
	ctx = promptcontext.Normalize(ctx)
	if len(ctx.Sources) == 0 {
		return promptcontext.SourceDocument{}
	}
	return ctx.Sources[0]
}

func firstOperation(ctx promptcontext.Context) promptcontext.OperationCandidate {
	ctx = promptcontext.Normalize(ctx)
	if len(ctx.Operations) == 0 {
		return promptcontext.OperationCandidate{}
	}
	return ctx.Operations[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func slug(value string) string {
	var b bytes.Buffer
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash && b.Len() > 0:
			b.WriteByte('_')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "resource"
	}
	return out
}
