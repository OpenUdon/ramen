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
	"github.com/OpenUdon/ramen/graph"
	"github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

const (
	missingGoalCode    = "ramen.authoring.missing_goal"
	missingMappingCode = "ramen.authoring.missing_mapping_metadata"
)

// Options configures the Ramen Authoring adapter.
type Options struct {
	Goal        string
	ProjectName string
	OutDir      string
	Context     promptcontext.Context
	Variables   []project.Variable
	Resources   []project.Resource
	Redaction   project.Redaction
	Validate    bool
	Graph       bool
	Plan        bool
	StatePath   string
}

// Result is the adapter outcome. Report is the product-neutral Authoring
// result; Ramen-specific artifacts stay in ProjectPath, Validation, Graph, and
// Plan.
type Result struct {
	Report      sharedreport.Result
	ProjectPath string
	Validation  *ramenvalidate.Result
	Graph       *graph.Document
	Plan        *plan.Result
}

type state struct {
	Goal        string
	ProjectName string
	Context     promptcontext.Context
	Variables   []project.Variable
	Resources   []project.Resource
	Redaction   project.Redaction
	Draft       *uws1.Document
	ProjectPath string
	Validation  *ramenvalidate.Result
	Graph       *graph.Document
	Plan        *plan.Result
}

type artifact struct {
	Path       string
	Validation *ramenvalidate.Result
	Graph      *graph.Document
	Plan       *plan.Result
}

// DraftProject drafts a native Ramen/UWS project from an operator goal,
// prompt-safe operation context, and optional Ramen-owned desired-state
// records.
func DraftProject(ctx context.Context, opts Options) (Result, error) {
	runtime := runtime{
		outDir:    firstNonEmpty(opts.OutDir, "."),
		validate:  opts.Validate,
		graph:     opts.Graph,
		plan:      opts.Plan,
		statePath: strings.TrimSpace(opts.StatePath),
	}
	initial := state{
		Goal:        strings.TrimSpace(opts.Goal),
		ProjectName: strings.TrimSpace(opts.ProjectName),
		Context:     promptcontext.Normalize(opts.Context),
		Variables:   append([]project.Variable(nil), opts.Variables...),
		Resources:   append([]project.Resource(nil), opts.Resources...),
		Redaction:   opts.Redaction,
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
	result := Result{ProjectPath: run.Artifact.Path, Validation: run.Artifact.Validation, Graph: run.Artifact.Graph, Plan: run.Artifact.Plan}
	if result.ProjectPath == "" {
		result.ProjectPath = run.Session.ProjectPath
	}
	if result.Validation == nil {
		result.Validation = run.Session.Validation
	}
	if result.Graph == nil {
		result.Graph = run.Session.Graph
	}
	if result.Plan == nil {
		result.Plan = run.Session.Plan
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
	outDir    string
	validate  bool
	graph     bool
	plan      bool
	statePath string
}

func (runtime) Normalize(s *state) {
	s.Goal = strings.TrimSpace(s.Goal)
	s.ProjectName = strings.TrimSpace(s.ProjectName)
	if s.ProjectName == "" {
		s.ProjectName = slug(firstNonEmpty(s.Goal, "ramen project"))
	}
	s.Context = promptcontext.Normalize(s.Context)
	s.Variables = normalizeVariables(s.Variables)
	s.Resources = normalizeResources(s.Resources)
	s.Redaction = normalizeRedaction(s.Redaction)
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
	if r.graph || r.plan {
		doc, err := project.Load(path)
		if err != nil {
			return artifact{}, err
		}
		if r.graph {
			graphDoc := graph.BuildProject(doc)
			out.Graph = &graphDoc
			s.Graph = &graphDoc
		}
		if r.plan {
			planResult, err := plan.Build(ctx, plan.Options{ProjectPath: path, StatePath: r.statePath})
			if err != nil {
				return artifact{}, err
			}
			out.Plan = planResult
			s.Plan = planResult
		}
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
	resources := append([]project.Resource(nil), s.Resources...)
	if len(resources) == 0 {
		resources = []project.Resource{defaultResource(s, source, operation, sourceID, sourceKind, sourceOperationID, resourceName)}
	}
	primaryRole := "create"
	if readOnlyResources(resources) {
		primaryRole = "read"
	}
	localOperationID := primaryRole + "_" + resourceName
	profile := project.Profile{
		Version:    project.Version,
		APISources: apiSourcesForContext(s.Context, source, operation, sourceID, sourceKind),
		Variables:  append([]project.Variable(nil), s.Variables...),
		Resources:  resources,
		Redaction:  s.Redaction,
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
				StepID:       localOperationID,
				OperationRef: localOperationID,
			}},
		}},
		Extensions: map[string]any{project.ExtensionKey: profile},
	}
}

