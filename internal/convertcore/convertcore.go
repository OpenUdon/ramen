// Package convertcore holds the format-neutral UWS document emission shared by
// the Terraform and Ansible conversion frontends. It validates a document once
// and writes it as both YAML and HCL so converted projects are available in
// either serialization.
package convertcore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

// WriteDocumentFormats validates a UWS document once and writes it as both
// YAML and HCL.
func WriteDocumentFormats(doc *uws1.Document, yamlPath, hclPath string) error {
	if err := doc.Validate(); err != nil {
		return err
	}
	yamlData, err := convert.MarshalYAML(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(yamlPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(yamlPath, yamlData, 0o644); err != nil {
		return err
	}
	hclData, err := marshalDocumentHCL(doc, yamlData)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(hclPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(hclPath, hclData, 0o644)
}

func marshalDocumentHCL(doc *uws1.Document, yamlData []byte) ([]byte, error) {
	hclData, err := convert.MarshalHCL(doc)
	if err != nil {
		return nil, err
	}
	hclData = trimHCLTrailingWhitespace(hclData)
	if ok, _ := hclDocumentMatchesYAML(hclData, yamlData); ok {
		return hclData, nil
	}

	escapedDoc, err := hclEscapedDocument(doc)
	if err != nil {
		return nil, err
	}
	hclData, err = convert.MarshalHCL(escapedDoc)
	if err != nil {
		return nil, err
	}
	hclData = trimHCLTrailingWhitespace(hclData)
	if ok, err := hclDocumentMatchesYAML(hclData, yamlData); ok {
		return hclData, nil
	} else if err != nil {
		return nil, fmt.Errorf("generated UWS HCL does not parse: %w", err)
	}
	return nil, fmt.Errorf("generated UWS HCL does not match generated YAML")
}

func trimHCLTrailingWhitespace(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return []byte(strings.Join(lines, "\n"))
}

func hclEscapedDocument(doc *uws1.Document) (*uws1.Document, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	v = escapeHCLTemplateIntroducers(v)
	data, err = json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var escaped uws1.Document
	if err := json.Unmarshal(data, &escaped); err != nil {
		return nil, err
	}
	return &escaped, nil
}

func escapeHCLTemplateIntroducers(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			x[k] = escapeHCLTemplateIntroducers(value)
		}
		return x
	case []any:
		for i, value := range x {
			x[i] = escapeHCLTemplateIntroducers(value)
		}
		return x
	case string:
		x = strings.ReplaceAll(x, "${", "$${")
		x = strings.ReplaceAll(x, "%{", "%%{")
		return x
	default:
		return v
	}
}

func hclDocumentMatchesYAML(hclData, yamlData []byte) (bool, error) {
	hclJSON, err := convert.HCLToJSON(hclData)
	if err != nil {
		return false, err
	}
	yamlJSON, err := convert.YAMLToJSON(yamlData)
	if err != nil {
		return false, err
	}
	var hclDoc, yamlDoc any
	if err := json.Unmarshal(hclJSON, &hclDoc); err != nil {
		return false, err
	}
	if err := json.Unmarshal(yamlJSON, &yamlDoc); err != nil {
		return false, err
	}
	return reflect.DeepEqual(hclDoc, yamlDoc), nil
}
