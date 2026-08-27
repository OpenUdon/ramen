package validate

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/OpenUdon/apitools"
	"github.com/OpenUdon/ramen/graph"
	"github.com/OpenUdon/ramen/internal/browsercontract"
	"github.com/OpenUdon/ramen/internal/tfconvert"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/uws/uws1"
)

const Version = "ramen.validate.v1"

type Options struct {
	ProjectPath string
	APISources  []APISourceInput
	Strict      bool
}

type APISourceInput struct {
	Kind string
	ID   string
	Path string
}

type Result struct {
	Version     string       `json:"version"`
	ProjectPath string       `json:"project_path,omitempty"`
	Strict      bool         `json:"strict,omitempty"`
	Valid       bool         `json:"valid"`
	Summary     Summary      `json:"summary"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type Summary struct {
	Errors      int `json:"errors"`
	Warnings    int `json:"warnings"`
	Diagnostics int `json:"diagnostics"`
}

type Diagnostic struct {
	Code          string `json:"code"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	Path          string `json:"path,omitempty"`
	Address       string `json:"address,omitempty"`
	APISourceKind string `json:"api_source_kind,omitempty"`
	APISourceID   string `json:"api_source_id,omitempty"`
	OperationID   string `json:"operation_id,omitempty"`
}

func Run(ctx context.Context, opts Options) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := &Result{
		Version: Version,
		Strict:  opts.Strict,
	}
	if strings.TrimSpace(opts.ProjectPath) == "" {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "validate.project_required", Severity: "error", Message: "--project is required"})
		finalize(result)
		return result, nil
	}
	doc, err := project.Load(opts.ProjectPath)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "validate.project_load_error", Severity: "error", Message: err.Error()})
		finalize(result)
		return result, nil
	}
	result.ProjectPath = doc.Path
	result.Diagnostics = append(result.Diagnostics, validateTerraformMetadata(doc.UWS)...)
	result.Diagnostics = append(result.Diagnostics, validateProfile(doc.Profile)...)
	sources, sourceDiagnostics := loadAPISources(ctx, doc.Dir, mergeAPISources(doc.Profile, opts.APISources))
	result.Diagnostics = append(result.Diagnostics, sourceDiagnostics...)
	result.Diagnostics = append(result.Diagnostics, validateOperations(doc, sources)...)
	result.Diagnostics = append(result.Diagnostics, unusedSourceDiagnostics(doc.Profile)...)
	contentTrust, err := contentTrustDiagnostics(ctx, doc)
	if err != nil {
		return nil, err
	}
	result.Diagnostics = append(result.Diagnostics, contentTrust...)
	if opts.Strict {
		for i := range result.Diagnostics {
			if result.Diagnostics[i].Severity == "warning" {
				result.Diagnostics[i].Severity = "error"
			}
		}
	}
	finalize(result)
	return result, nil
}

func validateTerraformMetadata(doc *uws1.Document) []Diagnostic {
	if doc == nil {
		return nil
	}
	var diagnostics []Diagnostic
	for _, operation := range doc.Operations {
		applicable, err := tfconvert.ValidateTerraformOperation(operation)
		if !applicable || err == nil {
			continue
		}
		operationID := ""
		if operation != nil {
			operationID = operation.OperationID
		}
		diagnostics = append(diagnostics, Diagnostic{
			Code:        "validate.terraform_metadata_invalid",
			Severity:    "error",
			Message:     err.Error(),
			OperationID: operationID,
		})
	}
	return diagnostics
}

