package requestbinding

import (
	"testing"

	"github.com/OpenUdon/ramen/tfmapping"
)

func TestBuildNestsDottedBodyRequestKeys(t *testing.T) {
	request := Build(Options{
		Object:      tfmapping.Object{Kind: "resource", Type: "aws_s3_bucket_public_access_block"},
		SourceKind:  tfmapping.APISourceKindAWSSmithy,
		OperationID: "PutPublicAccessBlock",
		Attributes: map[string]any{
			"block_public_policy": false,
		},
		Identity: map[string]any{
			"bucket": "example-bucket",
		},
		Identities: []tfmapping.IdentityAttribute{{
			Name:          "bucket",
			TerraformPath: "bucket",
			RequestKeys:   []string{"Bucket"},
			Required:      true,
		}},
	})

	body, ok := request["body"].(map[string]any)
	if !ok {
		t.Fatalf("expected request body, got %#v", request)
	}
	config, ok := body["PublicAccessBlockConfiguration"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested public access block configuration, got %#v", body)
	}
	if got := config["BlockPublicPolicy"]; got != false {
		t.Fatalf("unexpected public access block policy value: %#v", got)
	}
	if got := body["Bucket"]; got != "example-bucket" {
		t.Fatalf("unexpected bucket binding: %#v", got)
	}
	if _, ok := body["PublicAccessBlockConfiguration.BlockPublicPolicy"]; ok {
		t.Fatalf("body retained flat dotted key: %#v", body)
	}
}
