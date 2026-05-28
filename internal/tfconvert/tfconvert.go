package tfconvert

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/OpenUdon/apitools"
	"github.com/OpenUdon/ramen/tfmapping"
	"github.com/OpenUdon/tfconfig"
	"github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

const (
	defaultOutDir                = "./.ramen/convert"
	apiSourceStagingMarker       = ".ramen-tfconvert-api-source-staging.json"
	apiSourceStagingMarkerFormat = "ramen.tfconvert.api-source-staging.v1"
	apiSourceKindOpenAPI         = apitools.APISourceKindOpenAPI
	apiSourceKindAWSSmithy       = apitools.APISourceKindAWSSmithy
	apiSourceKindGoogleDiscovery = apitools.APISourceKindGoogleDiscovery
)

type Options struct {
	ConfigDir  string
	OpenAPIs   []OpenAPIInput
	APISources []APISourceInput
	Action     string
	Targets    []string
	OutDir     string
	Strict     bool
}

type OpenAPIInput struct {
	ID   string
	Path string
}

type APISourceInput struct {
	Kind string
	ID   string
	Path string
}

type Result struct {
	OutDir          string
	ProjectPath     string
	ConversionPath  string
	MappingsPath    string
	DiagnosticsJSON string
	DiagnosticsMD   string
	ReviewPath      string
	UWSPath         string
	PlanJSONPath    string
	PlanMDPath      string
	Diagnostics     []Diagnostic
	StrictFailed    bool
}

type apiSourceStagingOwnership struct {
	Version string   `json:"version"`
	Dirs    []string `json:"dirs"`
}

type Diagnostic struct {
	Code          string       `json:"code"`
	Severity      string       `json:"severity"`
	Message       string       `json:"message"`
	Address       string       `json:"address,omitempty"`
	ModuleAddress string       `json:"module_address,omitempty"`
	APISourceKind string       `json:"api_source_kind,omitempty"`
	APISourceID   string       `json:"api_source_id,omitempty"`
	SourceRange   *SourceRange `json:"source_range,omitempty"`
	TodoID        string       `json:"todo_id,omitempty"`
	StrictFailure bool         `json:"strict_failure,omitempty"`
}

type SourceRange struct {
	SourceID string   `json:"source_id,omitempty"`
	Path     string   `json:"path,omitempty"`
	Start    Position `json:"start"`
	End      Position `json:"end"`
}

type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
	Byte   int `json:"byte,omitempty"`
}

type strictFailureError struct {
	diagnostics []Diagnostic
}

func (e strictFailureError) Error() string {
	return fmt.Sprintf("strict mode failed with %d diagnostic(s)", len(e.diagnostics))
}

func Convert(ctx context.Context, opts Options) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opts = normalizeOptions(opts)

	doc, loadErr := tfconfig.LoadDir(opts.ConfigDir)
	conversion := conversionState{
		opts: opts,
		doc:  doc,
	}
	if loadErr != nil {
		conversion.addDiagnostic(Diagnostic{
			Code:          "tfconfig.load_error",
			Severity:      "error",
			Message:       loadErr.Error(),
			StrictFailure: true,
		})
	}
	conversion.addTFDiagnostics(doc.Diagnostics)
	for _, mod := range doc.Modules {
		conversion.addTFDiagnostics(mod.Diagnostics)
	}

	conversion.loadAPISources(ctx)
	conversion.collectBindings()
	conversion.collectSymbols()
	conversion.selectObjects()
	conversion.validateAction()
	conversion.mapObjects()
	conversion.ensureCredentialBindings()
	conversion.sortAll()

	result := &Result{
		OutDir:          opts.OutDir,
		ProjectPath:     filepath.Join(opts.OutDir, "project.md"),
		ConversionPath:  filepath.Join(opts.OutDir, "expected", "conversion.json"),
		MappingsPath:    filepath.Join(opts.OutDir, "expected", "mappings.json"),
		DiagnosticsJSON: filepath.Join(opts.OutDir, "expected", "diagnostics.json"),
		DiagnosticsMD:   filepath.Join(opts.OutDir, "expected", "diagnostics.md"),
		ReviewPath:      filepath.Join(opts.OutDir, "expected", "review.md"),
		UWSPath:         filepath.Join(opts.OutDir, "workflows", "workflow.uws.yaml"),
		PlanJSONPath:    filepath.Join(opts.OutDir, "expected", "plan.json"),
		PlanMDPath:      filepath.Join(opts.OutDir, "expected", "plan.md"),
		Diagnostics:     conversion.diagnostics,
		StrictFailed:    opts.Strict && hasStrictFailure(conversion.diagnostics),
	}

	if err := writeArtifacts(result, conversion); err != nil {
		return result, err
	}
	if result.StrictFailed && hasBlockingAPISourceDiagnostic(conversion.diagnostics) {
		return result, strictFailureError{diagnostics: strictDiagnostics(conversion.diagnostics)}
	}
	if result.StrictFailed {
		return result, strictFailureError{diagnostics: strictDiagnostics(conversion.diagnostics)}
	}
	return result, nil
}

func IsStrictFailure(err error) bool {
	_, ok := err.(strictFailureError)
	return ok
}

type conversionState struct {
	opts        Options
	doc         tfconfig.Document
	apiSources  []apiDoc
	bindings    []binding
	symbols     []symbolFact
	selected    []selectedObject
	mappings    []objectMapping
	diagnostics []Diagnostic
}

type apiDoc struct {
	ID          string
	Kind        string
	Path        string
	PackagePath string
	Index       apitools.OperationIndex
}

type binding struct {
	Name          string
	Address       string
	ModuleAddress string
	LocalName     string
	Alias         string
	Config        []attributeFact
}

type symbolFact struct {
	Kind          string
	Name          string
	ModuleAddress string
	Value         string
	Sensitive     bool
}

type selectedObject struct {
	Kind          string
	Address       string
	ModuleAddress string
	Type          string
	Name          string
	Provider      string
	Binding       string
	Config        []attributeFact
	Range         *tfconfig.SourceRange
}

type attributeFact struct {
	Path      string
	Value     string
	Sensitive bool
	TodoID    string
}

type objectMapping struct {
	Object             selectedObject
	Purpose            string
	Action             string
	SourceKind         string
	SourceID           string
	SourcePath         string
	OperationID        string
	Operation          apitools.OperationSummary
	IdentityAttributes []tfmapping.IdentityAttribute
	TodoID             string
	Ambiguous          bool
	Auth               []apitools.AuthRequirementSummary
}

type operationTarget = tfmapping.OperationTarget

func normalizeOptions(opts Options) Options {
	if strings.TrimSpace(opts.ConfigDir) == "" {
		opts.ConfigDir = "."
	}
	if strings.TrimSpace(opts.OutDir) == "" {
		opts.OutDir = defaultOutDir
	}
	opts.Action = strings.ToLower(strings.TrimSpace(opts.Action))
	for i := range opts.OpenAPIs {
		opts.OpenAPIs[i].ID = strings.TrimSpace(opts.OpenAPIs[i].ID)
		opts.OpenAPIs[i].Path = strings.TrimSpace(opts.OpenAPIs[i].Path)
	}
	for _, input := range opts.OpenAPIs {
		opts.APISources = append(opts.APISources, APISourceInput{Kind: apiSourceKindOpenAPI, ID: input.ID, Path: input.Path})
	}
	opts.OpenAPIs = nil
	for i := range opts.APISources {
		opts.APISources[i].Kind = normalizeAPISourceKind(opts.APISources[i].Kind)
		opts.APISources[i].ID = strings.TrimSpace(opts.APISources[i].ID)
		opts.APISources[i].Path = strings.TrimSpace(opts.APISources[i].Path)
	}
	slices.SortFunc(opts.OpenAPIs, func(a, b OpenAPIInput) int {
		if diff := cmp.Compare(a.ID, b.ID); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Path, b.Path)
	})
	slices.SortFunc(opts.APISources, func(a, b APISourceInput) int {
		left := []string{a.Kind, a.ID, a.Path}
		right := []string{b.Kind, b.ID, b.Path}
		return cmp.Compare(strings.Join(left, "\x00"), strings.Join(right, "\x00"))
	})
	for i := range opts.Targets {
		opts.Targets[i] = strings.TrimSpace(opts.Targets[i])
	}
	slices.Sort(opts.Targets)
	return opts
}