func defaultResource(s state, source promptcontext.SourceDocument, operation promptcontext.OperationCandidate, sourceID, sourceKind, sourceOperationID, resourceName string) project.Resource {
	sourcePath := firstNonEmpty(source.URI, source.ID)
	schema := schemaPathsForOperation(s.Context, operation)
	if len(schema) == 0 {
		schema = []project.SchemaPath{{
			Path:     "name",
			Type:     "string",
			Required: true,
			Identity: true,
		}}
	}
	identity := identityAttributesForSchema(schema)
	attributes := attributesForSchema(schema, resourceName)
	requestBindings := requestBindingsForSchema(schema, sourceOperationID)
	resourceRedaction := redactionForSchema(schema)
	return project.Resource{
		Address:    "resource." + resourceName,
		Kind:       "resource",
		Type:       resourceName,
		Name:       resourceName,
		Provider:   sourceKind,
		Attributes: attributes,
		Operations: map[string]project.OperationRole{
			"create": {
				Purpose:            "create",
				SourceKind:         sourceKind,
				SourceID:           sourceID,
				SourcePath:         sourcePath,
				OperationID:        sourceOperationID,
				CredentialBindings: append([]string(nil), operation.CredentialBindings...),
			},
		},
		IdentityAttributes: identity,
		Schema:             schema,
		RequestBindings:    requestBindings,
		RequiredOperations: []string{"create"},
		CredentialBindings: append([]string(nil), operation.CredentialBindings...),
		Redaction:          resourceRedaction,
	}
}

// ReadOnlyResource builds a Ramen resource skeleton for read/list-style goals.
func ReadOnlyResource(ctx promptcontext.Context, goal, projectName string) project.Resource {
	ctx = promptcontext.Normalize(ctx)
	source := firstSource(ctx)
	operation := firstOperation(ctx)
	sourceOperationID := firstNonEmpty(operation.OperationID, operation.ID)
	sourceID := firstNonEmpty(source.ID, operation.SourceID, "api")
	sourceKind := firstNonEmpty(source.Kind, operation.Metadata["source_kind"], "openapi")
	sourcePath := firstNonEmpty(source.URI, operation.Metadata["source_path"], source.ID)
	resourceName := slug(firstNonEmpty(projectName, goal, operation.Name, operation.OperationID, "read_resource"))
	var schema []project.SchemaPath
	if strings.TrimSpace(operation.RequestSchemaID) != "" {
		schema = schemaPathsForOperation(ctx, operation)
	}
	if len(schema) == 0 {
		schema = []project.SchemaPath{{
			Path:     "id",
			Type:     "string",
			Computed: true,
			Identity: true,
			ReadOnly: true,
		}}
	}
	for i := range schema {
		schema[i].ReadOnly = true
		if !schema[i].Identity {
			schema[i].Computed = true
		}
	}
	identity := identityAttributesForSchema(schema)
	attributes := map[string]any{}
	for _, attr := range identity {
		attributes[attr.Path] = resourceName
	}
	return project.Resource{
		Address:    "resource." + resourceName,
		Kind:       "resource",
		Type:       resourceName,
		Name:       resourceName,
		Provider:   sourceKind,
		Attributes: attributes,
		Operations: map[string]project.OperationRole{
			"read": {
				Purpose:            "read",
				SourceKind:         sourceKind,
				SourceID:           sourceID,
				SourcePath:         sourcePath,
				OperationID:        sourceOperationID,
				CredentialBindings: append([]string(nil), operation.CredentialBindings...),
			},
		},
		IdentityAttributes: identity,
		Schema:             schema,
		ResponseBindings:   responseBindingsForSchema(schema, sourceOperationID),
		RequiredOperations: []string{"read"},
		CredentialBindings: append([]string(nil), operation.CredentialBindings...),
		Redaction:          redactionForSchema(schema),
		Metadata: map[string]any{
			"goal":           strings.TrimSpace(goal),
			"operation_role": "read",
		},
	}
}

