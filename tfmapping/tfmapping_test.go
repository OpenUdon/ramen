package tfmapping

import "testing"

func TestDefaultRegistryNativeSourcePreferences(t *testing.T) {
	registry := DefaultRegistry()

	aws := registry.MapObject(Object{Kind: "resource", Type: "aws_iam_role", Provider: "provider.aws"}, "create", "create")
	if got, want := aws.Target.SourceKinds[0], APISourceKindAWSSmithy; got != want {
		t.Fatalf("AWS source preference = %q, want %q", got, want)
	}
	if got, want := aws.Target.OperationIDs[0], "POST_CreateRole"; got != want {
		t.Fatalf("AWS operation = %q, want %q", got, want)
	}

	google := registry.MapObject(Object{Kind: "resource", Type: "google_storage_bucket", Provider: "provider.google"}, "create", "create")
	if got, want := google.Target.SourceKinds[0], APISourceKindGoogleDiscovery; got != want {
		t.Fatalf("Google source preference = %q, want %q", got, want)
	}
	if got, want := google.Target.OperationIDs[0], "storage.buckets.insert"; got != want {
		t.Fatalf("Google operation = %q, want %q", got, want)
	}
}

func TestDefaultRegistryOpenAPIFallbackOrder(t *testing.T) {
	registry := DefaultRegistry()
	target := registry.OperationTarget(Object{Kind: "resource", Type: "aws_iam_role", Provider: "provider.aws"}, "create", "create")
	if got, want := target.SourceKinds, []string{APISourceKindAWSSmithy, APISourceKindOpenAPI}; !equalStrings(got, want) {
		t.Fatalf("AWS fallback order = %#v, want %#v", got, want)
	}
	target = registry.OperationTarget(Object{Kind: "resource", Type: "google_storage_bucket", Provider: "provider.google"}, "create", "create")
	if got, want := target.SourceKinds, []string{APISourceKindGoogleDiscovery, APISourceKindOpenAPI}; !equalStrings(got, want) {
		t.Fatalf("Google fallback order = %#v, want %#v", got, want)
	}
}

func TestDefaultRegistryIdentityAttributes(t *testing.T) {
	registry := DefaultRegistry()
	aws := registry.MapObject(Object{Kind: "resource", Type: "aws_iam_role"}, "create", "create")
	assertIdentity(t, aws.IdentityAttributes, IdentityAttribute{
		Name:          "role_name",
		TerraformPath: "name",
		RequestKeys:   []string{"RoleName"},
		ResponsePaths: []string{"Role.RoleName", "Role.Arn"},
		Required:      true,
	})

	google := registry.MapObject(Object{Kind: "resource", Type: "google_storage_bucket"}, "create", "create")
	assertIdentity(t, google.IdentityAttributes, IdentityAttribute{
		Name:          "bucket_name",
		TerraformPath: "name",
		RequestKeys:   []string{"name"},
		ResponsePaths: []string{"name", "id"},
		Required:      true,
	})

	s3Config := registry.MapObject(Object{Kind: "resource", Type: "aws_s3_bucket_public_access_block"}, "create", "create")
	assertIdentity(t, s3Config.IdentityAttributes, IdentityAttribute{
		Name:          "bucket",
		TerraformPath: "bucket",
		RequestKeys:   []string{"Bucket"},
		Required:      true,
	})

	s3Versioning := registry.MapObject(Object{Kind: "resource", Type: "aws_s3_bucket_versioning"}, "create", "create")
	assertIdentity(t, s3Versioning.IdentityAttributes, IdentityAttribute{
		Name:          "bucket",
		TerraformPath: "bucket",
		RequestKeys:   []string{"Bucket"},
		Required:      true,
	})
}

