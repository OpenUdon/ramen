package plan

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/OpenUdon/apitools"
	"github.com/OpenUdon/evidence/digest"
	"github.com/OpenUdon/ramen/governance"
	"github.com/OpenUdon/ramen/graph"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/state"
	"github.com/OpenUdon/ramen/tfmapping"
	"github.com/OpenUdon/tfconfig"
)

const Version = "ramen.plan.v1"
const MappingMetadataVersion = "ramen.mapping-metadata.v1"

type Options struct {
	ConfigDir   string
	ProjectPath string
	StatePath   string
	APISources  []APISourceInput
	VarFiles    []string
	Vars        []string
	PolicyFiles []string
	Approvers   []governance.Approver
	Workspace   string
	Action      string
	OutPath     string
	Targets     []string
	Excludes    []string
	Replaces    []string
	Destroy     bool
}

type APISourceInput struct {
	Kind string
	ID   string
	Path string
}

type Result struct {
	StatePath   string
	OutPath     string
	Plan        Document
	Diagnostics []Diagnostic
}

type Document struct {
	Version     string                 `json:"version"`
	ConfigDir   string                 `json:"config_dir"`
	ProjectPath string                 `json:"project_path,omitempty"`
	StatePath   string                 `json:"state_path"`
	Workspace   string                 `json:"workspace,omitempty"`
	Action      string                 `json:"action"`
	Inputs      project.ResolvedInputs `json:"inputs,omitempty"`
	Governance  governance.Result      `json:"governance,omitempty"`
	Rationale   string                 `json:"rationale,omitempty"`
	Controls    Controls               `json:"controls,omitempty"`
	APISources  []APISourceRef         `json:"api_sources,omitempty"`
	Approval    *Approval              `json:"approval,omitempty"`
	Errored     bool                   `json:"errored,omitempty"`
	Summary     Summary                `json:"summary"`
	Resources   []ResourcePlan         `json:"resources"`
	Diagnostics []Diagnostic           `json:"diagnostics,omitempty"`
}

type Controls struct {
	Targets  []string `json:"targets,omitempty"`
	Excludes []string `json:"excludes,omitempty"`
	Replaces []string `json:"replaces,omitempty"`
	Destroy  bool     `json:"destroy,omitempty"`
}

type APISourceRef struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Path   string `json:"path,omitempty"`
	Digest string `json:"digest,omitempty"`
}

type Approval struct {
	Version       string                `json:"version"`
	Digest        string                `json:"digest"`
	Rationale     string                `json:"rationale,omitempty"`
	ProjectDigest string                `json:"project_digest,omitempty"`
	StateDigest   string                `json:"state_digest,omitempty"`
	Controls      Controls              `json:"controls,omitempty"`
	APISources    []APISourceRef        `json:"api_sources,omitempty"`
	Approvers     []governance.Approver `json:"approvers,omitempty"`
}

type Summary struct {
	Create      int `json:"create"`
	Update      int `json:"update"`
	Delete      int `json:"delete"`
	Replace     int `json:"replace"`
	NoOp        int `json:"no_op"`
	Read        int `json:"read"`
	Diagnostics int `json:"diagnostics"`
}

type ResourcePlan struct {
	Address      string              `json:"address"`
	Kind         string              `json:"kind"`
	Type         string              `json:"type"`
	Name         string              `json:"name"`
	Provider     string              `json:"provider,omitempty"`
	Action       string              `json:"action"`
	Reason       string              `json:"reason"`
	DesiredHash  string              `json:"desired_hash,omitempty"`
	Dependencies []string            `json:"dependencies,omitempty"`
	Mapping      *MappingPlan        `json:"mapping,omitempty"`
	AI           *project.AIMetadata `json:"ai,omitempty"`
}

type MappingPlan struct {
	Purpose            string                        `json:"purpose"`
	SourceKind         string                        `json:"source_kind,omitempty"`
	SourceID           string                        `json:"source_id,omitempty"`
	SourcePath         string                        `json:"source_path,omitempty"`
	OperationID        string                        `json:"operation_id,omitempty"`
	IdentityAttributes []tfmapping.IdentityAttribute `json:"identity_attributes,omitempty"`
	Schema             []project.SchemaPath          `json:"schema,omitempty"`
	RequestBindings    []project.RequestBinding      `json:"request_bindings,omitempty"`
	ResponseBindings   []project.ResponseBinding     `json:"response_bindings,omitempty"`
	Normalizers        []project.Normalizer          `json:"normalizers,omitempty"`
	MappingLifecycle   *project.MappingLifecycle     `json:"mapping_lifecycle,omitempty"`
	RequiredOperations []string                      `json:"required_operations,omitempty"`
	AI                 *project.AIMetadata           `json:"ai,omitempty"`
}

type DesiredHashInput struct {
	Address         string
	Type            string
	Provider        string
	Attributes      map[string]string
	Lifecycle       map[string]any
	Mapping         *MappingHashInput
	APISourceDigest string
	InputsDigest    string
}

type MappingHashInput struct {
	MetadataVersion    string
	Purpose            string
	SourceKind         string
	SourceID           string
	OperationID        string
	IdentityAttributes []tfmapping.IdentityAttribute
	Schema             []project.SchemaPath
	RequestBindings    []project.RequestBinding
	ResponseBindings   []project.ResponseBinding
	Normalizers        []project.Normalizer
	MappingLifecycle   *project.MappingLifecycle
	RequiredOperations []string
}

