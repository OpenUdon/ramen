package tfconvert

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
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
