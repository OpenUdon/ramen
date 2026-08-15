package project

import (
	"encoding/json"
	"os"
	"testing"

	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestEmbeddedProjectSchemaValidatesOutsideRepository(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	data, err := json.Marshal(comprehensiveSchemaTestProfile())
	if err != nil {
		t.Fatal(err)
	}
	if err := validateProfileDocument(data); err != nil {
		t.Fatalf("embedded project schema failed outside repository: %v", err)
	}
}

func TestProjectSchemaRejectsInvalidStructuralFields(t *testing.T) {
	tests := map[string]string{
		"missing version":       `{}`,
		"wrong version":         `{"version":"ramen.project.v2"}`,
		"unknown profile field": `{"version":"ramen.project.v1","future":true}`,
		"unknown resource field": `{
  "version":"ramen.project.v1",
  "resources":[{"address":"example.test","kind":"resource","type":"example","future":true}]
}`,
		"unknown lifecycle field": `{
  "version":"ramen.project.v1",
  "resources":[{"address":"example.test","kind":"resource","type":"example","lifecycle":{"future":true}}]
}`,
		"missing operation id": `{
  "version":"ramen.project.v1",
  "resources":[{"address":"example.test","kind":"resource","type":"example","operations":{"create":{}}}]
}`,
		"incomplete candidate": `{
  "version":"ramen.project.v1",
  "candidate_workflows":[{"title":"Later"}]
}`,
		"invalid confidence type": `{
  "version":"ramen.project.v1",
  "resources":[{"address":"example.test","kind":"resource","type":"example","ai":{"confidence":{"score":"high"}}}]
}`,
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			if err := validateProfileDocument([]byte(document)); err == nil {
				t.Fatalf("invalid project profile was accepted: %s", document)
			}
		})
	}
}

func TestProjectSchemaAllowsDocumentedArbitraryMaps(t *testing.T) {
	document := []byte(`{
  "version":"ramen.project.v1",
  "metadata":{"custom":{"future":true}},
  "resources":[{
    "address":"example.test",
    "kind":"resource",
    "type":"example",
    "attributes":{"nested":{"anything":[1,true,null]}},
    "runtime_hints":{"retry":{"vendor_option":{"enabled":true}}},
    "metadata":{"owner":{"team":"platform"}}
  }]
}`)
	if err := validateProfileDocument(document); err != nil {
		t.Fatalf("documented arbitrary maps were rejected: %v", err)
	}
}

func TestProjectProfileSchemaAppliesAfterJSONYAMLAndHCLDecoding(t *testing.T) {
	doc := schemaTestUWSDocument(comprehensiveSchemaTestProfile())
	tests := map[string]struct {
		marshal   func(*uws1.Document) ([]byte, error)
		unmarshal func([]byte, *uws1.Document) error
	}{
		"json": {uwsconvert.MarshalJSON, uwsconvert.UnmarshalJSON},
		"yaml": {uwsconvert.MarshalYAML, uwsconvert.UnmarshalYAML},
		"hcl":  {uwsconvert.MarshalHCL, uwsconvert.UnmarshalHCL},
	}
	for name, format := range tests {
		t.Run(name, func(t *testing.T) {
			data, err := format.marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			var decoded uws1.Document
			if err := format.unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			profile, err := ProfileFromDocument(&decoded)
			if err != nil {
				t.Fatalf("validate decoded %s profile: %v", name, err)
			}
			if profile.Version != Version || len(profile.Resources) != 1 {
				t.Fatalf("decoded %s profile = %#v", name, profile)
			}
		})
	}
}

func TestProfileFromDocumentRejectsUnknownFieldsBeforeTypedDecode(t *testing.T) {
	doc := schemaTestUWSDocument(map[string]any{
		"version": Version,
		"unknown": "would previously be discarded",
	})
	if _, err := ProfileFromDocument(doc); err == nil {
		t.Fatal("profile with unknown field was accepted")
	}
}

func comprehensiveSchemaTestProfile() Profile {
	return Profile{
		Version:    Version,
		APISources: []APISource{{Kind: "openapi", ID: "widgets", Path: "api.json"}},
		Variables:  []Variable{{Name: "region", Type: "string", Default: "west", Sensitive: true}},
		CandidateWorkflows: []CandidateWorkflow{{
			Title: "Audit widgets", Outcome: "Audit changes", DeferralReason: "Later", PromotionTrigger: "Requested",
		}},
		Redaction: Redaction{Paths: []string{"example.test.secret"}},
		Metadata:  map[string]any{"source": "test", "custom": map[string]any{"enabled": true}},
		Resources: []Resource{{
			Address:      "example.test",
			Kind:         "resource",
			Type:         "example",
			Name:         "test",
			Provider:     "example",
			Attributes:   map[string]any{"name": "test", "nested": map[string]any{"count": 2}},
			Lifecycle:    Lifecycle{PreventDestroy: true, IgnorePaths: []string{"updated_at"}},
			Dependencies: []string{"data.example.source"},
			Operations: map[string]OperationRole{
				"create": {Purpose: "create", Method: "POST", SourceKind: "openapi", SourceID: "widgets", SourcePath: "api.json", OperationID: "createWidget", CredentialBindings: []string{"widgets"}, AI: &AIMetadata{Confidence: &Confidence{Score: 0.9, Reason: "fixture"}}},
			},
			IdentityAttributes: []IdentityAttribute{{Name: "name", Path: "name", RequestKeys: []string{"name"}, ResponsePaths: []string{"result.name"}, Required: true}},
			Schema:             []SchemaPath{{Path: "name", Type: "string", Required: true, Identity: true}},
			RequestBindings:    []RequestBinding{{OperationRole: "create", OperationID: "createWidget", Path: "name", RequestPath: "name", Location: "body", Required: true, Identity: true}},
			ResponseBindings:   []ResponseBinding{{OperationRole: "create", OperationID: "createWidget", ResponsePath: "result.name", StatePath: "name", Identity: true, Computed: true}},
			Normalizers:        []Normalizer{{Path: "name", Kind: "case_fold"}},
			MappingLifecycle:   &MappingLifecycle{OperationRoles: []string{"create"}, Paths: []MappingLifecyclePath{{Path: "name", Immutable: true}}},
			RequiredOperations: []string{"create"},
			CredentialBindings: []string{"widgets"},
			RuntimeHints:       &RuntimeHints{Retry: map[string]any{"max_attempts": 3}, Waiter: map[string]any{"until": "success"}, Settle: map[string]any{"duration": "1s"}},
			Redaction:          Redaction{Paths: []string{"secret"}},
			AI:                 &AIMetadata{Uncertainty: "none", Rationale: "fixture", Citations: []string{"test"}},
			Metadata:           map[string]any{"owner": "platform"},
		}},
	}
}

func schemaTestUWSDocument(profile any) *uws1.Document {
	return &uws1.Document{
		UWS:  "1.4.0",
		Info: &uws1.Info{Title: "schema_test", Version: "1.0.0"},
		Operations: []*uws1.Operation{{
			OperationID: "review",
			Request:     map[string]any{"x-test": true},
			Extensions:  map[string]any{uws1.ExtensionOperationProfile: "ramen-project-test"},
		}},
		Workflows:  []*uws1.Workflow{{WorkflowID: "main", Type: uws1.WorkflowTypeSequence, Steps: []*uws1.Step{{StepID: "review", OperationRef: "review"}}}},
		Extensions: map[string]any{ExtensionKey: profile},
	}
}
