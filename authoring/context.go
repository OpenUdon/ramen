package authoring

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/OpenUdon/apitools"
	"github.com/OpenUdon/authoring/promptcontext"
)

var newAPIToolsClient = func() *apitools.Client {
	return &apitools.Client{}
}

// APISourceInput identifies one API source document for interactive authoring.
// Local file paths are read in place; remote HTTP(S) URLs are materialized into
// DownloadDir before metadata extraction.
type APISourceInput struct {
	Kind        string
	ID          string
	Path        string
	DownloadDir string
}

// APISourceInputError reports an API source flag that cannot be converted
// into prompt-safe operation metadata.
type APISourceInputError struct {
	Code string
	Kind string
	ID   string
	Path string
}

func (err APISourceInputError) Error() string {
	switch err.Code {
	case "ramen.icot.api_source_unsupported":
		return fmt.Sprintf("unsupported API source kind %q for %s", err.Kind, firstNonEmpty(err.ID, err.Path, "input"))
	default:
		return "API source input must include non-empty kind, id, and path"
	}
}

// PromptContextFromAPISources loads API source metadata and translates it
// into Authoring's prompt-safe context contract.
func PromptContextFromAPISources(ctx context.Context, goal string, inputs []APISourceInput) (promptcontext.Context, error) {
	docs := make([]apitools.APISourceDocument, 0, len(inputs))
	sourceByName := map[string]APISourceInput{}
	for _, input := range inputs {
		rawKind := strings.TrimSpace(input.Kind)
		input.Kind = normalizeAPISourceKind(input.Kind)
		input.ID = strings.TrimSpace(input.ID)
		input.Path = strings.TrimSpace(input.Path)
		if rawKind == "" || input.ID == "" || input.Path == "" {
			return promptcontext.Context{}, APISourceInputError{Code: "ramen.icot.api_source_invalid", Kind: rawKind, ID: input.ID, Path: input.Path}
		}
		if input.Kind == "" {
			return promptcontext.Context{}, APISourceInputError{Code: "ramen.icot.api_source_unsupported", Kind: rawKind, ID: input.ID, Path: input.Path}
		}
		if isRemoteAPISource(input.Path) {
			materialized, err := materializeRemoteAPISource(ctx, input)
			if err != nil {
				return promptcontext.Context{}, err
			}
			input.Path = materialized
		} else {
			input.Path = absoluteLocalPath(input.Path)
		}
		rel := filepath.Join(input.Kind, input.ID, filepath.Base(input.Path))
		docs = append(docs, apitools.APISourceDocument{
			Kind:         input.Kind,
			Name:         input.ID,
			Path:         input.Path,
			RelativePath: rel,
		})
		sourceByName[input.ID] = input
	}
	inventory, err := apitools.BuildAPISourceOperationInventory(ctx, apitools.APISourceInventoryOptions{
		Documents: docs,
		Query:     strings.TrimSpace(goal),
	})
	if err != nil {
		return promptcontext.Context{}, err
	}
	out := promptcontext.Context{Version: promptcontext.Version, Metadata: map[string]string{"adapter": "ramen.icot"}}
	for _, doc := range inventory.Documents {
		input := sourceByName[firstNonEmpty(doc.Name, sourceNameFromRelativePath(doc.RelativePath))]
		out.Sources = append(out.Sources, promptcontext.SourceDocument{
			ID:      firstNonEmpty(input.ID, doc.Name, sourceNameFromRelativePath(doc.RelativePath)),
			Kind:    firstNonEmpty(input.Kind, "openapi"),
			Title:   doc.Title,
			Version: firstNonEmpty(doc.OpenAPI, doc.Swagger),
			URI:     firstNonEmpty(input.Path, doc.Path, doc.RelativePath, doc.URL),
			Summary: doc.Description,
			Metadata: map[string]string{
				"operation_count": intString(doc.OperationCount),
			},
		})
	}
	for _, op := range inventory.Operations {
		sourceID := firstNonEmpty(op.DocumentName, sourceNameFromRelativePath(op.DocumentRelativePath))
		input := sourceByName[sourceID]
		if sourceID == "" {
			sourceID = input.ID
		}
		requestSchemaID := ""
		if op.RequestBody != nil {
			requestSchemaID = "request:" + firstNonEmpty(op.OperationID, op.ID)
			out.Schemas = append(out.Schemas, schemaHintFromRequestBody(requestSchemaID, op.RequestBody))
		}
		metadata := map[string]string{
			"source_kind": firstNonEmpty(input.Kind, "openapi"),
			"source_path": firstNonEmpty(input.Path, op.DocumentPath, op.DocumentRelativePath),
		}
		parameters := operationParametersMetadataForOperation(op, input.Path)
		if len(parameters) > 0 {
			if data, err := json.Marshal(parameters); err == nil {
				metadata["parameters"] = string(data)
			}
		}
		out.Operations = append(out.Operations, promptcontext.OperationCandidate{
			ID:                 firstNonEmpty(op.ID, op.OperationID),
			SourceID:           sourceID,
			OperationID:        firstNonEmpty(op.OperationID, op.ID),
			Name:               op.OperationID,
			Verb:               op.Method,
			Path:               op.Path,
			Summary:            firstNonEmpty(op.Summary, op.Description),
			RequestSchemaID:    requestSchemaID,
			CredentialBindings: credentialBindings(op.Security),
			Tags:               append([]string(nil), op.Tags...),
			Confidence:         confidenceForScore(op.Score),
			SelectionRationale: selectionRationale(op.Score),
			Metadata:           metadata,
		})
	}
	for _, credential := range credentialsForOperations(inventory.Operations) {
		out.Credentials = append(out.Credentials, credential)
	}
	return promptcontext.Normalize(out), nil
}

