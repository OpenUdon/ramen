//go:build udon

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OpenUdon/ramen/executor"
	udonexec "github.com/OpenUdon/ramen/executor/udon"
)

func newUdonExecutor(outputDir string) (executor.Executor, error) {
	return udonexec.Executor{
		OutputDir:       strings.TrimSpace(outputDir),
		OutputProjector: projectUdonExecutorOutput,
	}, nil
}

type udonIdentityAttribute struct {
	Name          string   `json:"name"`
	RequestKeys   []string `json:"request_keys,omitempty"`
	ResponsePaths []string `json:"response_paths,omitempty"`
	Required      bool     `json:"required,omitempty"`
}

func projectUdonExecutorOutput(_ context.Context, req executor.Request, outputDir string) (executor.Result, error) {
	body := udonResponseBody(req)
	computed := computedProjection(body)
	if req.Action.Action != "delete" && len(computed) == 0 {
		return executor.Result{}, fmt.Errorf("udon output projection for %s produced no response body; output dir: %s", req.Action.Address, outputDir)
	}
	identity, err := identityProjection(req, body, computed)
	if err != nil {
		return executor.Result{}, err
	}
	if req.Action.Action != "delete" && len(identity) == 0 {
		return executor.Result{}, fmt.Errorf("udon output projection for %s produced no identity facts; output dir: %s", req.Action.Address, outputDir)
	}
	return executor.Result{
		Address:   req.Action.Address,
		Operation: req.Action.Mapping.OperationID,
		Identity:  identity,
		Computed:  computed,
	}, nil
}

func udonResponseBody(req executor.Request) any {
	if req.Document == nil {
		return nil
	}
	records := req.Document.ExecutionRecords()
	if len(records) == 0 {
		return nil
	}
	for _, op := range req.Document.Operations {
		if op == nil || strings.TrimSpace(op.OperationID) == "" {
			continue
		}
		if record, ok := records["op:"+op.OperationID]; ok {
			if value, ok := record.Outputs["body"]; ok {
				return value
			}
			if record.Result != nil {
				return record.Result
			}
		}
	}
	for _, record := range records {
		if value, ok := record.Outputs["body"]; ok {
			return value
		}
		if record.Result != nil {
			return record.Result
		}
	}
	return nil
}

func computedProjection(body any) map[string]any {
	switch typed := body.(type) {
	case nil:
		return nil
	case map[string]any:
		return cloneMapAny(typed)
	default:
		return map[string]any{"result": typed}
	}
}

func identityProjection(req executor.Request, body any, computed map[string]any) (map[string]any, error) {
	attrs, err := udonIdentityAttributes(req.Action.Metadata["identity_attributes"])
	if err != nil {
		return nil, err
	}
	identity := map[string]any{}
	for _, attr := range attrs {
		if strings.TrimSpace(attr.Name) == "" {
			continue
		}
		if value, ok := firstPathValue(body, attr.ResponsePaths); ok {
			identity[attr.Name] = value
			continue
		}
		if value, ok := firstRequestKeyValue(req, attr.RequestKeys); ok {
			identity[attr.Name] = value
			continue
		}
		if attr.Required {
			return nil, fmt.Errorf("udon output projection missing required identity %q for %s", attr.Name, req.Action.Address)
		}
	}
	if len(identity) == 0 {
		for _, key := range []string{"id", "ID", "name", "Name", "arn", "Arn", "ARN", "selfLink"} {
			if value, ok := lookupPath(computed, strings.Split(key, ".")); ok {
				identity[strings.ToLower(key)] = value
				break
			}
		}
	}
	if len(identity) == 0 && req.Action.Action == "read" && len(computed) == 0 {
		return map[string]any{}, nil
	}
	return identity, nil
}

func udonIdentityAttributes(raw string) ([]udonIdentityAttribute, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var attrs []udonIdentityAttribute
	if err := json.Unmarshal([]byte(raw), &attrs); err != nil {
		return nil, fmt.Errorf("parse udon identity projection metadata: %w", err)
	}
	return attrs, nil
}

func firstPathValue(root any, paths []string) (any, bool) {
	for _, path := range paths {
		parts := strings.Split(strings.TrimSpace(path), ".")
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		if value, ok := lookupPath(root, parts); ok {
			return value, true
		}
		if value, ok := lookupPathDeep(root, parts); ok {
			return value, true
		}
	}
	return nil, false
}

func firstRequestKeyValue(req executor.Request, keys []string) (any, bool) {
	if req.Document == nil {
		return nil, false
	}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		for _, op := range req.Document.Operations {
			if op == nil {
				continue
			}
			if value, ok := lookupKeyDeep(op.Request, key); ok {
				return value, true
			}
		}
	}
	return nil, false
}

func lookupPath(root any, parts []string) (any, bool) {
	current := root
	for _, part := range parts {
		switch typed := current.(type) {
		case map[string]any:
			value, ok := typed[part]
			if !ok {
				return nil, false
			}
			current = value
		case []any:
			return nil, false
		default:
			return nil, false
		}
	}
	return current, true
}

func lookupPathDeep(root any, parts []string) (any, bool) {
	if value, ok := lookupPath(root, parts); ok {
		return value, true
	}
	switch typed := root.(type) {
	case map[string]any:
		for _, value := range typed {
			if found, ok := lookupPathDeep(value, parts); ok {
				return found, true
			}
		}
	case []any:
		for _, value := range typed {
			if found, ok := lookupPathDeep(value, parts); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func lookupKeyDeep(root any, key string) (any, bool) {
	switch typed := root.(type) {
	case map[string]any:
		if value, ok := typed[key]; ok {
			return value, true
		}
		for _, value := range typed {
			if found, ok := lookupKeyDeep(value, key); ok {
				return found, true
			}
		}
	case []any:
		for _, value := range typed {
			if found, ok := lookupKeyDeep(value, key); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func cloneMapAny(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}
