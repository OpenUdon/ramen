package project

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestProjectSchemaFixedObjectsMatchWireTypes(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(embeddedProjectSchema, &schema); err != nil {
		t.Fatal(err)
	}
	definitions, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("project schema has no $defs object")
	}

	tests := []struct {
		name   string
		node   map[string]any
		goType reflect.Type
	}{
		{name: "profile", node: schema, goType: reflect.TypeFor[Profile]()},
		{name: "candidate-workflow", node: schemaDefinition(t, definitions, "candidate-workflow"), goType: reflect.TypeFor[CandidateWorkflow]()},
		{name: "variable", node: schemaDefinition(t, definitions, "variable"), goType: reflect.TypeFor[Variable]()},
		{name: "api-source", node: schemaDefinition(t, definitions, "api-source"), goType: reflect.TypeFor[APISource]()},
		{name: "resource", node: schemaDefinition(t, definitions, "resource"), goType: reflect.TypeFor[Resource]()},
		{name: "lifecycle", node: schemaDefinition(t, definitions, "lifecycle"), goType: reflect.TypeFor[Lifecycle]()},
		{name: "operation-role", node: schemaDefinition(t, definitions, "operation-role"), goType: reflect.TypeFor[OperationRole]()},
		{name: "runtime-hints", node: schemaDefinition(t, definitions, "runtime-hints"), goType: reflect.TypeFor[RuntimeHints]()},
		{name: "ai-metadata", node: schemaDefinition(t, definitions, "ai-metadata"), goType: reflect.TypeFor[AIMetadata]()},
		{name: "confidence", node: schemaDefinition(t, definitions, "confidence"), goType: reflect.TypeFor[Confidence]()},
		{name: "identity-attribute", node: schemaDefinition(t, definitions, "identity-attribute"), goType: reflect.TypeFor[IdentityAttribute]()},
		{name: "schema-path", node: schemaDefinition(t, definitions, "schema-path"), goType: reflect.TypeFor[SchemaPath]()},
		{name: "request-binding", node: schemaDefinition(t, definitions, "request-binding"), goType: reflect.TypeFor[RequestBinding]()},
		{name: "response-binding", node: schemaDefinition(t, definitions, "response-binding"), goType: reflect.TypeFor[ResponseBinding]()},
		{name: "normalizer", node: schemaDefinition(t, definitions, "normalizer"), goType: reflect.TypeFor[Normalizer]()},
		{name: "mapping-lifecycle", node: schemaDefinition(t, definitions, "mapping-lifecycle"), goType: reflect.TypeFor[MappingLifecycle]()},
		{name: "mapping-lifecycle-path", node: schemaDefinition(t, definitions, "mapping-lifecycle-path"), goType: reflect.TypeFor[MappingLifecyclePath]()},
		{name: "redaction", node: schemaDefinition(t, definitions, "redaction"), goType: reflect.TypeFor[Redaction]()},
	}
	covered := map[string]bool{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertSchemaPropertiesMatchWireType(t, test.node, test.goType)
		})
		covered[test.name] = true
	}
	for name, value := range definitions {
		node, ok := value.(map[string]any)
		if !ok || node["additionalProperties"] != false || node["properties"] == nil {
			continue
		}
		if !covered[name] {
			t.Errorf("closed project schema object %q has no wire-type parity assertion", name)
		}
	}
}

func schemaDefinition(t *testing.T, definitions map[string]any, name string) map[string]any {
	t.Helper()
	node, ok := definitions[name].(map[string]any)
	if !ok {
		t.Fatalf("project schema definition %q is missing or not an object", name)
	}
	return node
}

func assertSchemaPropertiesMatchWireType(t *testing.T, node map[string]any, goType reflect.Type) {
	t.Helper()
	properties, ok := node["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema object has no properties")
	}
	schemaNames := make([]string, 0, len(properties))
	for name := range properties {
		schemaNames = append(schemaNames, name)
	}
	slices.Sort(schemaNames)

	wireNames := make([]string, 0, goType.NumField())
	for i := range goType.NumField() {
		field := goType.Field(i)
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" {
			t.Fatalf("%s.%s has no JSON wire tag", goType.Name(), field.Name)
		}
		if name != "-" {
			wireNames = append(wireNames, name)
		}
	}
	slices.Sort(wireNames)
	if !slices.Equal(schemaNames, wireNames) {
		t.Fatalf("schema/type properties differ for %s: schema=%v type=%v", goType.Name(), schemaNames, wireNames)
	}
}

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
	doc.UWS = "1.9.1"
	doc.Operations[0].Outputs = map[string]string{"status": "$response.body.status"}
	doc.ContentTrust = &uws1.ContentTrust{Operations: map[string]*uws1.OperationContentTrust{
		"review": {Default: uws1.ContentTrustUnknown, Outputs: map[string]uws1.ContentTrustLevel{"status": uws1.ContentTrustTrusted}},
	}}
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
			if !reflect.DeepEqual(decoded.ContentTrust, doc.ContentTrust) {
				t.Fatalf("decoded %s contentTrust = %#v, want %#v", name, decoded.ContentTrust, doc.ContentTrust)
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
				"create": {Purpose: "create", Method: "POST", SourceKind: "openapi", SourceID: "widgets", SourcePath: "api.json", OperationID: "createWidget", UWSOperationRef: "create_widget", CredentialBindings: []string{"widgets"}, AI: &AIMetadata{Confidence: &Confidence{Score: 0.9, Reason: "fixture"}}},
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