func (c *conversionState) loadAPISources(ctx context.Context) {
	if len(c.opts.APISources) == 0 {
		c.addDiagnostic(Diagnostic{
			Code:          "api_source.missing",
			Severity:      "error",
			Message:       "at least one --api-source kind:id=PATH or --openapi id=PATH input is required",
			StrictFailure: true,
		})
		return
	}
	seen := map[string]bool{}
	seenPackagePaths := map[string]string{}
	for _, input := range c.opts.APISources {
		switch {
		case input.Kind == "":
			c.addDiagnostic(Diagnostic{Code: "api_source.invalid", Severity: "error", Message: "--api-source kind is required and must be openapi, aws-smithy, or google-discovery", StrictFailure: true})
			continue
		case input.ID == "":
			c.addDiagnostic(Diagnostic{Code: "api_source.invalid", Severity: "error", Message: "--api-source ID is required", APISourceKind: input.Kind, StrictFailure: true})
			continue
		case input.Path == "":
			c.addDiagnostic(Diagnostic{Code: "api_source.invalid", Severity: "error", Message: fmt.Sprintf("--api-source %s:%s path is required", input.Kind, input.ID), APISourceKind: input.Kind, APISourceID: input.ID, StrictFailure: true})
			continue
		case seen[input.ID]:
			c.addDiagnostic(Diagnostic{Code: "api_source.duplicate_id", Severity: "error", Message: fmt.Sprintf("API source ID %q is duplicated", input.ID), APISourceKind: input.Kind, APISourceID: input.ID, StrictFailure: true})
			continue
		}
		seen[input.ID] = true
		packagePath := packageAPISourcePath(input.Kind, input.ID, input.Path)
		if previousID, ok := seenPackagePaths[packagePath]; ok {
			c.addDiagnostic(Diagnostic{
				Code:          "api_source.package_path_collision",
				Severity:      "error",
				Message:       fmt.Sprintf("API source IDs %q and %q both stage to %q", previousID, input.ID, packagePath),
				APISourceKind: input.Kind,
				APISourceID:   input.ID,
				StrictFailure: true,
			})
			continue
		}
		seenPackagePaths[packagePath] = input.ID
		inventory, err := apitools.BuildAPISourceOperationInventory(ctx, apitools.APISourceInventoryOptions{
			Documents: []apitools.APISourceDocument{{
				Kind:         input.Kind,
				Name:         input.ID,
				Path:         input.Path,
				RelativePath: packagePath,
			}},
		})
		if err != nil {
			c.addDiagnostic(Diagnostic{Code: "api_source.load_error", Severity: "error", Message: err.Error(), APISourceKind: input.Kind, APISourceID: input.ID, StrictFailure: true})
			continue
		}
		for _, diag := range inventory.Diagnostics {
			c.addDiagnostic(Diagnostic{
				Code:          "api_source." + strings.ReplaceAll(diag.Code, ".", "_"),
				Severity:      normalizeSeverity(diag.Severity),
				Message:       diag.Message,
				APISourceKind: input.Kind,
				APISourceID:   input.ID,
				SourceRange:   &SourceRange{Path: diag.Path},
				StrictFailure: diag.Severity == "error",
			})
		}
		index, err := apitools.NewOperationIndex(inventory)
		if err != nil {
			c.addDiagnostic(Diagnostic{Code: "api_source.index_error", Severity: "error", Message: fmt.Sprintf("%s:%s: %v", input.Kind, input.ID, err), APISourceKind: input.Kind, APISourceID: input.ID, StrictFailure: true})
			continue
		}
		c.apiSources = append(c.apiSources, apiDoc{ID: input.ID, Kind: input.Kind, Path: input.Path, PackagePath: packagePath, Index: index})
	}
}

func (c *conversionState) collectBindings() {
	for _, mod := range c.doc.Modules {
		for _, cfg := range mod.ProviderConfigs {
			b := binding{
				Name:          normalizeBindingName(cfg.Address),
				Address:       cfg.Address,
				ModuleAddress: mod.Address,
				LocalName:     cfg.LocalName,
				Alias:         cfg.Alias,
				Config:        c.attributes(fullAddress(mod.Address, cfg.Address), mod.Address, cfg.Config),
			}
			c.bindings = append(c.bindings, b)
		}
	}
}

func (c *conversionState) collectSymbols() {
	for _, mod := range c.doc.Modules {
		for _, v := range mod.Variables {
			fact := symbolFact{Kind: "variable", Name: v.Name, ModuleAddress: mod.Address, Sensitive: v.Sensitive}
			if v.Default != nil {
				fact.Value = valueText(*v.Default)
				fact.Sensitive = fact.Sensitive || valueSensitive(*v.Default)
				c.maybeSensitiveDiagnostic(fullAddress(mod.Address, "var."+v.Name), mod.Address, "variable default", *v.Default)
			}
			c.symbols = append(c.symbols, fact)
		}
		for _, l := range mod.Locals {
			fact := symbolFact{Kind: "local", Name: l.Name, ModuleAddress: mod.Address}
			if l.Value != nil {
				fact.Value = valueText(*l.Value)
				fact.Sensitive = valueSensitive(*l.Value)
				c.maybeSensitiveDiagnostic(fullAddress(mod.Address, "local."+l.Name), mod.Address, "local value", *l.Value)
			}
			c.symbols = append(c.symbols, fact)
		}
		for _, out := range mod.Outputs {
			fact := symbolFact{Kind: "output", Name: out.Name, ModuleAddress: mod.Address, Sensitive: out.Sensitive}
			if out.Value != nil {
				fact.Value = valueText(*out.Value)
				fact.Sensitive = fact.Sensitive || valueSensitive(*out.Value)
				c.maybeSensitiveDiagnostic(fullAddress(mod.Address, "output."+out.Name), mod.Address, "output value", *out.Value)
			}
			c.symbols = append(c.symbols, fact)
		}
	}
}

func (c *conversionState) selectObjects() {
	targets := map[string]bool{}
	for _, target := range c.opts.Targets {
		if target != "" {
			targets[target] = false
		}
	}
	selectAll := len(targets) == 0
	for _, mod := range c.doc.Modules {
		for _, res := range mod.Resources {
			addr := fullAddress(mod.Address, res.Address)
			if selectAll || targetSelected(targets, addr) {
				obj := selectedObject{
					Kind:          "resource",
					Address:       addr,
					ModuleAddress: mod.Address,
					Type:          res.Type,
					Name:          res.Name,
					Provider:      providerAddress(res.Provider),
					Binding:       normalizeBindingName(providerAddress(res.Provider)),
					Config:        c.attributes(addr, mod.Address, res.Config),
					Range:         res.Range,
				}
				c.selected = append(c.selected, obj)
			}
		}
		for _, ds := range mod.DataSources {
			addr := fullAddress(mod.Address, ds.Address)
			if selectAll || targetSelected(targets, addr) {
				obj := selectedObject{
					Kind:          "data_source",
					Address:       addr,
					ModuleAddress: mod.Address,
					Type:          ds.Type,
					Name:          ds.Name,
					Provider:      providerAddress(ds.Provider),
					Binding:       normalizeBindingName(providerAddress(ds.Provider)),
					Config:        c.attributes(addr, mod.Address, ds.Config),
					Range:         ds.Range,
				}
				c.selected = append(c.selected, obj)
			}
		}
	}
	for target, matched := range targets {
		if !matched {
			c.addDiagnostic(Diagnostic{
				Code:          "target.unmatched",
				Severity:      "error",
				Message:       fmt.Sprintf("target %q did not match a managed resource or data source", target),
				Address:       target,
				StrictFailure: true,
			})
		}
	}
}

func targetSelected(targets map[string]bool, addr string) bool {
	if _, ok := targets[addr]; ok {
		targets[addr] = true
		return true
	}
	return false
}

