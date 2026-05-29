package tfmapping

import (
	"sort"
	"strings"
)

const (
	APISourceKindOpenAPI         = "openapi"
	APISourceKindAWSSmithy       = "aws-smithy"
	APISourceKindGoogleDiscovery = "google-discovery"
)

type Object struct {
	Kind     string
	Type     string
	Provider string
}

type IdentityAttribute struct {
	Name          string   `json:"name"`
	TerraformPath string   `json:"terraform_path"`
	RequestKeys   []string `json:"request_keys,omitempty"`
	ResponsePaths []string `json:"response_paths,omitempty"`
	Required      bool     `json:"required,omitempty"`
}

type DiagnosticCode string

const (
	DiagnosticCodeUnsupportedProvider DiagnosticCode = "mapping.unsupported_provider"
	DiagnosticCodeUnsupportedType     DiagnosticCode = "mapping.unsupported_type"
	DiagnosticCodeUnsupportedAction   DiagnosticCode = "mapping.unsupported_action"
	DiagnosticCodeMissingIdentity     DiagnosticCode = "mapping.missing_identity"
	DiagnosticCodeFallbackOnly        DiagnosticCode = "mapping.fallback_only"
)

type DiagnosticSeverity string

const (
	DiagnosticSeverityError   DiagnosticSeverity = "error"
	DiagnosticSeverityWarning DiagnosticSeverity = "warning"
	DiagnosticSeverityInfo    DiagnosticSeverity = "info"
)

type Diagnostic struct {
	Code     DiagnosticCode     `json:"code"`
	Severity DiagnosticSeverity `json:"severity"`
	Message  string             `json:"message"`
}

type OperationTarget struct {
	SourceKinds  []string `json:"source_kinds,omitempty"`
	SourceIDs    []string `json:"source_ids,omitempty"`
	OperationIDs []string `json:"operation_ids,omitempty"`
}

type Mapping struct {
	Object             Object              `json:"object"`
	Purpose            string              `json:"purpose"`
	Action             string              `json:"action"`
	Target             OperationTarget     `json:"target,omitempty"`
	IdentityAttributes []IdentityAttribute `json:"identity_attributes,omitempty"`
	Diagnostics        []Diagnostic        `json:"diagnostics,omitempty"`
}

type ProviderMapper interface {
	MapObject(obj Object, purpose, action string) Mapping
}

// SupportedType describes one Terraform type that a mapper can resolve to API
// source operations, and which object kinds it supports.
type SupportedType struct {
	Provider string   `json:"provider"`
	Type     string   `json:"type"`
	Kinds    []string `json:"kinds"`
}

// typeLister is an optional interface a ProviderMapper may implement to report
// the Terraform types it maps. Registry.SupportedTypes uses it to enumerate
// coverage without callers duplicating the hardcoded mapping list.
type typeLister interface {
	SupportedTypes() []SupportedType
}

type RegistryOption func(*Registry)

type Registry struct {
	providerMappers map[string]ProviderMapper
}

func NewRegistry(opts ...RegistryOption) Registry {
	registry := Registry{}
	for _, opt := range opts {
		if opt != nil {
			opt(&registry)
		}
	}
	return registry
}

func WithProviderMapper(provider string, mapper ProviderMapper) RegistryOption {
	return func(registry *Registry) {
		provider = normalizeProviderName(provider)
		if provider == "" {
			return
		}
		if registry.providerMappers == nil {
			registry.providerMappers = map[string]ProviderMapper{}
		}
		registry.providerMappers[provider] = mapper
	}
}

func DefaultRegistry() Registry {
	return NewRegistry()
}

func (Registry) IsProviderLocalDataSource(obj Object) bool {
	if objectProviderLocalName(obj) != "aws" || obj.Kind != "data_source" {
		return false
	}
	switch obj.Type {
	case "aws_iam_policy_document", "aws_partition", "aws_region":
		return true
	default:
		return false
	}
}

func (r Registry) OperationTarget(obj Object, purpose, action string) OperationTarget {
	return r.MapObject(obj, purpose, action).Target
}

func (r Registry) MapObject(obj Object, purpose, action string) Mapping {
	obj = normalizeObject(obj)
	purpose = normalizeToken(purpose)
	action = normalizeToken(action)
	provider := objectProviderLocalName(obj)
	if mapper, ok := r.providerMappers[provider]; ok {
		if mapper == nil {
			return unsupportedProviderMapping(obj, purpose, action, provider)
		}
		return mapper.MapObject(obj, purpose, action)
	}
	switch provider {
	case "aws":
		return awsMapper{}.MapObject(obj, purpose, action)
	case "google":
		return googleMapper{}.MapObject(obj, purpose, action)
	default:
		return unsupportedProviderMapping(obj, purpose, action, provider)
	}
}

