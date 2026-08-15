package tfconvert

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/OpenUdon/tfconfig"
)

const (
	providerSchemaFormatVersion = "1.0"
	maxProviderSchemaBytes      = 32 << 20
)

type providerSchemaSnapshot struct {
	FormatVersion   string                          `json:"format_version"`
	ProviderSchemas map[string]providerSchemaRecord `json:"provider_schemas"`
}

type providerSchemaRecord struct {
	Provider          providerSchemaType            `json:"provider"`
	ResourceSchemas   map[string]providerSchemaType `json:"resource_schemas"`
	DataSourceSchemas map[string]providerSchemaType `json:"data_source_schemas"`
}

type providerSchemaType struct {
	Version int                 `json:"version"`
	Block   providerSchemaBlock `json:"block"`
}

type providerSchemaBlock struct {
	Attributes map[string]providerSchemaAttribute `json:"attributes"`
	BlockTypes map[string]providerSchemaNested    `json:"block_types"`
}

type providerSchemaNested struct {
	NestingMode string              `json:"nesting_mode"`
	MinItems    int                 `json:"min_items,omitempty"`
	MaxItems    int                 `json:"max_items,omitempty"`
	Block       providerSchemaBlock `json:"block"`
}

type providerSchemaAttribute struct {
	Optional  bool `json:"optional,omitempty"`
	Required  bool `json:"required,omitempty"`
	Computed  bool `json:"computed,omitempty"`
	Sensitive bool `json:"sensitive,omitempty"`
}

type providerSchemaDoc struct {
	ID            string
	Path          string
	Address       string
	Schema        providerSchemaRecord
	ResourceTypes []string
	DataTypes     []string
}

type providerSchemaEvidence struct {
	ID              string   `json:"id"`
	Address         string   `json:"address"`
	FormatVersion   string   `json:"format_version"`
	Source          string   `json:"source"`
	ResourceTypes   []string `json:"resource_types,omitempty"`
	DataSourceTypes []string `json:"data_source_types,omitempty"`
}

func (c *conversionState) loadProviderSchemas() {
	seen := map[string]bool{}
	for _, input := range c.opts.ProviderSchemas {
		switch {
		case input.ID == "":
			c.addProviderSchemaDiagnostic(input.ID, "provider_schema.invalid_input", "--provider-schema ID is required")
			continue
		case input.Path == "":
			c.addProviderSchemaDiagnostic(input.ID, "provider_schema.invalid_input", fmt.Sprintf("--provider-schema %s path is required", input.ID))
			continue
		case seen[input.ID]:
			c.addProviderSchemaDiagnostic(input.ID, "provider_schema.duplicate_id", fmt.Sprintf("provider schema ID %q is duplicated", input.ID))
			continue
		}
		seen[input.ID] = true
		info, err := os.Stat(input.Path)
		if err != nil {
			c.addProviderSchemaDiagnostic(input.ID, "provider_schema.load_error", fmt.Sprintf("read provider schema %s: %v", input.Path, err))
			continue
		}
		if !info.Mode().IsRegular() {
			c.addProviderSchemaDiagnostic(input.ID, "provider_schema.load_error", fmt.Sprintf("provider schema %s is not a regular file", input.Path))
			continue
		}
		if info.Size() > maxProviderSchemaBytes {
			c.addProviderSchemaDiagnostic(input.ID, "provider_schema.too_large", fmt.Sprintf("provider schema %s is %d bytes; limit is %d", input.Path, info.Size(), maxProviderSchemaBytes))
			continue
		}
		data, err := os.ReadFile(input.Path)
		if err != nil {
			c.addProviderSchemaDiagnostic(input.ID, "provider_schema.load_error", fmt.Sprintf("read provider schema %s: %v", input.Path, err))
			continue
		}
		var snapshot providerSchemaSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			c.addProviderSchemaDiagnostic(input.ID, "provider_schema.invalid_json", fmt.Sprintf("parse provider schema %s: %v", input.Path, err))
			continue
		}
		if snapshot.FormatVersion != providerSchemaFormatVersion {
			c.addProviderSchemaDiagnostic(input.ID, "provider_schema.unsupported_format", fmt.Sprintf("provider schema %s has format_version %q; want %q", input.Path, snapshot.FormatVersion, providerSchemaFormatVersion))
			continue
		}
		address, schema, matches := selectProviderSchema(input.ID, snapshot.ProviderSchemas)
		switch len(matches) {
		case 0:
			c.addProviderSchemaDiagnostic(input.ID, "provider_schema.provider_not_found", fmt.Sprintf("provider schema %s does not contain provider %q", input.Path, input.ID))
			continue
		case 1:
		default:
			c.addProviderSchemaDiagnostic(input.ID, "provider_schema.provider_ambiguous", fmt.Sprintf("provider schema %s contains multiple providers matching %q: %s", input.Path, input.ID, strings.Join(matches, ", ")))
			continue
		}
		if len(schema.ResourceSchemas) == 0 && len(schema.DataSourceSchemas) == 0 {
			c.addProviderSchemaDiagnostic(input.ID, "provider_schema.empty", fmt.Sprintf("provider %q in %s has no resource or data-source schemas", address, input.Path))
			continue
		}
		c.providerSchemas = append(c.providerSchemas, providerSchemaDoc{
			ID: input.ID, Path: input.Path, Address: address, Schema: schema,
			ResourceTypes: sortedSchemaKeys(schema.ResourceSchemas),
			DataTypes:     sortedSchemaKeys(schema.DataSourceSchemas),
		})
	}
}