func validateProfile(profile project.Profile) []Diagnostic {
	var diagnostics []Diagnostic
	addresses := map[string]bool{}
	for _, resource := range profile.Resources {
		addresses[resource.Address] = true
		for _, dep := range resource.Dependencies {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			if strings.HasPrefix(dep, "data.") {
				continue
			}
			if !addresses[dep] {
				found := false
				for _, candidate := range profile.Resources {
					if candidate.Address == dep {
						found = true
						break
					}
				}
				if !found {
					diagnostics = append(diagnostics, Diagnostic{Code: "validate.dependency_missing", Severity: "error", Message: fmt.Sprintf("resource %s depends on missing resource %s", resource.Address, dep), Address: resource.Address})
				}
			}
		}
		for _, path := range resource.Lifecycle.IgnorePaths {
			if strings.TrimSpace(path) == "" {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.lifecycle_invalid", Severity: "error", Message: fmt.Sprintf("resource %s has an empty lifecycle ignore path", resource.Address), Address: resource.Address})
			}
		}
		for _, trigger := range resource.Lifecycle.ReplaceTriggeredBy {
			if strings.TrimSpace(trigger) == "" {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.lifecycle_invalid", Severity: "error", Message: fmt.Sprintf("resource %s has an empty replace trigger", resource.Address), Address: resource.Address})
			}
		}
		for _, identity := range resource.IdentityAttributes {
			if strings.TrimSpace(identity.Name) == "" || strings.TrimSpace(identity.Path) == "" {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.identity_invalid", Severity: "error", Message: fmt.Sprintf("resource %s identity attributes require name and path", resource.Address), Address: resource.Address})
			}
			if identity.Required && !hasAttributePath(resource.Attributes, identity.Path) {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.attribute_required", Severity: "error", Message: fmt.Sprintf("resource %s requires identity attribute %s", resource.Address, identity.Path), Address: resource.Address})
			}
		}
		diagnostics = append(diagnostics, validateSchema(resource, profile.Redaction)...)
		diagnostics = append(diagnostics, validateOperationRequirements(resource)...)
		diagnostics = append(diagnostics, validateRuntimeHints(resource)...)
		diagnostics = append(diagnostics, redactionDiagnostics(resource.Redaction, resource.Address)...)
		for purpose, role := range resource.Operations {
			rolePurpose := strings.TrimSpace(firstNonEmpty(role.Purpose, purpose))
			if rolePurpose == "" {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.operation_role_invalid", Severity: "error", Message: fmt.Sprintf("resource %s has an empty operation role", resource.Address), Address: resource.Address})
			} else if !knownOperationPurpose(rolePurpose) {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.operation_role_unknown", Severity: "warning", Message: fmt.Sprintf("resource %s uses non-standard operation role %s", resource.Address, rolePurpose), Address: resource.Address, OperationID: role.OperationID})
			}
			if strings.TrimSpace(role.SourceKind) == "" || strings.TrimSpace(role.SourceID) == "" {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.source_reference_missing", Severity: "error", Message: fmt.Sprintf("resource %s %s operation requires source_kind and source_id", resource.Address, rolePurpose), Address: resource.Address, OperationID: role.OperationID})
			}
		}
	}
	diagnostics = append(diagnostics, redactionDiagnostics(profile.Redaction, "")...)
	var nodes []graph.Node
	for _, resource := range profile.Resources {
		nodes = append(nodes, graph.Node{Address: resource.Address, DependsOn: resource.Dependencies})
	}
	if _, err := graph.Sort(nodes); err != nil {
		diagnostics = append(diagnostics, Diagnostic{Code: "validate.dependency_cycle", Severity: "error", Message: err.Error()})
	}
	return diagnostics
}

