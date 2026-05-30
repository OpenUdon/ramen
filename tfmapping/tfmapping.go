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
	Schema             []SchemaPath        `json:"schema,omitempty"`
	RequestBindings    []RequestBinding    `json:"request_bindings,omitempty"`
	ResponseBindings   []ResponseBinding   `json:"response_bindings,omitempty"`
	Normalizers        []Normalizer        `json:"normalizers,omitempty"`
	Lifecycle          *LifecycleSemantics `json:"lifecycle,omitempty"`
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
	case "azurerm":
		return azureRMMapper{}.MapObject(obj, purpose, action)
	case "cloudflare":
		return cloudflareMapper{}.MapObject(obj, purpose, action)
	case "google":
		return googleMapper{}.MapObject(obj, purpose, action)
	case "kubernetes":
		return kubernetesMapper{}.MapObject(obj, purpose, action)
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
	if _, ok := r.providerMappers["azurerm"]; !ok {
		add(azureRMMapper{}.SupportedTypes())
	}
	if _, ok := r.providerMappers["cloudflare"]; !ok {
		add(cloudflareMapper{}.SupportedTypes())
	}
	if _, ok := r.providerMappers["google"]; !ok {
		add(googleMapper{}.SupportedTypes())
	}
	if _, ok := r.providerMappers["kubernetes"]; !ok {
		add(kubernetesMapper{}.SupportedTypes())
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
		case "PutPublicAccessBlock", "GetPublicAccessBlock", "DeletePublicAccessBlock":
			switch attrPath {
			case "bucket":
				return []string{"Bucket"}
			case "block_public_acls":
				return []string{"PublicAccessBlockConfiguration.BlockPublicAcls"}
			case "block_public_policy":
				return []string{"PublicAccessBlockConfiguration.BlockPublicPolicy"}
			case "ignore_public_acls":
				return []string{"PublicAccessBlockConfiguration.IgnorePublicAcls"}
			case "restrict_public_buckets":
				return []string{"PublicAccessBlockConfiguration.RestrictPublicBuckets"}
			}
		case "PutBucketVersioning", "GetBucketVersioning":
			switch attrPath {
			case "bucket":
				return []string{"Bucket"}
			case "versioning_configuration.status", "versioning_configuration.0.status":
				return []string{"VersioningConfiguration.Status"}
			case "versioning_configuration.mfa_delete", "versioning_configuration.0.mfa_delete":
				return []string{"VersioningConfiguration.MFADelete"}
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
		case "AddPermission":
			switch attrPath {
			case "function_name":
				return []string{"FunctionName"}
			case "statement_id":
				return []string{"StatementId"}
			case "action":
				return []string{"Action"}
			case "principal":
				return []string{"Principal"}
			case "source_arn":
				return []string{"SourceArn"}
			case "source_account":
				return []string{"SourceAccount"}
			case "event_source_token":
				return []string{"EventSourceToken"}
			case "qualifier":
				return []string{"Qualifier"}
			case "principal_org_id":
				return []string{"PrincipalOrgID"}
			case "function_url_auth_type":
				return []string{"FunctionUrlAuthType"}
			case "invoked_via_function_url":
				return []string{"InvokedViaFunctionUrl"}
			}
		case "RemovePermission":
			switch attrPath {
			case "function_name":
				return []string{"FunctionName"}
			case "statement_id":
				return []string{"StatementId"}
			case "qualifier":
				return []string{"Qualifier"}
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
		case "CreateUser", "POST_CreateUser":
			switch attrPath {
			case "name":
				return []string{"UserName"}
			case "path":
				return []string{"Path"}
			case "permissions_boundary":
				return []string{"PermissionsBoundary"}
			}
		case "GetUser", "POST_GetUser", "DeleteUser", "POST_DeleteUser":
			if attrPath == "name" {
				return []string{"UserName"}
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
	case "azurerm":
		if sourceKind == APISourceKindOpenAPI {
			switch operationID {
			case "DatabaseAccounts_CreateOrUpdate", "DatabaseAccounts_Get", "DatabaseAccounts_Update", "DatabaseAccounts_Delete":
				switch attrPath {
				case "name":
					return []string{"accountName"}
				case "resource_group_name":
					return []string{"resourceGroupName"}
				case "location":
					return []string{"createUpdateParameters.location", "updateParameters.location"}
				case "kind":
					return []string{"createUpdateParameters.kind", "updateParameters.kind"}
				case "offer_type":
					return []string{"createUpdateParameters.properties.databaseAccountOfferType"}
				}
			}
		}
	case "kubernetes":
		if sourceKind == APISourceKindOpenAPI {
			switch operationID {
			case "createCoreV1Namespace":
				switch attrPath {
				case "metadata.name", "metadata.0.name":
					return []string{"metadata.name"}
				case "metadata.annotations", "metadata.0.annotations":
					return []string{"metadata.annotations"}
				case "metadata.labels", "metadata.0.labels":
					return []string{"metadata.labels"}
				}
			case "replaceCoreV1Namespace", "patchCoreV1Namespace":
				switch attrPath {
				case "metadata.name", "metadata.0.name":
					return []string{"name", "metadata.name"}
				case "metadata.annotations", "metadata.0.annotations":
					return []string{"metadata.annotations"}
				case "metadata.labels", "metadata.0.labels":
					return []string{"metadata.labels"}
				}
			case "readCoreV1Namespace", "deleteCoreV1Namespace":
				switch attrPath {
				case "metadata.name", "metadata.0.name":
					return []string{"name"}
				}
			case "createCoreV1NamespacedConfigMap", "createCoreV1NamespacedSecret", "createCoreV1NamespacedServiceAccount":
				switch attrPath {
				case "metadata.name", "metadata.0.name":
					return []string{"metadata.name"}
				case "metadata.namespace", "metadata.0.namespace":
					return []string{"namespace", "metadata.namespace"}
				case "metadata.annotations", "metadata.0.annotations":
					return []string{"metadata.annotations"}
				case "metadata.labels", "metadata.0.labels":
					return []string{"metadata.labels"}
				case "data":
					if operationID == "createCoreV1NamespacedConfigMap" || operationID == "createCoreV1NamespacedSecret" {
						return []string{"data"}
					}
				case "string_data", "stringData":
					if operationID == "createCoreV1NamespacedSecret" {
						return []string{"stringData"}
					}
				case "type":
					if operationID == "createCoreV1NamespacedSecret" {
						return []string{"type"}
					}
				case "binary_data", "binaryData":
					if operationID == "createCoreV1NamespacedConfigMap" {
						return []string{"binaryData"}
					}
				case "automount_service_account_token", "automountServiceAccountToken":
					if operationID == "createCoreV1NamespacedServiceAccount" {
						return []string{"automountServiceAccountToken"}
					}
				}
			case "replaceCoreV1NamespacedConfigMap", "replaceCoreV1NamespacedSecret", "replaceCoreV1NamespacedServiceAccount":
				switch attrPath {
				case "metadata.name", "metadata.0.name":
					return []string{"name", "metadata.name"}
				case "metadata.namespace", "metadata.0.namespace":
					return []string{"namespace", "metadata.namespace"}
				case "metadata.annotations", "metadata.0.annotations":
					return []string{"metadata.annotations"}
				case "metadata.labels", "metadata.0.labels":
					return []string{"metadata.labels"}
				case "data":
					if operationID == "replaceCoreV1NamespacedConfigMap" || operationID == "replaceCoreV1NamespacedSecret" {
						return []string{"data"}
					}
				case "string_data", "stringData":
					if operationID == "replaceCoreV1NamespacedSecret" {
						return []string{"stringData"}
					}
				case "type":
					if operationID == "replaceCoreV1NamespacedSecret" {
						return []string{"type"}
					}
				case "binary_data", "binaryData":
					if operationID == "replaceCoreV1NamespacedConfigMap" {
						return []string{"binaryData"}
					}
				case "automount_service_account_token", "automountServiceAccountToken":
					if operationID == "replaceCoreV1NamespacedServiceAccount" {
						return []string{"automountServiceAccountToken"}
					}
				}
			case "readCoreV1NamespacedConfigMap", "deleteCoreV1NamespacedConfigMap", "readCoreV1NamespacedSecret", "deleteCoreV1NamespacedSecret", "readCoreV1NamespacedServiceAccount", "deleteCoreV1NamespacedServiceAccount":
				switch attrPath {
				case "metadata.name", "metadata.0.name":
					return []string{"name"}
				case "metadata.namespace", "metadata.0.namespace":
					return []string{"namespace"}
				}
			case "createRbacAuthorizationV1NamespacedRole":
				switch attrPath {
				case "metadata.name", "metadata.0.name":
					return []string{"metadata.name"}
				case "metadata.namespace", "metadata.0.namespace":
					return []string{"namespace", "metadata.namespace"}
				case "metadata.annotations", "metadata.0.annotations":
					return []string{"metadata.annotations"}
				case "metadata.labels", "metadata.0.labels":
					return []string{"metadata.labels"}
				case "rule", "rules":
					return []string{"rules"}
				}
			case "replaceRbacAuthorizationV1NamespacedRole":
				switch attrPath {
				case "metadata.name", "metadata.0.name":
					return []string{"name", "metadata.name"}
				case "metadata.namespace", "metadata.0.namespace":
					return []string{"namespace", "metadata.namespace"}
				case "metadata.annotations", "metadata.0.annotations":
					return []string{"metadata.annotations"}
				case "metadata.labels", "metadata.0.labels":
					return []string{"metadata.labels"}
				case "rule", "rules":
					return []string{"rules"}
				}
			case "readRbacAuthorizationV1NamespacedRole", "deleteRbacAuthorizationV1NamespacedRole":
				switch attrPath {
				case "metadata.name", "metadata.0.name":
					return []string{"name"}
				case "metadata.namespace", "metadata.0.namespace":
					return []string{"namespace"}
				}
			}
		}
	case "cloudflare":
		if sourceKind == APISourceKindOpenAPI {
			switch operationID {
			case "r2-create-bucket":
				switch attrPath {
				case "account_id":
					return []string{"account_id"}
				case "name":
					return []string{"name"}
				case "location":
					return []string{"locationHint"}
				case "storage_class":
					return []string{"storageClass"}
				case "jurisdiction":
					return []string{"cf-r2-jurisdiction"}
				}
			case "r2-get-bucket", "r2-delete-bucket":
				switch attrPath {
				case "account_id":
					return []string{"account_id"}
				case "name":
					return []string{"bucket_name"}
				case "jurisdiction":
					return []string{"cf-r2-jurisdiction"}
				}
			case "r2-patch-bucket":
				switch attrPath {
				case "account_id":
					return []string{"account_id"}
				case "name":
					return []string{"bucket_name"}
				case "jurisdiction":
					return []string{"cf-r2-jurisdiction"}
				case "storage_class":
					return []string{"cf-r2-storage-class"}
				}
			case "d1-create-database":
				switch attrPath {
				case "account_id":
					return []string{"account_id"}
				case "name":
					return []string{"name"}
				case "jurisdiction":
					return []string{"jurisdiction"}
				case "primary_location_hint":
					return []string{"primary_location_hint"}
				case "read_replication":
					return []string{"read_replication"}
				}
			case "d1-get-database":
				switch attrPath {
				case "account_id":
					return []string{"account_id"}
				case "name":
					return []string{"database_id"}
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
	case "aws_s3_bucket_public_access_block":
		return s3BucketConfigurationMapping(mapping, purpose, action, "PutPublicAccessBlock", "GetPublicAccessBlock", "DeletePublicAccessBlock")
	case "aws_s3_bucket_versioning":
		return s3BucketVersioningMapping(mapping, purpose, action)
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
	case "aws_iam_user":
		if obj.Kind != "resource" {
			return unsupportedActionMapping(mapping, "AWS IAM user mapping supports managed resources")
		}
		mapping.IdentityAttributes = []IdentityAttribute{{
			Name:          "user_name",
			TerraformPath: "name",
			RequestKeys:   []string{"UserName"},
			ResponsePaths: []string{"User.UserName", "User.Arn", "User.UserId"},
			Required:      true,
		}}
		if purpose == "read" {
			mapping.Target = awsOperationTarget("iam", "POST_GetUser")
			return mapping
		}
		if purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = awsOperationTarget("iam", "POST_CreateUser")
			return mapping
		}
		if purpose == "delete" {
			mapping.Target = awsOperationTarget("iam", "POST_DeleteUser")
			return mapping
		}
		return unsupportedActionMapping(mapping, "AWS IAM user mapping supports read, create, and delete; update requires lifecycle metadata for old and new user names")
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
	case "aws_lambda_permission":
		if obj.Kind != "resource" {
			return unsupportedActionMapping(mapping, "AWS Lambda permission mapping supports managed resources")
		}
		mapping.IdentityAttributes = []IdentityAttribute{
			{
				Name:          "function_name",
				TerraformPath: "function_name",
				RequestKeys:   []string{"FunctionName"},
				Required:      true,
			},
			{
				Name:          "statement_id",
				TerraformPath: "statement_id",
				RequestKeys:   []string{"StatementId"},
				Required:      true,
			},
		}
		if purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = awsOperationTarget("lambda", "AddPermission")
			return mapping
		}
		if purpose == "delete" {
			mapping.Target = awsOperationTarget("lambda", "RemovePermission")
			return mapping
		}
		return unsupportedActionMapping(mapping, "AWS Lambda permission mapping supports create and delete; read/update require policy response parsing")
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
		{Provider: "aws", Type: "aws_s3_bucket_public_access_block", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_s3_bucket_versioning", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_caller_identity", Kinds: []string{"data_source"}},
		{Provider: "aws", Type: "aws_iam_role", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_iam_role_policy", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_iam_user", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_lambda_function", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_lambda_function_url", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_lambda_permission", Kinds: []string{"resource"}},
	}
}

func s3BucketVersioningMapping(mapping Mapping, purpose, action string) Mapping {
	if mapping.Object.Kind != "resource" {
		return unsupportedActionMapping(mapping, "AWS S3 bucket versioning mapping supports managed resources")
	}
	mapping.IdentityAttributes = []IdentityAttribute{{
		Name:          "bucket",
		TerraformPath: "bucket",
		RequestKeys:   []string{"Bucket"},
		Required:      true,
	}}
	switch {
	case (purpose == "create" || purpose == "update") && (action == "create" || action == "update" || action == "replace"):
		mapping.Target = awsOperationTarget("s3", "PutBucketVersioning")
		return mapping
	case purpose == "read":
		mapping.Target = awsOperationTarget("s3", "GetBucketVersioning")
		return mapping
	default:
		return unsupportedActionMapping(mapping, "AWS S3 bucket versioning mapping supports read, create, and update; delete requires a semantic suspend mapping")
	}
}

func s3BucketConfigurationMapping(mapping Mapping, purpose, action, putOperation, getOperation, deleteOperation string) Mapping {
	if mapping.Object.Kind != "resource" {
		return unsupportedActionMapping(mapping, "AWS S3 bucket configuration mapping supports managed resources")
	}
	mapping.IdentityAttributes = []IdentityAttribute{{
		Name:          "bucket",
		TerraformPath: "bucket",
		RequestKeys:   []string{"Bucket"},
		Required:      true,
	}}
	switch {
	case (purpose == "create" || purpose == "update") && (action == "create" || action == "update" || action == "replace"):
		mapping.Target = awsOperationTarget("s3", putOperation)
		return mapping
	case purpose == "read":
		mapping.Target = awsOperationTarget("s3", getOperation)
		return mapping
	case purpose == "delete":
		mapping.Target = awsOperationTarget("s3", deleteOperation)
		return mapping
	default:
		return unsupportedActionMapping(mapping, "AWS S3 bucket configuration mapping supports read, create, update, and delete")
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

type azureRMMapper struct{}

func (azureRMMapper) MapObject(obj Object, purpose, action string) Mapping {
	mapping := Mapping{Object: obj, Purpose: purpose, Action: action}
	switch obj.Type {
	case "azurerm_cosmosdb_account":
		if obj.Kind != "resource" && obj.Kind != "data_source" {
			return unsupportedActionMapping(mapping, "AzureRM Cosmos DB account mapping supports managed resources and data sources")
		}
		mapping.IdentityAttributes = []IdentityAttribute{
			{
				Name:          "account_name",
				TerraformPath: "name",
				RequestKeys:   []string{"accountName"},
				ResponsePaths: []string{"name", "id"},
				Required:      true,
			},
			{
				Name:          "resource_group_name",
				TerraformPath: "resource_group_name",
				RequestKeys:   []string{"resourceGroupName"},
				Required:      true,
			},
		}
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = azureOpenAPIOperationTarget("cosmos", "DatabaseAccounts_CreateOrUpdate")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "read" {
			mapping.Target = azureOpenAPIOperationTarget("cosmos", "DatabaseAccounts_Get")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "update" && (action == "update" || action == "replace") {
			mapping.Target = azureOpenAPIOperationTarget("cosmos", "DatabaseAccounts_Update")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "delete" {
			mapping.Target = azureOpenAPIOperationTarget("cosmos", "DatabaseAccounts_Delete")
			return mapping
		}
		if obj.Kind == "data_source" && purpose == "read" {
			mapping.Target = azureOpenAPIOperationTarget("cosmos", "DatabaseAccounts_Get")
			return mapping
		}
		return unsupportedActionMapping(mapping, "AzureRM Cosmos DB account mapping supports read, create, update, and delete")
	default:
		return unsupportedTypeMapping(mapping, "AzureRM")
	}
}

// SupportedTypes must track the type switch in azureRMMapper.MapObject.
func (azureRMMapper) SupportedTypes() []SupportedType {
	return []SupportedType{
		{Provider: "azurerm", Type: "azurerm_cosmosdb_account", Kinds: []string{"resource", "data_source"}},
	}
}

type cloudflareMapper struct{}

func (cloudflareMapper) MapObject(obj Object, purpose, action string) Mapping {
	mapping := Mapping{Object: obj, Purpose: purpose, Action: action}
	switch obj.Type {
	case "cloudflare_r2_bucket":
		if obj.Kind != "resource" {
			return unsupportedActionMapping(mapping, "Cloudflare R2 bucket mapping supports managed resources")
		}
		mapping.IdentityAttributes = []IdentityAttribute{
			{
				Name:          "account_id",
				TerraformPath: "account_id",
				RequestKeys:   []string{"account_id"},
				Required:      true,
			},
			{
				Name:          "bucket_name",
				TerraformPath: "name",
				RequestKeys:   []string{"name", "bucket_name"},
				ResponsePaths: []string{"result.name"},
				Required:      true,
			},
		}
		if purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = cloudflareOpenAPIOperationTarget("r2_bucket", "r2-create-bucket")
			return mapping
		}
		if purpose == "read" {
			mapping.Target = cloudflareOpenAPIOperationTarget("r2_bucket", "r2-get-bucket")
			return mapping
		}
		if purpose == "update" && (action == "update" || action == "replace") {
			mapping.Target = cloudflareOpenAPIOperationTarget("r2_bucket", "r2-patch-bucket")
			return mapping
		}
		if purpose == "delete" {
			mapping.Target = cloudflareOpenAPIOperationTarget("r2_bucket", "r2-delete-bucket")
			return mapping
		}
		return unsupportedActionMapping(mapping, "Cloudflare R2 bucket mapping supports read, create, update, and delete")
	case "cloudflare_d1_database":
		if obj.Kind != "resource" {
			return unsupportedActionMapping(mapping, "Cloudflare D1 database mapping supports managed resources")
		}
		mapping.IdentityAttributes = []IdentityAttribute{
			{
				Name:          "account_id",
				TerraformPath: "account_id",
				RequestKeys:   []string{"account_id"},
				Required:      true,
			},
			{
				Name:          "database_name",
				TerraformPath: "name",
				RequestKeys:   []string{"name", "database_id"},
				ResponsePaths: []string{"result.name", "result.uuid"},
				Required:      true,
			},
		}
		if purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = cloudflareOpenAPIOperationTarget("d1_database", "d1-create-database")
			return mapping
		}
		if purpose == "read" {
			mapping.Target = cloudflareOpenAPIOperationTarget("d1_database", "d1-get-database")
			return mapping
		}
		return unsupportedActionMapping(mapping, "Cloudflare D1 database mapping supports read and create; update/delete require response-derived database UUID handling")
	default:
		return unsupportedTypeMapping(mapping, "Cloudflare")
	}
}

// SupportedTypes must track the type switch in cloudflareMapper.MapObject.
func (cloudflareMapper) SupportedTypes() []SupportedType {
	return []SupportedType{
		{Provider: "cloudflare", Type: "cloudflare_d1_database", Kinds: []string{"resource"}},
		{Provider: "cloudflare", Type: "cloudflare_r2_bucket", Kinds: []string{"resource"}},
	}
}

type kubernetesMapper struct{}

func (kubernetesMapper) MapObject(obj Object, purpose, action string) Mapping {
	mapping := Mapping{Object: obj, Purpose: purpose, Action: action}
	switch obj.Type {
	case "kubernetes_namespace", "kubernetes_namespace_v1":
		if obj.Kind != "resource" && obj.Kind != "data_source" {
			return unsupportedActionMapping(mapping, "Kubernetes namespace mapping supports managed resources and data sources")
		}
		mapping.IdentityAttributes = []IdentityAttribute{{
			Name:          "namespace_name",
			TerraformPath: "metadata.name",
			RequestKeys:   []string{"name"},
			ResponsePaths: []string{"metadata.name"},
			Required:      true,
		}}
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "createCoreV1Namespace")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "read" {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "readCoreV1Namespace")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "update" && (action == "update" || action == "replace") {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "replaceCoreV1Namespace")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "delete" {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "deleteCoreV1Namespace")
			return mapping
		}
		if obj.Kind == "data_source" && purpose == "read" {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "readCoreV1Namespace")
			return mapping
		}
		return unsupportedActionMapping(mapping, "Kubernetes namespace mapping supports read, create, update, and delete")
	case "kubernetes_config_map_v1":
		if obj.Kind != "resource" && obj.Kind != "data_source" {
			return unsupportedActionMapping(mapping, "Kubernetes ConfigMap mapping supports managed resources and data sources")
		}
		mapping.IdentityAttributes = kubernetesNamespacedIdentityAttributes()
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "createCoreV1NamespacedConfigMap")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "read" {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "readCoreV1NamespacedConfigMap")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "update" && (action == "update" || action == "replace") {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "replaceCoreV1NamespacedConfigMap")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "delete" {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "deleteCoreV1NamespacedConfigMap")
			return mapping
		}
		if obj.Kind == "data_source" && purpose == "read" {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "readCoreV1NamespacedConfigMap")
			return mapping
		}
		return unsupportedActionMapping(mapping, "Kubernetes ConfigMap mapping supports read, create, update, and delete")
	case "kubernetes_service_account_v1":
		if obj.Kind != "resource" && obj.Kind != "data_source" {
			return unsupportedActionMapping(mapping, "Kubernetes ServiceAccount mapping supports managed resources and data sources")
		}
		mapping.IdentityAttributes = kubernetesNamespacedIdentityAttributes()
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "createCoreV1NamespacedServiceAccount")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "read" {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "readCoreV1NamespacedServiceAccount")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "update" && (action == "update" || action == "replace") {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "replaceCoreV1NamespacedServiceAccount")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "delete" {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "deleteCoreV1NamespacedServiceAccount")
			return mapping
		}
		if obj.Kind == "data_source" && purpose == "read" {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "readCoreV1NamespacedServiceAccount")
			return mapping
		}
		return unsupportedActionMapping(mapping, "Kubernetes ServiceAccount mapping supports read, create, update, and delete")
	case "kubernetes_secret_v1":
		if obj.Kind != "resource" && obj.Kind != "data_source" {
			return unsupportedActionMapping(mapping, "Kubernetes Secret mapping supports managed resources and data sources")
		}
		mapping.IdentityAttributes = kubernetesNamespacedIdentityAttributes()
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "createCoreV1NamespacedSecret")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "read" {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "readCoreV1NamespacedSecret")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "update" && (action == "update" || action == "replace") {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "replaceCoreV1NamespacedSecret")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "delete" {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "deleteCoreV1NamespacedSecret")
			return mapping
		}
		if obj.Kind == "data_source" && purpose == "read" {
			mapping.Target = kubernetesOpenAPIOperationTarget("core", "readCoreV1NamespacedSecret")
			return mapping
		}
		return unsupportedActionMapping(mapping, "Kubernetes Secret mapping supports read, create, update, and delete")
	case "kubernetes_role_v1":
		if obj.Kind != "resource" && obj.Kind != "data_source" {
			return unsupportedActionMapping(mapping, "Kubernetes Role mapping supports managed resources and data sources")
		}
		mapping.IdentityAttributes = kubernetesNamespacedIdentityAttributes()
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			mapping.Target = kubernetesOpenAPIOperationTarget("rbac", "createRbacAuthorizationV1NamespacedRole")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "read" {
			mapping.Target = kubernetesOpenAPIOperationTarget("rbac", "readRbacAuthorizationV1NamespacedRole")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "update" && (action == "update" || action == "replace") {
			mapping.Target = kubernetesOpenAPIOperationTarget("rbac", "replaceRbacAuthorizationV1NamespacedRole")
			return mapping
		}
		if obj.Kind == "resource" && purpose == "delete" {
			mapping.Target = kubernetesOpenAPIOperationTarget("rbac", "deleteRbacAuthorizationV1NamespacedRole")
			return mapping
		}
		if obj.Kind == "data_source" && purpose == "read" {
			mapping.Target = kubernetesOpenAPIOperationTarget("rbac", "readRbacAuthorizationV1NamespacedRole")
			return mapping
		}
		return unsupportedActionMapping(mapping, "Kubernetes Role mapping supports read, create, update, and delete")
	default:
		return unsupportedTypeMapping(mapping, "Kubernetes")
	}
}