func selectProviderSchema(id string, schemas map[string]providerSchemaRecord) (string, providerSchemaRecord, []string) {
	if schema, ok := schemas[id]; ok {
		return id, schema, []string{id}
	}
	var matches []string
	for address := range schemas {
		if path.Base(strings.TrimSpace(address)) == id {
			matches = append(matches, address)
		}
	}
	slices.Sort(matches)
	if len(matches) == 1 {
		return matches[0], schemas[matches[0]], matches
	}
	return "", providerSchemaRecord{}, matches
}

func sortedSchemaKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func (c *conversionState) addProviderSchemaDiagnostic(id, code, message string) {
	c.addDiagnostic(Diagnostic{
		Code: code, Severity: "error", Message: message,
		ProviderSchemaID: id, StrictFailure: true,
	})
}

func (c *conversionState) validateProviderConfigurations() {
	if len(c.providerSchemas) == 0 {
		return
	}
	for _, mod := range c.doc.Modules {
		for _, resource := range mod.Resources {
			c.validateProviderObject("resource", fullAddress(mod.Address, resource.Address), mod.Address, resource.Type, resource.Provider, resource.Config, resource.Range)
		}
		for _, dataSource := range mod.DataSources {
			c.validateProviderObject("data_source", fullAddress(mod.Address, dataSource.Address), mod.Address, dataSource.Type, dataSource.Provider, dataSource.Config, dataSource.Range)
		}
	}
}