func readOnlyResources(resources []project.Resource) bool {
	if len(resources) == 0 {
		return false
	}
	for _, resource := range resources {
		if !contains(resource.RequiredOperations, "read") {
			return false
		}
		for purpose := range resource.Operations {
			if purpose != "read" {
				return false
			}
		}
	}
	return true
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
	if result.Graph != nil && graphHasErrors(*result.Graph) {
		report.Status = sharedreport.StatusNeedsInput
	}
	if result.Plan != nil && result.Plan.Plan.Errored {
		report.Status = sharedreport.StatusNeedsInput
	}
	report.Diagnostics = diagnosticsForResult(result)
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

func apiSourcesForContext(ctx promptcontext.Context, fallback promptcontext.SourceDocument, operation promptcontext.OperationCandidate, sourceID, sourceKind string) []project.APISource {
	ctx = promptcontext.Normalize(ctx)
	var sources []project.APISource
	seen := map[string]bool{}
	add := func(source promptcontext.SourceDocument) {
		id := firstNonEmpty(source.ID, operation.SourceID, sourceID)
		kind := firstNonEmpty(source.Kind, sourceKind)
		path := firstNonEmpty(source.URI, source.ID)
		if id == "" || kind == "" {
			return
		}
		key := kind + "\x00" + id
		if seen[key] {
			return
		}
		seen[key] = true
		sources = append(sources, project.APISource{Kind: kind, ID: id, Path: path})
	}
	for _, source := range ctx.Sources {
		add(source)
	}
	if len(sources) == 0 {
		add(fallback)
	}
	return sources
}

func schemaPathsForOperation(ctx promptcontext.Context, operation promptcontext.OperationCandidate) []project.SchemaPath {
	schema := schemaForID(ctx, operation.RequestSchemaID)
	if schema.ID == "" {
		schema = firstSchema(ctx)
	}
	if schema.ID == "" {
		return nil
	}
	identity := identityField(schema.Fields)
	var out []project.SchemaPath
	for _, field := range schema.Fields {
		path := strings.TrimSpace(field.Name)
		if path == "" {
			continue
		}
		out = append(out, project.SchemaPath{
			Path:      path,
			Type:      firstNonEmpty(field.Type, "string"),
			Required:  field.Required || contains(schema.Required, path),
			Sensitive: field.Sensitive,
			Identity:  path == identity,
		})
	}
	return out
}

func schemaForID(ctx promptcontext.Context, id string) promptcontext.SchemaHint {
	id = strings.TrimSpace(id)
	if id == "" {
		return promptcontext.SchemaHint{}
	}
	ctx = promptcontext.Normalize(ctx)
	for _, schema := range ctx.Schemas {
		if schema.ID == id {
			return schema
		}
	}
	return promptcontext.SchemaHint{}
}

func firstSchema(ctx promptcontext.Context) promptcontext.SchemaHint {
	ctx = promptcontext.Normalize(ctx)
	if len(ctx.Schemas) == 0 {
		return promptcontext.SchemaHint{}
	}
	return ctx.Schemas[0]
}

func identityField(fields []promptcontext.FieldHint) string {
	for _, name := range []string{"name", "id"} {
		for _, field := range fields {
			if strings.EqualFold(strings.TrimSpace(field.Name), name) {
				return strings.TrimSpace(field.Name)
			}
		}
	}
	for _, field := range fields {
		if field.Required {
			return strings.TrimSpace(field.Name)
		}
	}
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimSpace(fields[0].Name)
}

func identityAttributesForSchema(schema []project.SchemaPath) []project.IdentityAttribute {
	var out []project.IdentityAttribute
	for _, path := range schema {
		if !path.Identity {
			continue
		}
		out = append(out, project.IdentityAttribute{
			Name:     firstNonEmpty(path.Path, "identity"),
			Path:     path.Path,
			Required: path.Required,
		})
	}
	if len(out) == 0 && len(schema) > 0 {
		out = append(out, project.IdentityAttribute{Name: schema[0].Path, Path: schema[0].Path, Required: schema[0].Required})
	}
	return out
}

func attributesForSchema(schema []project.SchemaPath, resourceName string) map[string]any {
	attributes := map[string]any{}
	for _, path := range schema {
		if !path.Required && !path.Identity {
			continue
		}
		value := resourceName
		if path.Sensitive {
			value = "${var." + slug(path.Path) + "}"
		}
		attributes[path.Path] = value
	}
	if len(attributes) == 0 {
		attributes["name"] = resourceName
	}
	return attributes
}

func requestBindingsForSchema(schema []project.SchemaPath, operationID string) []project.RequestBinding {
	var out []project.RequestBinding
	for _, path := range schema {
		if !path.Required && !path.Identity {
			continue
		}
		out = append(out, project.RequestBinding{
			OperationRole: "create",
			OperationID:   operationID,
			Path:          path.Path,
			RequestPath:   path.Path,
			Required:      path.Required,
			Identity:      path.Identity,
		})
	}
	return out
}

func responseBindingsForSchema(schema []project.SchemaPath, operationID string) []project.ResponseBinding {
	var out []project.ResponseBinding
	for _, path := range schema {
		out = append(out, project.ResponseBinding{
			OperationRole:           "read",
			OperationID:             operationID,
			ResponsePath:            path.Path,
			StatePath:               path.Path,
			Identity:                path.Identity,
			ResponseDerivedIdentity: path.ResponseDerivedIdentity,
			Computed:                path.Computed,
			Observed:                true,
			Sensitive:               path.Sensitive,
		})
	}
	return out
}

func redactionForSchema(schema []project.SchemaPath) project.Redaction {
	var paths []string
	for _, path := range schema {
		if path.Sensitive {
			paths = append(paths, path.Path)
		}
	}
	return project.Redaction{Paths: paths}
}

func normalizeVariables(variables []project.Variable) []project.Variable {
	out := make([]project.Variable, 0, len(variables))
	seen := map[string]bool{}
	for _, variable := range variables {
		variable.Name = strings.TrimSpace(variable.Name)
		variable.Type = strings.TrimSpace(variable.Type)
		variable.Description = strings.TrimSpace(variable.Description)
		if variable.Name == "" || seen[variable.Name] {
			continue
		}
		seen[variable.Name] = true
		out = append(out, variable)
	}
	return out
}

func normalizeResources(resources []project.Resource) []project.Resource {
	out := make([]project.Resource, 0, len(resources))
	for _, resource := range resources {
		resource.Address = strings.TrimSpace(resource.Address)
		resource.Kind = firstNonEmpty(resource.Kind, "resource")
		resource.Type = strings.TrimSpace(resource.Type)
		resource.Name = strings.TrimSpace(resource.Name)
		resource.Provider = strings.TrimSpace(resource.Provider)
		resource.Dependencies = uniqueStrings(resource.Dependencies)
		resource.RequiredOperations = uniqueStrings(resource.RequiredOperations)
		resource.CredentialBindings = uniqueStrings(resource.CredentialBindings)
		resource.Redaction = normalizeRedaction(resource.Redaction)
		if resource.Address == "" || resource.Type == "" {
			continue
		}
		out = append(out, resource)
	}
	return out
}

func normalizeRedaction(redaction project.Redaction) project.Redaction {
	return project.Redaction{Paths: uniqueStrings(redaction.Paths)}
}

func graphHasErrors(doc graph.Document) bool {
	for _, diagnostic := range doc.Diagnostics {
		if strings.EqualFold(diagnostic.Severity, "error") || strings.EqualFold(diagnostic.Severity, "blocking") {
			return true
		}
	}
	return false
}

func diagnosticsForResult(result Result) []trust.DiagnosticRecord {
	var records []trust.DiagnosticRecord
	if result.Validation != nil {
		for _, diagnostic := range result.Validation.Diagnostics {
			records = append(records, trust.DiagnosticRecord{
				Code:     diagnostic.Code,
				Severity: diagnostic.Severity,
				Message:  diagnostic.Message,
				Location: trust.DiagnosticLocation{
					Address:       diagnostic.Address,
					APISourceKind: diagnostic.APISourceKind,
					APISourceID:   diagnostic.APISourceID,
					OperationID:   diagnostic.OperationID,
				},
			})
		}
	}
	if result.Graph != nil {
		for _, diagnostic := range result.Graph.Diagnostics {
			records = append(records, trust.DiagnosticRecord{
				Code:     diagnostic.Code,
				Severity: diagnostic.Severity,
				Message:  diagnostic.Message,
				Location: trust.DiagnosticLocation{
					Address:       diagnostic.Address,
					APISourceKind: diagnostic.APISourceKind,
					APISourceID:   diagnostic.APISourceID,
					OperationID:   diagnostic.OperationID,
				},
			})
		}
	}
	if result.Plan != nil {
		for _, diagnostic := range result.Plan.Diagnostics {
			records = append(records, trust.DiagnosticRecord{
				Code:     diagnostic.Code,
				Severity: diagnostic.Severity,
				Message:  diagnostic.Message,
				Location: trust.DiagnosticLocation{
					Address:       diagnostic.Address,
					ModuleAddress: diagnostic.ModuleAddress,
					APISourceKind: diagnostic.APISourceKind,
					APISourceID:   diagnostic.APISourceID,
				},
			})
		}
	}
	return trust.NormalizeDiagnostics(records)
}

func contains(values []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
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