func validateSchema(resource project.Resource, profileRedaction project.Redaction) []Diagnostic {
	var diagnostics []Diagnostic
	schemaByPath := map[string]project.SchemaPath{}
	for _, schema := range resource.Schema {
		schema.Path = strings.TrimSpace(schema.Path)
		if schema.Path == "" {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.schema_invalid", Severity: "error", Message: fmt.Sprintf("resource %s schema paths must not be empty", resource.Address), Address: resource.Address})
			continue
		}
		if _, exists := schemaByPath[schema.Path]; exists {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.schema_duplicate", Severity: "error", Message: fmt.Sprintf("resource %s declares duplicate schema path %s", resource.Address, schema.Path), Address: resource.Address})
			continue
		}
		schemaByPath[schema.Path] = schema
		if schema.Required && !hasAttributePath(resource.Attributes, schema.Path) {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.attribute_required", Severity: "error", Message: fmt.Sprintf("resource %s requires attribute %s", resource.Address, schema.Path), Address: resource.Address})
		}
		if schema.Updateable && (schema.Immutable || schema.CreateOnly || schema.ReplaceOnChange) {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.schema_lifecycle_conflict", Severity: "error", Message: fmt.Sprintf("resource %s schema path %s cannot be updateable and replacement-only", resource.Address, schema.Path), Address: resource.Address})
		}
		if schema.Identity && schema.Updateable {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.identity_update_unsupported", Severity: "warning", Message: fmt.Sprintf("resource %s identity path %s is marked updateable; identity-changing update semantics require mapping-specific evidence", resource.Address, schema.Path), Address: resource.Address})
		}
		value, ok := attributeValue(resource.Attributes, schema.Path)
		if ok {
			if !schemaValueMatchesType(value, schema.Type) {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.type_invalid", Severity: "error", Message: fmt.Sprintf("resource %s attribute %s does not match type %s", resource.Address, schema.Path, schema.Type), Address: resource.Address})
			}
			if len(schema.EnumValues) > 0 && !enumValueAllowed(value, schema.EnumValues) {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.enum_invalid", Severity: "error", Message: fmt.Sprintf("resource %s attribute %s is not one of the declared enum values", resource.Address, schema.Path), Address: resource.Address})
			}
			if schema.Sensitive && !redactionCovers(profileRedaction, resource.Redaction, resource.Address, schema.Path) {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.sensitive_path_unredacted", Severity: "error", Message: fmt.Sprintf("resource %s sensitive attribute %s is not covered by redaction metadata", resource.Address, schema.Path), Address: resource.Address})
			}
		}
	}
	if len(schemaByPath) > 0 {
		for path := range flattenedAttributeValues(resource.Attributes) {
			if !schemaKnowsPath(path, schemaByPath) {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.attribute_unknown", Severity: "error", Message: fmt.Sprintf("resource %s attribute %s is not declared in mapping schema", resource.Address, path), Address: resource.Address})
			}
		}
	}
	for _, binding := range resource.RequestBindings {
		if strings.TrimSpace(binding.OperationRole) == "" || strings.TrimSpace(binding.Path) == "" || strings.TrimSpace(binding.RequestPath) == "" {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.binding_invalid", Severity: "error", Message: fmt.Sprintf("resource %s request bindings require operation_role, path, and request_path", resource.Address), Address: resource.Address})
			continue
		}
		if !resourceHasOperation(resource, binding.OperationRole) {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.binding_operation_missing", Severity: "error", Message: fmt.Sprintf("resource %s request binding references missing %s operation metadata", resource.Address, binding.OperationRole), Address: resource.Address, OperationID: binding.OperationID})
		} else if strings.TrimSpace(binding.OperationID) != "" && !operationIDMatchesRole(resource, binding.OperationRole, binding.OperationID) {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.binding_operation_mismatch", Severity: "error", Message: fmt.Sprintf("resource %s request binding operation_id %s does not match %s operation metadata", resource.Address, binding.OperationID, binding.OperationRole), Address: resource.Address, OperationID: binding.OperationID})
		}
		switch strings.TrimSpace(binding.Encoding) {
		case "", "base64":
		default:
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.binding_encoding_unknown", Severity: "error", Message: fmt.Sprintf("resource %s request binding encoding %q is not supported", resource.Address, binding.Encoding), Address: resource.Address, OperationID: binding.OperationID})
		}
		if binding.Required && !hasAttributePath(resource.Attributes, binding.Path) && !responseDerivedStatePath(resource, binding.Path) {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.attribute_required", Severity: "error", Message: fmt.Sprintf("resource %s requires request-bound attribute %s", resource.Address, binding.Path), Address: resource.Address})
		}
	}
	for _, binding := range resource.ResponseBindings {
		if strings.TrimSpace(binding.OperationRole) == "" || strings.TrimSpace(binding.ResponsePath) == "" || strings.TrimSpace(binding.StatePath) == "" {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.binding_invalid", Severity: "error", Message: fmt.Sprintf("resource %s response bindings require operation_role, response_path, and state_path", resource.Address), Address: resource.Address})
			continue
		}
		if !resourceHasOperation(resource, binding.OperationRole) {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.binding_operation_missing", Severity: "error", Message: fmt.Sprintf("resource %s response binding references missing %s operation metadata", resource.Address, binding.OperationRole), Address: resource.Address, OperationID: binding.OperationID})
		} else if strings.TrimSpace(binding.OperationID) != "" && !operationIDMatchesRole(resource, binding.OperationRole, binding.OperationID) {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.binding_operation_mismatch", Severity: "error", Message: fmt.Sprintf("resource %s response binding operation_id %s does not match %s operation metadata", resource.Address, binding.OperationID, binding.OperationRole), Address: resource.Address, OperationID: binding.OperationID})
		}
		if binding.Sensitive && !redactionCovers(profileRedaction, resource.Redaction, resource.Address, binding.StatePath) {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.sensitive_path_unredacted", Severity: "error", Message: fmt.Sprintf("resource %s sensitive response state %s is not covered by redaction metadata", resource.Address, binding.StatePath), Address: resource.Address})
		}
	}
	for _, normalizer := range resource.Normalizers {
		switch strings.TrimSpace(normalizer.Kind) {
		case "json_semantic", "case_fold", "unordered_collection", "empty_null_absent_equivalent", "sensitive_placeholder":
		default:
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.normalizer_unknown", Severity: "error", Message: fmt.Sprintf("resource %s normalizer %q is not supported", resource.Address, normalizer.Kind), Address: resource.Address})
		}
	}
	return diagnostics
}