// SupportedTypes enumerates every Terraform type the registry can map, drawing
// from registered provider mappers that implement typeLister and from the
// built-in aws/google mappers for providers that are not overridden. The result
// is deduplicated and sorted by provider then type.
func (r Registry) SupportedTypes() []SupportedType {
	var out []SupportedType
	seen := map[string]bool{}
	add := func(types []SupportedType) {
		for _, t := range types {
			key := t.Provider + "\x00" + t.Type
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, t)
		}
	}
	for _, mapper := range r.providerMappers {
		if lister, ok := mapper.(typeLister); ok {
			add(lister.SupportedTypes())
		}
	}
	if _, ok := r.providerMappers["aws"]; !ok {
		add(awsMapper{}.SupportedTypes())
	}
	if _, ok := r.providerMappers["google"]; !ok {
		add(googleMapper{}.SupportedTypes())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Type < out[j].Type
	})
	return out
}

func (Registry) RequestKeys(obj Object, sourceKind, operationID, attrPath string) []string {
	attrPath = strings.TrimSpace(attrPath)
	if attrPath == "" {
		return nil
	}
	switch objectProviderLocalName(obj) {
	case "aws":
		switch operationID {
		case "CreateBucket":
			if attrPath == "bucket" {
				return []string{"Bucket"}
			}
		case "PutBucketAccelerateConfiguration":
			switch attrPath {
			case "bucket":
				return []string{"Bucket"}
			case "status":
				return []string{"Status"}
			}
		case "GetBucketLocation", "HeadBucket":
			if attrPath == "bucket" {
				return []string{"Bucket"}
			}
		case "CreateFunction":
			switch attrPath {
			case "function_name":
				return []string{"FunctionName"}
			case "role":
				return []string{"Role"}
			case "handler":
				return []string{"Handler"}
			case "runtime":
				return []string{"Runtime"}
			}
		case "CreateFunctionUrlConfig", "UpdateFunctionUrlConfig", "DeleteFunctionUrlConfig":
			switch attrPath {
			case "function_name":
				return []string{"FunctionName"}
			case "authorization_type":
				return []string{"authorization_type"}
			}
		case "CreateRole", "POST_CreateRole":
			switch attrPath {
			case "name":
				return []string{"RoleName"}
			case "assume_role_policy":
				return []string{"AssumeRolePolicyDocument"}
			}
		case "GetRole", "POST_GetRole", "DeleteRole", "POST_DeleteRole":
			if attrPath == "name" {
				return []string{"RoleName"}
			}
		case "UpdateRole", "POST_UpdateRole":
			switch attrPath {
			case "name":
				return []string{"RoleName"}
			case "description":
				return []string{"Description"}
			}
		case "PutRolePolicy", "POST_PutRolePolicy":
			switch attrPath {
			case "role":
				return []string{"RoleName"}
			case "name":
				return []string{"PolicyName"}
			case "policy":
				return []string{"PolicyDocument"}
			}
		case "DeleteRolePolicy", "POST_DeleteRolePolicy":
			switch attrPath {
			case "role":
				return []string{"RoleName"}
			case "name":
				return []string{"PolicyName"}
			}
		}
	case "google":
		if sourceKind == APISourceKindGoogleDiscovery {
			switch operationID {
			case "storage.buckets.insert":
				switch attrPath {
				case "project":
					return []string{"project"}
				case "name":
					return []string{"name"}
				case "location":
					return []string{"location"}
				}
			case "storage.buckets.get", "storage.buckets.patch", "storage.buckets.delete":
				switch attrPath {
				case "name":
					return []string{"bucket"}
				case "project":
					return []string{"project"}
				case "location":
					return []string{"location"}
				}
			}
		}
	}
	return nil
}

func (Registry) StaticRequestBindings(obj Object, sourceID, sourcePath, operationID string) map[string]string {
	if objectProviderLocalName(obj) != "aws" {
		return nil
	}
	action := AWSQueryProtocolAction(operationID)
	version := awsQueryProtocolVersion(sourceID, sourcePath)
	if action == "" || version == "" {
		return nil
	}
	return map[string]string{
		"Action":  action,
		"Version": version,
	}
}

