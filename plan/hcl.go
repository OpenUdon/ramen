package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func writePlanHCL(path string, document Document) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()
	setStringAttr(body, "version", document.Version)
	setStringAttr(body, "config_dir", document.ConfigDir)
	setStringAttr(body, "project_path", document.ProjectPath)
	setStringAttr(body, "state_path", document.StatePath)
	setStringAttr(body, "workspace", document.Workspace)
	setStringAttr(body, "action", document.Action)
	setStringAttr(body, "rationale", document.Rationale)
	if document.Errored {
		body.SetAttributeValue("errored", cty.BoolVal(true))
	}
	setStructuredAttr(body, "inputs", document.Inputs)
	setStructuredAttr(body, "governance", document.Governance)
	setStructuredAttr(body, "controls", document.Controls)
	setStructuredAttr(body, "summary", document.Summary)
	for _, source := range document.APISources {
		block := hclwrite.NewBlock("api_source", []string{source.ID})
		blockBody := block.Body()
		setStringAttr(blockBody, "kind", source.Kind)
		setStringAttr(blockBody, "path", source.Path)
		setStringAttr(blockBody, "digest", source.Digest)
		body.AppendBlock(block)
	}
	if document.Approval != nil {
		block := hclwrite.NewBlock("approval", nil)
		setStructuredObjectBody(block.Body(), *document.Approval)
		body.AppendBlock(block)
	}
	for _, resource := range document.Resources {
		block := hclwrite.NewBlock("resource", []string{resource.Address})
		blockBody := block.Body()
		setStringAttr(blockBody, "kind", resource.Kind)
		setStringAttr(blockBody, "type", resource.Type)
		setStringAttr(blockBody, "name", resource.Name)
		setStringAttr(blockBody, "provider", resource.Provider)
		setStringAttr(blockBody, "action", resource.Action)
		setStringAttr(blockBody, "reason", resource.Reason)
		setStringAttr(blockBody, "desired_hash", resource.DesiredHash)
		setStructuredAttr(blockBody, "dependencies", resource.Dependencies)
		setStructuredAttr(blockBody, "mapping", resource.Mapping)
		setStructuredAttr(blockBody, "ai", resource.AI)
		body.AppendBlock(block)
	}
	for _, diagnostic := range document.Diagnostics {
		block := hclwrite.NewBlock("diagnostic", []string{diagnostic.Code})
		setStructuredObjectBody(block.Body(), diagnostic)
		body.AppendBlock(block)
	}
	data := hclwrite.Format(file.Bytes())
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func setStringAttr(body *hclwrite.Body, name, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	body.SetAttributeValue(name, cty.StringVal(value))
}

func setStructuredObjectBody(body *hclwrite.Body, value any) {
	fields, ok := jsonObject(value)
	if !ok {
		return
	}
	for key, field := range fields {
		if isEmptyJSONValue(field) {
			continue
		}
		body.SetAttributeValue(key, ctyValue(field))
	}
}

func setStructuredAttr(body *hclwrite.Body, name string, value any) {
	generic, ok := jsonValue(value)
	if !ok || isEmptyJSONValue(generic) {
		return
	}
	body.SetAttributeValue(name, ctyValue(generic))
}

func jsonObject(value any) (map[string]any, bool) {
	generic, ok := jsonValue(value)
	if !ok {
		return nil, false
	}
	object, ok := generic.(map[string]any)
	return object, ok
}

func jsonValue(value any) (any, bool) {
	if value == nil {
		return nil, false
	}
	data, err := json.Marshal(value)
	if err != nil || bytes.Equal(data, []byte("null")) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var generic any
	if err := decoder.Decode(&generic); err != nil {
		return nil, false
	}
	return generic, true
}

func isEmptyJSONValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func ctyValue(value any) cty.Value {
	switch typed := value.(type) {
	case nil:
		return cty.NullVal(cty.DynamicPseudoType)
	case bool:
		return cty.BoolVal(typed)
	case string:
		return cty.StringVal(typed)
	case json.Number:
		if i, err := typed.Int64(); err == nil {
			return cty.NumberIntVal(i)
		}
		if f, _, err := big.ParseFloat(typed.String(), 10, 256, big.ToNearestEven); err == nil {
			return cty.NumberVal(f)
		}
		return cty.StringVal(typed.String())
	case float64:
		return cty.NumberFloatVal(typed)
	case []any:
		if len(typed) == 0 {
			return cty.EmptyTupleVal
		}
		values := make([]cty.Value, 0, len(typed))
		for _, item := range typed {
			values = append(values, ctyValue(item))
		}
		return cty.TupleVal(values)
	case map[string]any:
		if len(typed) == 0 {
			return cty.EmptyObjectVal
		}
		values := make(map[string]cty.Value, len(typed))
		for key, item := range typed {
			values[key] = ctyValue(item)
		}
		return cty.ObjectVal(values)
	default:
		return cty.StringVal(fmt.Sprint(typed))
	}
}