func responseDerivedStatePath(resource project.Resource, path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	for _, binding := range resource.ResponseBindings {
		if binding.ResponseDerivedIdentity && strings.TrimSpace(binding.StatePath) == path {
			return true
		}
	}
	for _, schema := range resource.Schema {
		if schema.ResponseDerivedIdentity && strings.TrimSpace(schema.Path) == path {
			return true
		}
	}
	return false
}

func validateRuntimeHints(resource project.Resource) []Diagnostic {
	var diagnostics []Diagnostic
	if resource.RuntimeHints == nil {
		return diagnostics
	}
	if attempts, ok := positiveIntHint(resource.RuntimeHints.Retry, "max_attempts"); ok && attempts < 1 {
		diagnostics = append(diagnostics, Diagnostic{Code: "validate.retry_invalid", Severity: "error", Message: fmt.Sprintf("resource %s retry.max_attempts must be positive", resource.Address), Address: resource.Address})
	}
	diagnostics = append(diagnostics, validateSettleHint(resource)...)
	untilValue, hasUntil := resource.RuntimeHints.Waiter["until"]
	if !hasUntil {
		return diagnostics
	}
	until := strings.ToLower(strings.TrimSpace(fmt.Sprint(untilValue)))
	if until == "" {
		return diagnostics
	}
	switch until {
	case "exists", "missing", "success":
	default:
		diagnostics = append(diagnostics, Diagnostic{Code: "validate.waiter_invalid", Severity: "error", Message: fmt.Sprintf("resource %s waiter.until %q is not supported", resource.Address, until), Address: resource.Address})
		return diagnostics
	}
	if attempts, ok := positiveIntHint(resource.RuntimeHints.Waiter, "max_attempts"); ok && attempts < 1 {
		diagnostics = append(diagnostics, Diagnostic{Code: "validate.waiter_invalid", Severity: "error", Message: fmt.Sprintf("resource %s waiter.max_attempts must be positive", resource.Address), Address: resource.Address})
	}
	if until == "exists" || until == "missing" {
		if !resourceHasOperation(resource, "read") {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.waiter_read_role_missing", Severity: "error", Message: fmt.Sprintf("resource %s waiter.until %s requires read operation metadata", resource.Address, until), Address: resource.Address})
		}
	}
	return diagnostics
}