func (c *conversionState) validateAction() {
	if c.opts.Action != "" && !validAction(c.opts.Action) {
		c.addDiagnostic(Diagnostic{
			Code:          "action.invalid",
			Severity:      "error",
			Message:       fmt.Sprintf("action %q is invalid; use create, update, delete, or replace", c.opts.Action),
			StrictFailure: true,
		})
		return
	}
	for _, obj := range c.selected {
		if obj.Kind == "resource" && c.opts.Action == "" {
			c.addDiagnostic(Diagnostic{
				Code:          "action.required",
				Severity:      "error",
				Message:       "managed resources require --action create, update, delete, or replace",
				Address:       obj.Address,
				ModuleAddress: obj.ModuleAddress,
				SourceRange:   convertRange(obj.Range),
				StrictFailure: true,
			})
		}
	}
}

func (c *conversionState) mapObjects() {
	for _, obj := range c.selected {
		if isProviderLocalDataSource(obj) {
			continue
		}
		switch obj.Kind {
		case "data_source":
			if !c.mapObjectPurpose(obj, "read", "read") {
				c.mapObjectPurpose(obj, "list", "list")
			}
		case "resource":
			if !validAction(c.opts.Action) {
				continue
			}
			if c.opts.Action == "replace" {
				c.mapObjectPurpose(obj, "delete", "replace")
				c.mapObjectPurpose(obj, "create", "replace")
				continue
			}
			c.mapObjectPurpose(obj, c.opts.Action, c.opts.Action)
		}
	}
}

func isProviderLocalDataSource(obj selectedObject) bool {
	return tfmapping.DefaultRegistry().IsProviderLocalDataSource(tfmappingObject(obj))
}

func (c *conversionState) mapObjectPurpose(obj selectedObject, purpose, action string) bool {
	candidates := c.operationCandidates()
	provider := objectProviderLocalName(obj)
	mappingSpec := tfmapping.DefaultRegistry().MapObject(tfmappingObject(obj), purpose, action)
	if target := mappingSpec.Target; len(target.OperationIDs) > 0 {
		if operation, ok, ambiguous := findOperationByTarget(candidates, target); ok {
			mapping := objectMapping{Object: obj, Purpose: purpose, Action: action, IdentityAttributes: mappingSpec.IdentityAttributes}
			doc := apiSourceForOperation(c.apiSources, operation)
			mapping.SourceKind = doc.Kind
			mapping.SourceID = firstNonEmpty(operation.DocumentName, doc.ID)
			mapping.SourcePath = doc.PackagePath
			mapping.OperationID = operation.OperationID
			mapping.Operation = operation
			mapping.Auth = apitools.AuthRequirementsForOperation(provider, operation)
			c.addFallbackDiagnosticIfNeeded(obj, purpose, action, target, doc.Kind)
			c.mappings = append(c.mappings, mapping)
			return true
		} else if ambiguous {
			mapping := objectMapping{Object: obj, Purpose: purpose, Action: action, IdentityAttributes: mappingSpec.IdentityAttributes, Ambiguous: true}
			mapping.TodoID = todoID(obj.Address, purpose, action)
			mapping.SourcePath = defaultAPISourcePath(c.apiSources)
			c.addDiagnostic(Diagnostic{
				Code:          "operation.ambiguous",
				Severity:      "warning",
				Message:       fmt.Sprintf("multiple API source operations named %s may match %s %s for %s; expected source ID %s", strings.Join(target.OperationIDs, ", "), purpose, obj.Kind, obj.Address, strings.Join(target.SourceIDs, ", ")),
				Address:       obj.Address,
				ModuleAddress: obj.ModuleAddress,
				SourceRange:   convertRange(obj.Range),
				TodoID:        mapping.TodoID,
				StrictFailure: true,
			})
			c.mappings = append(c.mappings, mapping)
			return false
		}
	}
	selection := apitools.SelectOperationByHints(apitools.OperationSelectionHints{
		Provider: provider,
		Purpose:  purpose,
		Target:   strings.Join([]string{obj.Address, obj.Type, obj.Name}, " "),
	}, candidates)
	mapping := objectMapping{Object: obj, Purpose: purpose, Action: action, IdentityAttributes: mappingSpec.IdentityAttributes}
	switch {
	case selection.Found:
		doc := apiSourceForOperation(c.apiSources, selection.Operation)
		mapping.SourceKind = doc.Kind
		mapping.SourceID = firstNonEmpty(selection.Operation.DocumentName, doc.ID)
		mapping.SourcePath = doc.PackagePath
		mapping.OperationID = selection.Operation.OperationID
		mapping.Operation = selection.Operation
		mapping.Auth = apitools.AuthRequirementsForOperation(provider, selection.Operation)
		c.addMappingDiagnostic(obj, purpose, action, tfmapping.Diagnostic{
			Code:     tfmapping.DiagnosticCodeFallbackOnly,
			Severity: tfmapping.DiagnosticSeverityInfo,
			Message:  fmt.Sprintf("selected API source operation %s by fallback hints for %s", selection.Operation.OperationID, obj.Address),
		}, "")
		c.mappings = append(c.mappings, mapping)
		return true
	case selection.Ambiguous:
		mapping.Ambiguous = true
		mapping.TodoID = todoID(obj.Address, purpose, action)
		mapping.SourcePath = defaultAPISourcePath(c.apiSources)
		c.addDiagnostic(Diagnostic{
			Code:          "operation.ambiguous",
			Severity:      "warning",
			Message:       fmt.Sprintf("multiple API source operations may match %s %s for %s", purpose, obj.Kind, obj.Address),
			Address:       obj.Address,
			ModuleAddress: obj.ModuleAddress,
			SourceRange:   convertRange(obj.Range),
			TodoID:        mapping.TodoID,
			StrictFailure: true,
		})
	default:
		mapping.TodoID = todoID(obj.Address, purpose, action)
		mapping.SourcePath = defaultAPISourcePath(c.apiSources)
		c.addMappingDiagnostics(obj, purpose, action, mappingSpec.Diagnostics, mapping.TodoID)
	}
	c.mappings = append(c.mappings, mapping)
	return false
}

func (c *conversionState) addFallbackDiagnosticIfNeeded(obj selectedObject, purpose, action string, target operationTarget, selectedKind string) {
	if len(target.SourceKinds) == 0 || normalizeAPISourceKind(target.SourceKinds[0]) == normalizeAPISourceKind(selectedKind) {
		return
	}
	c.addMappingDiagnostic(obj, purpose, action, tfmapping.Diagnostic{
		Code:     tfmapping.DiagnosticCodeFallbackOnly,
		Severity: tfmapping.DiagnosticSeverityInfo,
		Message:  fmt.Sprintf("selected %s API source fallback for %s because preferred %s source was not selected", selectedKind, obj.Address, target.SourceKinds[0]),
	}, "")
}

func (c *conversionState) addMappingDiagnostics(obj selectedObject, purpose, action string, diagnostics []tfmapping.Diagnostic, todoID string) {
	if len(diagnostics) == 0 {
		diagnostics = []tfmapping.Diagnostic{{
			Code:     tfmapping.DiagnosticCodeUnsupportedType,
			Severity: tfmapping.DiagnosticSeverityWarning,
			Message:  fmt.Sprintf("no Ramen mapping is available for %s %s %s", purpose, obj.Kind, obj.Address),
		}}
	}
	for _, diag := range diagnostics {
		c.addMappingDiagnostic(obj, purpose, action, diag, todoID)
	}
}

func (c *conversionState) addMappingDiagnostic(obj selectedObject, purpose, action string, diag tfmapping.Diagnostic, todoID string) {
	if diag.Code == "" {
		diag.Code = tfmapping.DiagnosticCodeUnsupportedType
	}
	if diag.Severity == "" {
		diag.Severity = tfmapping.DiagnosticSeverityWarning
	}
	strictFailure := diag.Severity != tfmapping.DiagnosticSeverityInfo && diag.Code != tfmapping.DiagnosticCodeFallbackOnly
	message := strings.TrimSpace(diag.Message)
	if message == "" {
		message = fmt.Sprintf("mapping diagnostic for %s %s %s", purpose, action, obj.Address)
	}
	c.addDiagnostic(Diagnostic{
		Code:          string(diag.Code),
		Severity:      string(diag.Severity),
		Message:       message,
		Address:       obj.Address,
		ModuleAddress: obj.ModuleAddress,
		SourceRange:   convertRange(obj.Range),
		TodoID:        todoID,
		StrictFailure: strictFailure,
	})
}