type operationParameterMetadata struct {
	Name     string `json:"name,omitempty"`
	In       string `json:"in,omitempty"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required,omitempty"`
}

func operationParametersMetadata(parameters []apitools.ParameterSummary) []operationParameterMetadata {
	out := make([]operationParameterMetadata, 0, len(parameters))
	for _, parameter := range parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			continue
		}
		location := strings.ToLower(strings.TrimSpace(parameter.In))
		if location == "body" {
			continue
		}
		out = append(out, operationParameterMetadata{
			Name:     name,
			In:       location,
			Type:     firstNonEmpty(parameter.Type, parameter.Format, "string"),
			Required: parameter.Required,
		})
	}
	return out
}

func operationParametersMetadataForOperation(op apitools.OperationSummary, sourcePath string) []operationParameterMetadata {
	out := operationParametersMetadata(op.Parameters)
	seen := map[string]bool{}
	for _, parameter := range out {
		seen[strings.ToLower(parameter.In)+"\x00"+parameter.Name] = true
	}
	for _, name := range pathParameterNames(op.Path) {
		key := "path\x00" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, operationParameterMetadata{Name: name, In: "path", Type: "string", Required: true})
	}
	if apiVersionFromSourcePath(sourcePath) != "" && !parameterNameSeen(seen, "api-version") {
		out = append(out, operationParameterMetadata{Name: "api-version", In: "query", Type: "string", Required: true})
	}
	return out
}

func pathParameterNames(path string) []string {
	var out []string
	for {
		start := strings.Index(path, "{")
		if start < 0 {
			return out
		}
		rest := path[start+1:]
		end := strings.Index(rest, "}")
		if end < 0 {
			return out
		}
		name := strings.TrimSpace(rest[:end])
		if name != "" {
			out = append(out, name)
		}
		path = rest[end+1:]
	}
}

func parameterNameSeen(seen map[string]bool, name string) bool {
	for key := range seen {
		if strings.HasSuffix(key, "\x00"+name) {
			return true
		}
	}
	return false
}

