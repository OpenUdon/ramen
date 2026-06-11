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
	"github.com/OpenUdon/authoring/operationlifecycle"
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
		SourceDescriptions: sourceDescriptionsForAPISources(profile.APISources, sourceID, sourceKind, firstNonEmpty(source.URI, source.ID)),
		Operations:         uwsOperationsForResources(resources, localOperationID, sourceID, sourceOperationID, operation, s.Goal),
		Workflows: []*uws1.Workflow{{
			WorkflowID:  "main",
			Type:        uws1.WorkflowTypeSequence,
			Description: s.Goal,
			Steps:       uwsStepsForResources(resources, localOperationID),
		}},
		Extensions: map[string]any{project.ExtensionKey: profile},
	}
}

func sourceDescriptionsForAPISources(sources []project.APISource, fallbackID, fallbackKind, fallbackURL string) []*uws1.SourceDescription {
	var out []*uws1.SourceDescription
	for _, source := range sources {
		if strings.TrimSpace(source.ID) == "" {
			continue
		}
		out = append(out, &uws1.SourceDescription{
			Name: source.ID,
			URL:  firstNonEmpty(source.Path, fallbackURL, source.ID),
			Type: uws1.SourceDescriptionType(normalizeProjectSourceKind(firstNonEmpty(source.Kind, fallbackKind, "openapi"))),
		})
	}
	if len(out) == 0 {
		out = append(out, &uws1.SourceDescription{
			Name: firstNonEmpty(fallbackID, "api"),
			URL:  firstNonEmpty(fallbackURL, fallbackID, "api"),
			Type: uws1.SourceDescriptionType(normalizeProjectSourceKind(firstNonEmpty(fallbackKind, "openapi"))),
		})
	}
	return out
}

func uwsOperationsForResources(resources []project.Resource, fallbackOperationID, fallbackSourceID, fallbackSourceOperationID string, fallback promptcontext.OperationCandidate, goal string) []*uws1.Operation {
	var out []*uws1.Operation
	seen := map[string]bool{}
	for _, resource := range resources {
		for _, role := range orderedResourceRoles(resource) {
			opRole := resource.Operations[role]
			localID := localUWSOperationID(role, resource)
			if seen[localID] {
				continue
			}
			seen[localID] = true
			out = append(out, &uws1.Operation{
				OperationID:       localID,
				SourceDescription: firstNonEmpty(opRole.SourceID, fallbackSourceID),
				SourceOperationID: firstNonEmpty(opRole.OperationID, fallbackSourceOperationID),
				Description:       firstNonEmpty(fallback.Summary, goal),
			})
		}
	}
	if len(out) == 0 {
		return []*uws1.Operation{{
			OperationID:       fallbackOperationID,
			SourceDescription: fallbackSourceID,
			SourceOperationID: fallbackSourceOperationID,
			Description:       firstNonEmpty(fallback.Summary, goal),
		}}
	}
	return out
}

func uwsStepsForResources(resources []project.Resource, fallbackOperationID string) []*uws1.Step {
	var out []*uws1.Step
	seen := map[string]bool{}
	for _, resource := range resources {
		for _, role := range orderedResourceRoles(resource) {
			localID := localUWSOperationID(role, resource)
			if seen[localID] {
				continue
			}
			seen[localID] = true
			out = append(out, &uws1.Step{StepID: localID, OperationRef: localID})
		}
	}
	if len(out) == 0 {
		return []*uws1.Step{{StepID: fallbackOperationID, OperationRef: fallbackOperationID}}
	}
	return out
}

func orderedResourceRoles(resource project.Resource) []string {
	var roles []string
	for _, role := range resource.RequiredOperations {
		if _, ok := resource.Operations[role]; ok {
			roles = append(roles, role)
		}
	}
	if len(roles) == len(resource.Operations) {
		return roles
	}
	for role := range resource.Operations {
		if !contains(roles, role) {
			roles = append(roles, role)
		}
	}
	return roles
}