func findOperationByTarget(candidates []apitools.OperationSummary, target operationTarget) (apitools.OperationSummary, bool, bool) {
	var fallback []apitools.OperationSummary
	for _, kind := range slices.Clone(target.SourceKinds) {
		for _, operationID := range target.OperationIDs {
			var exact []apitools.OperationSummary
			for _, candidate := range candidates {
				if candidate.OperationID != operationID || !sourceKindMatches(candidate, kind) {
					continue
				}
				if sourceIDMatches(candidate.DocumentName, target.SourceIDs) {
					exact = append(exact, candidate)
				}
			}
			if len(exact) == 1 {
				return exact[0], true, false
			}
			if len(exact) > 1 {
				return apitools.OperationSummary{}, false, true
			}
		}
	}
	for _, operationID := range target.OperationIDs {
		for _, candidate := range candidates {
			if candidate.OperationID != operationID {
				continue
			}
			fallback = append(fallback, candidate)
		}
		if len(fallback) == 1 {
			return fallback[0], true, false
		}
		if len(fallback) > 1 {
			return apitools.OperationSummary{}, false, true
		}
	}
	return apitools.OperationSummary{}, false, false
}

func sourceKindMatches(operation apitools.OperationSummary, kind string) bool {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return true
	}
	if operation.Extensions != nil && strings.TrimSpace(operation.Extensions["x-uws-source-kind"]) == kind {
		return true
	}
	if strings.TrimSpace(operation.DocumentRelativePath) != "" {
		return strings.HasPrefix(filepath.ToSlash(operation.DocumentRelativePath), kind+"/")
	}
	if strings.TrimSpace(operation.DocumentPath) != "" {
		parts := strings.Split(filepath.ToSlash(operation.DocumentPath), "/")
		for _, part := range parts {
			if part == kind {
				return true
			}
		}
	}
	return kind == apiSourceKindOpenAPI && operation.Extensions == nil
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

func (c *conversionState) operationCandidates() []apitools.OperationSummary {
	var out []apitools.OperationSummary
	for _, doc := range c.apiSources {
		ops := apitools.SortedOperationSummaries(doc.Index.OperationIDs)
		for _, op := range ops {
			if op.DocumentName == "" {
				op.DocumentName = doc.ID
			}
			if op.DocumentRelativePath == "" {
				op.DocumentRelativePath = doc.PackagePath
			}
			if op.Extensions == nil {
				op.Extensions = map[string]string{}
			}
			op.Extensions["x-uws-source-kind"] = doc.Kind
			out = append(out, op)
		}
	}
	return out
}

func (c *conversionState) attributes(address, moduleAddress string, attrs []tfconfig.Attribute) []attributeFact {
	out := make([]attributeFact, 0, len(attrs))
	for _, attr := range attrs {
		fact := attributeFact{Path: attr.Path, Value: valueText(attr.Value), Sensitive: valueSensitive(attr.Value)}
		if fact.Sensitive {
			fact.TodoID = todoID(address+"."+attr.Path, "redaction", "review")
		}
		c.maybeSensitiveDiagnostic(address, moduleAddress, attr.Path, attr.Value)
		out = append(out, fact)
	}
	slices.SortFunc(out, func(a, b attributeFact) int { return cmp.Compare(a.Path, b.Path) })
	return out
}

func (c *conversionState) maybeSensitiveDiagnostic(address, moduleAddress, path string, value tfconfig.Value) {
	if !valueSensitive(value) {
		return
	}
	reason := "sensitive value"
	if value.SensitiveCandidate != nil {
		reason = value.SensitiveCandidate.Reason
	}
	c.addDiagnostic(Diagnostic{
		Code:          "redaction.review_required",
		Severity:      "warning",
		Message:       fmt.Sprintf("%s at %s is redacted and requires review", reason, path),
		Address:       address,
		ModuleAddress: moduleAddress,
		SourceRange:   convertRange(value.Range),
		TodoID:        todoID(address+"."+path, "redaction", "review"),
		StrictFailure: true,
	})
}

func (c *conversionState) addTFDiagnostics(diags []tfconfig.Diagnostic) {
	for _, diag := range diags {
		c.addDiagnostic(Diagnostic{
			Code:          firstNonEmpty(diag.Code, "tfconfig.diagnostic"),
			Severity:      normalizeSeverity(string(diag.Severity)),
			Message:       diagnosticMessage(diag),
			Address:       diag.Address,
			ModuleAddress: diag.ModuleAddress,
			SourceRange:   convertRange(diag.Range),
			StrictFailure: diag.Severity == tfconfig.DiagnosticError,
		})
	}
}

func (c *conversionState) addDiagnostic(diag Diagnostic) {
	diag.Code = strings.TrimSpace(diag.Code)
	diag.Severity = normalizeSeverity(diag.Severity)
	diag.Message = strings.TrimSpace(diag.Message)
	c.diagnostics = append(c.diagnostics, diag)
}

func (c *conversionState) ensureCredentialBindings() {
	existing := map[string]bool{}
	for _, binding := range c.bindings {
		existing[binding.Name] = true
	}
	for _, mapping := range c.mappings {
		for _, auth := range mapping.Auth {
			name := credentialBindingName(mapping.Object, auth)
			if name == "" || existing[name] {
				continue
			}
			c.bindings = append(c.bindings, binding{
				Name:      name,
				Address:   credentialBindingAddress(mapping.Object, auth),
				LocalName: objectProviderLocalName(mapping.Object),
			})
			existing[name] = true
		}
	}
}

func (c *conversionState) sortAll() {
	slices.SortFunc(c.bindings, func(a, b binding) int {
		if diff := cmp.Compare(a.Name, b.Name); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Address, b.Address)
	})
	slices.SortFunc(c.symbols, func(a, b symbolFact) int {
		left := strings.Join([]string{a.ModuleAddress, a.Kind, a.Name}, "\x00")
		right := strings.Join([]string{b.ModuleAddress, b.Kind, b.Name}, "\x00")
		return cmp.Compare(left, right)
	})
	slices.SortFunc(c.selected, func(a, b selectedObject) int { return cmp.Compare(a.Address, b.Address) })
	slices.SortFunc(c.mappings, func(a, b objectMapping) int {
		left := []string{a.Object.Address, a.Purpose, a.SourceKind, a.SourceID, a.SourcePath, a.OperationID, a.TodoID}
		right := []string{b.Object.Address, b.Purpose, b.SourceKind, b.SourceID, b.SourcePath, b.OperationID, b.TodoID}
		return cmp.Compare(strings.Join(left, "\x00"), strings.Join(right, "\x00"))
	})
	sortDiagnostics(c.diagnostics)
}

func writeArtifacts(result *Result, c conversionState) error {
	if err := validateAPISourceInputStagingSafety(result.OutDir, c.opts.APISources); err != nil {
		return err
	}
	if err := validateAPISourceStagingSafety(result.OutDir, c.apiSources); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(result.OutDir, "workflows"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(result.OutDir, "expected"), 0o755); err != nil {
		return err
	}
	if err := resetAPISourceStagingDirs(result.OutDir); err != nil {
		return err
	}
	if err := copyAPISourceDocuments(result.OutDir, c.apiSources); err != nil {
		return err
	}
	if err := writeAPISourceStagingMarker(result.OutDir, c.apiSources); err != nil {
		return err
	}
	if err := writeFile(result.ProjectPath, renderProject(c)); err != nil {
		return err
	}
	diagJSON, err := json.MarshalIndent(c.diagnostics, "", "  ")
	if err != nil {
		return err
	}
	diagJSON = append(diagJSON, '\n')
	if err := os.WriteFile(result.DiagnosticsJSON, diagJSON, 0o644); err != nil {
		return err
	}
	if err := writeFile(result.DiagnosticsMD, renderDiagnosticsMarkdown(c.diagnostics)); err != nil {
		return err
	}
	if err := writeJSONFile(result.ConversionPath, renderConversionArtifact(c)); err != nil {
		return err
	}
	if err := writeJSONFile(result.MappingsPath, renderMappingArtifacts(c)); err != nil {
		return err
	}
	if err := writeJSONFile(result.PlanJSONPath, renderPlanArtifact(c)); err != nil {
		return err
	}
	if err := writeFile(result.PlanMDPath, renderPlanMarkdown(c)); err != nil {
		return err
	}
	if err := writeUWSDocument(result.UWSPath, c); err != nil {
		return err
	}
	return writeFile(result.ReviewPath, renderReview(c))
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
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

func resetAPISourceStagingDirs(outDir string) error {
	ownedDirs, err := readAPISourceStagingMarker(outDir)
	if err != nil {
		return err
	}
	for _, dir := range []string{apiSourceKindOpenAPI, apiSourceKindAWSSmithy, apiSourceKindGoogleDiscovery} {
		path := filepath.Join(outDir, dir)
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspect staged API source directory %s: %w", dir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("refusing to reset API source staging path %s because it is not a directory", path)
		}
		if !ownedDirs[dir] {
			return fmt.Errorf("refusing to reset API source staging directory %s because it is not marked as owned by ramen convert tf; choose a dedicated --out directory or remove/relocate the existing %s directory", path, dir)
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("reset staged API source directory %s: %w", dir, err)
		}
	}
	return nil
}

