package diagnostic

import "slices"

const CatalogVersion = "ramen.diagnostics.v1"

type Catalog struct {
	Version string  `json:"version"`
	Entries []Entry `json:"entries"`
}

type Entry struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Meaning     string `json:"meaning"`
	LikelyCause string `json:"likely_cause"`
	Repair      string `json:"repair"`
}

func CatalogV1() Catalog {
	entries := []Entry{
		{Code: "api_source.invalid", Severity: "error", Meaning: "An API source flag is malformed.", LikelyCause: "Missing kind, ID, or path.", Repair: "Use --api-source KIND:ID=PATH with a supported kind."},
		{Code: "api_source.load_error", Severity: "error", Meaning: "An API source document could not be loaded.", LikelyCause: "The path is missing, unreadable, or not a supported API source document.", Repair: "Fix the local source path or regenerate the source inventory."},
		{Code: "graph.dependency_cycle", Severity: "error", Meaning: "The native resource graph contains a cycle.", LikelyCause: "Two or more resources depend on each other.", Repair: "Break the dependency cycle in the native project metadata."},
		{Code: "import.identity_invalid", Severity: "error", Meaning: "Import identity input is not a JSON object.", LikelyCause: "The --identity value is malformed or null.", Repair: "Provide a JSON object with the declared identity fields."},
		{Code: "mapping.operation_unavailable", Severity: "error", Meaning: "No API operation matched the resource role.", LikelyCause: "The API source lacks the operation or the mapping metadata is incomplete.", Repair: "Add or correct the operation role in the native project."},
		{Code: "approval.identity_invalid", Severity: "error", Meaning: "Approval identity metadata is invalid.", LikelyCause: "The approver identity or deterministic approved_at timestamp is missing.", Repair: "Provide --approved-by and --approved-at in RFC3339 format when adding approval metadata."},
		{Code: "plan.prevent_destroy", Severity: "error", Meaning: "A planned delete or replace violates prevent_destroy.", LikelyCause: "The resource lifecycle blocks destructive action.", Repair: "Remove the destructive control or intentionally change lifecycle metadata."},
		{Code: "policy.deny", Severity: "error", Meaning: "A plan-time policy denied a resource action.", LikelyCause: "A Ramen policy file denies the action or resource under review.", Repair: "Change the desired state, select a different policy, or route through the required governance process."},
		{Code: "policy.file_load_error", Severity: "error", Meaning: "A policy file could not be read.", LikelyCause: "The policy path is missing or unreadable.", Repair: "Fix the --policy-file path or permissions."},
		{Code: "policy.file_parse_error", Severity: "error", Meaning: "A policy file could not be parsed.", LikelyCause: "The policy file is not valid JSON or YAML.", Repair: "Fix the policy document syntax."},
		{Code: "policy.max_deletes", Severity: "error", Meaning: "A plan exceeds the policy delete/replacement limit.", LikelyCause: "The plan contains more destructive actions than the policy allows.", Repair: "Reduce the plan scope or update the policy through the governance process."},
		{Code: "policy.version_invalid", Severity: "error", Meaning: "A policy file declares an unsupported version.", LikelyCause: "The policy was written for a different Ramen policy contract.", Repair: "Use ramen.policy.v1 or update the Ramen binary and policy file together."},
		{Code: "policy.warn", Severity: "warning", Meaning: "A plan-time policy warning matched a resource action.", LikelyCause: "The action is allowed but requires operator attention.", Repair: "Review the warning before approving or executing the plan."},
		{Code: "project.operation_missing", Severity: "error", Meaning: "A required native project operation role is absent.", LikelyCause: "The resource lacks create, read, update, delete, or import metadata for the requested action.", Repair: "Add the missing operation role with source and operation IDs."},
		{Code: "run.approval_mismatch", Severity: "error", Meaning: "An imperative run approval digest does not match current inputs.", LikelyCause: "The UWS document, target set, workspace, or state path changed after review.", Repair: "Rerun ramen run --check and approve the new digest."},
		{Code: "run.document_invalid", Severity: "error", Meaning: "The imperative UWS document failed validation.", LikelyCause: "The UWS file is malformed or violates UWS schema rules.", Repair: "Fix the UWS document and run check mode again."},
		{Code: "run.executor_required", Severity: "error", Meaning: "Imperative execution requires a trusted executor.", LikelyCause: "No executor adapter was selected for a non-check run.", Repair: "Use --mock for public mock execution or configure an approved trusted executor adapter."},
		{Code: "state.open_error", Severity: "error", Meaning: "Ramen could not open local SQLite state.", LikelyCause: "The state path is missing, corrupt, locked, or newer than the binary supports.", Repair: "Run ramen init, inspect locks, restore a backup, or upgrade the binary."},
		{Code: "validate.dependency_cycle", Severity: "error", Meaning: "Validation found a resource dependency cycle.", LikelyCause: "The project graph is cyclic.", Repair: "Adjust dependencies so the graph is acyclic."},
		{Code: "validate.project_load_error", Severity: "error", Meaning: "The native project could not be loaded.", LikelyCause: "The project file is missing, invalid UWS, or has invalid Ramen metadata.", Repair: "Fix the project path and validate the UWS/Ramen profile."},
	}
	slices.SortFunc(entries, func(a, b Entry) int {
		if a.Code < b.Code {
			return -1
		}
		if a.Code > b.Code {
			return 1
		}
		return 0
	})
	return Catalog{Version: CatalogVersion, Entries: entries}
}