func AWSQueryProtocolAction(operationID string) string {
	operationID = strings.TrimSpace(operationID)
	for _, prefix := range []string{"GET_", "POST_"} {
		if strings.HasPrefix(operationID, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(operationID, prefix))
		}
	}
	return ""
}

type awsMapper struct{}

func (awsMapper) MapObject(obj Object, purpose, action string) Mapping {
	mapping := Mapping{Object: obj, Purpose: purpose, Action: action}
	switch obj.Type {
	case "aws_s3_bucket":
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = awsOperationTarget("s3", "CreateBucket")
			return mapping
		}
		if obj.Kind == "data_source" && purpose == "read" {
			mapping.Target = awsOperationTarget("s3", "GetBucketLocation")
			return mapping
		}
		if obj.Kind == "data_source" && purpose == "list" {
			mapping.Target = awsOperationTarget("s3", "ListBuckets")
			return mapping
		}
	case "aws_s3_bucket_accelerate_configuration":
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = awsOperationTarget("s3", "PutBucketAccelerateConfiguration")
			return mapping
		}
	case "aws_caller_identity":
		if obj.Kind == "data_source" && purpose == "read" {
			mapping.Target = awsOperationTarget("sts", "POST_GetCallerIdentity")
			return mapping
		}
	case "aws_iam_role":
		if obj.Kind != "resource" {
			return unsupportedActionMapping(mapping, "AWS IAM role mapping supports managed resources")
		}
		mapping.IdentityAttributes = []IdentityAttribute{{
			Name:          "role_name",
			TerraformPath: "name",
			RequestKeys:   []string{"RoleName"},
			ResponsePaths: []string{"Role.RoleName", "Role.Arn"},
			Required:      true,
		}}
		if purpose == "read" {
			mapping.Target = awsOperationTarget("iam", "POST_GetRole")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = awsOperationTarget("iam", "POST_CreateRole")
			return mapping
		}
		if purpose == "update" && (action == "update" || action == "replace") {
			mapping.Target = awsOperationTarget("iam", "POST_UpdateRole")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "delete" {
			mapping.Target = awsOperationTarget("iam", "POST_DeleteRole")
			return mapping
		}
		return unsupportedActionMapping(mapping, "AWS IAM role mapping supports read, create, update, and delete")
	case "aws_iam_role_policy":
		if obj.Kind == "resource" && (purpose == "create" || purpose == "update") && (action == "create" || action == "update" || action == "replace") {
			mapping.Target = awsOperationTarget("iam", "POST_PutRolePolicy")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "delete" {
			mapping.Target = awsOperationTarget("iam", "POST_DeleteRolePolicy")
			return mapping
		}
	case "aws_lambda_function":
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = awsOperationTarget("lambda", "CreateFunction")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "delete" {
			mapping.Target = awsOperationTarget("lambda", "DeleteFunction")
			return mapping
		}
	case "aws_lambda_function_url":
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = awsOperationTarget("lambda", "CreateFunctionUrlConfig")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "update" {
			mapping.Target = awsOperationTarget("lambda", "UpdateFunctionUrlConfig")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "delete" {
			mapping.Target = awsOperationTarget("lambda", "DeleteFunctionUrlConfig")
			return mapping
		}
	default:
		return unsupportedTypeMapping(mapping, "AWS")
	}
	return unsupportedActionMapping(mapping, "AWS mapping does not support this object kind, purpose, and action")
}

// SupportedTypes must track the type switch in awsMapper.MapObject.
func (awsMapper) SupportedTypes() []SupportedType {
	return []SupportedType{
		{Provider: "aws", Type: "aws_s3_bucket", Kinds: []string{"resource", "data_source"}},
		{Provider: "aws", Type: "aws_s3_bucket_accelerate_configuration", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_caller_identity", Kinds: []string{"data_source"}},
		{Provider: "aws", Type: "aws_iam_role", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_iam_role_policy", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_lambda_function", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_lambda_function_url", Kinds: []string{"resource"}},
	}
}

type googleMapper struct{}

func (googleMapper) MapObject(obj Object, purpose, action string) Mapping {
	mapping := Mapping{Object: obj, Purpose: purpose, Action: action}
	switch obj.Type {
	case "google_storage_bucket":
		if obj.Kind != "resource" && obj.Kind != "data_source" {
			return unsupportedActionMapping(mapping, "Google Storage bucket mapping supports managed resources and data sources")
		}
		mapping.IdentityAttributes = []IdentityAttribute{{
			Name:          "bucket_name",
			TerraformPath: "name",
			RequestKeys:   []string{"name"},
			ResponsePaths: []string{"name", "id"},
			Required:      true,
		}}
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = googleOperationTarget("storage", "storage.buckets.insert")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "read" {
			mapping.Target = googleOperationTarget("storage", "storage.buckets.get")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "update" && (action == "update" || action == "replace") {
			mapping.Target = googleOperationTarget("storage", "storage.buckets.patch")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "delete" {
			mapping.Target = googleOperationTarget("storage", "storage.buckets.delete")
			return mapping
		}
		if obj.Kind == "data_source" && purpose == "read" {
			mapping.Target = googleOperationTarget("storage", "storage.buckets.get")
			return mapping
		}
		return unsupportedActionMapping(mapping, "Google Storage bucket mapping supports read, create, update, and delete")
	default:
		return unsupportedTypeMapping(mapping, "Google")
	}
}

// SupportedTypes must track the type switch in googleMapper.MapObject.
func (googleMapper) SupportedTypes() []SupportedType {
	return []SupportedType{
		{Provider: "google", Type: "google_storage_bucket", Kinds: []string{"resource", "data_source"}},
	}
}

func awsOperationTarget(service, operationID string) OperationTarget {
	operationIDs := []string{operationID}
	if strings.HasPrefix(operationID, "POST_") || strings.HasPrefix(operationID, "GET_") {
		operationIDs = append(operationIDs, AWSQueryProtocolAction(operationID))
	}
	return OperationTarget{
		SourceKinds:  []string{APISourceKindAWSSmithy, APISourceKindOpenAPI},
		SourceIDs:    []string{service, "aws-" + service, "aws_" + service, "aws-" + service + "-smithy-model"},
		OperationIDs: operationIDs,
	}
}

func googleOperationTarget(service, operationID string) OperationTarget {
	return OperationTarget{
		SourceKinds:  []string{APISourceKindGoogleDiscovery, APISourceKindOpenAPI},
		SourceIDs:    []string{service, "google-" + service, "google-cloud-" + service, "google-cloud-" + service + "-discovery-v1"},
		OperationIDs: []string{operationID, normalizeName(operationID)},
	}
}

func awsQueryProtocolVersion(sourceID, sourcePath string) string {
	normalized := strings.ToLower(strings.Join([]string{sourceID, sourcePath}, " "))
	switch {
	case strings.Contains(normalized, "iam"):
		return "2010-05-08"
	case strings.Contains(normalized, "sts"):
		return "2011-06-15"
	default:
		return ""
	}
}

func objectProviderLocalName(obj Object) string {
	provider := normalizeProviderName(obj.Provider)
	if provider == "" {
		if before, _, ok := strings.Cut(obj.Type, "_"); ok {
			return before
		}
		return ""
	}
	before, _, _ := strings.Cut(provider, ".")
	return before
}

func normalizeObject(obj Object) Object {
	obj.Kind = normalizeToken(obj.Kind)
	obj.Type = strings.TrimSpace(obj.Type)
	obj.Provider = strings.TrimSpace(obj.Provider)
	return obj
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeProviderName(provider string) string {
	provider = strings.TrimPrefix(strings.TrimSpace(provider), "provider.")
	before, _, _ := strings.Cut(provider, ".")
	return before
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

func unsupportedProviderMapping(obj Object, purpose, action, provider string) Mapping {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = "unknown"
	}
	return Mapping{
		Object:  obj,
		Purpose: purpose,
		Action:  action,
		Diagnostics: []Diagnostic{{
			Code:     DiagnosticCodeUnsupportedProvider,
			Severity: DiagnosticSeverityWarning,
			Message:  "no Ramen mapping is registered for provider " + provider,
		}},
	}
}

func unsupportedTypeMapping(mapping Mapping, provider string) Mapping {
	mapping.Diagnostics = append(mapping.Diagnostics, Diagnostic{
		Code:     DiagnosticCodeUnsupportedType,
		Severity: DiagnosticSeverityWarning,
		Message:  provider + " mapping does not support Terraform type " + mapping.Object.Type,
	})
	return mapping
}

func unsupportedActionMapping(mapping Mapping, message string) Mapping {
	mapping.Diagnostics = append(mapping.Diagnostics, Diagnostic{
		Code:     DiagnosticCodeUnsupportedAction,
		Severity: DiagnosticSeverityWarning,
		Message:  message,
	})
	return mapping
}