func readAPISourceStagingMarker(outDir string) (map[string]bool, error) {
	path := filepath.Join(outDir, apiSourceStagingMarker)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("read API source staging marker: %w", err)
	}
	var marker apiSourceStagingOwnership
	if err := json.Unmarshal(data, &marker); err != nil {
		return nil, fmt.Errorf("read API source staging marker: %w", err)
	}
	if marker.Version != apiSourceStagingMarkerFormat {
		return nil, fmt.Errorf("read API source staging marker: unsupported version %q", marker.Version)
	}
	owned := map[string]bool{}
	for _, dir := range marker.Dirs {
		dir = normalizeAPISourceKind(dir)
		if dir != "" {
			owned[dir] = true
		}
	}
	return owned, nil
}

func writeAPISourceStagingMarker(outDir string, docs []apiDoc) error {
	seen := map[string]bool{}
	var dirs []string
	for _, doc := range docs {
		dir := normalizeAPISourceKind(doc.Kind)
		if dir == "" || strings.TrimSpace(doc.PackagePath) == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)
	marker := apiSourceStagingOwnership{Version: apiSourceStagingMarkerFormat, Dirs: dirs}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(outDir, apiSourceStagingMarker), data, 0o644)
}

func validateAPISourceStagingSafety(outDir string, docs []apiDoc) error {
	for _, doc := range docs {
		stagingDir, err := filepath.Abs(filepath.Join(outDir, doc.Kind))
		if err != nil {
			return err
		}
		stagingDir = filepath.Clean(stagingDir)
		sourcePath, err := filepath.Abs(doc.Path)
		if err != nil {
			return fmt.Errorf("resolve API source %s:%s source path: %w", doc.Kind, doc.ID, err)
		}
		sourcePath = filepath.Clean(sourcePath)
		if pathWithin(sourcePath, stagingDir) {
			return fmt.Errorf("stage API source %s:%s: source %s is inside generated API source staging directory %s; choose an --out directory outside API source inputs", doc.Kind, doc.ID, sourcePath, stagingDir)
		}
	}
	return nil
}

func pathWithin(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func copyAPISourceDocuments(outDir string, docs []apiDoc) error {
	for _, doc := range docs {
		if strings.TrimSpace(doc.PackagePath) == "" {
			continue
		}
		dst := filepath.Join(outDir, filepath.FromSlash(doc.PackagePath))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyRegularFile(doc.Path, dst); err != nil {
			return fmt.Errorf("stage API source %s:%s: %w", doc.Kind, doc.ID, err)
		}
	}
	return nil
}

func copyRegularFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file: %s", src)
	}
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, dst)
}

func validateAPISourceInputStagingSafety(outDir string, inputs []APISourceInput) error {
	for _, input := range inputs {
		if strings.TrimSpace(input.Path) == "" {
			continue
		}
		kind := normalizeAPISourceKind(input.Kind)
		if kind == "" {
			continue
		}
		stagingDir, err := filepath.Abs(filepath.Join(outDir, kind))
		if err != nil {
			return err
		}
		stagingDir = filepath.Clean(stagingDir)
		sourcePath, err := filepath.Abs(input.Path)
		if err != nil {
			return fmt.Errorf("resolve API source %s:%s source path: %w", kind, firstNonEmpty(input.ID, input.Path), err)
		}
		sourcePath = filepath.Clean(sourcePath)
		if pathWithin(sourcePath, stagingDir) {
			return fmt.Errorf("stage API source %s:%s: source %s is inside generated API source staging directory %s; choose an --out directory outside API source inputs", kind, firstNonEmpty(input.ID, input.Path), sourcePath, stagingDir)
		}
	}
	return nil
}

func renderProject(c conversionState) string {
	var b strings.Builder
	b.WriteString("# Ramen Terraform Conversion Draft\n\n")
	b.WriteString("This package is unapproved review scaffolding generated from static Terraform/OpenTofu facts. It does not execute Terraform, providers, API source operations, or UWS workflows.\n\n")
	b.WriteString("```ramen-policy\n")
	b.WriteString("runtimes:\n")
	b.WriteString("  openapi: true\n")
	b.WriteString("  http: true\n")
	b.WriteString("  fnct: true\n")
	b.WriteString("  cmd: false\n")
	b.WriteString("  ssh: false\n")
	b.WriteString("```\n\n")
	b.WriteString("## Goal\n\n")
	b.WriteString("Review static Terraform/OpenTofu configuration facts against local API source operation candidates and produce a Ramen-owned UWS-facing package candidate for human review.\n\n")
	fmt.Fprintf(&b, "- Config directory: `%s`\n", c.opts.ConfigDir)
	fmt.Fprintf(&b, "- Action: `%s`\n", firstNonEmpty(c.opts.Action, "none"))
	fmt.Fprintf(&b, "- Strict mode: `%t`\n", c.opts.Strict)
	b.WriteString("\n## Inputs\n\n")
	for _, sym := range c.symbols {
		if sym.Kind != "variable" {
			continue
		}
		required := "required"
		if strings.TrimSpace(sym.Value) != "" {
			required = "optional default preserved symbolically"
		}
		sensitive := ""
		if sym.Sensitive {
			sensitive = " sensitive"
		}
		fmt.Fprintf(&b, "- `%s`: string, %s%s Terraform variable.\n", normalizeName(fullAddress(sym.ModuleAddress, sym.Name)), required, sensitive)
	}
	if len(c.symbols) == 0 {
		b.WriteString("- No Terraform variables were selected.\n")
	}
	b.WriteString("\n## Outputs\n\n")
	b.WriteString("- `review_package`: generated Ramen conversion artifacts for review; no operational result is produced by conversion.\n")
	b.WriteString("\n## External Systems and API Sources\n\n")
	for _, doc := range c.apiSources {
		fmt.Fprintf(&b, "- `%s` `%s`: source `%s`, staged package path `%s`.\n", doc.Kind, doc.ID, doc.Path, doc.PackagePath)
	}
	if len(c.apiSources) == 0 {
		b.WriteString("- none loaded\n")
	}
	b.WriteString("\n## Runtime Policy\n\n")
	b.WriteString("- Only API-source-bound UWS artifacts are generated by this conversion package.\n")
	b.WriteString("- `cmd` and `ssh` are not allowed by this conversion package.\n")
	b.WriteString("\n## Data Flow\n\n")
	for _, obj := range c.selected {
		if isProviderLocalDataSource(obj) {
			fmt.Fprintf(&b, "- Terraform `%s` `%s` is provider-local metadata preserved symbolically; no API source operation is generated.\n", obj.Kind, obj.Address)
			for _, attr := range obj.Config {
				fmt.Fprintf(&b, "- `%s.%s`: symbolic Terraform expression `%s`.\n", obj.Address, attr.Path, attr.Value)
			}
			continue
		}
		fmt.Fprintf(&b, "- Terraform `%s` `%s` maps to a symbolic Ramen review operation using provider binding `%s`.\n", obj.Kind, obj.Address, firstNonEmpty(obj.Binding, "default"))
		for _, attr := range obj.Config {
			if attr.Sensitive {
				fmt.Fprintf(&b, "- `%s.%s`: sensitive symbolic value, review TODO `%s`.\n", obj.Address, attr.Path, attr.TodoID)
				continue
			}
			fmt.Fprintf(&b, "- `%s.%s`: symbolic Terraform expression `%s`.\n", obj.Address, attr.Path, attr.Value)
		}
	}
	if len(c.selected) == 0 {
		b.WriteString("- none\n")
	}
	b.WriteString("\n## Function Contracts\n\n")
	b.WriteString("- No custom function adapters are generated by Terraform conversion.\n")
	b.WriteString("\n## Credentials and Secrets\n\n")
	if len(c.bindings) == 0 {
		b.WriteString("- No provider credential bindings were declared in the selected Terraform configuration.\n")
	} else {
		for _, binding := range c.bindings {
			fmt.Fprintf(&b, "- `%s`: symbolic provider credential binding for `%s`; credential values must be supplied outside generated artifacts.\n", binding.Name, binding.Address)
		}
	}
	b.WriteString("- Sensitive or secret-like Terraform values are redacted into symbolic review inputs and must not appear as literals in generated artifacts.\n")
	b.WriteString("\n## Safety and Approval Boundary\n\n")
	b.WriteString("- Generated artifacts are unapproved by default and require human review before trusted executor handoff.\n")
	b.WriteString("- Side-effectful API source operations require review, sandbox proof-run approval, and trusted-runtime approval before production execution.\n")
	b.WriteString("- Direct production execution is not performed by conversion or synthesis.\n")
	b.WriteString("\n## Fallback Behavior\n\n")
	b.WriteString("- Unmatched Terraform targets, missing API source inputs, ambiguous operation matches, unresolved operation TODOs, and sensitive redaction TODOs remain diagnostics.\n")
	b.WriteString("- Strict mode fails when strict-failure diagnostics remain.\n")
	b.WriteString("- Unresolved conversion diagnostics remain explicit in Ramen-owned review artifacts so unsafe assumptions are visible to reviewers.\n")
	b.WriteString("\n## Diagnostics\n\n")
	if len(c.diagnostics) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, diag := range c.diagnostics {
			fmt.Fprintf(&b, "- `%s` %s: %s\n", diag.Code, diag.Severity, diag.Message)
		}
	}
	return b.String()
}