func TestDefaultRegistryInitialResourceOperationTargets(t *testing.T) {
	registry := DefaultRegistry()
	tests := []struct {
		name      string
		obj       Object
		purpose   string
		action    string
		operation string
	}{
		{name: "iam read", obj: Object{Kind: "resource", Type: "aws_iam_role"}, purpose: "read", action: "read", operation: "POST_GetRole"},
		{name: "iam create", obj: Object{Kind: "resource", Type: "aws_iam_role"}, purpose: "create", action: "create", operation: "POST_CreateRole"},
		{name: "iam update", obj: Object{Kind: "resource", Type: "aws_iam_role"}, purpose: "update", action: "update", operation: "POST_UpdateRole"},
		{name: "iam delete", obj: Object{Kind: "resource", Type: "aws_iam_role"}, purpose: "delete", action: "delete", operation: "POST_DeleteRole"},
		{name: "s3 public access block create", obj: Object{Kind: "resource", Type: "aws_s3_bucket_public_access_block"}, purpose: "create", action: "create", operation: "PutPublicAccessBlock"},
		{name: "s3 public access block read", obj: Object{Kind: "resource", Type: "aws_s3_bucket_public_access_block"}, purpose: "read", action: "read", operation: "GetPublicAccessBlock"},
		{name: "s3 public access block update", obj: Object{Kind: "resource", Type: "aws_s3_bucket_public_access_block"}, purpose: "update", action: "update", operation: "PutPublicAccessBlock"},
		{name: "s3 public access block delete", obj: Object{Kind: "resource", Type: "aws_s3_bucket_public_access_block"}, purpose: "delete", action: "delete", operation: "DeletePublicAccessBlock"},
		{name: "s3 bucket versioning create", obj: Object{Kind: "resource", Type: "aws_s3_bucket_versioning"}, purpose: "create", action: "create", operation: "PutBucketVersioning"},
		{name: "s3 bucket versioning read", obj: Object{Kind: "resource", Type: "aws_s3_bucket_versioning"}, purpose: "read", action: "read", operation: "GetBucketVersioning"},
		{name: "s3 bucket versioning update", obj: Object{Kind: "resource", Type: "aws_s3_bucket_versioning"}, purpose: "update", action: "update", operation: "PutBucketVersioning"},
		{name: "bucket read", obj: Object{Kind: "resource", Type: "google_storage_bucket"}, purpose: "read", action: "read", operation: "storage.buckets.get"},
		{name: "bucket create", obj: Object{Kind: "resource", Type: "google_storage_bucket"}, purpose: "create", action: "create", operation: "storage.buckets.insert"},
		{name: "bucket update", obj: Object{Kind: "resource", Type: "google_storage_bucket"}, purpose: "update", action: "update", operation: "storage.buckets.patch"},
		{name: "bucket delete", obj: Object{Kind: "resource", Type: "google_storage_bucket"}, purpose: "delete", action: "delete", operation: "storage.buckets.delete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := registry.MapObject(tt.obj, tt.purpose, tt.action)
			if len(mapping.Target.OperationIDs) == 0 || mapping.Target.OperationIDs[0] != tt.operation {
				t.Fatalf("operation IDs = %#v, want first %q", mapping.Target.OperationIDs, tt.operation)
			}
			if len(mapping.Diagnostics) != 0 {
				t.Fatalf("unexpected diagnostics: %#v", mapping.Diagnostics)
			}
		})
	}
}

