package authoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenUdon/apitools"
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
	Report         sharedreport.Result
	ProjectPath    string
	ProjectHCLPath string
	Validation     *ramenvalidate.Result
	Graph          *graph.Document
	Plan           *plan.Result
}

type state struct {
	Goal           string
	ProjectName    string
	Context        promptcontext.Context
	Variables      []project.Variable
	Resources      []project.Resource
	Redaction      project.Redaction
	Draft          *uws1.Document
	ProjectPath    string
	ProjectHCLPath string
	Validation     *ramenvalidate.Result
	Graph          *graph.Document
	Plan           *plan.Result
}

type artifact struct {
	Path       string
	HCLPath    string
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
	result := Result{ProjectPath: run.Artifact.Path, ProjectHCLPath: run.Artifact.HCLPath, Validation: run.Artifact.Validation, Graph: run.Artifact.Graph, Plan: run.Artifact.Plan}
	if result.ProjectPath == "" {
		result.ProjectPath = run.Session.ProjectPath
	}
	if result.ProjectHCLPath == "" {
		result.ProjectHCLPath = run.Session.ProjectHCLPath
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

func (r runtime) Normalize(s *state) {
	s.Goal = strings.TrimSpace(s.Goal)
	s.ProjectName = strings.TrimSpace(s.ProjectName)
	if s.ProjectName == "" {
		s.ProjectName = slug(firstNonEmpty(s.Goal, "ramen project"))
	}
	s.Context = promptcontext.Normalize(s.Context)
	s.Variables = normalizeVariables(s.Variables)
	s.Resources = normalizeResources(s.Resources)
	s.Context = projectRelativePromptContext(r.outDir, s.Context)
	s.Resources = projectRelativeResources(r.outDir, s.Resources)
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
	if err := stageDraftAPISources(ctx, r.outDir, s.Draft); err != nil {
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
	hclPath := hclSibling(path)
	hclData, err := uwsconvert.MarshalHCL(s.Draft)
	if err != nil {
		return artifact{}, err
	}
	if len(hclData) == 0 || hclData[len(hclData)-1] != '\n' {
		hclData = append(hclData, '\n')
	}
	if err := lifecycle.AtomicWrite(hclPath, hclData, 0o644); err != nil {
		return artifact{}, err
	}
	out := artifact{Path: path, HCLPath: hclPath}
	s.ProjectPath = path
	s.ProjectHCLPath = hclPath
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
	operation := firstOperation(s.Context)
	source := sourceForOperation(s.Context, operation)
	sourceOperationID := firstNonEmpty(operation.OperationID, operation.ID)
	sourceID := firstNonEmpty(source.ID, operation.SourceID, "api")
	sourceKind := firstNonEmpty(source.Kind, "openapi")
	resourceName := slug(firstNonEmpty(s.ProjectName, s.Goal, "resource"))
	resources := append([]project.Resource(nil), s.Resources...)
	if len(resources) == 0 {
		resources = []project.Resource{defaultResource(s, source, operation, sourceID, sourceKind, sourceOperationID, resourceName)}
	}
	primaryRole := primaryOperationRole(resources)
	localOperationID := primaryRole + "_" + resourceName
	profile := project.Profile{
		Version:    project.Version,
		APISources: apiSourcesForContext(s.Context, source, operation, sourceID, sourceKind, resources),
		Variables:  variablesForResources(s.Variables, resources),
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
	roleName := operationRoleForVerb(operation.Verb)
	method := strings.ToUpper(strings.TrimSpace(operation.Verb))
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
	requestBindings := requestBindingsForSchemaRole(schema, sourceOperationID, roleName)
	resourceRedaction := redactionForSchema(schema)
	return project.Resource{
		Address:    "resource." + resourceName,
		Kind:       "resource",
		Type:       resourceName,
		Name:       resourceName,
		Provider:   sourceKind,
		Attributes: attributes,
		Operations: map[string]project.OperationRole{
			roleName: {
				Purpose:            roleName,
				Method:             method,
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
		RequiredOperations: []string{roleName},
		CredentialBindings: append([]string(nil), operation.CredentialBindings...),
		Redaction:          resourceRedaction,
	}
}

// APIOperationResource builds a resource whose action follows the selected API
// operation method instead of forcing the operation into Terraform-style create.
func APIOperationResource(ctx promptcontext.Context, goal, projectName string) project.Resource {
	ctx = promptcontext.Normalize(ctx)
	operation := firstOperation(ctx)
	source := sourceForOperation(ctx, operation)
	sourceOperationID := firstNonEmpty(operation.OperationID, operation.ID)
	sourceID := firstNonEmpty(source.ID, operation.SourceID, "api")
	sourceKind := firstNonEmpty(source.Kind, operation.Metadata["source_kind"], "openapi")
	sourcePath := firstNonEmpty(source.URI, operation.Metadata["source_path"], source.ID)
	resourceName := slug(firstNonEmpty(projectName, goal, operation.Name, operation.OperationID, "api_operation"))
	roleName := operationRoleForVerb(operation.Verb)
	method := strings.ToUpper(strings.TrimSpace(operation.Verb))
	parameters := operationParameters(operation)
	schema := schemaPathsForParameters(parameters)
	if strings.TrimSpace(operation.RequestSchemaID) != "" {
		schema = append(schema, schemaPathsForOperation(ctx, operation)...)
	}
	if len(schema) == 0 {
		schema = append(schema, project.SchemaPath{Path: "id", Type: "string", Computed: roleName == "read", Identity: true, ReadOnly: roleName == "read"})
	} else if !schemaHasIdentity(schema) {
		schema = append(schema, project.SchemaPath{Path: "id", Type: "string", Computed: roleName == "read", Identity: true, ReadOnly: roleName == "read"})
	}
	if roleName == "read" {
		for i := range schema {
			if schema[i].Required && !schema[i].Computed {
				continue
			}
			schema[i].ReadOnly = true
			if !schema[i].Identity {
				schema[i].Computed = true
			}
		}
	}
	identity := identityAttributesForSchema(schema)
	attributes := map[string]any{}
	for _, parameter := range parameters {
		if !parameter.Required {
			continue
		}
		attributes[parameter.Name] = parameterAttributeValueForGoal(parameter, sourcePath, goal, resourceName)
	}
	if len(attributes) == 0 {
		for _, attr := range identity {
			attributes[attr.Path] = resourceName
		}
	}
	requestBindings := requestBindingsForParametersRole(parameters, sourceOperationID, roleName)
	if roleName != "read" {
		requestBindings = append(requestBindings, requestBindingsForSchemaRole(schemaPathsExcludingParameters(schema, parameters), sourceOperationID, roleName)...)
	}
	return project.Resource{
		Address:    "resource." + resourceName,
		Kind:       "resource",
		Type:       resourceName,
		Name:       resourceName,
		Provider:   sourceKind,
		Attributes: attributes,
		Operations: map[string]project.OperationRole{
			roleName: {
				Purpose:            roleName,
				Method:             method,
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
		ResponseBindings:   responseBindingsForSchema(responseSchemaPaths(schema), sourceOperationID),
		RequiredOperations: []string{roleName},
		CredentialBindings: append([]string(nil), operation.CredentialBindings...),
		Redaction:          redactionForSchema(schema),
		Metadata: map[string]any{
			"goal":           strings.TrimSpace(goal),
			"operation_role": roleName,
			"api_method":     method,
		},
	}
}

// ReadOnlyResource builds a Ramen resource skeleton for read/list-style goals.
func ReadOnlyResource(ctx promptcontext.Context, goal, projectName string) project.Resource {
	return APIOperationResource(ctx, goal, projectName)
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

func primaryOperationRole(resources []project.Resource) string {
	if len(resources) == 0 {
		return "post"
	}
	resource := resources[0]
	if len(resource.RequiredOperations) > 0 {
		if role := strings.TrimSpace(resource.RequiredOperations[0]); role != "" {
			return role
		}
	}
	if len(resource.Operations) == 1 {
		for role := range resource.Operations {
			if role = strings.TrimSpace(role); role != "" {
				return role
			}
		}
	}
	if readOnlyResources(resources) {
		return "read"
	}
	if _, ok := resource.Operations["create"]; ok {
		return "create"
	}
	return "post"
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
		if result.ProjectHCLPath != "" {
			report.Artifacts = append(report.Artifacts, trust.ArtifactRecord{Path: result.ProjectHCLPath, Kind: "ramen.project.hcl", Required: false})
		}
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

func hclSibling(path string) string {
	switch {
	case strings.HasSuffix(path, ".yaml"):
		return strings.TrimSuffix(path, ".yaml") + ".hcl"
	case strings.HasSuffix(path, ".yml"):
		return strings.TrimSuffix(path, ".yml") + ".hcl"
	default:
		return path + ".hcl"
	}
}

func stageDraftAPISources(ctx context.Context, outDir string, doc *uws1.Document) error {
	if doc == nil {
		return nil
	}
	profile, err := project.ProfileFromDocument(doc)
	if err != nil {
		return err
	}
	replacements := map[string]string{}
	usedNames := map[string]bool{
		project.DefaultFile: true,
		"project.uws.hcl":   true,
	}
	for i := range profile.APISources {
		source := &profile.APISources[i]
		source.Path = strings.TrimSpace(source.Path)
		if source.Path == "" {
			continue
		}
		if !isRemoteAPISource(source.Path) {
			continue
		}
		if !isHTTPAPISource(source.Path) {
			return fmt.Errorf("stage API source %s:%s: unsupported remote API source scheme in %q; use http(s) or a local file path", source.Kind, source.ID, source.Path)
		}
		imported, err := newAPIToolsClient().Import(ctx, apitools.ImportOptions{
			URL:  source.Path,
			Dir:  outDir,
			Name: stagedAPISourceName(source.Kind, source.ID, source.Path, usedNames),
		})
		if err != nil {
			return fmt.Errorf("stage API source %s:%s: %w", source.Kind, source.ID, err)
		}
		source.Path = filepath.ToSlash(mustRel(outDir, imported.Path))
		replacements[sourceKey(source.Kind, source.ID)] = source.Path
	}
	if len(replacements) == 0 {
		return nil
	}
	for i := range profile.Resources {
		for purpose, role := range profile.Resources[i].Operations {
			if staged := replacements[sourceKey(role.SourceKind, role.SourceID)]; staged != "" {
				role.SourcePath = staged
				profile.Resources[i].Operations[purpose] = role
			}
		}
	}
	if doc.Extensions == nil {
		doc.Extensions = map[string]any{}
	}
	doc.Extensions[project.ExtensionKey] = profile
	for _, sourceDescription := range doc.SourceDescriptions {
		if sourceDescription == nil {
			continue
		}
		for _, source := range profile.APISources {
			if sourceDescription.Name == source.ID {
				sourceDescription.URL = source.Path
				break
			}
		}
	}
	return nil
}

func stagedAPISourceName(kind, id, sourcePath string, used map[string]bool) string {
	stem := slug(firstNonEmpty(id, kind, strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath)), "api-source"))
	ext := strings.ToLower(filepath.Ext(sourcePath))
	if ext == "" {
		ext = ".json"
	}
	name := stem + ext
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d%s", stem, i, ext)
	}
	used[name] = true
	return name
}

func mustRel(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

func sourceKey(kind, id string) string {
	return strings.TrimSpace(kind) + "\x00" + strings.TrimSpace(id)
}

func projectRelativePromptContext(outDir string, ctx promptcontext.Context) promptcontext.Context {
	for i := range ctx.Sources {
		ctx.Sources[i].URI = projectRelativeLocalPath(outDir, ctx.Sources[i].URI)
	}
	for i := range ctx.Operations {
		if ctx.Operations[i].Metadata == nil {
			continue
		}
		if path := ctx.Operations[i].Metadata["source_path"]; path != "" {
			ctx.Operations[i].Metadata["source_path"] = projectRelativeLocalPath(outDir, path)
		}
	}
	return ctx
}

func projectRelativeResources(outDir string, resources []project.Resource) []project.Resource {
	for i := range resources {
		for purpose, role := range resources[i].Operations {
			role.SourcePath = projectRelativeLocalPath(outDir, role.SourcePath)
			resources[i].Operations[purpose] = role
		}
	}
	return resources
}

func projectRelativeLocalPath(outDir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "://") || strings.HasPrefix(path, "urn:") {
		return path
	}
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(path)
	}
	absPath := path
	absOutDir := outDir
	if absOutDir == "" {
		absOutDir = "."
	}
	if !filepath.IsAbs(absOutDir) {
		wd, err := os.Getwd()
		if err != nil {
			return filepath.ToSlash(path)
		}
		absOutDir = filepath.Join(wd, filepath.FromSlash(absOutDir))
	}
	rel, err := filepath.Rel(absOutDir, absPath)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
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

func apiSourcesForContext(ctx promptcontext.Context, fallback promptcontext.SourceDocument, operation promptcontext.OperationCandidate, sourceID, sourceKind string, resources []project.Resource) []project.APISource {
	ctx = promptcontext.Normalize(ctx)
	var sources []project.APISource
	seen := map[string]bool{}
	addProjectSource := func(kind, id, path string) {
		id = strings.TrimSpace(id)
		kind = strings.TrimSpace(kind)
		path = strings.TrimSpace(path)
		if id == "" || kind == "" || path == "" {
			return
		}
		key := kind + "\x00" + id
		if seen[key] {
			return
		}
		seen[key] = true
		sources = append(sources, project.APISource{Kind: kind, ID: id, Path: path})
	}
	addPromptSource := func(source promptcontext.SourceDocument, fallbackID, fallbackKind string) {
		addProjectSource(firstNonEmpty(source.Kind, fallbackKind), firstNonEmpty(source.ID, fallbackID), firstNonEmpty(source.URI, source.ID))
	}
	if operation.SourceID != "" {
		for _, source := range ctx.Sources {
			if source.ID == operation.SourceID {
				addPromptSource(source, operation.SourceID, sourceKind)
			}
		}
	} else {
		for _, source := range ctx.Sources {
			addPromptSource(source, sourceID, sourceKind)
		}
	}
	for _, resource := range resources {
		for _, role := range resource.Operations {
			roleKind := strings.TrimSpace(role.SourceKind)
			roleID := strings.TrimSpace(role.SourceID)
			if roleID == "" {
				continue
			}
			matched := false
			for _, source := range ctx.Sources {
				if strings.TrimSpace(source.ID) != roleID {
					continue
				}
				if roleKind != "" && strings.TrimSpace(source.Kind) != "" && strings.TrimSpace(source.Kind) != roleKind {
					continue
				}
				addPromptSource(source, roleID, firstNonEmpty(roleKind, sourceKind))
				matched = true
			}
			if !matched && roleKind != "" && strings.TrimSpace(role.SourcePath) != "" {
				addProjectSource(roleKind, roleID, role.SourcePath)
			}
		}
	}
	if len(sources) == 0 {
		addPromptSource(fallback, firstNonEmpty(operation.SourceID, sourceID), sourceKind)
	}
	return sources
}

func sourceForOperation(ctx promptcontext.Context, operation promptcontext.OperationCandidate) promptcontext.SourceDocument {
	ctx = promptcontext.Normalize(ctx)
	if operation.SourceID != "" {
		for _, source := range ctx.Sources {
			if source.ID == operation.SourceID {
				return source
			}
		}
	}
	return firstSource(ctx)
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

func operationParameters(operation promptcontext.OperationCandidate) []operationParameterMetadata {
	raw := strings.TrimSpace(operation.Metadata["parameters"])
	if raw == "" {
		return nil
	}
	var parameters []operationParameterMetadata
	if err := json.Unmarshal([]byte(raw), &parameters); err != nil {
		return nil
	}
	out := parameters[:0]
	for _, parameter := range parameters {
		parameter.Name = strings.TrimSpace(parameter.Name)
		parameter.In = strings.ToLower(strings.TrimSpace(parameter.In))
		parameter.Type = firstNonEmpty(parameter.Type, "string")
		if parameter.Name == "" {
			continue
		}
		out = append(out, parameter)
	}
	return out
}

func schemaPathsForParameters(parameters []operationParameterMetadata) []project.SchemaPath {
	var out []project.SchemaPath
	for _, parameter := range parameters {
		if !parameter.Required {
			continue
		}
		out = append(out, project.SchemaPath{
			Path:     parameter.Name,
			Type:     firstNonEmpty(parameter.Type, "string"),
			Required: true,
		})
	}
	return out
}

func requestBindingsForParameters(parameters []operationParameterMetadata, operationID string) []project.RequestBinding {
	return requestBindingsForParametersRole(parameters, operationID, "read")
}

func requestBindingsForParametersRole(parameters []operationParameterMetadata, operationID, role string) []project.RequestBinding {
	var out []project.RequestBinding
	for _, parameter := range parameters {
		if !parameter.Required {
			continue
		}
		out = append(out, project.RequestBinding{
			OperationRole: role,
			OperationID:   operationID,
			Path:          parameter.Name,
			RequestPath:   parameter.Name,
			Location:      firstNonEmpty(parameter.In, "query"),
			Required:      true,
		})
	}
	return out
}

func schemaPathsExcludingParameters(schema []project.SchemaPath, parameters []operationParameterMetadata) []project.SchemaPath {
	parameterNames := map[string]bool{}
	for _, parameter := range parameters {
		parameterNames[parameter.Name] = true
	}
	var out []project.SchemaPath
	for _, path := range schema {
		if parameterNames[path.Path] {
			continue
		}
		out = append(out, path)
	}
	return out
}

func parameterAttributeValue(parameter operationParameterMetadata, sourcePath string) any {
	return parameterAttributeValueForGoal(parameter, sourcePath, "", "")
}

func parameterAttributeValueForGoal(parameter operationParameterMetadata, sourcePath, goal, resourceName string) any {
	switch strings.ToLower(parameter.Name) {
	case "subscriptionid", "subscription_id":
		return "${var.azure_subscription_id}"
	case "api-version", "apiversion", "api_version":
		if version := apiVersionFromSourcePath(sourcePath); version != "" {
			return version
		}
		return "${var.api_version}"
	case "resourcegroupname", "resource_group_name":
		if value := valueAfterPhrase(goal, "resource group"); value != "" {
			return value
		}
	case "servername", "server_name":
		if value := valueAfterPhrase(goal, "server"); value != "" {
			return value
		}
	case "databasename", "database_name":
		if value := valueAfterPhrase(goal, "database named"); value != "" {
			return value
		}
		if value := valueAfterPhrase(goal, "database"); value != "" {
			return value
		}
	default:
		if strings.ToLower(strings.TrimSpace(parameter.In)) == "path" {
			if value := valueAfterPhrase(goal, parameter.Name); value != "" {
				return value
			}
			if value := valueAfterPhrase(goal, "named"); value != "" {
				return value
			}
			if strings.TrimSpace(resourceName) != "" {
				return resourceName
			}
		}
		return "${var." + slug(parameter.Name) + "}"
	}
	if strings.TrimSpace(resourceName) != "" {
		return resourceName
	}
	return "${var." + slug(parameter.Name) + "}"
}

func valueAfterPhrase(text, phrase string) string {
	text = strings.TrimSpace(text)
	phrase = strings.TrimSpace(phrase)
	if text == "" || phrase == "" {
		return ""
	}
	lower := strings.ToLower(text)
	index := strings.Index(lower, strings.ToLower(phrase))
	if index < 0 {
		return ""
	}
	rest := strings.TrimSpace(text[index+len(phrase):])
	rest = strings.TrimLeft(rest, " :='\"")
	if rest == "" {
		return ""
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	value := strings.Trim(fields[0], " ,.;'\"")
	switch strings.ToLower(value) {
	case "", "named", "called", "on", "in", "after", "with":
		if len(fields) < 2 {
			return ""
		}
		value = strings.Trim(fields[1], " ,.;'\"")
	}
	return value
}

func apiVersionFromSourcePath(sourcePath string) string {
	parts := strings.FieldsFunc(filepath.ToSlash(sourcePath), func(r rune) bool {
		return r == '/'
	})
	for _, part := range parts {
		if looksLikeAPIVersion(part) {
			return part
		}
	}
	return ""
}

func looksLikeAPIVersion(value string) bool {
	if len(value) != len("2006-01-02") {
		return false
	}
	for i, r := range value {
		switch i {
		case 4, 7:
			if r != '-' {
				return false
			}
		default:
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func schemaHasIdentity(schema []project.SchemaPath) bool {
	for _, path := range schema {
		if path.Identity {
			return true
		}
	}
	return false
}

func responseSchemaPaths(schema []project.SchemaPath) []project.SchemaPath {
	var out []project.SchemaPath
	for _, path := range schema {
		if path.Computed || path.ReadOnly || path.ResponseDerivedIdentity {
			out = append(out, path)
		}
	}
	return out
}

func variablesForResources(existing []project.Variable, resources []project.Resource) []project.Variable {
	out := append([]project.Variable(nil), existing...)
	seen := map[string]bool{}
	for _, variable := range out {
		seen[strings.TrimSpace(variable.Name)] = true
	}
	for _, resource := range resources {
		for _, value := range resource.Attributes {
			for _, name := range variableRefs(value) {
				if seen[name] {
					continue
				}
				seen[name] = true
				out = append(out, project.Variable{Name: name, Type: "string"})
			}
		}
	}
	return out
}

func variableRefs(value any) []string {
	text, ok := value.(string)
	if !ok {
		return nil
	}
	var out []string
	for {
		start := strings.Index(text, "${var.")
		if start < 0 {
			return out
		}
		rest := text[start+len("${var."):]
		end := strings.Index(rest, "}")
		if end < 0 {
			return out
		}
		name := strings.TrimSpace(rest[:end])
		if name != "" {
			out = append(out, name)
		}
		text = rest[end+1:]
	}
}

func requestBindingsForSchema(schema []project.SchemaPath, operationID string) []project.RequestBinding {
	return requestBindingsForSchemaRole(schema, operationID, "create")
}

func requestBindingsForSchemaRole(schema []project.SchemaPath, operationID, role string) []project.RequestBinding {
	var out []project.RequestBinding
	for _, path := range schema {
		if !path.Required && !path.Identity {
			continue
		}
		out = append(out, project.RequestBinding{
			OperationRole: role,
			OperationID:   operationID,
			Path:          path.Path,
			RequestPath:   path.Path,
			Required:      path.Required,
			Identity:      path.Identity,
		})
	}
	return out
}

func operationRoleForVerb(verb string) string {
	switch strings.ToUpper(strings.TrimSpace(verb)) {
	case "GET", "HEAD":
		return "read"
	case "DELETE":
		return "delete"
	case "POST":
		return "post"
	case "PUT":
		return "put"
	case "PATCH":
		return "patch"
	default:
		return "post"
	}
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