// SupportedTypes must track the type switch in kubernetesMapper.MapObject.
func (kubernetesMapper) SupportedTypes() []SupportedType {
	return []SupportedType{
		{Provider: "kubernetes", Type: "kubernetes_config_map_v1", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_namespace", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_namespace_v1", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_role_v1", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_secret_v1", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_service_account_v1", Kinds: []string{"resource", "data_source"}},
	}
}

func kubernetesNamespacedIdentityAttributes() []IdentityAttribute {
	return []IdentityAttribute{
		{
			Name:          "name",
			TerraformPath: "metadata.name",
			RequestKeys:   []string{"name"},
			ResponsePaths: []string{"metadata.name"},
			Required:      true,
		},
		{
			Name:          "namespace",
			TerraformPath: "metadata.namespace",
			RequestKeys:   []string{"namespace"},
			ResponsePaths: []string{"metadata.namespace"},
			Required:      true,
		},
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

func azureOpenAPIOperationTarget(service, operationID string) OperationTarget {
	return OperationTarget{
		SourceKinds: []string{APISourceKindOpenAPI},
		SourceIDs: []string{
			service,
			"azure-" + service,
			"azure-" + service + "-resource-manager",
			"azure-" + service + "-db-resource-manager",
			"azure-" + service + "-db-resource-manager-openapi",
		},
		OperationIDs: []string{operationID, normalizeName(operationID)},
	}
}

func kubernetesOpenAPIOperationTarget(service, operationID string) OperationTarget {
	return OperationTarget{
		SourceKinds:  []string{APISourceKindOpenAPI},
		SourceIDs:    []string{service, "kubernetes", "kubernetes-" + service, "k8s", "k8s-swagger"},
		OperationIDs: []string{operationID, normalizeName(operationID)},
	}
}

func cloudflareOpenAPIOperationTarget(service, operationID string) OperationTarget {
	return OperationTarget{
		SourceKinds:  []string{APISourceKindOpenAPI},
		SourceIDs:    []string{service, "cloudflare", "cloudflare-api", "cloudflare-api-openapi"},
		OperationIDs: []string{operationID, normalizeName(operationID)},
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
