// Package ansibleconvert converts Ansible playbooks into reviewable UWS
// workflow artifacts. UWS 1.6 output binds module operations to ansible-module
// sources; UWS 1.5 compatibility output emits extension-owned module calls.
// Conversion is review-first and fail-closed: constructs that cannot be
// faithfully lowered (complex Jinja2, dynamic includes, unknown modules)
// become diagnostics, not guesses. The converter never executes Ansible,
// modules, or UWS workflows.
package ansibleconvert

const (
	TargetUWS16 = "1.6"
	TargetUWS15 = "1.5"
)

// ArgspecInput names a collection argspec document (uws.ansible.1.0 shape)
// supplied on the command line as ID=PATH. The ID is the raw argspec lookup
// source; conversion sanitizes it before emitting a UWS sourceDescription name.
type ArgspecInput struct {
	ID   string
	Path string
}

// Options configures a single playbook conversion.
type Options struct {
	PlaybookPath     string
	Argspecs         []ArgspecInput
	OutDir           string
	Strict           bool
	ProjectDir       string
	RolesPaths       []string
	CollectionsPaths []string
	InventoryPaths   []string
	ExtraVars        []string
	// TargetUWS selects the emitted document shape. Empty defaults to "1.6".
	// Target "1.5" emits extension-owned Ansible module leaves.
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
