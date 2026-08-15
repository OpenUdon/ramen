// Package ansibleconvert converts Ansible playbooks into reviewable UWS
// workflow artifacts. Module operations are extension-owned calls carrying
// ramen.ansible-module-call.1.0 for every supported UWS target. Conversion is
// review-first and fail-closed: constructs that cannot be
// faithfully lowered (complex Jinja2, dynamic includes, unknown modules)
// become diagnostics, not guesses. The converter never executes Ansible,
// modules, or UWS workflows.
package ansibleconvert

// Supported values for Options.TargetUWS. These select only the uws version
// declared by the emitted document; the shape is identical across all three,
// because Ansible module leaves are always extension-owned operations carrying
// ramen.ansible-module-call.1.0.
const (
	TargetUWS15 = "1.5"
	TargetUWS16 = "1.6"
	TargetUWS17 = "1.7"
)

// ArgspecInput names a collection argspec document (ramen.ansible.1.0 shape)
// supplied on the command line as ID=PATH. The ID is the raw argspec lookup
// key and is preserved in emitted module-call review references.
type ArgspecInput struct {
	ID   string
	Path string
}

// Options configures a single playbook conversion.
type Options struct {
	PlaybookPath string
	Argspecs     []ArgspecInput
	OutDir       string
	// Strict is accepted for CLI compatibility but is not used by Ansible
	// conversion. Unsupported constructs always emit strict-failure diagnostics.
	Strict           bool
	ProjectDir       string
	RolesPaths       []string
	CollectionsPaths []string
	InventoryPaths   []string
	ExtraVars        []string
	// TargetUWS selects the uws version declared by the emitted document:
	// "1.5", "1.6", or "1.7". Empty defaults to "1.5", the most widely
	// compatible version that accepts the module-call supplement.
	TargetUWS string
	// IgnoreUnsupported allows workflow artifacts to be written even when
	// strict-failure diagnostics were emitted. The resulting workflow omits the
	// unsupported constructs identified by diagnostics.
	IgnoreUnsupported bool
}

// LowerOptions configures static lowering choices after playbook parsing.
type LowerOptions struct {
	HostFanOut bool
	TargetUWS  string
}

// Diagnostic codes emitted by the Ansible converter. Catalog entries live in
// the ramen diagnostic package.
const (
	CodeJinjaUnsupported    = "ansible.jinja_unsupported"
	CodeModuleUnknown       = "ansible.module_unknown"
	CodeArgspecViolation    = "ansible.argspec_violation"
	CodeNoLogLiteral        = "ansible.nolog_literal"
	CodeAlwaysUnsupported   = "ansible.always_unsupported"
	CodeRescueTodo          = "ansible.rescue_todo"
	CodeDynamicInclude      = "ansible.dynamic_include"
	CodeDirectiveTodo       = "ansible.directive_todo"
	CodeDelegateUnsupported = "ansible.delegate_unsupported"
	CodeHostsRuntimeOwned   = "ansible.hosts_runtime_owned"
	CodeHandlerUnnotified   = "ansible.handler_unnotified"
	CodePlaybookShape       = "ansible.playbook_shape"
	CodeStaticResolution    = "ansible.static_resolution"
	CodeVariableConflict    = "ansible.variable_conflict"
)

// Diagnostic is a single review finding produced during conversion.
type Diagnostic struct {
	Code          string `json:"code"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	Task          string `json:"task,omitempty"`
	StrictFailure bool   `json:"strict_failure"`
}

// Result reports where conversion artifacts were written.
type Result struct {
	UWSPath         string
	HCLPath         string
	DiagnosticsJSON string
	DiagnosticsMD   string
	ReviewMD        string
	Diagnostics     []Diagnostic
	StrictFailures  int
}