type conversionArtifact struct {
	Version   string           `json:"version"`
	ConfigDir string           `json:"config_dir"`
	Action    string           `json:"action,omitempty"`
	Strict    bool             `json:"strict"`
	Symbols   []symbolFact     `json:"symbols,omitempty"`
	Bindings  []binding        `json:"bindings,omitempty"`
	Objects   []selectedObject `json:"objects,omitempty"`
}

type mappingArtifact struct {
	Address            string                        `json:"address"`
	Kind               string                        `json:"kind"`
	Type               string                        `json:"type"`
	Purpose            string                        `json:"purpose"`
	Action             string                        `json:"action"`
	SourceKind         string                        `json:"source_kind,omitempty"`
	SourceID           string                        `json:"source_id,omitempty"`
	SourcePath         string                        `json:"source_path,omitempty"`
	OperationID        string                        `json:"operation_id,omitempty"`
	IdentityAttributes []tfmapping.IdentityAttribute `json:"identity_attributes,omitempty"`
	TodoID             string                        `json:"todo_id,omitempty"`
	Ambiguous          bool                          `json:"ambiguous,omitempty"`
	Credentials        []string                      `json:"credentials,omitempty"`
}

type planArtifact struct {
	Version     string            `json:"version"`
	Workflow    string            `json:"workflow"`
	Action      string            `json:"action,omitempty"`
	Steps       []mappingArtifact `json:"steps"`
	Diagnostics []Diagnostic      `json:"diagnostics,omitempty"`
}

func renderConversionArtifact(c conversionState) conversionArtifact {
	return conversionArtifact{
		Version:   "ramen.tfconvert.conversion.v1",
		ConfigDir: c.opts.ConfigDir,
		Action:    c.opts.Action,
		Strict:    c.opts.Strict,
		Symbols:   c.symbols,
		Bindings:  c.bindings,
		Objects:   c.selected,
	}
}

func renderMappingArtifacts(c conversionState) []mappingArtifact {
	out := make([]mappingArtifact, 0, len(c.mappings))
	for _, mapping := range c.mappings {
		out = append(out, mappingArtifactFor(mapping))
	}
	return out
}

func mappingArtifactFor(mapping objectMapping) mappingArtifact {
	var credentials []string
	for _, auth := range mapping.Auth {
		if name := credentialBindingName(mapping.Object, auth); name != "" {
			credentials = append(credentials, name)
		}
	}
	slices.Sort(credentials)
	return mappingArtifact{
		Address:            mapping.Object.Address,
		Kind:               mapping.Object.Kind,
		Type:               mapping.Object.Type,
		Purpose:            mapping.Purpose,
		Action:             mapping.Action,
		SourceKind:         mapping.SourceKind,
		SourceID:           mapping.SourceID,
		SourcePath:         mapping.SourcePath,
		OperationID:        mapping.OperationID,
		IdentityAttributes: slices.Clone(mapping.IdentityAttributes),
		TodoID:             mapping.TodoID,
		Ambiguous:          mapping.Ambiguous,
		Credentials:        credentials,
	}
}

func renderPlanArtifact(c conversionState) planArtifact {
	return planArtifact{
		Version:     "ramen.tfconvert.plan.v1",
		Workflow:    "terraform_conversion_draft",
		Action:      c.opts.Action,
		Steps:       renderMappingArtifacts(c),
		Diagnostics: c.diagnostics,
	}
}