func (c *conversionState) validateProviderObject(kind, address, moduleAddress, objectType string, provider *tfconfig.ProviderRef, config []tfconfig.Attribute, objectRange *tfconfig.SourceRange) {
	localName := providerLocalNameForSchema(provider, objectType)
	doc, ok := c.providerSchemaForLocalName(localName)
	if !ok {
		return
	}
	var objectSchema providerSchemaType
	switch kind {
	case "resource":
		objectSchema, ok = doc.Schema.ResourceSchemas[objectType]
	case "data_source":
		objectSchema, ok = doc.Schema.DataSourceSchemas[objectType]
	}
	if !ok {
		code := "provider_schema.resource_type_missing"
		label := "resource"
		if kind == "data_source" {
			code = "provider_schema.data_source_type_missing"
			label = "data source"
		}
		c.addProviderObjectDiagnostic(doc.ID, code, fmt.Sprintf("offline provider schema %q does not define %s type %q", doc.ID, label, objectType), address, moduleAddress, objectRange)
		return
	}

	configured := map[string]*tfconfig.SourceRange{}
	for _, attribute := range config {
		root := providerSchemaRootPath(attribute.Path)
		if root == "" {
			continue
		}
		if _, exists := configured[root]; !exists {
			configured[root] = firstRange(attribute.Range, attribute.Value.Range)
		}
		mode, exists := objectSchema.Block.Attributes[root]
		if exists {
			if !validProviderSchemaAttributeMode(mode) {
				c.addProviderObjectDiagnostic(doc.ID, "provider_schema.attribute_mode_invalid", fmt.Sprintf("offline provider schema %q has an invalid required/optional/computed mode for %s.%s", doc.ID, objectType, root), address, moduleAddress, firstRange(attribute.Range, objectRange))
				continue
			}
			if mode.Computed && !mode.Optional && !mode.Required {
				c.addProviderObjectDiagnostic(doc.ID, "provider_schema.computed_only_configured", fmt.Sprintf("%s configures computed-only provider attribute %q according to offline schema %q", address, root, doc.ID), address, moduleAddress, firstRange(attribute.Range, objectRange))
			}
			continue
		}
		if _, exists := objectSchema.Block.BlockTypes[root]; exists {
			continue
		}
		c.addProviderObjectDiagnostic(doc.ID, "provider_schema.attribute_unknown", fmt.Sprintf("%s configures attribute or block %q that is absent from offline schema %q for %s", address, root, doc.ID, objectType), address, moduleAddress, firstRange(attribute.Range, objectRange))
	}

	for _, name := range sortedSchemaKeys(objectSchema.Block.Attributes) {
		mode := objectSchema.Block.Attributes[name]
		if !validProviderSchemaAttributeMode(mode) {
			c.addProviderObjectDiagnostic(doc.ID, "provider_schema.attribute_mode_invalid", fmt.Sprintf("offline provider schema %q has an invalid required/optional/computed mode for %s.%s", doc.ID, objectType, name), address, moduleAddress, objectRange)
			continue
		}
		if mode.Required {
			if _, exists := configured[name]; !exists {
				c.addProviderObjectDiagnostic(doc.ID, "provider_schema.required_attribute_missing", fmt.Sprintf("%s omits required provider attribute %q according to offline schema %q", address, name, doc.ID), address, moduleAddress, objectRange)
			}
		}
	}
	for _, name := range sortedSchemaKeys(objectSchema.Block.BlockTypes) {
		block := objectSchema.Block.BlockTypes[name]
		if block.MinItems > 0 {
			if _, exists := configured[name]; !exists {
				c.addProviderObjectDiagnostic(doc.ID, "provider_schema.required_block_missing", fmt.Sprintf("%s omits provider block %q with min_items %d according to offline schema %q", address, name, block.MinItems, doc.ID), address, moduleAddress, objectRange)
			}
		}
	}
}

func providerLocalNameForSchema(provider *tfconfig.ProviderRef, objectType string) string {
	if provider != nil {
		if localName := strings.TrimSpace(provider.LocalName); localName != "" {
			return localName
		}
		if localName := providerLocalName(provider.Address); localName != "" {
			return localName
		}
	}
	localName, _, _ := strings.Cut(strings.TrimSpace(objectType), "_")
	return localName
}

func (c *conversionState) providerSchemaForLocalName(localName string) (providerSchemaDoc, bool) {
	for _, schema := range c.providerSchemas {
		if schema.ID == localName || path.Base(schema.Address) == localName {
			return schema, true
		}
	}
	return providerSchemaDoc{}, false
}

func providerSchemaRootPath(attributePath string) string {
	attributePath = strings.TrimSpace(attributePath)
	if index := strings.IndexAny(attributePath, ".["); index >= 0 {
		return attributePath[:index]
	}
	return attributePath
}

func validProviderSchemaAttributeMode(attribute providerSchemaAttribute) bool {
	if attribute.Required {
		return !attribute.Optional && !attribute.Computed
	}
	return attribute.Optional || attribute.Computed
}

func (c *conversionState) addProviderObjectDiagnostic(id, code, message, address, moduleAddress string, rng *tfconfig.SourceRange) {
	c.addDiagnostic(Diagnostic{
		Code: code, Severity: "error", Message: message, Address: address,
		ModuleAddress: moduleAddress, ProviderSchemaID: id,
		SourceRange: convertRange(rng), StrictFailure: true,
	})
}

func renderProviderSchemaEvidence(docs []providerSchemaDoc) []providerSchemaEvidence {
	out := make([]providerSchemaEvidence, 0, len(docs))
	for _, doc := range docs {
		out = append(out, providerSchemaEvidence{
			ID: doc.ID, Address: doc.Address, FormatVersion: providerSchemaFormatVersion,
			Source: path.Base(doc.Path), ResourceTypes: slices.Clone(doc.ResourceTypes), DataSourceTypes: slices.Clone(doc.DataTypes),
		})
	}
	return out
}