type Diagnostic struct {
	Code          string `json:"code"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	Address       string `json:"address,omitempty"`
	ModuleAddress string `json:"module_address,omitempty"`
	APISourceKind string `json:"api_source_kind,omitempty"`
	APISourceID   string `json:"api_source_id,omitempty"`
}

type sourceDoc struct {
	ID         string
	Kind       string
	Path       string
	Digest     string
	Operations []apitools.OperationSummary
}

type objectFact struct {
	Address       string
	ModuleAddress string
	Kind          string
	Type          string
	Name          string
	Provider      string
	Config        []tfconfig.Attribute
	Lifecycle     *tfconfig.Lifecycle
	DependsOn     []tfconfig.Reference
	References    []tfconfig.Reference
}

type lifecyclePlan struct {
	PreventDestroy bool
	IgnoreAll      bool
	IgnorePaths    []string
	Diagnostics    []Diagnostic
	Hash           map[string]any
}

type operationMatch struct {
	Operation apitools.OperationSummary
	Source    sourceDoc
}

func Build(ctx context.Context, opts Options) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opts = normalizeOptions(opts)
	if strings.TrimSpace(opts.ProjectPath) != "" {
		return buildProject(ctx, opts)
	}
	doc, loadErr := tfconfig.LoadDir(opts.ConfigDir)
	var diagnostics []Diagnostic
	if loadErr != nil {
		diagnostics = append(diagnostics, Diagnostic{Code: "tfconfig.load_error", Severity: "error", Message: loadErr.Error()})
	}
	diagnostics = append(diagnostics, tfDiagnostics(doc.Diagnostics)...)
	for _, mod := range doc.Modules {
		diagnostics = append(diagnostics, tfDiagnostics(mod.Diagnostics)...)
	}
	diagnostics = append(diagnostics, staticBlockDiagnostics(doc)...)

	apiSources, apiDiagnostics := loadAPISources(ctx, opts.APISources)
	diagnostics = append(diagnostics, apiDiagnostics...)

	var store *state.Store
	if strings.TrimSpace(opts.StatePath) != "" {
		opened, err := state.OpenReadOnly(ctx, opts.StatePath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Code: "state.open_error", Severity: "error", Message: err.Error()})
		} else {
			store = opened
			defer store.Close()
		}
	}

	objects := collectResources(doc)
	nodes := graphNodes(objects)
	sortedNodes, err := graph.Sort(nodes)
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{Code: "graph.cycle", Severity: "error", Message: err.Error()})
		sortedNodes = nodes
	}
	selection, selectionDiagnostics := selectNodes(sortedNodes, controlsFromOptions(opts))
	diagnostics = append(diagnostics, selectionDiagnostics...)
	objectsByAddress := map[string]objectFact{}
	for _, obj := range objects {
		objectsByAddress[obj.Address] = obj
	}

	var resources []ResourcePlan
	desiredAddresses := map[string]bool{}
	for _, node := range sortedNodes {
		if !selection[node.Address] {
			continue
		}
		obj, ok := objectsByAddress[node.Address]
		if !ok || obj.Kind != "resource" {
			continue
		}
		desiredAddresses[obj.Address] = true
		resourcePlan := planResource(ctx, store, obj, node.DependsOn, opts.Action, apiSources, slices.Contains(opts.Replaces, obj.Address))
		diagnostics = append(diagnostics, resourcePlan.diagnostics...)
		resources = append(resources, resourcePlan.resource)
	}
	deletePlans, deleteDiagnostics := planDeletes(ctx, store, desiredAddresses, apiSources)
	diagnostics = append(diagnostics, deleteDiagnostics...)
	resources = append(resources, deletePlans...)
	governanceResult, governanceDiagnostics := evaluateGovernance(opts, resources, "")
	diagnostics = append(diagnostics, governanceDiagnostics...)
	approvers, approverDiagnostics := normalizeApprovers(opts.Approvers)
	diagnostics = append(diagnostics, approverDiagnostics...)
	sortDiagnostics(diagnostics)
	errored := hasErrorDiagnostics(diagnostics)
	if errored {
		resources = nil
	}
	document := Document{
		Version:     Version,
		ConfigDir:   opts.ConfigDir,
		StatePath:   opts.StatePath,
		Workspace:   opts.Workspace,
		Action:      opts.Action,
		Governance:  governanceResult,
		Controls:    controlsFromOptions(opts),
		APISources:  apiSourceRefs(apiSources),
		Errored:     errored,
		Resources:   resources,
		Diagnostics: diagnostics,
	}
	document.Summary = summarize(document)
	document.Approval = buildApproval(document, "", stateBaselineDigest(ctx, store), approvers)
	result := &Result{StatePath: opts.StatePath, OutPath: opts.OutPath, Plan: document, Diagnostics: diagnostics}
	if opts.OutPath != "" {
		if err := writeJSON(opts.OutPath, document); err != nil {
			return result, err
		}
	}
	return result, nil
}

func buildProject(ctx context.Context, opts Options) (*Result, error) {
	proj, err := project.Load(opts.ProjectPath)
	var diagnostics []Diagnostic
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{Code: "project.load_error", Severity: "error", Message: err.Error()})
		document := Document{
			Version:     Version,
			ConfigDir:   opts.ConfigDir,
			ProjectPath: opts.ProjectPath,
			StatePath:   opts.StatePath,
			Action:      opts.Action,
			Rationale:   projectRationale(project.Profile{}),
			Errored:     true,
			Diagnostics: diagnostics,
		}
		document.Summary = summarize(document)
		result := &Result{StatePath: opts.StatePath, OutPath: opts.OutPath, Plan: document, Diagnostics: diagnostics}
		if opts.OutPath != "" {
			if writeErr := writeJSON(opts.OutPath, document); writeErr != nil {
				return result, writeErr
			}
		}
		return result, nil
	}
	resolvedProfile, inputs, valueDiagnostics := project.ResolveProfile(proj.Profile, proj.Dir, project.ValuesOptions{VarFiles: opts.VarFiles, Vars: opts.Vars})
	diagnostics = append(diagnostics, valuePlanDiagnostics(valueDiagnostics)...)
	if err := project.ValidateProfile(resolvedProfile); err != nil {
		diagnostics = append(diagnostics, Diagnostic{Code: "values.profile_invalid", Severity: "error", Message: err.Error()})
	}
	proj.Profile = resolvedProfile
	apiInputs := projectAPISourceInputs(proj.Profile, opts.APISources)
	apiSources, apiDiagnostics := loadAPISources(ctx, apiInputs)
	diagnostics = append(diagnostics, apiDiagnostics...)

	var store *state.Store
	if strings.TrimSpace(opts.StatePath) != "" {
		opened, err := state.OpenReadOnly(ctx, opts.StatePath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Code: "state.open_error", Severity: "error", Message: err.Error()})
		} else {
			store = opened
			defer store.Close()
		}
	}

	resourcesByAddress := map[string]project.Resource{}
	var nodes []graph.Node
	for _, resource := range proj.Profile.Resources {
		if resource.Kind != "resource" {
			continue
		}
		resourcesByAddress[resource.Address] = resource
		nodes = append(nodes, graph.Node{Address: resource.Address, DependsOn: slices.Clone(resource.Dependencies)})
	}
	sortedNodes, err := graph.Sort(nodes)
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{Code: "graph.cycle", Severity: "error", Message: err.Error()})
		sortedNodes = nodes
	}
	selection, selectionDiagnostics := selectNodes(sortedNodes, controlsFromOptions(opts))
	diagnostics = append(diagnostics, selectionDiagnostics...)

	var resources []ResourcePlan
	desiredAddresses := map[string]bool{}
	for _, node := range sortedNodes {
		if !selection[node.Address] {
			continue
		}
		resource := resourcesByAddress[node.Address]
		desiredAddresses[resource.Address] = true
		resourcePlan := planProjectResource(ctx, store, proj.Profile, resource, node.DependsOn, opts.Action, apiSources, slices.Contains(opts.Replaces, resource.Address), inputs.Digest)
		diagnostics = append(diagnostics, resourcePlan.diagnostics...)
		resources = append(resources, resourcePlan.resource)
	}
	deletePlans, deleteDiagnostics := planDeletes(ctx, store, desiredAddresses, apiSources)
	diagnostics = append(diagnostics, deleteDiagnostics...)
	resources = append(resources, deletePlans...)
	governanceResult, governanceDiagnostics := evaluateGovernance(opts, resources, inputs.Digest)
	diagnostics = append(diagnostics, governanceDiagnostics...)
	approvers, approverDiagnostics := normalizeApprovers(opts.Approvers)
	diagnostics = append(diagnostics, approverDiagnostics...)
	sortDiagnostics(diagnostics)
	errored := hasErrorDiagnostics(diagnostics)
	if errored {
		resources = nil
	}
	document := Document{
		Version:     Version,
		ConfigDir:   opts.ConfigDir,
		ProjectPath: proj.Path,
		StatePath:   opts.StatePath,
		Workspace:   opts.Workspace,
		Action:      opts.Action,
		Inputs:      inputs,
		Governance:  governanceResult,
		Rationale:   projectRationale(proj.Profile),
		Controls:    controlsFromOptions(opts),
		APISources:  apiSourceRefs(apiSources),
		Errored:     errored,
		Resources:   resources,
		Diagnostics: diagnostics,
	}
	document.Summary = summarize(document)
	document.Approval = buildApproval(document, projectDigest(proj.Profile), stateBaselineDigest(ctx, store), approvers)
	result := &Result{StatePath: opts.StatePath, OutPath: opts.OutPath, Plan: document, Diagnostics: diagnostics}
	if opts.OutPath != "" {
		if err := writeJSON(opts.OutPath, document); err != nil {
			return result, err
		}
	}
	return result, nil
}

func planDeletes(ctx context.Context, store *state.Store, desiredAddresses map[string]bool, sources []sourceDoc) ([]ResourcePlan, []Diagnostic) {
	if store == nil {
		return nil, nil
	}
	current, err := store.ListCurrentResources(ctx)
	if err != nil {
		return nil, []Diagnostic{{Code: "state.read_error", Severity: "error", Message: err.Error()}}
	}
	var resources []ResourcePlan
	var diagnostics []Diagnostic
	for _, snap := range current {
		if desiredAddresses[snap.Address] {
			continue
		}
		obj := objectFact{Address: snap.Address, Kind: "resource", Type: snap.Type, Provider: snap.Provider}
		mapping, mappingDiagnostics := mapResource(obj, "delete", "delete", sources, true)
		diagnostics = append(diagnostics, mappingDiagnostics...)
		resources = append(resources, ResourcePlan{
			Address:  snap.Address,
			Kind:     "resource",
			Type:     snap.Type,
			Provider: snap.Provider,
			Action:   "delete",
			Reason:   "resource is recorded in state but not declared in configuration",
			Mapping:  mapping,
		})
	}
	return resources, diagnostics
}

func selectNodes(nodes []graph.Node, controls Controls) (map[string]bool, []Diagnostic) {
	byAddress := map[string]graph.Node{}
	reverse := map[string][]string{}
	for _, node := range nodes {
		byAddress[node.Address] = node
		for _, dep := range node.DependsOn {
			reverse[dep] = append(reverse[dep], node.Address)
		}
	}
	selected := map[string]bool{}
	var diagnostics []Diagnostic
	targets := uniqueNonEmpty(controls.Targets)
	excludes := uniqueNonEmpty(controls.Excludes)
	replaces := uniqueNonEmpty(controls.Replaces)
	if len(targets) == 0 {
		for _, node := range nodes {
			selected[node.Address] = true
		}
	} else {
		for _, target := range targets {
			if _, ok := byAddress[target]; !ok {
				diagnostics = append(diagnostics, Diagnostic{Code: "plan.target_unknown", Severity: "error", Message: fmt.Sprintf("target %s does not match a native resource address", target), Address: target})
				continue
			}
			addDependencyClosure(selected, byAddress, target)
		}
	}
	excluded := map[string]bool{}
	for _, exclude := range excludes {
		if _, ok := byAddress[exclude]; !ok {
			diagnostics = append(diagnostics, Diagnostic{Code: "plan.exclude_unknown", Severity: "error", Message: fmt.Sprintf("exclude %s does not match a native resource address", exclude), Address: exclude})
			continue
		}
		addDependentClosure(excluded, reverse, exclude)
	}
	for _, replace := range replaces {
		if _, ok := byAddress[replace]; !ok {
			diagnostics = append(diagnostics, Diagnostic{Code: "plan.replace_unknown", Severity: "error", Message: fmt.Sprintf("replace %s does not match a native resource address", replace), Address: replace})
		}
	}
	for address := range excluded {
		if len(targets) > 0 && selected[address] {
			diagnostics = append(diagnostics, Diagnostic{Code: "plan.selection_conflict", Severity: "error", Message: fmt.Sprintf("resource %s is both selected and excluded", address), Address: address})
		}
		delete(selected, address)
	}
	return selected, diagnostics
}

func addDependencyClosure(selected map[string]bool, byAddress map[string]graph.Node, address string) {
	if selected[address] {
		return
	}
	node, ok := byAddress[address]
	if !ok {
		return
	}
	selected[address] = true
	for _, dep := range node.DependsOn {
		addDependencyClosure(selected, byAddress, dep)
	}
}

func addDependentClosure(selected map[string]bool, reverse map[string][]string, address string) {
	if selected[address] {
		return
	}
	selected[address] = true
	for _, dependent := range reverse[address] {
		addDependentClosure(selected, reverse, dependent)
	}
}

type plannedResource struct {
	resource    ResourcePlan
	diagnostics []Diagnostic
}

func planResource(ctx context.Context, store *state.Store, obj objectFact, dependencies []string, requestedAction string, sources []sourceDoc, forcedReplace bool) plannedResource {
	lifecycle := analyzeLifecycle(obj)
	diagnostics := slices.Clone(lifecycle.Diagnostics)
	desiredMapping, desiredMappingDiagnostics := mapResource(obj, desiredPurpose(requestedAction), requestedAction, sources, requestedAction == "create" || requestedAction == "delete")
	diagnostics = append(diagnostics, desiredMappingDiagnostics...)
	hash := desiredHash(obj, lifecycle, desiredMapping, sources)
	action := "create"
	reason := "resource is not recorded in state"
	var current *state.ResourceSnapshot
	if store != nil {
		var err error
		current, err = store.CurrentResource(ctx, obj.Address)
		if err != nil {
			return plannedResource{
				resource:    ResourcePlan{Address: obj.Address, Kind: obj.Kind, Type: obj.Type, Name: obj.Name, Provider: obj.Provider, Action: "error", Reason: err.Error(), DesiredHash: hash, Dependencies: dependencies},
				diagnostics: []Diagnostic{{Code: "state.read_error", Severity: "error", Message: err.Error(), Address: obj.Address, ModuleAddress: obj.ModuleAddress}},
			}
		}
		if requestedAction == "delete" {
			if current != nil {
				action = "delete"
				reason = "destroy requested for recorded resource"
			} else {
				action = "no-op"
				reason = "destroy requested but resource is not recorded in state"
			}
		} else if forcedReplace && current != nil {
			action = "replace"
			reason = "replacement forced by plan control"
		} else if current != nil && current.DesiredHash == hash {
			action = "no-op"
			reason = "recorded desired hash matches configuration"
		} else if current != nil {
			action = "update"
			reason = "recorded desired hash differs from configuration"
		}
	} else if requestedAction == "delete" {
		action = "no-op"
		reason = "destroy requested but state is unavailable"
	} else if forcedReplace {
		reason = "replacement requested but state is unavailable; create planned"
	}
	if (action == "delete" || action == "replace") && lifecycle.PreventDestroy {
		diagnostics = append(diagnostics, Diagnostic{
			Code:          "plan.prevent_destroy",
			Severity:      "error",
			Message:       fmt.Sprintf("%s has prevent_destroy set and cannot be deleted or replaced", obj.Address),
			Address:       obj.Address,
			ModuleAddress: obj.ModuleAddress,
		})
	}
	purpose := requestedAction
	if action == "update" {
		purpose = "update"
	}
	if action == "no-op" {
		purpose = "read"
	}
	mapping := desiredMapping
	if purpose != desiredMapping.Purpose || action != requestedAction {
		actualMapping, actualDiagnostics := mapResource(obj, purpose, action, sources, mappingRequired(requestedAction, action))
		mapping = actualMapping
		diagnostics = append(diagnostics, actualDiagnostics...)
	}
	return plannedResource{
		resource: ResourcePlan{
			Address:      obj.Address,
			Kind:         obj.Kind,
			Type:         obj.Type,
			Name:         obj.Name,
			Provider:     obj.Provider,
			Action:       action,
			Reason:       reason,
			DesiredHash:  hash,
			Dependencies: slices.Clone(dependencies),
			Mapping:      mapping,
		},
		diagnostics: diagnostics,
	}
}

func planProjectResource(ctx context.Context, store *state.Store, profile project.Profile, resource project.Resource, dependencies []string, requestedAction string, sources []sourceDoc, forcedReplace bool, inputsDigest string) plannedResource {
	lifecycle := analyzeProjectLifecycle(resource)
	diagnostics := slices.Clone(lifecycle.Diagnostics)
	desiredMapping, desiredMappingDiagnostics := mapProjectResource(profile, resource, desiredPurpose(requestedAction), requestedAction, sources, requestedAction == "create" || requestedAction == "delete")
	diagnostics = append(diagnostics, desiredMappingDiagnostics...)
	hash := desiredProjectHash(resource, lifecycle, desiredMapping, sources, inputsDigest)
	action := "create"
	reason := "resource is not recorded in state"
	var current *state.ResourceSnapshot
	if store != nil {
		var err error
		current, err = store.CurrentResource(ctx, resource.Address)
		if err != nil {
			return plannedResource{
				resource:    ResourcePlan{Address: resource.Address, Kind: resource.Kind, Type: resource.Type, Name: resource.Name, Provider: resource.Provider, Action: "error", Reason: err.Error(), DesiredHash: hash, Dependencies: dependencies},
				diagnostics: []Diagnostic{{Code: "state.read_error", Severity: "error", Message: err.Error(), Address: resource.Address}},
			}
		}
		if requestedAction == "delete" {
			if current != nil {
				action = "delete"
				reason = "destroy requested for recorded resource"
			} else {
				action = "no-op"
				reason = "destroy requested but resource is not recorded in state"
			}
		} else if forcedReplace && current != nil {
			action = "replace"
			reason = "replacement forced by plan control"
		} else if current != nil && current.DesiredHash == hash {
			action = "no-op"
			reason = "recorded desired hash matches native project"
		} else if current != nil && projectReplacementChanged(resource, current, desiredMapping) {
			action = "replace"
			reason = "replace-on-change path differs from recorded state"
		} else if current != nil {
			action = "update"
			reason = "recorded desired hash differs from native project"
		}
	} else if requestedAction == "delete" {
		action = "no-op"
		reason = "destroy requested but state is unavailable"
	} else if forcedReplace {
		reason = "replacement requested but state is unavailable; create planned"
	}
	if (action == "delete" || action == "replace") && lifecycle.PreventDestroy {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "plan.prevent_destroy",
			Severity: "error",
			Message:  fmt.Sprintf("%s has prevent_destroy set and cannot be deleted or replaced", resource.Address),
			Address:  resource.Address,
		})
	}
	purpose := requestedAction
	if action == "update" {
		purpose = "update"
	}
	if action == "no-op" {
		purpose = "read"
	}
	mapping := desiredMapping
	if mapping == nil || purpose != desiredMapping.Purpose || action != requestedAction {
		actualMapping, actualDiagnostics := mapProjectResource(profile, resource, purpose, action, sources, mappingRequired(requestedAction, action))
		mapping = actualMapping
		diagnostics = append(diagnostics, actualDiagnostics...)
	}
	return plannedResource{
		resource: ResourcePlan{
			Address:      resource.Address,
			Kind:         resource.Kind,
			Type:         resource.Type,
			Name:         resource.Name,
			Provider:     resource.Provider,
			Action:       action,
			Reason:       reason,
			DesiredHash:  hash,
			Dependencies: slices.Clone(dependencies),
			Mapping:      mapping,
			AI:           resource.AI,
		},
		diagnostics: diagnostics,
	}
}

func desiredPurpose(action string) string {
	if action == "delete" {
		return "delete"
	}
	return "create"
}

func mappingRequired(requestedAction, actualAction string) bool {
	return requestedAction == "create" && (actualAction == "create" || actualAction == "update" || actualAction == "replace") ||
		requestedAction == "delete" && actualAction == "delete"
}

func mapResource(obj objectFact, purpose, action string, sources []sourceDoc, required bool) (*MappingPlan, []Diagnostic) {
	spec := tfmapping.DefaultRegistry().MapObject(tfmapping.Object{Kind: obj.Kind, Type: obj.Type, Provider: obj.Provider}, purpose, action)
	var diagnostics []Diagnostic
	for _, diag := range spec.Diagnostics {
		severity := string(diag.Severity)
		if required && len(spec.Target.OperationIDs) == 0 {
			severity = "error"
		}
		diagnostics = append(diagnostics, Diagnostic{Code: string(diag.Code), Severity: severity, Message: diag.Message, Address: obj.Address, ModuleAddress: obj.ModuleAddress})
	}
	if len(spec.Target.OperationIDs) == 0 {
		return mappingPlanForSpec(purpose, spec), diagnostics
	}
	if match, ambiguous := findOperation(sources, spec.Target); ambiguous {
		diagnostics = append(diagnostics, Diagnostic{
			Code:          "mapping.operation_ambiguous",
			Severity:      "error",
			Message:       fmt.Sprintf("multiple loaded API source operations matched %s for %s", strings.Join(spec.Target.OperationIDs, ", "), obj.Address),
			Address:       obj.Address,
			ModuleAddress: obj.ModuleAddress,
		})
		return mappingPlanForSpec(purpose, spec), diagnostics
	} else if match != nil {
		mapping := mappingPlanForSpec(purpose, spec)
		mapping.SourceKind = match.Source.Kind
		mapping.SourceID = firstNonEmpty(match.Operation.DocumentName, match.Source.ID)
		mapping.SourcePath = match.Source.Path
		mapping.OperationID = match.Operation.OperationID
		return mapping, diagnostics
	}
	severity := "warning"
	if required {
		severity = "error"
	}
	diagnostics = append(diagnostics, Diagnostic{
		Code:          "mapping.operation_unavailable",
		Severity:      severity,
		Message:       fmt.Sprintf("no loaded API source operation matched %s for %s", strings.Join(spec.Target.OperationIDs, ", "), obj.Address),
		Address:       obj.Address,
		ModuleAddress: obj.ModuleAddress,
	})
	return mappingPlanForSpec(purpose, spec), diagnostics
}

func mapProjectResource(profile project.Profile, resource project.Resource, purpose, action string, sources []sourceDoc, required bool) (*MappingPlan, []Diagnostic) {
	role, ok := projectOperationRole(resource, purpose, action)
	if !ok {
		severity := "warning"
		if required {
			severity = "error"
		}
		return mappingPlanForProjectResource(resource, purpose), []Diagnostic{{
			Code:     "project.operation_missing",
			Severity: severity,
			Message:  fmt.Sprintf("native project resource %s has no %s operation role", resource.Address, purpose),
			Address:  resource.Address,
		}}
	}
	source := project.SourceForRole(profile, role)
	sourceKind := firstNonEmpty(role.SourceKind, source.Kind)
	sourceID := firstNonEmpty(role.SourceID, source.ID)
	sourcePath := firstNonEmpty(role.SourcePath, source.Path)
	mapping := mappingPlanForProjectResource(resource, purpose)
	mapping.SourceKind = sourceKind
	mapping.SourceID = sourceID
	mapping.SourcePath = sourcePath
	mapping.OperationID = role.OperationID
	mapping.AI = role.AI
	diagnostics := validateProjectOperation(resource, mapping, sources, required)
	return mapping, diagnostics
}

func mappingPlanForSpec(purpose string, spec tfmapping.Mapping) *MappingPlan {
	return &MappingPlan{
		Purpose:            purpose,
		IdentityAttributes: slices.Clone(spec.IdentityAttributes),
		Schema:             schemaPathsFromMapping(spec.Schema),
		RequestBindings:    requestBindingsFromMapping(spec.RequestBindings),
		ResponseBindings:   responseBindingsFromMapping(spec.ResponseBindings),
		Normalizers:        normalizersFromMapping(spec.Normalizers),
		MappingLifecycle:   mappingLifecycleFromMapping(spec.Lifecycle),
	}
}

func mappingPlanForProjectResource(resource project.Resource, purpose string) *MappingPlan {
	return &MappingPlan{
		Purpose:            purpose,
		IdentityAttributes: projectIdentityAttributes(resource.IdentityAttributes),
		Schema:             slices.Clone(resource.Schema),
		RequestBindings:    slices.Clone(resource.RequestBindings),
		ResponseBindings:   slices.Clone(resource.ResponseBindings),
		Normalizers:        slices.Clone(resource.Normalizers),
		MappingLifecycle:   cloneMappingLifecycle(resource.MappingLifecycle),
		RequiredOperations: slices.Clone(resource.RequiredOperations),
	}
}

func projectOperationRole(resource project.Resource, purpose, action string) (project.OperationRole, bool) {
	keys := []string{purpose}
	if action != "" && action != purpose {
		keys = append(keys, action)
	}
	if purpose == "delete" || action == "delete" {
		keys = append(keys, "remove_config", "detach", "disable", "suspend")
	}
	for _, key := range keys {
		if role, ok := resource.Operations[key]; ok {
			return role, true
		}
	}
	return project.OperationRole{}, false
}

func validateProjectOperation(resource project.Resource, mapping *MappingPlan, sources []sourceDoc, required bool) []Diagnostic {
	if mapping == nil || strings.TrimSpace(mapping.OperationID) == "" {
		return nil
	}
	var matches []sourceDoc
	for _, source := range sources {
		if mapping.SourceKind != "" && source.Kind != mapping.SourceKind {
			continue
		}
		if mapping.SourceID != "" && source.ID != mapping.SourceID {
			continue
		}
		if mapping.SourceID == "" && mapping.SourcePath != "" && filepath.Clean(source.Path) != filepath.Clean(mapping.SourcePath) {
			continue
		}
		for _, op := range source.Operations {
			if op.OperationID == mapping.OperationID {
				matches = append(matches, source)
				break
			}
		}
	}
	switch len(matches) {
	case 0:
		severity := "warning"
		if required {
			severity = "error"
		}
		return []Diagnostic{{
			Code:          "project.operation_unavailable",
			Severity:      severity,
			Message:       fmt.Sprintf("native project operation %s for %s was not found in loaded API sources", mapping.OperationID, resource.Address),
			Address:       resource.Address,
			APISourceKind: mapping.SourceKind,
			APISourceID:   mapping.SourceID,
		}}
	case 1:
		return nil
	default:
		return []Diagnostic{{
			Code:          "project.operation_ambiguous",
			Severity:      "error",
			Message:       fmt.Sprintf("native project operation %s for %s matched multiple loaded API sources", mapping.OperationID, resource.Address),
			Address:       resource.Address,
			APISourceKind: mapping.SourceKind,
			APISourceID:   mapping.SourceID,
		}}
	}
}

func projectIdentityAttributes(attrs []project.IdentityAttribute) []tfmapping.IdentityAttribute {
	out := make([]tfmapping.IdentityAttribute, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, tfmapping.IdentityAttribute{
			Name:          attr.Name,
			TerraformPath: attr.Path,
			RequestKeys:   slices.Clone(attr.RequestKeys),
			ResponsePaths: slices.Clone(attr.ResponsePaths),
			Required:      attr.Required,
		})
	}
	return out
}

func schemaPathsFromMapping(paths []tfmapping.SchemaPath) []project.SchemaPath {
	out := make([]project.SchemaPath, 0, len(paths))
	for _, path := range paths {
		out = append(out, project.SchemaPath{
			Path:                    path.Path,
			Type:                    path.Type,
			EnumValues:              slices.Clone(path.EnumValues),
			Required:                path.Required,
			Optional:                path.Optional,
			Computed:                path.Computed,
			Sensitive:               path.Sensitive,
			Identity:                path.Identity,
			ResponseDerivedIdentity: path.ResponseDerivedIdentity,
			Immutable:               path.Immutable,
			CreateOnly:              path.CreateOnly,
			Updateable:              path.Updateable,
			ReplaceOnChange:         path.ReplaceOnChange,
			ReadOnly:                path.ReadOnly,
			Ignored:                 path.Ignored,
		})
	}
	return out
}

func requestBindingsFromMapping(bindings []tfmapping.RequestBinding) []project.RequestBinding {
	out := make([]project.RequestBinding, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, project.RequestBinding{
			OperationRole: string(binding.OperationRole),
			OperationID:   binding.OperationID,
			Path:          binding.Path,
			RequestPath:   binding.RequestPath,
			Required:      binding.Required,
			Identity:      binding.Identity,
		})
	}
	return out
}

func responseBindingsFromMapping(bindings []tfmapping.ResponseBinding) []project.ResponseBinding {
	out := make([]project.ResponseBinding, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, project.ResponseBinding{
			OperationRole:           string(binding.OperationRole),
			OperationID:             binding.OperationID,
			ResponsePath:            binding.ResponsePath,
			StatePath:               binding.StatePath,
			Identity:                binding.Identity,
			ResponseDerivedIdentity: binding.ResponseDerivedIdentity,
			Computed:                binding.Computed,
			Observed:                binding.Observed,
			Sensitive:               binding.Sensitive,
		})
	}
	return out
}

func normalizersFromMapping(normalizers []tfmapping.Normalizer) []project.Normalizer {
	out := make([]project.Normalizer, 0, len(normalizers))
	for _, normalizer := range normalizers {
		out = append(out, project.Normalizer{Path: normalizer.Path, Kind: string(normalizer.Kind)})
	}
	return out
}

func mappingLifecycleFromMapping(lifecycle *tfmapping.LifecycleSemantics) *project.MappingLifecycle {
	if lifecycle == nil {
		return nil
	}
	out := &project.MappingLifecycle{
		OperationRoles: make([]string, 0, len(lifecycle.OperationRoles)),
		Paths:          make([]project.MappingLifecyclePath, 0, len(lifecycle.Paths)),
	}
	for _, role := range lifecycle.OperationRoles {
		out.OperationRoles = append(out.OperationRoles, string(role))
	}
	for _, path := range lifecycle.Paths {
		out.Paths = append(out.Paths, project.MappingLifecyclePath{
			Path:            path.Path,
			Immutable:       path.Immutable,
			CreateOnly:      path.CreateOnly,
			Updateable:      path.Updateable,
			ReplaceOnChange: path.ReplaceOnChange,
			Computed:        path.Computed,
			Ignored:         path.Ignored,
		})
	}
	return out
}

func cloneMappingLifecycle(lifecycle *project.MappingLifecycle) *project.MappingLifecycle {
	if lifecycle == nil {
		return nil
	}
	return &project.MappingLifecycle{
		OperationRoles: slices.Clone(lifecycle.OperationRoles),
		Paths:          slices.Clone(lifecycle.Paths),
	}
}

func findOperation(sources []sourceDoc, target tfmapping.OperationTarget) (*operationMatch, bool) {
	for _, kind := range target.SourceKinds {
		for _, operationID := range target.OperationIDs {
			var matches []operationMatch
			for _, source := range sources {
				if source.Kind != kind || !sourceIDMatches(source.ID, target.SourceIDs) {
					continue
				}
				for _, op := range source.Operations {
					if op.OperationID == operationID {
						matches = append(matches, operationMatch{Operation: op, Source: source})
					}
				}
			}
			if len(matches) == 1 {
				return &matches[0], false
			}
			if len(matches) > 1 {
				return nil, true
			}
		}
	}
	for _, operationID := range target.OperationIDs {
		var matches []operationMatch
		for _, source := range sources {
			for _, op := range source.Operations {
				if op.OperationID == operationID {
					matches = append(matches, operationMatch{Operation: op, Source: source})
				}
			}
		}
		if len(matches) == 1 {
			return &matches[0], false
		}
		if len(matches) > 1 {
			return nil, true
		}
	}
	return nil, false
}

func desiredHash(obj objectFact, lifecycle lifecyclePlan, mapping *MappingPlan, sources []sourceDoc) string {
	attrs := map[string]string{}
	if !lifecycle.IgnoreAll {
		ignored := map[string]bool{}
		for _, path := range lifecycle.IgnorePaths {
			ignored[path] = true
		}
		for _, attr := range obj.Config {
			if ignored[attr.Path] {
				continue
			}
			attrs[attr.Path] = valueText(attr.Value)
		}
	}
	var mappingHash *MappingHashInput
	selectedDigest := ""
	if mapping != nil {
		mappingHash = &MappingHashInput{
			MetadataVersion:    MappingMetadataVersion,
			Purpose:            mapping.Purpose,
			SourceKind:         mapping.SourceKind,
			SourceID:           mapping.SourceID,
			OperationID:        mapping.OperationID,
			IdentityAttributes: mapping.IdentityAttributes,
			Schema:             mapping.Schema,
			RequestBindings:    mapping.RequestBindings,
			ResponseBindings:   mapping.ResponseBindings,
			Normalizers:        mapping.Normalizers,
			MappingLifecycle:   mapping.MappingLifecycle,
			RequiredOperations: mapping.RequiredOperations,
		}
		selectedDigest = selectedSourceDigest(mapping, sources)
	}
	return DesiredHash(DesiredHashInput{
		Address:         obj.Address,
		Type:            obj.Type,
		Provider:        obj.Provider,
		Attributes:      attrs,
		Lifecycle:       lifecycle.Hash,
		Mapping:         mappingHash,
		APISourceDigest: selectedDigest,
	})
}

func desiredProjectHash(resource project.Resource, lifecycle lifecyclePlan, mapping *MappingPlan, sources []sourceDoc, inputsDigest string) string {
	attrs := projectHashAttributes(resource, mapping)
	if lifecycle.IgnoreAll {
		attrs = map[string]string{}
	} else {
		for _, path := range lifecycle.IgnorePaths {
			delete(attrs, path)
		}
	}
	var mappingHash *MappingHashInput
	selectedDigest := ""
	if mapping != nil {
		mappingHash = &MappingHashInput{
			MetadataVersion:    MappingMetadataVersion,
			Purpose:            mapping.Purpose,
			SourceKind:         mapping.SourceKind,
			SourceID:           mapping.SourceID,
			OperationID:        mapping.OperationID,
			IdentityAttributes: mapping.IdentityAttributes,
			Schema:             mapping.Schema,
			RequestBindings:    mapping.RequestBindings,
			ResponseBindings:   mapping.ResponseBindings,
			Normalizers:        mapping.Normalizers,
			MappingLifecycle:   mapping.MappingLifecycle,
			RequiredOperations: mapping.RequiredOperations,
		}
		selectedDigest = selectedSourceDigest(mapping, sources)
	}
	return DesiredHash(DesiredHashInput{
		Address:         resource.Address,
		Type:            resource.Type,
		Provider:        resource.Provider,
		Attributes:      attrs,
		Lifecycle:       lifecycle.Hash,
		Mapping:         mappingHash,
		APISourceDigest: selectedDigest,
		InputsDigest:    inputsDigest,
	})
}

func projectHashAttributes(resource project.Resource, mapping *MappingPlan) map[string]string {
	attrs := project.AttributeStrings(resource.Attributes)
	for path := range projectHashExcludedPaths(mapping) {
		delete(attrs, path)
	}
	return attrs
}

func projectHashExcludedPaths(mapping *MappingPlan) map[string]bool {
	paths := map[string]bool{}
	if mapping == nil {
		return paths
	}
	for _, schema := range mapping.Schema {
		if schema.Computed || schema.ReadOnly || schema.ResponseDerivedIdentity || schema.Ignored {
			if path := strings.TrimSpace(schema.Path); path != "" {
				paths[path] = true
			}
		}
	}
	if mapping.MappingLifecycle != nil {
		for _, path := range mapping.MappingLifecycle.Paths {
			if path.Computed || path.Ignored {
				if name := strings.TrimSpace(path.Path); name != "" {
					paths[name] = true
				}
			}
		}
	}
	for _, binding := range mapping.ResponseBindings {
		if binding.ResponseDerivedIdentity {
			if path := strings.TrimSpace(binding.StatePath); path != "" {
				paths[path] = true
			}
		}
	}
	return paths
}

func projectReplacementChanged(resource project.Resource, current *state.ResourceSnapshot, mapping *MappingPlan) bool {
	for path := range projectReplacementPaths(mapping) {
		desired, ok := dottedProjectAttribute(resource.Attributes, path)
		if !ok {
			continue
		}
		currentValue, ok := snapshotPathValue(current, path, mapping)
		if !ok || !reflect.DeepEqual(jsonComparable(desired), jsonComparable(currentValue)) {
			return true
		}
	}
	return false
}

func projectReplacementPaths(mapping *MappingPlan) map[string]bool {
	paths := map[string]bool{}
	if mapping == nil {
		return paths
	}
	for _, schema := range mapping.Schema {
		if schema.ReplaceOnChange || schema.Immutable || schema.CreateOnly {
			if path := strings.TrimSpace(schema.Path); path != "" {
				paths[path] = true
			}
		}
	}
	if mapping.MappingLifecycle != nil {
		for _, path := range mapping.MappingLifecycle.Paths {
			if path.ReplaceOnChange || path.Immutable || path.CreateOnly {
				if name := strings.TrimSpace(path.Path); name != "" {
					paths[name] = true
				}
			}
		}
	}
	return paths
}

func dottedProjectAttribute(attrs map[string]any, path string) (any, bool) {
	return dottedMapValue(attrs, path)
}

func snapshotPathValue(current *state.ResourceSnapshot, path string, mapping *MappingPlan) (any, bool) {
	if current == nil {
		return nil, false
	}
	paths := []string{path}
	if mapping != nil {
		for _, attr := range mapping.IdentityAttributes {
			if attr.TerraformPath == path || attr.Name == path {
				paths = append(paths, attr.Name, attr.TerraformPath)
				paths = append(paths, attr.ResponsePaths...)
			}
		}
	}
	for _, text := range []string{current.IdentityJSON, current.AttributesJSON} {
		if strings.TrimSpace(text) == "" {
			continue
		}
		var values map[string]any
		if err := json.Unmarshal([]byte(text), &values); err != nil {
			continue
		}
		for _, candidate := range paths {
			if value, ok := dottedMapValue(values, candidate); ok {
				return value, true
			}
		}
	}
	return nil, false
}

func dottedMapValue(values map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" || values == nil {
		return nil, false
	}
	if value, ok := values[path]; ok {
		return value, true
	}
	var cur any = values
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func jsonComparable(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return value
	}
	return out
}

func DesiredHash(input DesiredHashInput) string {
	payload := map[string]any{
		"address":           input.Address,
		"type":              input.Type,
		"provider":          input.Provider,
		"attrs":             input.Attributes,
		"lifecycle":         input.Lifecycle,
		"mapping":           input.Mapping,
		"api_source_digest": input.APISourceDigest,
		"inputs_digest":     input.InputsDigest,
	}
	data, _ := json.Marshal(payload)
	return digest.SHA256String(data)
}

func selectedSourceDigest(mapping *MappingPlan, sources []sourceDoc) string {
	if mapping == nil {
		return ""
	}
	for _, source := range sources {
		if source.Kind == mapping.SourceKind && source.ID == mapping.SourceID {
			return source.Digest
		}
	}
	for _, source := range sources {
		if source.Kind == mapping.SourceKind && source.Path == mapping.SourcePath {
			return source.Digest
		}
	}
	return ""
}

func analyzeLifecycle(obj objectFact) lifecyclePlan {
	if obj.Lifecycle == nil {
		return lifecyclePlan{Hash: map[string]any{}}
	}
	plan := lifecyclePlan{Hash: map[string]any{}}
	for _, diag := range obj.Lifecycle.Diagnostics {
		plan.Diagnostics = append(plan.Diagnostics, Diagnostic{
			Code:          firstNonEmpty(diag.Code, "plan.lifecycle_diagnostic"),
			Severity:      normalizeSeverity(string(diag.Severity)),
			Message:       diagnosticMessage(diag),
			Address:       firstNonEmpty(diag.Address, obj.Address),
			ModuleAddress: firstNonEmpty(diag.ModuleAddress, obj.ModuleAddress),
		})
	}
	if obj.Lifecycle.CreateBeforeDestroy != nil {
		value, ok := boolValue(*obj.Lifecycle.CreateBeforeDestroy)
		if !ok {
			plan.Diagnostics = append(plan.Diagnostics, Diagnostic{
				Code:          "plan.lifecycle_unsupported",
				Severity:      "error",
				Message:       "create_before_destroy must be a static boolean value",
				Address:       obj.Address,
				ModuleAddress: obj.ModuleAddress,
			})
		}
		plan.Hash["create_before_destroy"] = value
		if ok && value {
			plan.Diagnostics = append(plan.Diagnostics, Diagnostic{
				Code:          "plan.create_before_destroy_unsupported",
				Severity:      "error",
				Message:       "create_before_destroy is not supported by static Ramen planning yet",
				Address:       obj.Address,
				ModuleAddress: obj.ModuleAddress,
			})
		}
	}
	if obj.Lifecycle.PreventDestroy != nil {
		value, ok := boolValue(*obj.Lifecycle.PreventDestroy)
		if !ok {
			plan.Diagnostics = append(plan.Diagnostics, Diagnostic{
				Code:          "plan.lifecycle_unsupported",
				Severity:      "error",
				Message:       "prevent_destroy must be a static boolean value",
				Address:       obj.Address,
				ModuleAddress: obj.ModuleAddress,
			})
		}
		plan.PreventDestroy = value
		plan.Hash["prevent_destroy"] = value
	}
	for _, value := range obj.Lifecycle.IgnoreChanges {
		paths, all, ok := ignoreChangesValue(value)
		if !ok {
			plan.Diagnostics = append(plan.Diagnostics, Diagnostic{
				Code:          "plan.ignore_changes_unsupported",
				Severity:      "error",
				Message:       "ignore_changes must be all or a static list of attribute paths",
				Address:       obj.Address,
				ModuleAddress: obj.ModuleAddress,
			})
			continue
		}
		if all {
			plan.IgnoreAll = true
		}
		plan.IgnorePaths = append(plan.IgnorePaths, paths...)
	}
	if len(obj.Lifecycle.ReplaceTriggeredBy) > 0 {
		plan.Diagnostics = append(plan.Diagnostics, Diagnostic{
			Code:          "plan.replace_triggered_by_unsupported",
			Severity:      "error",
			Message:       "replace_triggered_by is not supported by static Ramen planning yet",
			Address:       obj.Address,
			ModuleAddress: obj.ModuleAddress,
		})
	}
	if len(obj.Lifecycle.Preconditions) > 0 {
		plan.Diagnostics = append(plan.Diagnostics, Diagnostic{
			Code:          "plan.precondition_unsupported",
			Severity:      "error",
			Message:       "lifecycle precondition blocks are not supported by static Ramen planning yet",
			Address:       obj.Address,
			ModuleAddress: obj.ModuleAddress,
		})
	}
	if len(obj.Lifecycle.Postconditions) > 0 {
		plan.Diagnostics = append(plan.Diagnostics, Diagnostic{
			Code:          "plan.postcondition_unsupported",
			Severity:      "error",
			Message:       "lifecycle postcondition blocks are not supported by static Ramen planning yet",
			Address:       obj.Address,
			ModuleAddress: obj.ModuleAddress,
		})
	}
	slices.Sort(plan.IgnorePaths)
	plan.IgnorePaths = slices.Compact(plan.IgnorePaths)
	plan.Hash["ignore_all"] = plan.IgnoreAll
	plan.Hash["ignore_paths"] = slices.Clone(plan.IgnorePaths)
	return plan
}

func analyzeProjectLifecycle(resource project.Resource) lifecyclePlan {
	plan := lifecyclePlan{Hash: map[string]any{}}
	plan.PreventDestroy = resource.Lifecycle.PreventDestroy
	plan.IgnoreAll = resource.Lifecycle.IgnoreAll
	plan.IgnorePaths = slices.Clone(resource.Lifecycle.IgnorePaths)
	slices.Sort(plan.IgnorePaths)
	plan.IgnorePaths = slices.Compact(plan.IgnorePaths)
	if len(resource.Lifecycle.ReplaceTriggeredBy) > 0 {
		values := slices.Clone(resource.Lifecycle.ReplaceTriggeredBy)
		slices.Sort(values)
		plan.Hash["replace_triggered_by"] = values
	}
	plan.Hash["prevent_destroy"] = plan.PreventDestroy
	plan.Hash["ignore_all"] = plan.IgnoreAll
	plan.Hash["ignore_paths"] = slices.Clone(plan.IgnorePaths)
	return plan
}

func boolValue(value tfconfig.Value) (bool, bool) {
	if value.Literal != nil {
		if b, ok := value.Literal.(bool); ok {
			return b, true
		}
	}
	switch strings.ToLower(strings.TrimSpace(value.Expression)) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func ignoreChangesValue(value tfconfig.Value) ([]string, bool, bool) {
	expr := strings.TrimSpace(value.Expression)
	if strings.EqualFold(expr, "all") {
		return nil, true, true
	}
	if len(value.References) > 0 && !strings.HasPrefix(expr, "[") {
		return nil, false, false
	}
	if literal, ok := value.Literal.(string); ok && strings.EqualFold(strings.TrimSpace(literal), "all") {
		return nil, true, true
	}
	var paths []string
	switch literal := value.Literal.(type) {
	case []any:
		for _, item := range literal {
			path, ok := item.(string)
			if !ok || strings.TrimSpace(path) == "" {
				return nil, false, false
			}
			paths = append(paths, strings.TrimSpace(path))
		}
	case []string:
		for _, path := range literal {
			if strings.TrimSpace(path) == "" {
				return nil, false, false
			}
			paths = append(paths, strings.TrimSpace(path))
		}
	default:
		if strings.HasPrefix(expr, "[") && strings.HasSuffix(expr, "]") {
			inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(expr, "["), "]"))
			if inner == "" {
				return nil, false, true
			}
			for _, part := range strings.Split(inner, ",") {
				path := strings.Trim(strings.TrimSpace(part), `"`)
				if path == "" || strings.ContainsAny(path, "${}") {
					return nil, false, false
				}
				paths = append(paths, path)
			}
		} else if expr != "" {
			path := strings.Trim(expr, `"`)
			if path == "" || strings.ContainsAny(path, "${}[]") {
				return nil, false, false
			}
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths), false, true
}

