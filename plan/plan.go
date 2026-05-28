package plan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OpenUdon/apitools"
	"github.com/OpenUdon/ramen/graph"
	"github.com/OpenUdon/ramen/state"
	"github.com/OpenUdon/ramen/tfmapping"
	"github.com/OpenUdon/tfconfig"
)

const Version = "ramen.plan.v1"

type Options struct {
	ConfigDir  string
	StatePath  string
	APISources []APISourceInput
	Action     string
	OutPath    string
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
	Version     string         `json:"version"`
	ConfigDir   string         `json:"config_dir"`
	StatePath   string         `json:"state_path"`
	Action      string         `json:"action"`
	Summary     Summary        `json:"summary"`
	Resources   []ResourcePlan `json:"resources"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
}

type Summary struct {
	Create      int `json:"create"`
	Update      int `json:"update"`
	Delete      int `json:"delete"`
	NoOp        int `json:"no_op"`
	Read        int `json:"read"`
	Diagnostics int `json:"diagnostics"`
}

type ResourcePlan struct {
	Address      string       `json:"address"`
	Kind         string       `json:"kind"`
	Type         string       `json:"type"`
	Name         string       `json:"name"`
	Provider     string       `json:"provider,omitempty"`
	Action       string       `json:"action"`
	Reason       string       `json:"reason"`
	DesiredHash  string       `json:"desired_hash,omitempty"`
	Dependencies []string     `json:"dependencies,omitempty"`
	Mapping      *MappingPlan `json:"mapping,omitempty"`
}

type MappingPlan struct {
	Purpose            string                        `json:"purpose"`
	SourceKind         string                        `json:"source_kind,omitempty"`
	SourceID           string                        `json:"source_id,omitempty"`
	OperationID        string                        `json:"operation_id,omitempty"`
	IdentityAttributes []tfmapping.IdentityAttribute `json:"identity_attributes,omitempty"`
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
	DependsOn     []tfconfig.Reference
	References    []tfconfig.Reference
}

func Build(ctx context.Context, opts Options) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opts = normalizeOptions(opts)
	doc, loadErr := tfconfig.LoadDir(opts.ConfigDir)
	var diagnostics []Diagnostic
	if loadErr != nil {
		diagnostics = append(diagnostics, Diagnostic{Code: "tfconfig.load_error", Severity: "error", Message: loadErr.Error()})
	}
	diagnostics = append(diagnostics, tfDiagnostics(doc.Diagnostics)...)
	for _, mod := range doc.Modules {
		diagnostics = append(diagnostics, tfDiagnostics(mod.Diagnostics)...)
	}

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
	objectsByAddress := map[string]objectFact{}
	for _, obj := range objects {
		objectsByAddress[obj.Address] = obj
	}

	var resources []ResourcePlan
	desiredAddresses := map[string]bool{}
	for _, node := range sortedNodes {
		obj, ok := objectsByAddress[node.Address]
		if !ok || obj.Kind != "resource" {
			continue
		}
		desiredAddresses[obj.Address] = true
		resourcePlan := planResource(ctx, store, obj, node.DependsOn, opts.Action, apiSources)
		diagnostics = append(diagnostics, resourcePlan.diagnostics...)
		resources = append(resources, resourcePlan.resource)
	}
	deletePlans, deleteDiagnostics := planDeletes(ctx, store, desiredAddresses, apiSources)
	diagnostics = append(diagnostics, deleteDiagnostics...)
	resources = append(resources, deletePlans...)
	sortDiagnostics(diagnostics)
	document := Document{
		Version:     Version,
		ConfigDir:   opts.ConfigDir,
		StatePath:   opts.StatePath,
		Action:      opts.Action,
		Resources:   resources,
		Diagnostics: diagnostics,
	}
	document.Summary = summarize(document)
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
		mapping, mappingDiagnostics := mapResource(obj, "delete", "delete", sources)
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

type plannedResource struct {
	resource    ResourcePlan
	diagnostics []Diagnostic
}

func planResource(ctx context.Context, store *state.Store, obj objectFact, dependencies []string, requestedAction string, sources []sourceDoc) plannedResource {
	hash := desiredHash(obj)
	action := "create"
	reason := "resource is not recorded in state"
	if store != nil {
		current, err := store.CurrentResource(ctx, obj.Address)
		if err != nil {
			return plannedResource{
				resource:    ResourcePlan{Address: obj.Address, Kind: obj.Kind, Type: obj.Type, Name: obj.Name, Provider: obj.Provider, Action: "error", Reason: err.Error(), DesiredHash: hash, Dependencies: dependencies},
				diagnostics: []Diagnostic{{Code: "state.read_error", Severity: "error", Message: err.Error(), Address: obj.Address, ModuleAddress: obj.ModuleAddress}},
			}
		}
		if current != nil && current.DesiredHash == hash {
			action = "no-op"
			reason = "recorded desired hash matches configuration"
		} else if current != nil {
			action = "update"
			reason = "recorded desired hash differs from configuration"
		}
	}
	purpose := requestedAction
	if action == "update" {
		purpose = "update"
	}
	if action == "no-op" {
		purpose = "read"
	}
	mapping, diagnostics := mapResource(obj, purpose, action, sources)
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
			Dependencies: append([]string(nil), dependencies...),
			Mapping:      mapping,
		},
		diagnostics: diagnostics,
	}
}

func mapResource(obj objectFact, purpose, action string, sources []sourceDoc) (*MappingPlan, []Diagnostic) {
	spec := tfmapping.DefaultRegistry().MapObject(tfmapping.Object{Kind: obj.Kind, Type: obj.Type, Provider: obj.Provider}, purpose, action)
	var diagnostics []Diagnostic
	for _, diag := range spec.Diagnostics {
		diagnostics = append(diagnostics, Diagnostic{Code: string(diag.Code), Severity: string(diag.Severity), Message: diag.Message, Address: obj.Address, ModuleAddress: obj.ModuleAddress})
	}
	if len(spec.Target.OperationIDs) == 0 {
		return &MappingPlan{Purpose: purpose, IdentityAttributes: spec.IdentityAttributes}, diagnostics
	}
	if operation, source, ok := findOperation(sources, spec.Target); ok {
		return &MappingPlan{
			Purpose:            purpose,
			SourceKind:         source.Kind,
			SourceID:           firstNonEmpty(operation.DocumentName, source.ID),
			OperationID:        operation.OperationID,
			IdentityAttributes: spec.IdentityAttributes,
		}, diagnostics
	}
	diagnostics = append(diagnostics, Diagnostic{
		Code:          "mapping.operation_unavailable",
		Severity:      "warning",
		Message:       fmt.Sprintf("no loaded API source operation matched %s for %s", strings.Join(spec.Target.OperationIDs, ", "), obj.Address),
		Address:       obj.Address,
		ModuleAddress: obj.ModuleAddress,
	})
	return &MappingPlan{Purpose: purpose, IdentityAttributes: spec.IdentityAttributes}, diagnostics
}

func findOperation(sources []sourceDoc, target tfmapping.OperationTarget) (apitools.OperationSummary, sourceDoc, bool) {
	for _, kind := range target.SourceKinds {
		for _, operationID := range target.OperationIDs {
			for _, source := range sources {
				if source.Kind != kind || !sourceIDMatches(source.ID, target.SourceIDs) {
					continue
				}
				for _, op := range source.Operations {
					if op.OperationID == operationID {
						return op, source, true
					}
				}
			}
		}
	}
	for _, operationID := range target.OperationIDs {
		for _, source := range sources {
			for _, op := range source.Operations {
				if op.OperationID == operationID {
					return op, source, true
				}
			}
		}
	}
	return apitools.OperationSummary{}, sourceDoc{}, false
}

func desiredHash(obj objectFact) string {
	attrs := map[string]string{}
	for _, attr := range obj.Config {
		attrs[attr.Path] = valueText(attr.Value)
	}
	payload := map[string]any{
		"address":  obj.Address,
		"type":     obj.Type,
		"provider": obj.Provider,
		"attrs":    attrs,
	}
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
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
				DependsOn:     res.DependsOn,
				References:    res.References,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out
}

func graphNodes(objects []objectFact) []graph.Node {
	known := make([]string, 0, len(objects))
	for _, obj := range objects {
		known = append(known, obj.Address)
	}
	sort.Slice(known, func(i, j int) bool { return len(known[i]) > len(known[j]) })
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
	sort.Strings(out)
	return out
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
		ops := append([]apitools.OperationSummary(nil), inventory.Operations...)
		sort.Slice(ops, func(i, j int) bool { return ops[i].OperationID < ops[j].OperationID })
		docs = append(docs, sourceDoc{ID: input.ID, Kind: input.Kind, Path: input.Path, Operations: ops})
	}
	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Kind != docs[j].Kind {
			return docs[i].Kind < docs[j].Kind
		}
		return docs[i].ID < docs[j].ID
	})
	return docs, diagnostics
}

func normalizeOptions(opts Options) Options {
	if strings.TrimSpace(opts.ConfigDir) == "" {
		opts.ConfigDir = "."
	}
	if strings.TrimSpace(opts.StatePath) == "" {
		opts.StatePath = state.DefaultPath(opts.ConfigDir)
	}
	opts.Action = strings.ToLower(strings.TrimSpace(opts.Action))
	if opts.Action == "" {
		opts.Action = "create"
	}
	for i := range opts.APISources {
		opts.APISources[i].Kind = normalizeAPISourceKind(opts.APISources[i].Kind)
		opts.APISources[i].ID = strings.TrimSpace(opts.APISources[i].ID)
		opts.APISources[i].Path = strings.TrimSpace(opts.APISources[i].Path)
	}
	sort.Slice(opts.APISources, func(i, j int) bool {
		left := opts.APISources[i].Kind + "\x00" + opts.APISources[i].ID + "\x00" + opts.APISources[i].Path
		right := opts.APISources[j].Kind + "\x00" + opts.APISources[j].ID + "\x00" + opts.APISources[j].Path
		return left < right
	})
	return opts
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

func sortDiagnostics(diags []Diagnostic) {
	sort.Slice(diags, func(i, j int) bool {
		left := diags[i].Code + "\x00" + diags[i].Address + "\x00" + diags[i].Message
		right := diags[j].Code + "\x00" + diags[j].Address + "\x00" + diags[j].Message
		return left < right
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
