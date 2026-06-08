package tfmapping

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMappingHardeningContractsMarshal(t *testing.T) {
	mapping := Mapping{
		Object: Object{Kind: "resource", Type: "aws_example"},
		Schema: []SchemaPath{{
			Path:                    "url",
			Type:                    "string",
			Computed:                true,
			Identity:                true,
			ResponseDerivedIdentity: true,
		}},
		RequestBindings: []RequestBinding{{
			OperationRole: OperationRoleCreate,
			Path:          "name",
			RequestPath:   "QueueName",
			Encoding:      "base64",
			Required:      true,
		}},
		ResponseBindings: []ResponseBinding{{
			OperationRole:           OperationRoleCreate,
			ResponsePath:            "QueueUrl",
			StatePath:               "url",
			Identity:                true,
			ResponseDerivedIdentity: true,
		}},
		Normalizers: []Normalizer{{
			Path: "policy",
			Kind: NormalizerJSONSemantic,
		}},
		Lifecycle: &LifecycleSemantics{
			OperationRoles: []OperationRole{OperationRoleCreate, OperationRoleRead, OperationRoleDelete, OperationRoleRemoveConfig},
			Paths: []LifecyclePath{{
				Path:            "name",
				ReplaceOnChange: true,
			}},
		},
	}

	data, err := json.Marshal(mapping)
	if err != nil {
		t.Fatalf("marshal mapping contract: %v", err)
	}
	for _, want := range []string{
		`"response_derived_identity":true`,
		`"request_path":"QueueName"`,
		`"encoding":"base64"`,
		`"response_path":"QueueUrl"`,
		`"kind":"json_semantic"`,
		`"remove_config"`,
		`"replace_on_change":true`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("mapping contract JSON missing %s: %s", want, data)
		}
	}
}