func collectResources(doc tfconfig.Document) []objectFact {
	var out []objectFact
	for _, mod := range doc.Modules {
		for _, res := range mod.Resources {
			out = append(out, objectFact{
				Address:       fullAddress(mod.Address, res.Address),
				ModuleAddress: mod.Address,
				Kind:          "resource",
				Type:          res.Type,
				Name:          res.Name,
				Provider:      providerAddress(res.Provider),
				Config:        res.Config,
				Lifecycle:     res.Lifecycle,
				DependsOn:     res.DependsOn,
				References:    res.References,
			})
		}
	}
	slices.SortFunc(out, func(a, b objectFact) int { return cmp.Compare(a.Address, b.Address) })
	return out
}

func graphNodes(objects []objectFact) []graph.Node {
	known := make([]string, 0, len(objects))
	for _, obj := range objects {
		known = append(known, obj.Address)
	}
	slices.SortFunc(known, func(a, b string) int {
		if diff := cmp.Compare(len(b), len(a)); diff != 0 {
			return diff
		}
		return cmp.Compare(a, b)
	})
	var nodes []graph.Node
	for _, obj := range objects {
		nodes = append(nodes, graph.Node{Address: obj.Address, DependsOn: dependenciesFor(obj, known)})
	}
	return nodes
}