func renderPlanMarkdown(c conversionState) string {
	var b strings.Builder
	b.WriteString("# Ramen Terraform Conversion Plan\n\n")
	if len(c.mappings) == 0 {
		b.WriteString("No API source operations were mapped.\n")
		return b.String()
	}
	for _, mapping := range c.mappings {
		fmt.Fprintf(&b, "## %s %s\n\n", mapping.Object.Kind, mapping.Object.Address)
		fmt.Fprintf(&b, "- Action: `%s`\n", mapping.Action)
		fmt.Fprintf(&b, "- Purpose: `%s`\n", mapping.Purpose)
		if mapping.OperationID != "" {
			fmt.Fprintf(&b, "- Operation: `%s` from `%s` `%s`\n", mapping.OperationID, mapping.SourceKind, mapping.SourceID)
		}
		if mapping.TodoID != "" {
			fmt.Fprintf(&b, "- TODO: `%s`\n", mapping.TodoID)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func writeUWSDocument(path string, c conversionState) error {
	doc := renderUWSDocument(c)
	if err := doc.Validate(); err != nil {
		return err
	}
	data, err := convert.MarshalYAML(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func renderUWSDocument(c conversionState) *uws1.Document {
	doc := &uws1.Document{
		UWS: "1.4.0",
		Info: &uws1.Info{
			Title:       "terraform_conversion_draft",
			Description: "Draft review scaffold generated from static Terraform/OpenTofu configuration.",
			Version:     "1.0.0",
		},
		Operations: []*uws1.Operation{},
		Workflows: []*uws1.Workflow{{
			WorkflowID:  "main",
			Type:        uws1.WorkflowTypeSequence,
			Description: "Review mapped Terraform/OpenTofu objects as API source operations.",
			Steps:       []*uws1.Step{},
		}},
	}
	sourceNames := map[string]string{}
	for _, mapping := range c.mappings {
		operationID := normalizeName(mapping.Object.Address + "_" + mapping.Purpose)
		if operationID == "" {
			operationID = normalizeName(firstNonEmpty(mapping.TodoID, "review_operation"))
		}
		op := &uws1.Operation{
			OperationID: operationID,
			Description: fmt.Sprintf("Review %s %s for Terraform %s %s", mapping.Purpose, mapping.Action, mapping.Object.Kind, mapping.Object.Address),
			Request:     operationRequest(mapping),
		}
		if mapping.OperationID != "" && mapping.SourcePath != "" {
			op.SourceDescription = sourceDescriptionForMapping(doc, sourceNames, mapping)
			op.SourceOperationID = mapping.OperationID
		} else {
			op.Extensions = map[string]any{uws1.ExtensionOperationProfile: "ramen-review-todo"}
			if op.Request == nil {
				op.Request = map[string]any{}
			}
			op.Request["x-ramen-todo"] = firstNonEmpty(mapping.TodoID, "operation.unresolved")
		}
		doc.Operations = append(doc.Operations, op)
		doc.Workflows[0].Steps = append(doc.Workflows[0].Steps, &uws1.Step{
			StepID:       operationID,
			OperationRef: operationID,
			Body: map[string]any{
				"terraform_address": mapping.Object.Address,
				"terraform_type":    mapping.Object.Type,
				"purpose":           mapping.Purpose,
				"action":            mapping.Action,
			},
		})
	}
	return doc
}

func sourceDescriptionForMapping(doc *uws1.Document, sourceNames map[string]string, mapping objectMapping) string {
	key := mapping.SourcePath
	if name := sourceNames[key]; name != "" {
		return name
	}
	name := normalizeName(firstNonEmpty(mapping.SourceID, mapping.SourceKind, "api_source"))
	if name == "" {
		name = "api_source"
	}
	base := name
	for i := 2; sourceDescriptionNameUsed(doc, name); i++ {
		name = fmt.Sprintf("%s_%d", base, i)
	}
	sourceNames[key] = name
	doc.SourceDescriptions = append(doc.SourceDescriptions, &uws1.SourceDescription{
		Name: name,
		URL:  filepath.ToSlash(mapping.SourcePath),
		Type: sourceDescriptionType(mapping.SourceKind),
	})
	return name
}

func sourceDescriptionNameUsed(doc *uws1.Document, name string) bool {
	for _, source := range doc.SourceDescriptions {
		if source != nil && source.Name == name {
			return true
		}
	}
	return false
}

func sourceDescriptionType(kind string) uws1.SourceDescriptionType {
	switch normalizeAPISourceKind(kind) {
	case apiSourceKindAWSSmithy:
		return uws1.SourceDescriptionTypeAWSSmithy
	case apiSourceKindGoogleDiscovery:
		return uws1.SourceDescriptionTypeGoogleDiscovery
	default:
		return uws1.SourceDescriptionTypeOpenAPI
	}
}

func operationRequest(mapping objectMapping) map[string]any {
	body := map[string]any{}
	header := map[string]any{}
	cookie := map[string]any{}
	path := map[string]any{}
	query := map[string]any{}
	terraformAttrs := map[string]any{}
	terraform := map[string]any{
		"object": map[string]any{
			"address": mapping.Object.Address,
			"kind":    mapping.Object.Kind,
			"type":    mapping.Object.Type,
			"name":    mapping.Object.Name,
		},
		"attributes": terraformAttrs,
	}
	var credentials []string
	for _, attr := range mapping.Object.Config {
		if strings.TrimSpace(attr.Path) == "" {
			continue
		}
		terraformAttrs[attr.Path] = attr.Value
		for _, requestKey := range terraformAPIRequestKeys(mapping, attr.Path) {
			setRequestBinding(requestLocationForKey(mapping.Operation, requestKey), requestKey, attr.Value, path, query, header, cookie, body)
		}
	}
	if len(mapping.IdentityAttributes) > 0 {
		terraform["identity_attributes"] = mapping.IdentityAttributes
	}
	for requestKey, value := range awsQueryProtocolStaticBindings(mapping) {
		setRequestBinding(requestLocationForKey(mapping.Operation, requestKey), requestKey, value, path, query, header, cookie, body)
	}
	for _, auth := range mapping.Auth {
		bindingName := credentialBindingName(mapping.Object, auth)
		if bindingName == "" {
			continue
		}
		credentials = append(credentials, bindingName)
		if mapping.SourceKind != apiSourceKindOpenAPI {
			continue
		}
		for _, requestKey := range credentialRequestKeys(auth) {
			setRequestBinding(credentialRequestLocation(auth), requestKey, bindingName, path, query, header, cookie, body)
		}
	}
	request := map[string]any{"x-ramen-terraform": terraform}
	if len(path) > 0 {
		request["path"] = path
	}
	if len(query) > 0 {
		request["query"] = query
	}
	if len(header) > 0 {
		request["header"] = header
	}
	if len(cookie) > 0 {
		request["cookie"] = cookie
	}
	if len(body) > 0 {
		request["body"] = body
	}
	if len(credentials) > 0 {
		slices.Sort(credentials)
		request["x-ramen-credential-bindings"] = credentials
	}
	return request
}

func setRequestBinding(location, key string, value any, path, query, header, cookie, body map[string]any) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	switch normalizeRequestLocation(location) {
	case "path":
		if _, ok := path[key]; !ok {
			path[key] = value
		}
	case "query":
		if _, ok := query[key]; !ok {
			query[key] = value
		}
	case "header":
		if _, ok := header[key]; !ok {
			header[key] = value
		}
	case "cookie":
		if _, ok := cookie[key]; !ok {
			cookie[key] = value
		}
	default:
		if _, ok := body[key]; !ok {
			body[key] = value
		}
	}
}

func requestLocationForKey(operation apitools.OperationSummary, key string) string {
	key = strings.TrimSpace(key)
	for _, param := range operation.Parameters {
		if strings.TrimSpace(param.Name) == key {
			return param.In
		}
	}
	if operation.RequestBody != nil {
		for _, field := range operation.RequestBody.Fields {
			if strings.TrimSpace(field.Path) == key {
				return "body"
			}
		}
	}
	return "body"
}

func normalizeRequestLocation(location string) string {
	switch strings.ToLower(strings.TrimSpace(location)) {
	case "path", "label":
		return "path"
	case "query", "queryparams":
		return "query"
	case "header", "prefixheaders":
		return "header"
	case "cookie":
		return "cookie"
	default:
		return "body"
	}
}

func credentialRequestLocation(auth apitools.AuthRequirementSummary) string {
	if auth.Kind == "aws_signature" {
		return "header"
	}
	return auth.In
}

func renderDiagnosticsMarkdown(diags []Diagnostic) string {
	var b strings.Builder
	b.WriteString("# Terraform Conversion Diagnostics\n\n")
	if len(diags) == 0 {
		b.WriteString("No diagnostics.\n")
		return b.String()
	}
	for _, diag := range diags {
		fmt.Fprintf(&b, "## %s\n\n", diag.Code)
		fmt.Fprintf(&b, "- Severity: `%s`\n", diag.Severity)
		if diag.Address != "" {
			fmt.Fprintf(&b, "- Address: `%s`\n", diag.Address)
		}
		if diag.ModuleAddress != "" {
			fmt.Fprintf(&b, "- Module: `%s`\n", diag.ModuleAddress)
		}
		if diag.TodoID != "" {
			fmt.Fprintf(&b, "- TODO: `%s`\n", diag.TodoID)
		}
		fmt.Fprintf(&b, "- Strict failure: `%t`\n", diag.StrictFailure)
		fmt.Fprintf(&b, "\n%s\n\n", diag.Message)
	}
	return b.String()
}

func renderReview(c conversionState) string {
	var b strings.Builder
	b.WriteString("# Terraform Conversion Review\n\n")
	b.WriteString("Generated artifacts are draft review scaffolding and are not approved for trusted execution.\n\n")
	b.WriteString("## Operation Mappings\n\n")
	if len(c.mappings) == 0 {
		b.WriteString("- none\n")
	}
	for _, mapping := range c.mappings {
		ref := mapping.TodoID
		if mapping.OperationID != "" {
			ref = mapping.SourcePath + ":" + mapping.OperationID
		}
		fmt.Fprintf(&b, "- `%s` %s/%s -> `%s`\n", mapping.Object.Address, mapping.Action, mapping.Purpose, ref)
		for _, identity := range mapping.IdentityAttributes {
			fmt.Fprintf(&b, "  - Identity `%s`: Terraform `%s`, request `%s`, response `%s`\n", identity.Name, identity.TerraformPath, strings.Join(identity.RequestKeys, ", "), strings.Join(identity.ResponsePaths, ", "))
		}
		for _, auth := range mapping.Auth {
			fmt.Fprintf(&b, "  - Auth `%s`: %s\n", auth.Scheme, auth.Description)
		}
	}
	b.WriteString("\n## Provider Bindings\n\n")
	if len(c.bindings) == 0 {
		b.WriteString("- none\n")
	}
	for _, binding := range c.bindings {
		fmt.Fprintf(&b, "- `%s` from `%s`\n", binding.Name, binding.Address)
	}
	b.WriteString("\n## Symbolic Facts\n\n")
	for _, sym := range c.symbols {
		fmt.Fprintf(&b, "- %s `%s` = `%s`\n", sym.Kind, fullAddress(sym.ModuleAddress, sym.Name), sym.Value)
	}
	b.WriteString("\n## Redaction\n\n")
	wrote := false
	for _, diag := range c.diagnostics {
		if strings.HasPrefix(diag.Code, "redaction.") {
			fmt.Fprintf(&b, "- `%s`: %s\n", diag.TodoID, diag.Message)
			wrote = true
		}
	}
	if !wrote {
		b.WriteString("- none\n")
	}
	return b.String()
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

func providerAddress(ref *tfconfig.ProviderRef) string {
	if ref == nil {
		return ""
	}
	return ref.Address
}

func providerLocalName(address string) string {
	address = strings.TrimPrefix(strings.TrimSpace(address), "provider.")
	if head, _, ok := strings.Cut(address, "."); ok {
		return head
	}
	return address
}

func objectProviderLocalName(obj selectedObject) string {
	if provider := providerLocalName(obj.Provider); provider != "" {
		return provider
	}
	if provider, _, ok := strings.Cut(strings.TrimSpace(obj.Type), "_"); ok {
		return provider
	}
	return ""
}

func awsQueryProtocolStaticBindings(mapping objectMapping) map[string]string {
	return tfmapping.DefaultRegistry().StaticRequestBindings(tfmappingObject(mapping.Object), mapping.SourceID, mapping.SourcePath, mapping.OperationID)
}

func terraformAPIRequestKeys(mapping objectMapping, attrPath string) []string {
	return tfmapping.DefaultRegistry().RequestKeys(tfmappingObject(mapping.Object), mapping.SourceKind, mapping.OperationID, attrPath)
}

func tfmappingObject(obj selectedObject) tfmapping.Object {
	return tfmapping.Object{
		Kind:     obj.Kind,
		Type:     obj.Type,
		Provider: obj.Provider,
	}
}

func credentialBindingName(obj selectedObject, auth apitools.AuthRequirementSummary) string {
	switch auth.Kind {
	case "aws_signature":
		provider := credentialProviderName(obj)
		scheme := firstNonEmpty(auth.Scheme, "sigv4")
		return normalizeName(provider + "_" + scheme)
	case "oauth2":
		if auth.Dialect == "gcp" || objectProviderLocalName(obj) == "google" {
			return normalizeName(firstNonEmpty(objectProviderLocalName(obj), "google") + "_oauth2")
		}
	}
	return ""
}

func credentialBindingAddress(obj selectedObject, auth apitools.AuthRequirementSummary) string {
	if auth.Kind == "oauth2" {
		return "provider." + firstNonEmpty(objectProviderLocalName(obj), "google") + ".oauth2"
	}
	provider := strings.TrimPrefix(firstNonEmpty(strings.TrimSpace(obj.Provider), "provider."+credentialProviderName(obj)), "provider.")
	scheme := firstNonEmpty(auth.Scheme, "sigv4")
	return "provider." + provider + "." + scheme
}

func credentialProviderName(obj selectedObject) string {
	if binding := strings.TrimSpace(obj.Binding); binding != "" && binding != "default" {
		return binding
	}
	return firstNonEmpty(objectProviderLocalName(obj), "aws")
}

func credentialRequestKeys(auth apitools.AuthRequirementSummary) []string {
	if auth.Kind != "aws_signature" {
		return nil
	}
	return []string{firstNonEmpty(auth.ParameterName, auth.Scheme, "Authorization"), "Authorization"}
}

func normalizeBindingName(address string) string {
	address = strings.TrimPrefix(strings.TrimSpace(address), "provider.")
	return normalizeName(address)
}

var invalidNameChars = regexp.MustCompile(`[^A-Za-z0-9_]+`)

func normalizeName(value string) string {
	value = strings.Trim(value, ".")
	value = strings.ReplaceAll(value, ".", "_")
	value = invalidNameChars.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "default"
	}
	if value[0] >= '0' && value[0] <= '9' {
		value = "n_" + value
	}
	return strings.ToLower(value)
}

func valueText(value tfconfig.Value) string {
	if valueSensitive(value) {
		return "${sensitive." + firstNonEmpty(sensitiveCandidatePath(value), "value") + "}"
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

func valueSensitive(value tfconfig.Value) bool {
	return value.Sensitive || value.Redacted || value.SensitiveCandidate != nil
}

func safeDefault(value string) bool {
	return value != "" && !strings.HasPrefix(value, "${") && len(value) < 160
}

func sensitiveCandidatePath(value tfconfig.Value) string {
	if value.SensitiveCandidate != nil && value.SensitiveCandidate.AttributePath != "" {
		return normalizeName(value.SensitiveCandidate.AttributePath)
	}
	return ""
}

func todoID(address, purpose, action string) string {
	return "todo." + normalizeName(address) + "." + normalizeName(purpose) + "." + normalizeName(action)
}

func validAction(action string) bool {
	switch action {
	case "create", "update", "delete", "replace":
		return true
	default:
		return false
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

func convertRange(rng *tfconfig.SourceRange) *SourceRange {
	if rng == nil {
		return nil
	}
	return &SourceRange{
		SourceID: rng.SourceID,
		Path:     rng.Path,
		Start:    Position{Line: rng.Start.Line, Column: rng.Start.Column, Byte: rng.Start.Byte},
		End:      Position{Line: rng.End.Line, Column: rng.End.Column, Byte: rng.End.Byte},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func packageAPISourcePath(kind, id, sourcePath string) string {
	kind = normalizeAPISourceKind(kind)
	if kind == "" {
		kind = apiSourceKindOpenAPI
	}
	ext := strings.ToLower(filepath.Ext(sourcePath))
	switch ext {
	case ".json", ".yaml", ".yml":
	default:
		if kind == apiSourceKindOpenAPI {
			ext = ".yaml"
		} else {
			ext = ".json"
		}
	}
	return filepath.ToSlash(filepath.Join(kind, normalizeName(id)+ext))
}

func defaultAPISourcePath(docs []apiDoc) string {
	if len(docs) == 0 {
		return ""
	}
	return docs[0].PackagePath
}

func apiSourceForOperation(docs []apiDoc, operation apitools.OperationSummary) apiDoc {
	for _, doc := range docs {
		if operation.DocumentName != "" && doc.ID == operation.DocumentName {
			return doc
		}
		if operation.DocumentPath != "" && doc.Path == operation.DocumentPath {
			return doc
		}
	}
	return apiDoc{}
}

func normalizeAPISourceKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", apiSourceKindOpenAPI, "swagger":
		if strings.TrimSpace(kind) == "" {
			return ""
		}
		return apiSourceKindOpenAPI
	case apiSourceKindAWSSmithy, "smithy", "smithy-json":
		return apiSourceKindAWSSmithy
	case apiSourceKindGoogleDiscovery, "discovery", "google":
		return apiSourceKindGoogleDiscovery
	default:
		return ""
	}
}

func sortDiagnostics(diags []Diagnostic) {
	slices.SortFunc(diags, func(a, b Diagnostic) int {
		left := []string{a.Code, a.Address, a.ModuleAddress, a.TodoID, a.Message}
		right := []string{b.Code, b.Address, b.ModuleAddress, b.TodoID, b.Message}
		return cmp.Compare(strings.Join(left, "\x00"), strings.Join(right, "\x00"))
	})
}

func hasStrictFailure(diags []Diagnostic) bool {
	for _, diag := range diags {
		if diag.StrictFailure {
			return true
		}
	}
	return false
}

func hasBlockingAPISourceDiagnostic(diags []Diagnostic) bool {
	for _, diag := range diags {
		switch diag.Code {
		case "api_source.missing", "api_source.invalid", "api_source.duplicate_id", "api_source.package_path_collision", "api_source.load_error", "api_source.index_error", "api_source.document_read", "api_source.document_parse", "api_source.document_kind":
			return true
		}
	}
	return false
}

func strictDiagnostics(diags []Diagnostic) []Diagnostic {
	var out []Diagnostic
	for _, diag := range diags {
		if diag.StrictFailure {
			out = append(out, diag)
		}
	}
	return out
}