func validateSettleHint(resource project.Resource) []Diagnostic {
	var diagnostics []Diagnostic
	if resource.RuntimeHints == nil || len(resource.RuntimeHints.Settle) == 0 {
		return diagnostics
	}
	settle := resource.RuntimeHints.Settle
	before := strings.ToLower(strings.TrimSpace(fmt.Sprint(settle["before"])))
	if before == "" {
		before = "delete"
	}
	if before != "delete" {
		diagnostics = append(diagnostics, Diagnostic{Code: "validate.settle_invalid", Severity: "error", Message: fmt.Sprintf("resource %s settle.before %q is not supported", resource.Address, before), Address: resource.Address})
	}
	readExpect := strings.ToLower(strings.TrimSpace(fmt.Sprint(settle["read_expect"])))
	if readExpect == "" {
		readExpect = "exists"
	}
	if readExpect != "exists" {
		diagnostics = append(diagnostics, Diagnostic{Code: "validate.settle_invalid", Severity: "error", Message: fmt.Sprintf("resource %s settle.read_expect %q is not supported", resource.Address, readExpect), Address: resource.Address})
	}
	if duration, ok := positiveDurationHint(settle, "duration"); !ok || duration <= 0 {
		diagnostics = append(diagnostics, Diagnostic{Code: "validate.settle_invalid", Severity: "error", Message: fmt.Sprintf("resource %s settle.duration must be a positive duration", resource.Address), Address: resource.Address})
	}
	if interval, ok := positiveDurationHint(settle, "interval"); !ok || interval <= 0 {
		diagnostics = append(diagnostics, Diagnostic{Code: "validate.settle_invalid", Severity: "error", Message: fmt.Sprintf("resource %s settle.interval must be a positive duration", resource.Address), Address: resource.Address})
	}
	if !resourceHasOperation(resource, "read") {
		diagnostics = append(diagnostics, Diagnostic{Code: "validate.settle_read_role_missing", Severity: "error", Message: fmt.Sprintf("resource %s settle requires read operation metadata", resource.Address), Address: resource.Address})
	}
	return diagnostics
}

func positiveIntHint(values map[string]any, key string) (int64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	value, ok := values[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), typed == float64(int64(typed))
	case float32:
		return int64(typed), typed == float32(int64(typed))
	default:
		return 0, true
	}
}

func positiveDurationHint(values map[string]any, key string) (time.Duration, bool) {
	if len(values) == 0 {
		return 0, false
	}
	value, ok := values[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case time.Duration:
		return typed, true
	case int:
		return time.Duration(typed) * time.Millisecond, true
	case int64:
		return time.Duration(typed) * time.Millisecond, true
	case float64:
		return time.Duration(typed) * time.Millisecond, typed == float64(int64(typed))
	case float32:
		return time.Duration(typed) * time.Millisecond, typed == float32(int64(typed))
	case string:
		parsed, err := time.ParseDuration(strings.TrimSpace(typed))
		if err != nil {
			return 0, true
		}
		return parsed, true
	default:
		return 0, true
	}
}

func validateOperationRequirements(resource project.Resource) []Diagnostic {
	var diagnostics []Diagnostic
	required := map[string]bool{}
	for _, role := range resource.RequiredOperations {
		if strings.TrimSpace(role) != "" {
			required[strings.TrimSpace(role)] = true
		}
	}
	if resource.MappingLifecycle != nil {
		for _, role := range resource.MappingLifecycle.OperationRoles {
			role = strings.TrimSpace(role)
			if role != "" && role != "noop" {
				required[role] = true
			}
		}
	}
	for role := range required {
		if !resourceHasOperation(resource, role) {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.operation_role_missing", Severity: "error", Message: fmt.Sprintf("resource %s requires %s operation metadata", resource.Address, role), Address: resource.Address})
		}
	}
	return diagnostics
}

func resourceHasOperation(resource project.Resource, required string) bool {
	required = strings.TrimSpace(required)
	for key, role := range resource.Operations {
		if firstNonEmpty(role.Purpose, key) == required {
			return true
		}
	}
	return false
}

func operationIDMatchesRole(resource project.Resource, required, operationID string) bool {
	required = strings.TrimSpace(required)
	operationID = strings.TrimSpace(operationID)
	for key, role := range resource.Operations {
		if firstNonEmpty(role.Purpose, key) == required && strings.TrimSpace(role.OperationID) == operationID {
			return true
		}
	}
	return false
}

