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

func TestDefaultRegistrySupportedTypes(t *testing.T) {
	registry := DefaultRegistry()
	got := registry.SupportedTypes()
	want := []SupportedType{
		{Provider: "aws", Type: "aws_caller_identity", Kinds: []string{"data_source"}},
		{Provider: "aws", Type: "aws_iam_role", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_iam_role_policy", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_iam_user", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_lambda_function", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_lambda_function_url", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_lambda_permission", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_s3_bucket", Kinds: []string{"resource", "data_source"}},
		{Provider: "aws", Type: "aws_s3_bucket_accelerate_configuration", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_s3_bucket_public_access_block", Kinds: []string{"resource"}},
		{Provider: "aws", Type: "aws_s3_bucket_versioning", Kinds: []string{"resource"}},
		{Provider: "azurerm", Type: "azurerm_cosmosdb_account", Kinds: []string{"resource", "data_source"}},
		{Provider: "cloudflare", Type: "cloudflare_d1_database", Kinds: []string{"resource"}},
		{Provider: "cloudflare", Type: "cloudflare_r2_bucket", Kinds: []string{"resource"}},
		{Provider: "google", Type: "google_storage_bucket", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_config_map_v1", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_namespace", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_namespace_v1", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_role_v1", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_secret_v1", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_service_account_v1", Kinds: []string{"resource", "data_source"}},
	}
	if !equalSupportedTypes(got, want) {
		t.Fatalf("supported types = %#v, want %#v", got, want)
	}
}

func TestDefaultRegistrySupportedTypesHaveMappingTargets(t *testing.T) {
	registry := DefaultRegistry()
	for _, supported := range registry.SupportedTypes() {
		for _, kind := range supported.Kinds {
			purpose, action := "create", "create"
			if kind == "data_source" {
				purpose, action = "read", "read"
			}
			obj := Object{Kind: kind, Type: supported.Type, Provider: "provider." + supported.Provider}
			mapping := registry.MapObject(obj, purpose, action)
			if len(mapping.Diagnostics) > 0 {
				t.Fatalf("%s %s %s mapping has diagnostics: %#v", supported.Provider, kind, supported.Type, mapping.Diagnostics)
			}
			if len(mapping.Target.OperationIDs) == 0 {
				t.Fatalf("%s %s %s mapping has no operation target: %#v", supported.Provider, kind, supported.Type, mapping)
			}
		}
	}
}

func TestRegistrySupportedTypesHonorsProviderOverrides(t *testing.T) {
	registry := NewRegistry(WithProviderMapper("aws", fakeListingMapper{}))
	got := registry.SupportedTypes()
	want := []SupportedType{
		{Provider: "aws", Type: "aws_override_resource", Kinds: []string{"resource"}},
		{Provider: "azurerm", Type: "azurerm_cosmosdb_account", Kinds: []string{"resource", "data_source"}},
		{Provider: "cloudflare", Type: "cloudflare_d1_database", Kinds: []string{"resource"}},
		{Provider: "cloudflare", Type: "cloudflare_r2_bucket", Kinds: []string{"resource"}},
		{Provider: "google", Type: "google_storage_bucket", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_config_map_v1", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_namespace", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_namespace_v1", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_role_v1", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_secret_v1", Kinds: []string{"resource", "data_source"}},
		{Provider: "kubernetes", Type: "kubernetes_service_account_v1", Kinds: []string{"resource", "data_source"}},
	}
	if !equalSupportedTypes(got, want) {
		t.Fatalf("supported types with override = %#v, want %#v", got, want)
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

	iamUser := registry.MapObject(Object{Kind: "resource", Type: "aws_iam_user"}, "create", "create")
	assertIdentity(t, iamUser.IdentityAttributes, IdentityAttribute{
		Name:          "user_name",
		TerraformPath: "name",
		RequestKeys:   []string{"UserName"},
		ResponsePaths: []string{"User.UserName", "User.Arn", "User.UserId"},
		Required:      true,
	})

	lambdaPermission := registry.MapObject(Object{Kind: "resource", Type: "aws_lambda_permission"}, "create", "create")
	assertIdentities(t, lambdaPermission.IdentityAttributes, []IdentityAttribute{
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
	})

	r2Bucket := registry.MapObject(Object{Kind: "resource", Type: "cloudflare_r2_bucket"}, "create", "create")
	assertIdentities(t, r2Bucket.IdentityAttributes, []IdentityAttribute{
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
	})

	d1Database := registry.MapObject(Object{Kind: "resource", Type: "cloudflare_d1_database"}, "create", "create")
	assertIdentities(t, d1Database.IdentityAttributes, []IdentityAttribute{
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
	})

	configMap := registry.MapObject(Object{Kind: "resource", Type: "kubernetes_config_map_v1", Provider: "provider.kubernetes"}, "create", "create")
	assertIdentities(t, configMap.IdentityAttributes, []IdentityAttribute{
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
		{name: "caller identity read", obj: Object{Kind: "data_source", Type: "aws_caller_identity"}, purpose: "read", action: "read", operation: "POST_GetCallerIdentity"},
		{name: "iam read", obj: Object{Kind: "resource", Type: "aws_iam_role"}, purpose: "read", action: "read", operation: "POST_GetRole"},
		{name: "iam create", obj: Object{Kind: "resource", Type: "aws_iam_role"}, purpose: "create", action: "create", operation: "POST_CreateRole"},
		{name: "iam update", obj: Object{Kind: "resource", Type: "aws_iam_role"}, purpose: "update", action: "update", operation: "POST_UpdateRole"},
		{name: "iam delete", obj: Object{Kind: "resource", Type: "aws_iam_role"}, purpose: "delete", action: "delete", operation: "POST_DeleteRole"},
		{name: "iam role policy create", obj: Object{Kind: "resource", Type: "aws_iam_role_policy"}, purpose: "create", action: "create", operation: "POST_PutRolePolicy"},
		{name: "iam role policy update", obj: Object{Kind: "resource", Type: "aws_iam_role_policy"}, purpose: "update", action: "update", operation: "POST_PutRolePolicy"},
		{name: "iam role policy delete", obj: Object{Kind: "resource", Type: "aws_iam_role_policy"}, purpose: "delete", action: "delete", operation: "POST_DeleteRolePolicy"},
		{name: "iam user read", obj: Object{Kind: "resource", Type: "aws_iam_user"}, purpose: "read", action: "read", operation: "POST_GetUser"},
		{name: "iam user create", obj: Object{Kind: "resource", Type: "aws_iam_user"}, purpose: "create", action: "create", operation: "POST_CreateUser"},
		{name: "iam user delete", obj: Object{Kind: "resource", Type: "aws_iam_user"}, purpose: "delete", action: "delete", operation: "POST_DeleteUser"},
		{name: "lambda function create", obj: Object{Kind: "resource", Type: "aws_lambda_function"}, purpose: "create", action: "create", operation: "CreateFunction"},
		{name: "lambda function delete", obj: Object{Kind: "resource", Type: "aws_lambda_function"}, purpose: "delete", action: "delete", operation: "DeleteFunction"},
		{name: "lambda function url create", obj: Object{Kind: "resource", Type: "aws_lambda_function_url"}, purpose: "create", action: "create", operation: "CreateFunctionUrlConfig"},
		{name: "lambda function url update", obj: Object{Kind: "resource", Type: "aws_lambda_function_url"}, purpose: "update", action: "update", operation: "UpdateFunctionUrlConfig"},
		{name: "lambda function url delete", obj: Object{Kind: "resource", Type: "aws_lambda_function_url"}, purpose: "delete", action: "delete", operation: "DeleteFunctionUrlConfig"},
		{name: "lambda permission create", obj: Object{Kind: "resource", Type: "aws_lambda_permission"}, purpose: "create", action: "create", operation: "AddPermission"},
		{name: "lambda permission delete", obj: Object{Kind: "resource", Type: "aws_lambda_permission"}, purpose: "delete", action: "delete", operation: "RemovePermission"},
		{name: "s3 bucket create", obj: Object{Kind: "resource", Type: "aws_s3_bucket"}, purpose: "create", action: "create", operation: "CreateBucket"},
		{name: "s3 bucket data source read", obj: Object{Kind: "data_source", Type: "aws_s3_bucket"}, purpose: "read", action: "read", operation: "GetBucketLocation"},
		{name: "s3 bucket data source list", obj: Object{Kind: "data_source", Type: "aws_s3_bucket"}, purpose: "list", action: "list", operation: "ListBuckets"},
		{name: "s3 accelerate create", obj: Object{Kind: "resource", Type: "aws_s3_bucket_accelerate_configuration"}, purpose: "create", action: "create", operation: "PutBucketAccelerateConfiguration"},
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
		{name: "cosmos account create", obj: Object{Kind: "resource", Type: "azurerm_cosmosdb_account"}, purpose: "create", action: "create", operation: "DatabaseAccounts_CreateOrUpdate"},
		{name: "cosmos account read", obj: Object{Kind: "resource", Type: "azurerm_cosmosdb_account"}, purpose: "read", action: "read", operation: "DatabaseAccounts_Get"},
		{name: "cosmos account update", obj: Object{Kind: "resource", Type: "azurerm_cosmosdb_account"}, purpose: "update", action: "update", operation: "DatabaseAccounts_Update"},
		{name: "cosmos account delete", obj: Object{Kind: "resource", Type: "azurerm_cosmosdb_account"}, purpose: "delete", action: "delete", operation: "DatabaseAccounts_Delete"},
		{name: "cloudflare r2 bucket create", obj: Object{Kind: "resource", Type: "cloudflare_r2_bucket"}, purpose: "create", action: "create", operation: "r2-create-bucket"},
		{name: "cloudflare r2 bucket read", obj: Object{Kind: "resource", Type: "cloudflare_r2_bucket"}, purpose: "read", action: "read", operation: "r2-get-bucket"},
		{name: "cloudflare r2 bucket update", obj: Object{Kind: "resource", Type: "cloudflare_r2_bucket"}, purpose: "update", action: "update", operation: "r2-patch-bucket"},
		{name: "cloudflare r2 bucket delete", obj: Object{Kind: "resource", Type: "cloudflare_r2_bucket"}, purpose: "delete", action: "delete", operation: "r2-delete-bucket"},
		{name: "cloudflare d1 database create", obj: Object{Kind: "resource", Type: "cloudflare_d1_database"}, purpose: "create", action: "create", operation: "d1-create-database"},
		{name: "cloudflare d1 database read", obj: Object{Kind: "resource", Type: "cloudflare_d1_database"}, purpose: "read", action: "read", operation: "d1-get-database"},
		{name: "namespace create", obj: Object{Kind: "resource", Type: "kubernetes_namespace"}, purpose: "create", action: "create", operation: "createCoreV1Namespace"},
		{name: "namespace read", obj: Object{Kind: "resource", Type: "kubernetes_namespace"}, purpose: "read", action: "read", operation: "readCoreV1Namespace"},
		{name: "namespace update", obj: Object{Kind: "resource", Type: "kubernetes_namespace_v1"}, purpose: "update", action: "update", operation: "replaceCoreV1Namespace"},
		{name: "namespace delete", obj: Object{Kind: "resource", Type: "kubernetes_namespace_v1"}, purpose: "delete", action: "delete", operation: "deleteCoreV1Namespace"},
		{name: "configmap create", obj: Object{Kind: "resource", Type: "kubernetes_config_map_v1"}, purpose: "create", action: "create", operation: "createCoreV1NamespacedConfigMap"},
		{name: "configmap read", obj: Object{Kind: "resource", Type: "kubernetes_config_map_v1"}, purpose: "read", action: "read", operation: "readCoreV1NamespacedConfigMap"},
		{name: "configmap update", obj: Object{Kind: "resource", Type: "kubernetes_config_map_v1"}, purpose: "update", action: "update", operation: "replaceCoreV1NamespacedConfigMap"},
		{name: "configmap delete", obj: Object{Kind: "resource", Type: "kubernetes_config_map_v1"}, purpose: "delete", action: "delete", operation: "deleteCoreV1NamespacedConfigMap"},
		{name: "serviceaccount create", obj: Object{Kind: "resource", Type: "kubernetes_service_account_v1"}, purpose: "create", action: "create", operation: "createCoreV1NamespacedServiceAccount"},
		{name: "serviceaccount read", obj: Object{Kind: "resource", Type: "kubernetes_service_account_v1"}, purpose: "read", action: "read", operation: "readCoreV1NamespacedServiceAccount"},
		{name: "serviceaccount update", obj: Object{Kind: "resource", Type: "kubernetes_service_account_v1"}, purpose: "update", action: "update", operation: "replaceCoreV1NamespacedServiceAccount"},
		{name: "serviceaccount delete", obj: Object{Kind: "resource", Type: "kubernetes_service_account_v1"}, purpose: "delete", action: "delete", operation: "deleteCoreV1NamespacedServiceAccount"},
		{name: "secret create", obj: Object{Kind: "resource", Type: "kubernetes_secret_v1"}, purpose: "create", action: "create", operation: "createCoreV1NamespacedSecret"},
		{name: "secret read", obj: Object{Kind: "resource", Type: "kubernetes_secret_v1"}, purpose: "read", action: "read", operation: "readCoreV1NamespacedSecret"},
		{name: "secret update", obj: Object{Kind: "resource", Type: "kubernetes_secret_v1"}, purpose: "update", action: "update", operation: "replaceCoreV1NamespacedSecret"},
		{name: "secret delete", obj: Object{Kind: "resource", Type: "kubernetes_secret_v1"}, purpose: "delete", action: "delete", operation: "deleteCoreV1NamespacedSecret"},
		{name: "role create", obj: Object{Kind: "resource", Type: "kubernetes_role_v1"}, purpose: "create", action: "create", operation: "createRbacAuthorizationV1NamespacedRole"},
		{name: "role read", obj: Object{Kind: "resource", Type: "kubernetes_role_v1"}, purpose: "read", action: "read", operation: "readRbacAuthorizationV1NamespacedRole"},
		{name: "role update", obj: Object{Kind: "resource", Type: "kubernetes_role_v1"}, purpose: "update", action: "update", operation: "replaceRbacAuthorizationV1NamespacedRole"},
		{name: "role delete", obj: Object{Kind: "resource", Type: "kubernetes_role_v1"}, purpose: "delete", action: "delete", operation: "deleteRbacAuthorizationV1NamespacedRole"},
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
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "aws_iam_user", Provider: "provider.aws"}, APISourceKindAWSSmithy, "POST_CreateUser", "name")
	if len(keys) != 1 || keys[0] != "UserName" {
		t.Fatalf("unexpected IAM user request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "aws_lambda_permission", Provider: "provider.aws"}, APISourceKindAWSSmithy, "AddPermission", "statement_id")
	if len(keys) != 1 || keys[0] != "StatementId" {
		t.Fatalf("unexpected Lambda permission request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "google_storage_bucket", Provider: "provider.google"}, APISourceKindGoogleDiscovery, "storage.buckets.patch", "name")
	if len(keys) != 1 || keys[0] != "bucket" {
		t.Fatalf("unexpected Google bucket request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "azurerm_cosmosdb_account", Provider: "provider.azurerm"}, APISourceKindOpenAPI, "DatabaseAccounts_CreateOrUpdate", "resource_group_name")
	if len(keys) != 1 || keys[0] != "resourceGroupName" {
		t.Fatalf("unexpected Azure Cosmos DB request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "kubernetes_namespace", Provider: "provider.kubernetes"}, APISourceKindOpenAPI, "createCoreV1Namespace", "metadata.0.name")
	if len(keys) != 1 || keys[0] != "metadata.name" {
		t.Fatalf("unexpected Kubernetes namespace request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "kubernetes_config_map_v1", Provider: "provider.kubernetes"}, APISourceKindOpenAPI, "readCoreV1NamespacedConfigMap", "metadata.namespace")
	if len(keys) != 1 || keys[0] != "namespace" {
		t.Fatalf("unexpected Kubernetes ConfigMap namespace request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "kubernetes_config_map_v1", Provider: "provider.kubernetes"}, APISourceKindOpenAPI, "createCoreV1NamespacedConfigMap", "metadata.namespace")
	if len(keys) != 2 || keys[0] != "namespace" || keys[1] != "metadata.namespace" {
		t.Fatalf("unexpected Kubernetes ConfigMap create namespace request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "kubernetes_config_map_v1", Provider: "provider.kubernetes"}, APISourceKindOpenAPI, "replaceCoreV1NamespacedConfigMap", "data")
	if len(keys) != 1 || keys[0] != "data" {
		t.Fatalf("unexpected Kubernetes ConfigMap data request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "kubernetes_config_map_v1", Provider: "provider.kubernetes"}, APISourceKindOpenAPI, "replaceCoreV1NamespacedConfigMap", "binary_data")
	if len(keys) != 1 || keys[0] != "binaryData" {
		t.Fatalf("unexpected Kubernetes ConfigMap binary data request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "kubernetes_service_account_v1", Provider: "provider.kubernetes"}, APISourceKindOpenAPI, "createCoreV1NamespacedServiceAccount", "metadata.name")
	if len(keys) != 1 || keys[0] != "metadata.name" {
		t.Fatalf("unexpected Kubernetes ServiceAccount request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "kubernetes_secret_v1", Provider: "provider.kubernetes"}, APISourceKindOpenAPI, "replaceCoreV1NamespacedSecret", "string_data")
	if len(keys) != 1 || keys[0] != "stringData" {
		t.Fatalf("unexpected Kubernetes Secret string data request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "kubernetes_secret_v1", Provider: "provider.kubernetes"}, APISourceKindOpenAPI, "readCoreV1NamespacedSecret", "metadata.namespace")
	if len(keys) != 1 || keys[0] != "namespace" {
		t.Fatalf("unexpected Kubernetes Secret read namespace request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "kubernetes_role_v1", Provider: "provider.kubernetes"}, APISourceKindOpenAPI, "createRbacAuthorizationV1NamespacedRole", "rule")
	if len(keys) != 1 || keys[0] != "rules" {
		t.Fatalf("unexpected Kubernetes Role rules request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "kubernetes_role_v1", Provider: "provider.kubernetes"}, APISourceKindOpenAPI, "createRbacAuthorizationV1NamespacedRole", "metadata.namespace")
	if len(keys) != 2 || keys[0] != "namespace" || keys[1] != "metadata.namespace" {
		t.Fatalf("unexpected Kubernetes Role create namespace request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "cloudflare_r2_bucket", Provider: "provider.cloudflare"}, APISourceKindOpenAPI, "r2-create-bucket", "name")
	if len(keys) != 1 || keys[0] != "name" {
		t.Fatalf("unexpected Cloudflare R2 create request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "cloudflare_r2_bucket", Provider: "provider.cloudflare"}, APISourceKindOpenAPI, "r2-patch-bucket", "storage_class")
	if len(keys) != 1 || keys[0] != "cf-r2-storage-class" {
		t.Fatalf("unexpected Cloudflare R2 update request keys: %#v", keys)
	}
	keys = registry.RequestKeys(Object{Kind: "resource", Type: "cloudflare_d1_database", Provider: "provider.cloudflare"}, APISourceKindOpenAPI, "d1-get-database", "name")
	if len(keys) != 1 || keys[0] != "database_id" {
		t.Fatalf("unexpected Cloudflare D1 read request keys: %#v", keys)
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

type fakeListingMapper struct{}

func (fakeListingMapper) MapObject(obj Object, purpose, action string) Mapping {
	return Mapping{
		Object:  obj,
		Purpose: purpose,
		Action:  action,
		Target: OperationTarget{
			SourceKinds:  []string{APISourceKindOpenAPI},
			SourceIDs:    []string{"override"},
			OperationIDs: []string{"override.create"},
		},
	}
}

func (fakeListingMapper) SupportedTypes() []SupportedType {
	return []SupportedType{{Provider: "aws", Type: "aws_override_resource", Kinds: []string{"resource"}}}
}

func assertIdentity(t *testing.T, got []IdentityAttribute, want IdentityAttribute) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("identity attributes = %#v, want one", got)
	}
	assertIdentities(t, got, []IdentityAttribute{want})
}

func assertIdentities(t *testing.T, got, want []IdentityAttribute) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("identity attributes = %#v, want %#v", got, want)
	}
	for i := range want {
		assertIdentityAt(t, got[i], want[i])
	}
}

func assertIdentityAt(t *testing.T, got IdentityAttribute, want IdentityAttribute) {
	t.Helper()
	if got.Name != want.Name || got.TerraformPath != want.TerraformPath || got.Required != want.Required {
		t.Fatalf("identity = %#v, want %#v", got, want)
	}
	if !equalStrings(got.RequestKeys, want.RequestKeys) || !equalStrings(got.ResponsePaths, want.ResponsePaths) {
		t.Fatalf("identity = %#v, want %#v", got, want)
	}
}

func equalSupportedTypes(left, right []SupportedType) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].Provider != right[i].Provider || left[i].Type != right[i].Type || !equalStrings(left[i].Kinds, right[i].Kinds) {
			return false
		}
	}
	return true
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