func dependenciesFor(obj objectFact, known []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(ref tfconfig.Reference) {
		for _, candidate := range known {
			if candidate == obj.Address {
				continue
			}
			if ref.Traversal == candidate || ref.Subject == candidate || strings.HasPrefix(ref.Traversal, candidate+".") {
				if !seen[candidate] {
					seen[candidate] = true
					out = append(out, candidate)
				}
				return
			}
		}
	}
	for _, ref := range obj.DependsOn {
		add(ref)
	}
	for _, ref := range obj.References {
		add(ref)
	}
	slices.Sort(out)
	return out
}

func projectAPISourceInputs(profile project.Profile, overrides []APISourceInput) []APISourceInput {
	inputs := make([]APISourceInput, 0, len(profile.APISources))
	index := map[string]int{}
	for _, source := range profile.APISources {
		key := source.Kind + "\x00" + source.ID
		index[key] = len(inputs)
		inputs = append(inputs, APISourceInput{Kind: source.Kind, ID: source.ID, Path: source.Path})
	}
	for _, override := range overrides {
		key := normalizeAPISourceKind(override.Kind) + "\x00" + strings.TrimSpace(override.ID)
		if pos, ok := index[key]; ok {
			inputs[pos] = override
			continue
		}
		index[key] = len(inputs)
		inputs = append(inputs, override)
	}
	return inputs
}

