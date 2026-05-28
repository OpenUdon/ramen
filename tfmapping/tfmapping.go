package tfmapping

import (
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

type OperationTarget struct {
	SourceKinds  []string `json:"source_kinds,omitempty"`
	SourceIDs    []string `json:"source_ids,omitempty"`
	OperationIDs []string `json:"operation_ids,omitempty"`
}

type Registry struct{}

func DefaultRegistry() Registry {
	return Registry{}
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

func (Registry) OperationTarget(obj Object, purpose, action string) OperationTarget {
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	action = strings.ToLower(strings.TrimSpace(action))
	switch objectProviderLocalName(obj) {
	case "aws":
		return awsOperationTargetForObject(obj, purpose, action)
	case "google":
		return googleOperationTargetForObject(obj, purpose, action)
	default:
		return OperationTarget{}
	}
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
			if sourceKind == APISourceKindAWSSmithy {
				switch attrPath {
				case "name":
					return []string{"RoleName"}
				case "assume_role_policy":
					return []string{"AssumeRolePolicyDocument"}
				}
			}
		case "DeleteRole", "POST_DeleteRole":
			if attrPath == "name" {
				return []string{"RoleName"}
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
		if sourceKind == APISourceKindGoogleDiscovery && operationID == "storage.buckets.insert" {
			switch attrPath {
			case "project":
				return []string{"project"}
			case "name":
				return []string{"name"}
			case "location":
				return []string{"location"}
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

func awsOperationTargetForObject(obj Object, purpose, action string) OperationTarget {
	switch obj.Type {
	case "aws_s3_bucket":
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			return awsOperationTarget("s3", "CreateBucket")
		}
		if obj.Kind == "data_source" && purpose == "read" {
			return awsOperationTarget("s3", "GetBucketLocation")
		}
		if obj.Kind == "data_source" && purpose == "list" {
			return awsOperationTarget("s3", "ListBuckets")
		}
	case "aws_s3_bucket_accelerate_configuration":
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			return awsOperationTarget("s3", "PutBucketAccelerateConfiguration")
		}
	case "aws_caller_identity":
		if obj.Kind == "data_source" && purpose == "read" {
			return awsOperationTarget("sts", "POST_GetCallerIdentity")
		}
	case "aws_iam_role":
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			return awsOperationTarget("iam", "POST_CreateRole")
		}
		if obj.Kind == "resource" && purpose == "delete" {
			return awsOperationTarget("iam", "POST_DeleteRole")
		}
	case "aws_iam_role_policy":
		if obj.Kind == "resource" && (purpose == "create" || purpose == "update") && (action == "create" || action == "update" || action == "replace") {
			return awsOperationTarget("iam", "POST_PutRolePolicy")
		}
		if obj.Kind == "resource" && purpose == "delete" {
			return awsOperationTarget("iam", "POST_DeleteRolePolicy")
		}
	case "aws_lambda_function":
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			return awsOperationTarget("lambda", "CreateFunction")
		}
		if obj.Kind == "resource" && purpose == "delete" {
			return awsOperationTarget("lambda", "DeleteFunction")
		}
	case "aws_lambda_function_url":
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			return awsOperationTarget("lambda", "CreateFunctionUrlConfig")
		}
		if obj.Kind == "resource" && purpose == "update" {
			return awsOperationTarget("lambda", "UpdateFunctionUrlConfig")
		}
		if obj.Kind == "resource" && purpose == "delete" {
			return awsOperationTarget("lambda", "DeleteFunctionUrlConfig")
		}
	}
	return OperationTarget{}
}

func googleOperationTargetForObject(obj Object, purpose, action string) OperationTarget {
	switch obj.Type {
	case "google_storage_bucket":
		if obj.Kind == "resource" && purpose == "create" && (action == "create" || action == "replace") {
			return googleOperationTarget("storage", "storage.buckets.insert")
		}
		if obj.Kind == "data_source" && purpose == "read" {
			return googleOperationTarget("storage", "storage.buckets.get")
		}
	}
	return OperationTarget{}
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
	provider := strings.TrimPrefix(strings.TrimSpace(obj.Provider), "provider.")
	if provider == "" {
		if before, _, ok := strings.Cut(obj.Type, "_"); ok {
			return before
		}
		return ""
	}
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
