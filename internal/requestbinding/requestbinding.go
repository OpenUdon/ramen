package requestbinding

import (
	"strings"

	"github.com/OpenUdon/ramen/tfmapping"
)

type Options struct {
	Object      tfmapping.Object
	SourceKind  string
	SourceID    string
	SourcePath  string
	OperationID string
	Attributes  map[string]any
	Identity    map[string]any
	Identities  []tfmapping.IdentityAttribute
	Metadata    map[string]any
	Extension   string
}

func Build(opts Options) map[string]any {
	body := map[string]any{}
	query := map[string]any{}
	registry := tfmapping.DefaultRegistry()
	for attrPath, value := range opts.Attributes {
		if strings.TrimSpace(attrPath) == "" {
			continue
		}
		for _, requestKey := range registry.RequestKeys(opts.Object, opts.SourceKind, opts.OperationID, attrPath) {
			setBody(body, requestKey, value)
		}
	}
	for _, identity := range opts.Identities {
		value, ok := identityValue(opts.Identity, identity)
		if !ok {
			continue
		}
		requestKeys := registry.RequestKeys(opts.Object, opts.SourceKind, opts.OperationID, identity.TerraformPath)
		if len(requestKeys) == 0 {
			requestKeys = identity.RequestKeys
		}
		for _, requestKey := range requestKeys {
			setBody(body, requestKey, value)
		}
	}
	static := registry.StaticRequestBindings(opts.Object, opts.SourceID, opts.SourcePath, opts.OperationID)
	if len(static) == 0 && opts.SourceKind == tfmapping.APISourceKindAWSSmithy {
		static = registry.StaticRequestBindings(opts.Object, opts.SourceID, opts.SourcePath, "POST_"+opts.OperationID)
	}
	for requestKey, value := range static {
		setFlat(query, requestKey, value)
	}
	request := map[string]any{}
	if opts.Extension != "" && len(opts.Metadata) > 0 {
		request[opts.Extension] = opts.Metadata
	}
	if len(query) > 0 {
		request["query"] = query
	}
	if len(body) > 0 {
		request["body"] = body
	}
	return request
}

func setFlat(target map[string]any, key string, value any) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	if _, ok := target[key]; !ok {
		target[key] = value
	}
}

func setBody(target map[string]any, key string, value any) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	parts := strings.Split(key, ".")
	current := target
	for _, part := range parts[:len(parts)-1] {
		part = strings.TrimSpace(part)
		if part == "" {
			return
		}
		next, ok := current[part]
		if !ok {
			nested := map[string]any{}
			current[part] = nested
			current = nested
			continue
		}
		nested, ok := next.(map[string]any)
		if !ok {
			return
		}
		current = nested
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return
	}
	if _, ok := current[last]; !ok {
		current[last] = value
	}
}

func identityValue(identity map[string]any, attr tfmapping.IdentityAttribute) (any, bool) {
	if len(identity) == 0 {
		return nil, false
	}
	for _, key := range []string{attr.Name, attr.TerraformPath} {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if value, ok := identity[key]; ok {
			return value, true
		}
	}
	return nil, false
}