func valuePlanDiagnostics(diags []project.ValueDiagnostic) []Diagnostic {
	out := make([]Diagnostic, 0, len(diags))
	for _, diag := range diags {
		out = append(out, Diagnostic{
			Code:     diag.Code,
			Severity: normalizeSeverity(diag.Severity),
			Message:  diag.Message,
			Address:  diag.Path,
		})
	}
	return out
}

func evaluateGovernance(opts Options, resources []ResourcePlan, inputsDigest string) (governance.Result, []Diagnostic) {
	engine, loadDecisions := governance.LoadPolicyFiles(opts.PolicyFiles)
	result := governance.MergeResults(governance.Result{Version: governance.ResultVersion, Decisions: loadDecisions}, engine.Evaluate(governanceInput(opts, resources, inputsDigest)))
	return result, governancePlanDiagnostics(result.Decisions)
}

func governanceInput(opts Options, resources []ResourcePlan, inputsDigest string) governance.Input {
	input := governance.Input{Action: opts.Action, InputsDigest: inputsDigest, Resources: make([]governance.Resource, 0, len(resources))}
	for _, resource := range resources {
		input.Resources = append(input.Resources, governance.Resource{
			Address:  resource.Address,
			Type:     resource.Type,
			Provider: resource.Provider,
			Action:   resource.Action,
		})
	}
	return input
}

