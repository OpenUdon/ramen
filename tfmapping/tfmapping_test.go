package tfmapping

import "testing"

func TestDefaultRegistryNativeSourcePreferences(t *testing.T) {
	registry := DefaultRegistry()

	aws := registry.OperationTarget(Object{Kind: "resource", Type: "aws_iam_role", Provider: "provider.aws"}, "create", "create")
	if got, want := aws.SourceKinds[0], APISourceKindAWSSmithy; got != want {
		t.Fatalf("AWS source preference = %q, want %q", got, want)
	}
	if got, want := aws.OperationIDs[0], "POST_CreateRole"; got != want {
		t.Fatalf("AWS operation = %q, want %q", got, want)
	}

	google := registry.OperationTarget(Object{Kind: "resource", Type: "google_storage_bucket", Provider: "provider.google"}, "create", "create")
	if got, want := google.SourceKinds[0], APISourceKindGoogleDiscovery; got != want {
		t.Fatalf("Google source preference = %q, want %q", got, want)
	}
	if got, want := google.OperationIDs[0], "storage.buckets.insert"; got != want {
		t.Fatalf("Google operation = %q, want %q", got, want)
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
	static := registry.StaticRequestBindings(obj, "iam", "aws-smithy/iam.json", "POST_CreateRole")
	if static["Action"] != "CreateRole" || static["Version"] != "2010-05-08" {
		t.Fatalf("unexpected AWS query static bindings: %#v", static)
	}
}
