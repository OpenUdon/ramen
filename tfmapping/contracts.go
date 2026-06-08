package tfmapping

// SchemaPath describes provider-neutral metadata for a desired or observed
// resource path. Command-specific validation and planning packages consume this
// contract; provider SDK schemas do not.
type SchemaPath struct {
	Path                    string   `json:"path"`
	Type                    string   `json:"type,omitempty"`
	EnumValues              []string `json:"enum_values,omitempty"`
	Required                bool     `json:"required,omitempty"`
	Optional                bool     `json:"optional,omitempty"`
	Computed                bool     `json:"computed,omitempty"`
	Sensitive               bool     `json:"sensitive,omitempty"`
	Identity                bool     `json:"identity,omitempty"`
	ResponseDerivedIdentity bool     `json:"response_derived_identity,omitempty"`
	Immutable               bool     `json:"immutable,omitempty"`
	CreateOnly              bool     `json:"create_only,omitempty"`
	Updateable              bool     `json:"updateable,omitempty"`
	ReplaceOnChange         bool     `json:"replace_on_change,omitempty"`
	ReadOnly                bool     `json:"read_only,omitempty"`
	Ignored                 bool     `json:"ignored,omitempty"`
}

// RequestBinding maps one desired-state or identity path into one API request
// path for a specific operation role.
type RequestBinding struct {
	OperationRole OperationRole `json:"operation_role"`
	OperationID   string        `json:"operation_id,omitempty"`
	Path          string        `json:"path"`
	RequestPath   string        `json:"request_path"`
	Location      string        `json:"location,omitempty"`
	Encoding      string        `json:"encoding,omitempty"`
	Required      bool          `json:"required,omitempty"`
	Identity      bool          `json:"identity,omitempty"`
}

// ResponseBinding maps one API response path into state, identity, computed,
// observed, or sensitive evidence.
type ResponseBinding struct {
	OperationRole           OperationRole `json:"operation_role"`
	OperationID             string        `json:"operation_id,omitempty"`
	ResponsePath            string        `json:"response_path"`
	StatePath               string        `json:"state_path"`
	Identity                bool          `json:"identity,omitempty"`
	ResponseDerivedIdentity bool          `json:"response_derived_identity,omitempty"`
	Computed                bool          `json:"computed,omitempty"`
	Observed                bool          `json:"observed,omitempty"`
	Sensitive               bool          `json:"sensitive,omitempty"`
}

type NormalizerKind string

const (
	NormalizerJSONSemantic              NormalizerKind = "json_semantic"
	NormalizerUnorderedCollection       NormalizerKind = "unordered_collection"
	NormalizerEmptyNullAbsentEquivalent NormalizerKind = "empty_null_absent_equivalent"
	NormalizerCaseFold                  NormalizerKind = "case_fold"
	NormalizerSensitivePlaceholder      NormalizerKind = "sensitive_placeholder"
)

type Normalizer struct {
	Path string         `json:"path"`
	Kind NormalizerKind `json:"kind"`
}

type OperationRole string

const (
	OperationRoleCreate       OperationRole = "create"
	OperationRoleRead         OperationRole = "read"
	OperationRoleUpdate       OperationRole = "update"
	OperationRoleReplace      OperationRole = "replace"
	OperationRoleDelete       OperationRole = "delete"
	OperationRoleSuspend      OperationRole = "suspend"
	OperationRoleDisable      OperationRole = "disable"
	OperationRoleDetach       OperationRole = "detach"
	OperationRoleRemoveConfig OperationRole = "remove_config"
	OperationRoleNoop         OperationRole = "noop"
)

type LifecycleSemantics struct {
	OperationRoles []OperationRole `json:"operation_roles,omitempty"`
	Paths          []LifecyclePath `json:"paths,omitempty"`
}

type LifecyclePath struct {
	Path            string `json:"path"`
	Immutable       bool   `json:"immutable,omitempty"`
	CreateOnly      bool   `json:"create_only,omitempty"`
	Updateable      bool   `json:"updateable,omitempty"`
	ReplaceOnChange bool   `json:"replace_on_change,omitempty"`
	Computed        bool   `json:"computed,omitempty"`
	Ignored         bool   `json:"ignored,omitempty"`
}