func hasAttributePath(attrs map[string]any, path string) bool {
	_, ok := attributeValue(attrs, path)
	if ok {
		return true
	}
	prefix := strings.TrimSpace(path) + "."
	for candidate := range flattenedAttributeValues(attrs) {
		if strings.HasPrefix(candidate, prefix) {
			return true
		}
	}
	return false
}

func attributeValue(attrs map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	var cur any = attrs
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

func flattenedAttributeValues(attrs map[string]any) map[string]any {
	out := map[string]any{}
	var visit func(string, any)
	visit = func(prefix string, value any) {
		if m, ok := value.(map[string]any); ok && len(m) > 0 {
			for key, child := range m {
				next := key
				if prefix != "" {
					next = prefix + "." + key
				}
				visit(next, child)
			}
			return
		}
		if prefix != "" {
			out[prefix] = value
		}
	}
	for key, value := range attrs {
		visit(key, value)
	}
	return out
}

func schemaKnowsPath(path string, schemaByPath map[string]project.SchemaPath) bool {
	for schemaPath := range schemaByPath {
		if path == schemaPath || strings.HasPrefix(path, schemaPath+".") || strings.HasPrefix(schemaPath, path+".") {
			return true
		}
	}
	return false
}

func schemaValueMatchesType(value any, typeName string) bool {
	switch strings.ToLower(strings.TrimSpace(typeName)) {
	case "":
		return true
	case "string":
		_, ok := value.(string)
		return ok
	case "bool", "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		switch value.(type) {
		case int, int64, float64, float32:
			return true
		default:
			return false
		}
	case "integer":
		switch v := value.(type) {
		case int, int64:
			return true
		case float64:
			return v == float64(int64(v))
		case float32:
			return v == float32(int64(v))
		default:
			return false
		}
	case "object", "map":
		_, ok := value.(map[string]any)
		return ok
	case "array", "list", "set":
		_, ok := value.([]any)
		return ok
	default:
		return true
	}
}

func enumValueAllowed(value any, allowed []string) bool {
	valueString, ok := value.(string)
	if !ok {
		return false
	}
	for _, allowedValue := range allowed {
		if valueString == allowedValue {
			return true
		}
	}
	return false
}

func redactionCovers(profileRedaction, resourceRedaction project.Redaction, address, path string) bool {
	targets := map[string]bool{strings.TrimSpace(path): true}
	if strings.TrimSpace(address) != "" && strings.TrimSpace(path) != "" {
		targets[strings.TrimSpace(address)+"."+strings.TrimSpace(path)] = true
	}
	for _, redaction := range []project.Redaction{resourceRedaction, profileRedaction} {
		for _, candidate := range redaction.Paths {
			if targets[strings.TrimSpace(candidate)] {
				return true
			}
		}
	}
	return false
}

type sourceDoc struct {
	Kind       string
	ID         string
	Path       string
	Operations map[string]bool
	Browser    *browsercontract.Profile
}