func localUWSOperationID(role string, resource project.Resource) string {
	return slug(role) + "_" + slug(firstNonEmpty(resource.Type, resource.Name, "resource"))
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
	sourceKind := normalizeProjectSourceKind(firstNonEmpty(source.Kind, operation.Metadata["source_kind"], "openapi"))
	sourcePath := firstNonEmpty(source.URI, operation.Metadata["source_path"], source.ID)
	resourceName := slug(firstNonEmpty(projectName, goal, operation.Name, operation.OperationID, "api_operation"))
	roleName := operationRoleForVerb(operation.Verb)
	method := strings.ToUpper(strings.TrimSpace(operation.Verb))
	parameters := operationParameters(operation)
	var schema []project.SchemaPath
	if strings.TrimSpace(operation.RequestSchemaID) != "" {
		schema = append(schema, schemaPathsForOperation(ctx, operation)...)
	}
	responseSchema := schemaPathsForResponse(ctx, operation)
	if roleName == "read" {
		schema = mergeSchemaPaths(schema, responseSchema)
	}
	schema = mergeSchemaPaths(schema, schemaPathsForLifecycleParameters(parameters, schema))
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
	requestBindings := requestBindingsForParametersRoleSchema(parameters, sourceOperationID, roleName, schema)
	if roleName != "read" {
		requestBindings = append(requestBindings, requestBindingsForSchemaRole(schemaPathsExcludingParameters(schema, parameters), sourceOperationID, roleName)...)
	}
	responseBindings := []project.ResponseBinding(nil)
	if roleName == "read" {
		responseBindings = responseBindingsForSchema(responseSchema, sourceOperationID)
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
		ResponseBindings:   responseBindings,
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

// APILifecycleResource builds a desired-state resource by conservatively
// expanding a selected API operation into same-source lifecycle roles.
func APILifecycleResource(ctx promptcontext.Context, seed promptcontext.OperationCandidate, goal, projectName string) project.Resource {
	ctx = promptcontext.Normalize(ctx)
	expansion := operationlifecycle.Expand(ctx, seed, operationlifecycle.Options{Goal: goal, DesiredState: true})
	expansion = preferKubernetesRBACReplaceUpdate(ctx, expansion)
	if len(expansion.Roles) == 0 {
		return APIOperationResource(promptcontext.Context{Sources: ctx.Sources, Operations: []promptcontext.OperationCandidate{seed}, Schemas: ctx.Schemas, Credentials: ctx.Credentials, Metadata: ctx.Metadata}, goal, projectName)
	}
	primary := expansion.Roles[0]
	operation := primary.Operation
	source := sourceForOperation(ctx, operation)
	sourceID := firstNonEmpty(source.ID, operation.SourceID, "api")
	sourceKind := normalizeProjectSourceKind(firstNonEmpty(source.Kind, operation.Metadata["source_kind"], "openapi"))
	sourcePath := firstNonEmpty(source.URI, operation.Metadata["source_path"], source.ID)
	resourceName := slug(firstNonEmpty(projectName, goal, operation.Name, operation.OperationID, "api_operation"))
	operations := map[string]project.OperationRole{}
	var required []string
	var schema []project.SchemaPath
	var requestBindings []project.RequestBinding
	var responseBindings []project.ResponseBinding
	var credentials []string
	var readResponseSchema []project.SchemaPath
	readOperationID := ""
	hasSourceLRO := false
	for _, candidate := range expansion.Roles {
		role := candidate.Role
		op := candidate.Operation
		opSource := sourceForOperation(ctx, op)
		opSourceID := firstNonEmpty(opSource.ID, op.SourceID, sourceID)
		opSourceKind := normalizeProjectSourceKind(firstNonEmpty(opSource.Kind, op.Metadata["source_kind"], sourceKind))
		opSourcePath := firstNonEmpty(opSource.URI, op.Metadata["source_path"], sourcePath)
		opID := firstNonEmpty(op.OperationID, op.ID)
		operations[role] = project.OperationRole{
			Purpose:            role,
			Method:             strings.ToUpper(strings.TrimSpace(op.Verb)),
			SourceKind:         opSourceKind,
			SourceID:           opSourceID,
			SourcePath:         opSourcePath,
			OperationID:        opID,
			CredentialBindings: append([]string(nil), op.CredentialBindings...),
		}
		required = append(required, role)
		credentials = append(credentials, op.CredentialBindings...)
		if sourceSupportedLongRunningOperation(op) {
			hasSourceLRO = true
		}
		parameters := operationParameters(op)
		if role != "read" && role != "delete" {
			bodySchema := schemaPathsForOperation(ctx, op)
			schema = mergeSchemaPaths(schema, bodySchema)
			requestBindings = append(requestBindings, requestBindingsForSchemaRole(schemaPathsExcludingParameters(bodySchema, parameters), opID, role)...)
		}
		if role == "read" {
			readResponseSchema = schemaPathsForResponse(ctx, op)
			schema = mergeSchemaPaths(schema, readResponseSchema)
		}
		parameterSchema := schemaPathsForLifecycleParameters(parameters, schema)
		schema = mergeSchemaPaths(schema, parameterSchema)
		requestBindings = append(requestBindings, requestBindingsForParametersRoleSchema(parameters, opID, role, schema)...)
		if role == "read" {
			readOperationID = opID
		}
	}
	if len(schema) == 0 {
		schema = []project.SchemaPath{{Path: "id", Type: "string", Required: true, Identity: true}}
	} else if !schemaHasIdentity(schema) {
		schema[0].Identity = true
	}
	identity := identityAttributesForSchema(schema)
	attributes := map[string]any{}
	for _, candidate := range expansion.Roles {
		for _, parameter := range operationParameters(candidate.Operation) {
			if !parameter.Required {
				continue
			}
			path := parameterBindingPath(parameter, schema)
			setAttributePath(attributes, path, parameterAttributeValueForGoal(parameter, sourcePath, goal, resourceName))
		}
	}
	for _, path := range schema {
		if !path.Required && !path.Identity {
			continue
		}
		if _, ok := attributes[path.Path]; ok {
			continue
		}
		value := schemaAttributeValueForGoal(path, goal, resourceName)
		if path.Sensitive {
			value = "${var." + slug(path.Path) + "}"
		}
		setAttributePath(attributes, path.Path, value)
	}
	if readOperationID != "" {
		responseBindings = responseBindingsForSchema(readResponseSchema, readOperationID)
	}
	runtimeHints := runtimeHintsForSourceMetadata(hasSourceLRO, readOperationID != "")
	return project.Resource{
		Address:            "resource." + resourceName,
		Kind:               "resource",
		Type:               resourceName,
		Name:               resourceName,
		Provider:           sourceKind,
		Attributes:         attributes,
		Operations:         operations,
		IdentityAttributes: identity,
		Schema:             mergeSchemaPaths(nil, schema),
		RequestBindings:    dedupeRequestBindings(requestBindings),
		ResponseBindings:   responseBindings,
		RequiredOperations: required,
		CredentialBindings: uniqueStrings(credentials),
		Redaction:          redactionForSchema(schema),
		RuntimeHints:       runtimeHints,
		Metadata: map[string]any{
			"goal":                strings.TrimSpace(goal),
			"operation_role":      primary.Role,
			"api_method":          strings.ToUpper(strings.TrimSpace(operation.Verb)),
			"lifecycle_family":    expansion.FamilyKey,
			"lifecycle_expansion": "operationlifecycle",
		},
	}
}

func preferKubernetesRBACReplaceUpdate(ctx promptcontext.Context, expansion operationlifecycle.Expansion) operationlifecycle.Expansion {
	for i, role := range expansion.Roles {
		if role.Role != "update" {
			continue
		}
		opID := firstNonEmpty(role.Operation.OperationID, role.Operation.ID)
		if !strings.HasPrefix(opID, "patchRbacAuthorizationV1") {
			continue
		}
		replaceID := "replace" + strings.TrimPrefix(opID, "patch")
		for _, op := range ctx.Operations {
			candidateID := firstNonEmpty(op.OperationID, op.ID)
			if candidateID != replaceID || op.SourceID != role.Operation.SourceID || op.Path != role.Operation.Path {
				continue
			}
			expansion.Roles[i].Operation = op
			expansion.Roles[i].Confidence = "high"
			expansion.Roles[i].Reason = "Ramen Kubernetes RBAC parity evidence prefers replace update"
			break
		}
	}
	return expansion
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
		kind = normalizeProjectSourceKind(kind)
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

func schemaPathsForResponse(ctx promptcontext.Context, operation promptcontext.OperationCandidate) []project.SchemaPath {
	schema := schemaForID(ctx, operation.ResponseSchemaID)
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
		isIdentity := path == identity
		out = append(out, project.SchemaPath{
			Path:                    path,
			Type:                    firstNonEmpty(field.Type, "string"),
			Required:                field.Required || contains(schema.Required, path),
			Sensitive:               field.Sensitive,
			Identity:                isIdentity,
			Computed:                !isIdentity,
			ReadOnly:                true,
			ResponseDerivedIdentity: isIdentity,
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
	for _, suffix := range []string{".name", ".id"} {
		for _, field := range fields {
			name := strings.TrimSpace(field.Name)
			if strings.HasSuffix(strings.ToLower(name), suffix) {
				return name
			}
		}
	}
	for _, field := range fields {
		if field.Required {
			name := strings.TrimSpace(field.Name)
			if !scopeLikeSchemaField(name) {
				return name
			}
		}
	}
	for _, field := range fields {
		name := strings.TrimSpace(field.Name)
		if !scopeLikeSchemaField(name) {
			return name
		}
	}
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimSpace(fields[0].Name)
}

func scopeLikeSchemaField(name string) bool {
	switch normalizedParameterName(name) {
	case "subscriptionid", "apiversion", "kind", "location":
		return true
	default:
		return false
	}
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

func schemaPathsForLifecycleParameters(parameters []operationParameterMetadata, schema []project.SchemaPath) []project.SchemaPath {
	var out []project.SchemaPath
	hasIdentity := schemaHasIdentity(schema)
	for _, parameter := range parameters {
		if !parameter.Required {
			continue
		}
		bindingPath := parameterBindingPath(parameter, schema)
		identity := parameterIdentity(parameter) && !(hasIdentity && parameterScopeOnly(parameter))
		if schemaPathExists(schema, bindingPath) {
			if identity {
				out = append(out, project.SchemaPath{
					Path:     bindingPath,
					Type:     firstNonEmpty(parameter.Type, "string"),
					Required: true,
					Identity: true,
				})
			}
			continue
		}
		out = append(out, project.SchemaPath{
			Path:     bindingPath,
			Type:     firstNonEmpty(parameter.Type, "string"),
			Required: true,
			Identity: identity,
		})
	}
	return out
}

func schemaPathExists(schema []project.SchemaPath, want string) bool {
	want = strings.TrimSpace(want)
	for _, path := range schema {
		if strings.TrimSpace(path.Path) == want {
			return true
		}
	}
	return false
}

func parameterIdentity(parameter operationParameterMetadata) bool {
	if strings.EqualFold(strings.TrimSpace(parameter.In), "path") {
		return !parameterScopeOnly(parameter)
	}
	name := strings.ToLower(strings.TrimSpace(parameter.Name))
	return name == "id" || strings.HasSuffix(name, "id") || strings.Contains(name, "name")
}

func parameterScopeOnly(parameter operationParameterMetadata) bool {
	name := normalizedParameterName(parameter.Name)
	switch name {
	case "subscriptionid", "apiversion", "kind", "location":
		return true
	default:
		return false
	}
}

func requestBindingsForParameters(parameters []operationParameterMetadata, operationID string) []project.RequestBinding {
	return requestBindingsForParametersRole(parameters, operationID, "read")
}

func requestBindingsForParametersRole(parameters []operationParameterMetadata, operationID, role string) []project.RequestBinding {
	return requestBindingsForParametersRoleSchema(parameters, operationID, role, nil)
}

func requestBindingsForParametersRoleSchema(parameters []operationParameterMetadata, operationID, role string, schema []project.SchemaPath) []project.RequestBinding {
	var out []project.RequestBinding
	for _, parameter := range parameters {
		if !parameter.Required {
			continue
		}
		bindingPath := parameterBindingPath(parameter, schema)
		out = append(out, project.RequestBinding{
			OperationRole: role,
			OperationID:   operationID,
			Path:          bindingPath,
			RequestPath:   parameter.Name,
			Location:      firstNonEmpty(parameter.In, "query"),
			Required:      true,
			Identity:      schemaPathIsIdentity(schema, bindingPath),
		})
	}
	return out
}

func parameterBindingPath(parameter operationParameterMetadata, schema []project.SchemaPath) string {
	name := strings.TrimSpace(parameter.Name)
	if name == "" {
		return ""
	}
	if schemaPathExists(schema, name) {
		return name
	}
	for _, candidate := range []string{"metadata." + name, "metadata." + lowerCamelToSnake(name), "metadata." + strings.ToLower(name)} {
		if schemaPathExists(schema, candidate) {
			return candidate
		}
	}
	if strings.EqualFold(name, "name") {
		for _, path := range schema {
			if path.Identity && strings.HasSuffix(strings.ToLower(path.Path), ".name") {
				return path.Path
			}
		}
	}
	if alias := canonicalIdentityAliasPath(name, schema); alias != "" {
		return alias
	}
	return name
}

func canonicalIdentityAliasPath(name string, schema []project.SchemaPath) string {
	alias := normalizedParameterName(name)
	if alias == "" {
		return ""
	}
	switch alias {
	case "bucketname", "bucket", "databasename", "databaseid", "object", "objectname", "managedfolder", "managedfoldername":
	default:
		return ""
	}
	for _, candidate := range []string{"name", "metadata.name", "result.name"} {
		if schemaPathExists(schema, candidate) {
			return candidate
		}
	}
	for _, path := range schema {
		if path.Identity && strings.HasSuffix(strings.ToLower(path.Path), ".name") {
			return path.Path
		}
	}
	return ""
}

func normalizedParameterName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, "-", "")
	return name
}

func schemaPathIsIdentity(schema []project.SchemaPath, want string) bool {
	want = strings.TrimSpace(want)
	for _, path := range schema {
		if path.Path == want {
			return path.Identity
		}
	}
	return false
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
	if variable := variableForSchemaPath(goal, resourceName, parameter.Name); variable != "" {
		return "${var." + variable + "}"
	}
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

func schemaAttributeValueForGoal(path project.SchemaPath, goal, resourceName string) any {
	schemaPath := strings.TrimSpace(path.Path)
	if strings.EqualFold(path.Type, "array") {
		return []any{}
	}
	if strings.EqualFold(path.Type, "object") {
		if strings.HasSuffix(strings.ToLower(schemaPath), ".labels") && strings.Contains(strings.ToLower(goal), "parity labels") {
			lane := parityLaneFromText(goal)
			labels := map[string]any{"app.kubernetes.io/managed-by": "ramen-parity"}
			if lane != "" {
				labels["ramen.openudon.dev/lane"] = lane
			}
			return labels
		}
		return map[string]any{}
	}
	if variable := variableForSchemaPath(goal, resourceName, schemaPath); variable != "" {
		return "${var." + variable + "}"
	}
	if strings.TrimSpace(resourceName) != "" {
		return resourceName
	}
	return "${var." + slug(schemaPath) + "}"
}

func variableForSchemaPath(goal, resourceName, schemaPath string) string {
	variables := variableRefsInText(goal)
	if len(variables) == 0 {
		return ""
	}
	pathTokens := tokenSet(schemaPath)
	resourceTokens := tokenSet(resourceName)
	best := ""
	bestScore := 0
	for _, variable := range variables {
		pathScore := 0
		resourceScore := 0
		varTokens := tokenSet(variable)
		for token := range varTokens {
			if pathTokens[token] {
				pathScore += 4
			}
			if resourceTokens[token] {
				resourceScore += 3
			}
		}
		if pathScore == 0 {
			continue
		}
		score := pathScore + resourceScore
		if strings.HasSuffix(strings.ToLower(schemaPath), ".namespace") && varTokens["namespace"] {
			score += 10
		}
		if strings.HasSuffix(strings.ToLower(schemaPath), ".name") && varTokens["name"] {
			score += 4
		}
		if strings.HasSuffix(strings.ToLower(schemaPath), ".name") && varTokens["namespace"] && len(variables) > 1 {
			score -= 6
		}
		if score > bestScore {
			best = variable
			bestScore = score
		}
	}
	if bestScore <= 0 {
		return ""
	}
	return best
}

func variableRefsInText(text string) []string {
	return variableRefs(text)
}

func tokenSet(value string) map[string]bool {
	out := map[string]bool{}
	value = value + " " + lowerCamelToSnake(value)
	for _, part := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == '/' || r == ' ' || r == ':' || r == '{' || r == '}'
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			out[part] = true
		}
	}
	return out
}

func parityLaneFromText(text string) string {
	lower := strings.ToLower(text)
	for _, field := range strings.Fields(lower) {
		field = strings.Trim(field, "`'\".,;:()[]{}")
		if len(field) == 3 && field[0] >= 'a' && field[0] <= 'z' && field[1] >= '0' && field[1] <= '9' && field[2] >= '0' && field[2] <= '9' {
			return field
		}
	}
	return ""
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

func responseSchemaPathsForRead(schema []project.SchemaPath) []project.SchemaPath {
	out := responseSchemaPaths(schema)
	if len(out) > 0 {
		return out
	}
	for _, path := range schema {
		if path.Identity {
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
		for _, value := range flattenedAttributeValues(resource.Attributes) {
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

func setAttributePath(attributes map[string]any, path string, value any) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	parts := strings.Split(path, ".")
	cur := attributes
	for _, part := range parts[:len(parts)-1] {
		next, _ := cur[part].(map[string]any)
		if next == nil {
			next = map[string]any{}
			cur[part] = next
		}
		cur = next
	}
	leaf := parts[len(parts)-1]
	if existing, ok := cur[leaf].(map[string]any); ok {
		if incoming, ok := value.(map[string]any); ok {
			for key, child := range incoming {
				existing[key] = child
			}
			return
		}
	}
	cur[leaf] = value
}

func flattenedAttributeValues(attrs map[string]any) []any {
	var out []any
	var visit func(any)
	visit = func(value any) {
		out = append(out, value)
		if m, ok := value.(map[string]any); ok {
			for _, child := range m {
				visit(child)
			}
		}
	}
	for _, value := range attrs {
		visit(value)
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

func normalizeProjectSourceKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "google_discovery", "google-discovery", "discovery", "google":
		return "google-discovery"
	case "aws_smithy", "aws-smithy", "smithy", "smithy-json":
		return "aws-smithy"
	case "grpc_protobuf", "grpc-protobuf":
		return "grpc-protobuf"
	default:
		return strings.TrimSpace(kind)
	}
}

func lowerCamelToSnake(value string) string {
	var b strings.Builder
	for i, r := range value {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

func mergeSchemaPaths(base []project.SchemaPath, extra []project.SchemaPath) []project.SchemaPath {
	out := append([]project.SchemaPath(nil), base...)
	index := map[string]int{}
	for i, path := range out {
		key := strings.TrimSpace(path.Path)
		if key != "" {
			index[key] = i
		}
	}
	for _, path := range extra {
		path.Path = strings.TrimSpace(path.Path)
		if path.Path == "" {
			continue
		}
		if i, ok := index[path.Path]; ok {
			out[i] = mergeSchemaPath(out[i], path)
			continue
		}
		index[path.Path] = len(out)
		out = append(out, path)
	}
	return out
}

func mergeSchemaPath(a, b project.SchemaPath) project.SchemaPath {
	preserveWritable := !a.Computed && !a.ReadOnly && !a.ResponseDerivedIdentity && (b.Computed || b.ReadOnly || b.ResponseDerivedIdentity)
	if a.Type == "" {
		a.Type = b.Type
	}
	a.Required = a.Required || b.Required
	a.Identity = a.Identity || b.Identity
	a.Sensitive = a.Sensitive || b.Sensitive
	a.Computed = a.Computed || b.Computed
	a.ReadOnly = a.ReadOnly || b.ReadOnly
	a.Immutable = a.Immutable || b.Immutable
	a.CreateOnly = a.CreateOnly || b.CreateOnly
	a.Updateable = a.Updateable || b.Updateable
	a.ReplaceOnChange = a.ReplaceOnChange || b.ReplaceOnChange
	a.ResponseDerivedIdentity = a.ResponseDerivedIdentity || b.ResponseDerivedIdentity
	a.Optional = a.Optional || b.Optional
	a.Ignored = a.Ignored || b.Ignored
	if preserveWritable {
		a.Computed = false
		a.ReadOnly = false
		a.ResponseDerivedIdentity = false
	}
	if b.Required && b.Identity && !b.ResponseDerivedIdentity {
		a.Computed = false
		a.ReadOnly = false
		a.ResponseDerivedIdentity = false
	}
	return a
}

func dedupeRequestBindings(bindings []project.RequestBinding) []project.RequestBinding {
	seen := map[string]bool{}
	var out []project.RequestBinding
	for _, binding := range bindings {
		key := strings.Join([]string{binding.OperationRole, binding.OperationID, binding.Path, binding.RequestPath, binding.Location}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, binding)
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

func sourceSupportedLongRunningOperation(operation promptcontext.OperationCandidate) bool {
	value := strings.ToLower(strings.TrimSpace(operation.Metadata["x-ms-long-running-operation"]))
	return value == "true"
}

func runtimeHintsForSourceMetadata(hasSourceLRO, hasRead bool) *project.RuntimeHints {
	if !hasSourceLRO || !hasRead {
		return nil
	}
	return &project.RuntimeHints{
		Retry: map[string]any{
			"max_attempts": 3,
		},
		Waiter: map[string]any{
			"until":        "exists",
			"max_attempts": 3,
		},
	}
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