func TestDefaultRegistryDiagnostics(t *testing.T) {
	registry := DefaultRegistry()
	tests := []struct {
		name string
		obj  Object
		want DiagnosticCode
	}{
		{name: "provider", obj: Object{Kind: "resource", Type: "example_resource", Provider: "provider.example"}, want: DiagnosticCodeUnsupportedProvider},
		{name: "type", obj: Object{Kind: "resource", Type: "aws_instance", Provider: "provider.aws"}, want: DiagnosticCodeUnsupportedType},
		{name: "action", obj: Object{Kind: "resource", Type: "aws_iam_role", Provider: "provider.aws"}, want: DiagnosticCodeUnsupportedAction},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := registry.MapObject(tt.obj, "import", "import")
			if len(mapping.Diagnostics) != 1 {
				t.Fatalf("diagnostics = %#v, want one", mapping.Diagnostics)
			}
			if got := mapping.Diagnostics[0].Code; got != tt.want {
				t.Fatalf("diagnostic code = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRegistryProviderMapperOverride(t *testing.T) {
	registry := NewRegistry(WithProviderMapper("aws", fakeMapper{}))
	mapping := registry.MapObject(Object{Kind: "resource", Type: "aws_iam_role", Provider: "provider.aws"}, "create", "create")
	if got, want := mapping.Target.OperationIDs, []string{"override.create"}; !equalStrings(got, want) {
		t.Fatalf("override operation IDs = %#v, want %#v", got, want)
	}
	if got := mapping.IdentityAttributes[0].Name; got != "override_id" {
		t.Fatalf("override identity = %q", got)
	}
}

func TestDefaultRegistryProviderLocalDataSources(t *testing.T) {
	registry := DefaultRegistry()
	for _, typ := range []string{"aws_iam_policy_document", "aws_partition", "aws_region"} {
		if !registry.IsProviderLocalDataSource(Object{Kind: "data_source", Type: typ}) {
			t.Fatalf("%s should be provider-local", typ)
		}
	}
	if registry.IsProviderLocalDataSource(Object{Kind: "data_source", Type: "aws_caller_identity"}) {
		t.Fatal("aws_caller_identity should map to STS")
	}
}

func TestDefaultRegistryRequestHints(t *testing.T) {
	registry := DefaultRegistry()
	obj := Object{Kind: "resource", Type: "aws_iam_role", Provider: "provider.aws"}
	keys := registry.RequestKeys(obj, APISourceKindAWSSmithy, "POST_CreateRole", "assume_role_policy")
	if len(keys) != 1 || keys[0] != "AssumeRolePolicyDocument" {
		t.Fatalf("unexpected IAM request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "aws_lambda_function", Provider: "provider.aws"}, APISourceKindOpenAPI, "CreateFunction", "function_name")
	if len(keys) != 1 || keys[0] != "FunctionName" {
		t.Fatalf("unexpected Lambda request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "aws_s3_bucket", Provider: "provider.aws"}, APISourceKindOpenAPI, "CreateBucket", "bucket")
	if len(keys) != 1 || keys[0] != "Bucket" {
		t.Fatalf("unexpected S3 request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "aws_s3_bucket_public_access_block", Provider: "provider.aws"}, APISourceKindAWSSmithy, "PutPublicAccessBlock", "block_public_policy")
	if len(keys) != 1 || keys[0] != "PublicAccessBlockConfiguration.BlockPublicPolicy" {
		t.Fatalf("unexpected S3 public access block request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "aws_s3_bucket_versioning", Provider: "provider.aws"}, APISourceKindAWSSmithy, "PutBucketVersioning", "versioning_configuration.status")
	if len(keys) != 1 || keys[0] != "VersioningConfiguration.Status" {
		t.Fatalf("unexpected S3 bucket versioning request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "google_storage_bucket", Provider: "provider.google"}, APISourceKindGoogleDiscovery, "storage.buckets.patch", "name")
	if len(keys) != 1 || keys[0] != "bucket" {
		t.Fatalf("unexpected Google bucket request keys: %#v", keys)
	}
	static := registry.StaticRequestBindings(obj, "iam", "aws-smithy/iam.json", "POST_CreateRole")
	if static["Action"] != "CreateRole" || static["Version"] != "2010-05-08" {
		t.Fatalf("unexpected AWS query static bindings: %#v", static)
	}
}

type fakeMapper struct{}

func (fakeMapper) MapObject(obj Object, purpose, action string) Mapping {
	return Mapping{
		Object:  obj,
		Purpose: purpose,
		Action:  action,
		Target: OperationTarget{
			SourceKinds:  []string{APISourceKindOpenAPI},
			SourceIDs:    []string{"override"},
			OperationIDs: []string{"override.create"},
		},
		IdentityAttributes: []IdentityAttribute{{Name: "override_id", TerraformPath: "id", Required: true}},
	}
}

func assertIdentity(t *testing.T, got []IdentityAttribute, want IdentityAttribute) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("identity attributes = %#v, want one", got)
	}
	if got[0].Name != want.Name || got[0].TerraformPath != want.TerraformPath || got[0].Required != want.Required {
		t.Fatalf("identity = %#v, want %#v", got[0], want)
	}
	if !equalStrings(got[0].RequestKeys, want.RequestKeys) || !equalStrings(got[0].ResponsePaths, want.ResponsePaths) {
		t.Fatalf("identity = %#v, want %#v", got[0], want)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