func governancePlanDiagnostics(decisions []governance.Decision) []Diagnostic {
	out := make([]Diagnostic, 0, len(decisions))
	for _, decision := range decisions {
		if decision.Severity != "error" && decision.Severity != "warning" {
			continue
		}
		out = append(out, Diagnostic{
			Code:     decision.Code,
			Severity: normalizeSeverity(decision.Severity),
			Message:  decision.Message,
			Address:  decision.Address,
		})
	}
	return out
}

func normalizeApprovers(approvers []governance.Approver) ([]governance.Approver, []Diagnostic) {
	out := make([]governance.Approver, 0, len(approvers))
	var diagnostics []Diagnostic
	for _, approver := range approvers {
		normalized, err := governance.NormalizeApprover(approver)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Code: "approval.identity_invalid", Severity: "error", Message: err.Error()})
			continue
		}
		out = append(out, normalized)
	}
	slices.SortFunc(out, func(a, b governance.Approver) int {
		return strings.Compare(a.Identity+a.Role+a.ApprovedAt.Format(time.RFC3339Nano), b.Identity+b.Role+b.ApprovedAt.Format(time.RFC3339Nano))
	})
	return out, diagnostics
}

func loadAPISources(ctx context.Context, inputs []APISourceInput) ([]sourceDoc, []Diagnostic) {
	var docs []sourceDoc
	var diagnostics []Diagnostic
	seenIDs := map[string]bool{}
	for _, input := range inputs {
		input.Kind = normalizeAPISourceKind(input.Kind)
		input.ID = strings.TrimSpace(input.ID)
		input.Path = strings.TrimSpace(input.Path)
		switch {
		case input.Kind == "":
			diagnostics = append(diagnostics, Diagnostic{Code: "api_source.invalid", Severity: "error", Message: "--api-source kind is required and must be openapi, aws-smithy, or google-discovery"})
			continue
		case input.ID == "":
			diagnostics = append(diagnostics, Diagnostic{Code: "api_source.invalid", Severity: "error", Message: "--api-source ID is required", APISourceKind: input.Kind})
			continue
		case input.Path == "":
			diagnostics = append(diagnostics, Diagnostic{Code: "api_source.invalid", Severity: "error", Message: "--api-source path is required", APISourceKind: input.Kind, APISourceID: input.ID})
			continue
		case seenIDs[input.ID]:
			diagnostics = append(diagnostics, Diagnostic{Code: "api_source.duplicate_id", Severity: "error", Message: fmt.Sprintf("API source ID %q is duplicated", input.ID), APISourceKind: input.Kind, APISourceID: input.ID})
			continue
		}
		seenIDs[input.ID] = true
		digest, digestErr := fileDigest(input.Path)
		if digestErr != nil {
			diagnostics = append(diagnostics, Diagnostic{Code: "api_source.load_error", Severity: "error", Message: digestErr.Error(), APISourceKind: input.Kind, APISourceID: input.ID})
			continue
		}
		inventory, err := apitools.BuildAPISourceOperationInventory(ctx, apitools.APISourceInventoryOptions{
			Documents: []apitools.APISourceDocument{{
				Kind:         input.Kind,
				Name:         input.ID,
				Path:         input.Path,
				RelativePath: packageAPISourcePath(input.Kind, input.ID, input.Path),
			}},
		})
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Code: "api_source.load_error", Severity: "error", Message: err.Error(), APISourceKind: input.Kind, APISourceID: input.ID})
			continue
		}
		for _, diag := range inventory.Diagnostics {
			diagnostics = append(diagnostics, Diagnostic{Code: "api_source." + strings.ReplaceAll(diag.Code, ".", "_"), Severity: normalizeSeverity(diag.Severity), Message: diag.Message, APISourceKind: input.Kind, APISourceID: input.ID})
		}
		ops := slices.Clone(inventory.Operations)
		slices.SortFunc(ops, func(a, b apitools.OperationSummary) int { return cmp.Compare(a.OperationID, b.OperationID) })
		docs = append(docs, sourceDoc{ID: input.ID, Kind: input.Kind, Path: input.Path, Digest: digest, Operations: ops})
	}
	slices.SortFunc(docs, func(a, b sourceDoc) int {
		if diff := cmp.Compare(a.Kind, b.Kind); diff != 0 {
			return diff
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return docs, diagnostics
}

func normalizeOptions(opts Options) Options {
	if strings.TrimSpace(opts.ConfigDir) == "" {
		opts.ConfigDir = "."
	}
	if strings.TrimSpace(opts.StatePath) == "" {
		if path, err := state.WorkspacePath(stateBaseDir(opts.ProjectPath, opts.ConfigDir), opts.Workspace); err == nil {
			opts.StatePath = path
		} else {
			opts.StatePath = state.DefaultPath(stateBaseDir(opts.ProjectPath, opts.ConfigDir))
		}
	}
	opts.ProjectPath = strings.TrimSpace(opts.ProjectPath)
	opts.Workspace = strings.TrimSpace(opts.Workspace)
	opts.Action = strings.ToLower(strings.TrimSpace(opts.Action))
	if opts.Destroy {
		opts.Action = "delete"
	}
	if opts.Action == "" {
		opts.Action = "create"
	}
	opts.Targets = uniqueNonEmpty(opts.Targets)
	opts.Excludes = uniqueNonEmpty(opts.Excludes)
	opts.Replaces = uniqueNonEmpty(opts.Replaces)
	for i := range opts.APISources {
		opts.APISources[i].Kind = normalizeAPISourceKind(opts.APISources[i].Kind)
		opts.APISources[i].ID = strings.TrimSpace(opts.APISources[i].ID)
		opts.APISources[i].Path = strings.TrimSpace(opts.APISources[i].Path)
	}
	slices.SortFunc(opts.APISources, func(a, b APISourceInput) int {
		left := a.Kind + "\x00" + a.ID + "\x00" + a.Path
		right := b.Kind + "\x00" + b.ID + "\x00" + b.Path
		return cmp.Compare(left, right)
	})
	return opts
}

func controlsFromOptions(opts Options) Controls {
	return Controls{
		Targets:  slices.Clone(opts.Targets),
		Excludes: slices.Clone(opts.Excludes),
		Replaces: slices.Clone(opts.Replaces),
		Destroy:  opts.Destroy || opts.Action == "delete",
	}
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	slices.Sort(out)
	return out
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
	if projectPath != "" {
		return filepath.Dir(projectPath)
	}
	return configDir
}

func writeJSON(path string, value any) error {
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

func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return digest.SHA256String(data), nil
}

func apiSourceRefs(sources []sourceDoc) []APISourceRef {
	refs := make([]APISourceRef, 0, len(sources))
	for _, source := range sources {
		refs = append(refs, APISourceRef{Kind: source.Kind, ID: source.ID, Path: source.Path, Digest: source.Digest})
	}
	slices.SortFunc(refs, func(a, b APISourceRef) int {
		left := a.Kind + "\x00" + a.ID + "\x00" + a.Path
		right := b.Kind + "\x00" + b.ID + "\x00" + b.Path
		return cmp.Compare(left, right)
	})
	return refs
}

func projectDigest(profile project.Profile) string {
	data, err := json.Marshal(profile)
	if err != nil {
		return ""
	}
	return digest.SHA256String(data)
}

func stateBaselineDigest(ctx context.Context, store *state.Store) string {
	if store == nil {
		return ""
	}
	current, err := store.ListCurrentResources(ctx)
	if err != nil {
		return ""
	}
	type row struct {
		Address        string `json:"address"`
		Type           string `json:"type,omitempty"`
		Provider       string `json:"provider,omitempty"`
		DesiredHash    string `json:"desired_hash,omitempty"`
		IdentityJSON   string `json:"identity_json,omitempty"`
		AttributesJSON string `json:"attributes_json,omitempty"`
		Status         string `json:"status,omitempty"`
		SourceKind     string `json:"source_kind,omitempty"`
		SourceID       string `json:"source_id,omitempty"`
		OperationID    string `json:"operation_id,omitempty"`
	}
	rows := make([]row, 0, len(current))
	for _, snap := range current {
		rows = append(rows, row{Address: snap.Address, Type: snap.Type, Provider: snap.Provider, DesiredHash: snap.DesiredHash, IdentityJSON: snap.IdentityJSON, AttributesJSON: snap.AttributesJSON, Status: snap.Status, SourceKind: snap.SourceKind, SourceID: snap.SourceID, OperationID: snap.OperationID})
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return ""
	}
	return digest.SHA256String(data)
}

func projectRationale(profile project.Profile) string {
	if profile.Metadata == nil {
		return ""
	}
	for _, key := range []string{"rationale", "plan_rationale"} {
		if value, ok := profile.Metadata[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildApproval(doc Document, projectDigest, stateDigest string, approvers []governance.Approver) *Approval {
	approval := &Approval{
		Version:       "ramen.approval.v1",
		Rationale:     doc.Rationale,
		ProjectDigest: projectDigest,
		StateDigest:   stateDigest,
		Controls:      doc.Controls,
		APISources:    slices.Clone(doc.APISources),
		Approvers:     slices.Clone(approvers),
	}
	approval.Digest = approvalDigest(doc, approval)
	return approval
}

func approvalDigest(doc Document, approval *Approval) string {
	payload := struct {
		Version       string                 `json:"version"`
		PlanVersion   string                 `json:"plan_version"`
		ConfigDir     string                 `json:"config_dir"`
		ProjectPath   string                 `json:"project_path,omitempty"`
		StatePath     string                 `json:"state_path"`
		Workspace     string                 `json:"workspace,omitempty"`
		Action        string                 `json:"action"`
		Inputs        project.ResolvedInputs `json:"inputs,omitempty"`
		Governance    governance.Result      `json:"governance,omitempty"`
		Errored       bool                   `json:"errored,omitempty"`
		Rationale     string                 `json:"rationale,omitempty"`
		Controls      Controls               `json:"controls,omitempty"`
		APISources    []APISourceRef         `json:"api_sources,omitempty"`
		Approvers     []governance.Approver  `json:"approvers,omitempty"`
		ProjectDigest string                 `json:"project_digest,omitempty"`
		StateDigest   string                 `json:"state_digest,omitempty"`
		Resources     []ResourcePlan         `json:"resources,omitempty"`
		Diagnostics   []Diagnostic           `json:"diagnostics,omitempty"`
	}{
		Version:       approval.Version,
		PlanVersion:   doc.Version,
		ConfigDir:     doc.ConfigDir,
		ProjectPath:   doc.ProjectPath,
		StatePath:     doc.StatePath,
		Workspace:     doc.Workspace,
		Action:        doc.Action,
		Inputs:        doc.Inputs,
		Governance:    doc.Governance,
		Errored:       doc.Errored,
		Rationale:     doc.Rationale,
		Controls:      approval.Controls,
		APISources:    approval.APISources,
		Approvers:     approval.Approvers,
		ProjectDigest: approval.ProjectDigest,
		StateDigest:   approval.StateDigest,
		Resources:     doc.Resources,
		Diagnostics:   doc.Diagnostics,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return digest.SHA256String(data)
}

func VerifyApproval(doc Document) error {
	if doc.Errored {
		return fmt.Errorf("plan is marked errored")
	}
	if doc.Approval == nil || strings.TrimSpace(doc.Approval.Digest) == "" {
		return fmt.Errorf("plan approval artifact is missing")
	}
	if doc.Approval.Rationale != doc.Rationale {
		return fmt.Errorf("plan approval rationale mismatch")
	}
	if err := governance.RequirementsSatisfied(doc.Governance.ApprovalRequirements, doc.Approval.Approvers); err != nil {
		return fmt.Errorf("plan approval requirement unsatisfied: %w", err)
	}
	if got := approvalDigest(doc, doc.Approval); got != doc.Approval.Digest {
		return fmt.Errorf("plan approval digest mismatch")
	}
	return nil
}

func summarize(doc Document) Summary {
	summary := Summary{Diagnostics: len(doc.Diagnostics)}
	for _, resource := range doc.Resources {
		switch resource.Action {
		case "create":
			summary.Create++
		case "update":
			summary.Update++
		case "delete":
			summary.Delete++
		case "replace":
			summary.Replace++
		case "no-op":
			summary.NoOp++
		case "read":
			summary.Read++
		}
	}
	return summary
}

func tfDiagnostics(diags []tfconfig.Diagnostic) []Diagnostic {
	out := make([]Diagnostic, 0, len(diags))
	for _, diag := range diags {
		out = append(out, Diagnostic{
			Code:          firstNonEmpty(diag.Code, "tfconfig.diagnostic"),
			Severity:      normalizeSeverity(string(diag.Severity)),
			Message:       diagnosticMessage(diag),
			Address:       diag.Address,
			ModuleAddress: diag.ModuleAddress,
		})
	}
	return out
}

func staticBlockDiagnostics(doc tfconfig.Document) []Diagnostic {
	var diagnostics []Diagnostic
	for _, mod := range doc.Modules {
		for _, moved := range mod.Moved {
			diagnostics = append(diagnostics, Diagnostic{
				Code:          "plan.moved_unsupported",
				Severity:      "error",
				Message:       fmt.Sprintf("moved block from %s to %s is not supported by static Ramen planning yet", moved.From, moved.To),
				Address:       firstNonEmpty(moved.To, moved.From),
				ModuleAddress: mod.Address,
			})
		}
		for _, imp := range mod.Imports {
			diagnostics = append(diagnostics, Diagnostic{
				Code:          "plan.import_unsupported",
				Severity:      "error",
				Message:       fmt.Sprintf("import block for %s is not supported by static Ramen planning yet", imp.To),
				Address:       imp.To,
				ModuleAddress: mod.Address,
			})
		}
		for _, removed := range mod.Removed {
			diagnostics = append(diagnostics, Diagnostic{
				Code:          "plan.removed_unsupported",
				Severity:      "error",
				Message:       fmt.Sprintf("removed block for %s is not supported by static Ramen planning yet", removed.From),
				Address:       removed.From,
				ModuleAddress: mod.Address,
			})
		}
	}
	return diagnostics
}

func hasErrorDiagnostics(diagnostics []Diagnostic) bool {
	for _, diag := range diagnostics {
		if diag.Severity == "error" {
			return true
		}
	}
	return false
}

func sortDiagnostics(diags []Diagnostic) {
	slices.SortFunc(diags, func(a, b Diagnostic) int {
		left := a.Code + "\x00" + a.Address + "\x00" + a.Message
		right := b.Code + "\x00" + b.Address + "\x00" + b.Message
		return cmp.Compare(left, right)
	})
}

func valueText(value tfconfig.Value) string {
	if value.Sensitive || value.Redacted || value.SensitiveCandidate != nil {
		return "${sensitive}"
	}
	if value.Expression != "" {
		return value.Expression
	}
	if value.UnknownReason != "" {
		return "${unknown:" + value.UnknownReason + "}"
	}
	if value.Literal != nil {
		data, err := json.Marshal(value.Literal)
		if err == nil {
			return string(data)
		}
	}
	return string(value.Kind)
}

func providerAddress(ref *tfconfig.ProviderRef) string {
	if ref == nil {
		return ""
	}
	return ref.Address
}

func fullAddress(moduleAddress, objectAddress string) string {
	moduleAddress = strings.TrimSpace(moduleAddress)
	objectAddress = strings.TrimSpace(objectAddress)
	if moduleAddress == "" {
		return objectAddress
	}
	if objectAddress == "" {
		return moduleAddress
	}
	return moduleAddress + "." + objectAddress
}

func normalizeAPISourceKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case apitools.APISourceKindOpenAPI, "swagger":
		return apitools.APISourceKindOpenAPI
	case apitools.APISourceKindAWSSmithy, "smithy", "smithy-json":
		return apitools.APISourceKindAWSSmithy
	case apitools.APISourceKindGoogleDiscovery, "discovery", "google":
		return apitools.APISourceKindGoogleDiscovery
	default:
		return ""
	}
}

func normalizeSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "error", "warning", "info":
		return strings.ToLower(strings.TrimSpace(severity))
	case "warn":
		return "warning"
	default:
		return "warning"
	}
}

func diagnosticMessage(diag tfconfig.Diagnostic) string {
	if diag.Detail == "" {
		return diag.Summary
	}
	return diag.Summary + ": " + diag.Detail
}

func packageAPISourcePath(kind, id, sourcePath string) string {
	ext := strings.ToLower(filepath.Ext(sourcePath))
	switch ext {
	case ".json", ".yaml", ".yml":
	default:
		if kind == apitools.APISourceKindOpenAPI {
			ext = ".yaml"
		} else {
			ext = ".json"
		}
	}
	return filepath.ToSlash(filepath.Join(kind, normalizeName(id)+ext))
}

func normalizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func sourceIDMatches(sourceID string, expected []string) bool {
	sourceID = normalizeName(sourceID)
	for _, candidate := range expected {
		if sourceID == normalizeName(candidate) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