func materializeRemoteAPISource(ctx context.Context, input APISourceInput) (string, error) {
	if !isHTTPAPISource(input.Path) {
		return "", fmt.Errorf("remote API source %q uses unsupported download scheme; use http(s) or a local file path", input.Path)
	}
	dir := strings.TrimSpace(input.DownloadDir)
	if dir == "" {
		return "", fmt.Errorf("remote API source %q requires a download directory", input.Path)
	}
	imported, err := newAPIToolsClient().Import(ctx, apitools.ImportOptions{
		URL:  input.Path,
		Dir:  dir,
		Name: input.ID,
	})
	if err != nil {
		return "", err
	}
	return imported.Path, nil
}

func absoluteLocalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "://") || strings.HasPrefix(path, "urn:") {
		return path
	}
	abs, err := filepath.Abs(filepath.FromSlash(path))
	if err != nil {
		return path
	}
	return abs
}

func isRemoteAPISource(path string) bool {
	path = strings.TrimSpace(path)
	return strings.Contains(path, "://") && !strings.HasPrefix(strings.ToLower(path), "file://")
}

func isHTTPAPISource(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

func schemaHintFromRequestBody(id string, body *apitools.RequestBodySummary) promptcontext.SchemaHint {
	if body == nil {
		return promptcontext.SchemaHint{}
	}
	schema := promptcontext.SchemaHint{
		ID:        id,
		Purpose:   "request",
		Summary:   body.Description,
		Required:  append([]string(nil), body.RequiredFieldPaths...),
		MediaType: first(body.ContentTypes),
	}
	for _, field := range body.Fields {
		schema.Fields = append(schema.Fields, promptcontext.FieldHint{
			Name:     field.Path,
			Type:     firstNonEmpty(field.Type, field.Format, field.Ref, "string"),
			Required: field.Required || slices.Contains(body.RequiredFieldPaths, field.Path),
			Summary:  field.Description,
		})
	}
	if len(schema.Fields) == 0 && body.Schema != nil {
		schema.Summary = firstNonEmpty(schema.Summary, body.Schema.Description)
		schema.Required = append(schema.Required, body.Schema.Required...)
		for _, prop := range body.Schema.Properties {
			schema.Fields = append(schema.Fields, promptcontext.FieldHint{
				Name:     prop.Name,
				Type:     firstNonEmpty(prop.Type, prop.Format, prop.Ref, "string"),
				Required: prop.Required || slices.Contains(body.Schema.Required, prop.Name),
				Summary:  prop.Description,
			})
		}
	}
	return schema
}

func credentialsForOperations(operations []apitools.OperationSummary) []promptcontext.CredentialBinding {
	seen := map[string]bool{}
	var out []promptcontext.CredentialBinding
	for _, op := range operations {
		for _, security := range op.Security {
			name := strings.TrimSpace(security.Name)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, promptcontext.CredentialBinding{
				Name:     name,
				Kind:     firstNonEmpty(security.Type, security.Scheme),
				Scope:    security.In,
				Required: true,
				Summary:  security.Description,
			})
		}
	}
	return out
}

func credentialBindings(security []apitools.SecuritySummary) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range security {
		name := strings.TrimSpace(item.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func normalizeAPISourceKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case apitools.APISourceKindOpenAPI, "swagger":
		return apitools.APISourceKindOpenAPI
	case apitools.APISourceKindAWSSmithy, "smithy", "smithy-json":
		return apitools.APISourceKindAWSSmithy
	case apitools.APISourceKindGoogleDiscovery, "google_discovery", "discovery", "google":
		return apitools.APISourceKindGoogleDiscovery
	default:
		return ""
	}
}

func sourceNameFromRelativePath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func confidenceForScore(score int) string {
	switch {
	case score >= 20:
		return "high"
	case score > 0:
		return "medium"
	default:
		return ""
	}
}

func selectionRationale(score int) string {
	if score <= 0 {
		return ""
	}
	return "ranked from local API source metadata"
}

func intString(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func first(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