func loadAPISources(ctx context.Context, projectDir string, inputs []APISourceInput) ([]sourceDoc, []Diagnostic) {
	var docs []sourceDoc
	var diagnostics []Diagnostic
	seen := map[string]bool{}
	for _, input := range inputs {
		input.Kind = normalizeAPISourceKind(input.Kind)
		input.ID = strings.TrimSpace(input.ID)
		input.Path = strings.TrimSpace(input.Path)
		if input.Kind == "" || input.ID == "" || input.Path == "" {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.api_source_invalid", Severity: "error", Message: "API source inputs require kind, id, and path", APISourceKind: input.Kind, APISourceID: input.ID})
			continue
		}
		key := input.Kind + "\x00" + input.ID
		if seen[key] {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.api_source_duplicate", Severity: "error", Message: fmt.Sprintf("API source %s:%s is duplicated", input.Kind, input.ID), APISourceKind: input.Kind, APISourceID: input.ID})
			continue
		}
		seen[key] = true
		if input.Kind == string(uws1.SourceDescriptionTypeBrowserProfile) {
			profile, err := browsercontract.LoadProfile(projectDir, input.Path)
			if err != nil {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.api_source_load_error", Severity: "error", Message: err.Error(), APISourceKind: input.Kind, APISourceID: input.ID})
				continue
			}
			operations := make(map[string]bool, len(profile.Actions))
			for operationID := range profile.Actions {
				operations[operationID] = true
			}
			docs = append(docs, sourceDoc{Kind: input.Kind, ID: input.ID, Path: profile.Path, Operations: operations, Browser: profile})
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
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.api_source_load_error", Severity: "error", Message: err.Error(), APISourceKind: input.Kind, APISourceID: input.ID})
			continue
		}
		for _, diag := range inventory.Diagnostics {
			// Prompt-only sanitization diagnostics remain available to authoring
			// callers. Runtime project validation consumes the bounded operation
			// index, not prompt prose, so those diagnostics do not make an otherwise
			// valid API source noisy or invalid here.
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(diag.Code)), "prompt.") {
				continue
			}
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.api_source_" + strings.ReplaceAll(diag.Code, ".", "_"), Severity: normalizeSeverity(diag.Severity), Message: diag.Message, APISourceKind: input.Kind, APISourceID: input.ID})
		}
		ops := map[string]bool{}
		for _, op := range inventory.Operations {
			if strings.TrimSpace(op.OperationID) != "" {
				ops[op.OperationID] = true
			}
		}
		docs = append(docs, sourceDoc{Kind: input.Kind, ID: input.ID, Path: input.Path, Operations: ops})
	}
	slices.SortFunc(docs, func(a, b sourceDoc) int {
		if diff := cmp.Compare(a.Kind, b.Kind); diff != 0 {
			return diff
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return docs, diagnostics
}

func validateOperations(doc *project.Document, sources []sourceDoc) []Diagnostic {
	var diagnostics []Diagnostic
	for _, resource := range doc.Profile.Resources {
		for purpose, role := range resource.Operations {
			rolePurpose := firstNonEmpty(role.Purpose, purpose)
			roleKind := normalizeAPISourceKind(role.SourceKind)
			if roleKind == "" {
				roleKind = strings.TrimSpace(role.SourceKind)
			}
			var matchedSource *sourceDoc
			for i := range sources {
				source := &sources[i]
				if source.Kind == roleKind && source.ID == strings.TrimSpace(role.SourceID) {
					matchedSource = source
					break
				}
			}
			if matchedSource == nil {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.source_reference_missing", Severity: "error", Message: fmt.Sprintf("resource %s %s operation references missing API source %s:%s", resource.Address, rolePurpose, role.SourceKind, role.SourceID), Address: resource.Address, APISourceKind: role.SourceKind, APISourceID: role.SourceID, OperationID: role.OperationID})
				continue
			}
			if strings.TrimSpace(role.OperationID) == "" {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.operation_missing", Severity: "error", Message: fmt.Sprintf("resource %s %s operation requires operation_id", resource.Address, rolePurpose), Address: resource.Address, APISourceKind: role.SourceKind, APISourceID: role.SourceID})
				continue
			}
			if !matchedSource.Operations[role.OperationID] {
				diagnostics = append(diagnostics, Diagnostic{Code: "validate.operation_unknown", Severity: "error", Message: fmt.Sprintf("operation %s for resource %s was not found in API source %s:%s", role.OperationID, resource.Address, role.SourceKind, role.SourceID), Address: resource.Address, APISourceKind: role.SourceKind, APISourceID: role.SourceID, OperationID: role.OperationID})
				continue
			}
			if matchedSource.Browser != nil {
				_, err := browsercontract.ValidateRole(doc, resource, rolePurpose, role, project.APISource{Kind: matchedSource.Kind, ID: matchedSource.ID, Path: matchedSource.Path}, matchedSource.Browser)
				if err != nil {
					diagnostics = append(diagnostics, Diagnostic{Code: "validate.browser_contract_invalid", Severity: "error", Message: err.Error(), Address: resource.Address, APISourceKind: role.SourceKind, APISourceID: role.SourceID, OperationID: role.OperationID})
				}
			}
		}
	}
	return diagnostics
}

func unusedSourceDiagnostics(profile project.Profile) []Diagnostic {
	used := map[string]bool{}
	for _, resource := range profile.Resources {
		for _, role := range resource.Operations {
			used[sourceKey(role.SourceKind, role.SourceID)] = true
		}
	}
	var diagnostics []Diagnostic
	for _, source := range profile.APISources {
		if !used[sourceKey(source.Kind, source.ID)] {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.api_source_unused", Severity: "warning", Message: fmt.Sprintf("API source %s:%s is not referenced by any resource operation", source.Kind, source.ID), APISourceKind: source.Kind, APISourceID: source.ID})
		}
	}
	return diagnostics
}

func redactionDiagnostics(redaction project.Redaction, address string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, path := range redaction.Paths {
		if strings.TrimSpace(path) == "" {
			diagnostics = append(diagnostics, Diagnostic{Code: "validate.redaction_invalid", Severity: "error", Message: "redaction paths must not be empty", Address: address})
		}
	}
	return diagnostics
}

func mergeAPISources(profile project.Profile, overrides []APISourceInput) []APISourceInput {
	byKey := map[string]APISourceInput{}
	for _, source := range profile.APISources {
		input := APISourceInput{Kind: normalizeAPISourceKind(source.Kind), ID: strings.TrimSpace(source.ID), Path: source.Path}
		byKey[sourceKey(input.Kind, input.ID)] = input
	}
	for _, override := range overrides {
		override.Kind = normalizeAPISourceKind(override.Kind)
		override.ID = strings.TrimSpace(override.ID)
		override.Path = strings.TrimSpace(override.Path)
		byKey[sourceKey(override.Kind, override.ID)] = override
	}
	inputs := make([]APISourceInput, 0, len(byKey))
	for _, input := range byKey {
		inputs = append(inputs, input)
	}
	slices.SortFunc(inputs, func(a, b APISourceInput) int {
		if diff := cmp.Compare(a.Kind, b.Kind); diff != 0 {
			return diff
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return inputs
}

func sourceKey(kind, id string) string {
	normalizedKind := normalizeAPISourceKind(kind)
	if normalizedKind == "" {
		normalizedKind = strings.TrimSpace(kind)
	}
	return normalizedKind + "\x00" + strings.TrimSpace(id)
}

func finalize(result *Result) {
	slices.SortFunc(result.Diagnostics, func(a, b Diagnostic) int {
		left := []string{a.Severity, a.Code, a.Path, a.Address, a.APISourceKind, a.APISourceID, a.OperationID, a.Message}
		right := []string{b.Severity, b.Code, b.Path, b.Address, b.APISourceKind, b.APISourceID, b.OperationID, b.Message}
		return slices.Compare(left, right)
	})
	for _, diag := range result.Diagnostics {
		switch diag.Severity {
		case "error":
			result.Summary.Errors++
		case "warning":
			result.Summary.Warnings++
		}
	}
	result.Summary.Diagnostics = len(result.Diagnostics)
	result.Valid = result.Summary.Errors == 0
}

func knownOperationPurpose(purpose string) bool {
	switch strings.TrimSpace(purpose) {
	case "read", "create", "update", "delete", "post", "put", "patch", "import", "replace", "suspend", "detach", "disable", "remove_config", "noop":
		return true
	default:
		return false
	}
}

func normalizeAPISourceKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "openapi", "openapi3", "swagger":
		return "openapi"
	case "aws-smithy", "smithy", "aws":
		return "aws-smithy"
	case "google-discovery", "google", "discovery":
		return "google-discovery"
	case "asyncapi":
		return "asyncapi"
	case "graphql":
		return "graphql"
	case "openrpc":
		return "openrpc"
	case "grpc-protobuf", "grpc_protobuf", "protobuf", "proto":
		return "grpc-protobuf"
	case "odata":
		return "odata"
	case "browser-profile":
		return "browser-profile"
	default:
		return ""
	}
}

func normalizeSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "error":
		return "error"
	case "warning", "warn":
		return "warning"
	default:
		return "info"
	}
}

func packageAPISourcePath(kind, id, sourcePath string) string {
	name := strings.TrimSpace(id)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	}
	ext := filepath.Ext(sourcePath)
	if ext == "" {
		ext = ".json"
	}
	return filepath.ToSlash(filepath.Join(kind, name+ext))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
