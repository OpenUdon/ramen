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
		{Code: "apply.approval_required", Severity: "error", Meaning: "Apply found mutations but no explicit approval.", LikelyCause: "--auto-approve or a verified approval artifact was not supplied.", Repair: "Review the plan and rerun apply with explicit approval."},
		{Code: "apply.document_invalid", Severity: "error", Meaning: "Ramen could not build a valid executor document for an apply action.", LikelyCause: "The mapping metadata, operation, source path, or request binding is incomplete.", Repair: "Fix the mapping/project metadata and rebuild the plan."},
		{Code: "apply.executor_failed", Severity: "error", Meaning: "The trusted executor returned an execution error.", LikelyCause: "The executor could not complete the requested API operation.", Repair: "Inspect executor feedback and retry after correcting the underlying failure."},
		{Code: "apply.executor_required", Severity: "error", Meaning: "Apply requires an explicit trusted executor.", LikelyCause: "No mock or trusted executor adapter was selected.", Repair: "Use --mock for public mock execution or configure an approved trusted executor adapter."},
		{Code: "apply.executor_unsuccessful", Severity: "error", Meaning: "The trusted executor completed but reported an unsuccessful mutation.", LikelyCause: "The API operation failed or did not converge according to executor evidence.", Repair: "Inspect executor output and mapping metadata before retrying."},
		{Code: "apply.executor_unsupported", Severity: "error", Meaning: "The selected executor does not satisfy the action requirements.", LikelyCause: "Required protocol, auth, output, idempotency, retry, or waiter capabilities are missing.", Repair: "Select an executor with the required capabilities or reduce the requested action."},
		{Code: "destroy.operation_missing", Severity: "error", Meaning: "Destroy has no safe operation role for a resource.", LikelyCause: "The resource lacks delete, detach, disable, suspend, or remove-config operation metadata.", Repair: "Add an explicit destroy operation role or keep the resource out of destroy scope."},
		{Code: "graph.dependency_cycle", Severity: "error", Meaning: "The native resource graph contains a cycle.", LikelyCause: "Two or more resources depend on each other.", Repair: "Break the dependency cycle in the native project metadata."},
		{Code: "import.identity_invalid", Severity: "error", Meaning: "Import identity input is not a JSON object.", LikelyCause: "The --identity value is malformed or null.", Repair: "Provide a JSON object with the declared identity fields."},
		{Code: "import.identity_missing", Severity: "error", Meaning: "Required import identity metadata is missing.", LikelyCause: "The native project declares a required identity field that was not supplied.", Repair: "Provide the missing identity key or adjust the resource identity metadata."},
		{Code: "import.identity_schema_missing", Severity: "error", Meaning: "A native project resource has no importable identity metadata.", LikelyCause: "The resource lacks identity attributes, schema identity paths, and response-derived identity bindings.", Repair: "Declare structured identity metadata before importing the resource."},
		{Code: "import.identity_unknown", Severity: "error", Meaning: "Import identity input includes an undeclared key.", LikelyCause: "The supplied identity field is not present in the native resource identity schema.", Repair: "Use only declared identity keys or add the missing identity metadata."},
		{Code: "import.operation_missing", Severity: "error", Meaning: "A native project resource cannot be verified for import.", LikelyCause: "The resource lacks an import or read operation role.", Repair: "Add an import/read operation role or defer import verification for this resource."},
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
		{Code: "validate.attribute_required", Severity: "error", Meaning: "A required mapped attribute is missing.", LikelyCause: "Mapping schema, identity metadata, or request binding metadata declares a required path that is absent from the resource.", Repair: "Add the required desired-state attribute or adjust the mapping metadata."},
		{Code: "validate.attribute_unknown", Severity: "error", Meaning: "A resource declares an attribute outside its mapping schema.", LikelyCause: "The desired state includes a path that Ramen does not know how to validate, plan, or bind for this mapped type.", Repair: "Remove the attribute or add explicit mapping schema and binding metadata for it."},
		{Code: "validate.binding_invalid", Severity: "error", Meaning: "A request or response binding is incomplete.", LikelyCause: "The binding is missing its operation role, desired/state path, request path, or response path.", Repair: "Complete the mapping binding metadata before validating or planning the resource."},
		{Code: "validate.project_load_error", Severity: "error", Meaning: "The native project could not be loaded.", LikelyCause: "The project file is missing, invalid UWS, or has invalid Ramen metadata.", Repair: "Fix the project path and validate the UWS/Ramen profile."},
		{Code: "validate.enum_invalid", Severity: "error", Meaning: "A mapped attribute has a value outside the declared enum set.", LikelyCause: "The desired value is unsupported by the API source or mapping metadata.", Repair: "Use one of the declared enum values or update the mapping metadata if the API has changed."},
		{Code: "validate.operation_role_missing", Severity: "error", Meaning: "A resource is missing an operation role required by mapping metadata.", LikelyCause: "The resource declares required operations or mapping lifecycle roles that are absent from its operations map.", Repair: "Add the missing operation role with source and operation IDs, or remove the unsupported lifecycle requirement."},
		{Code: "validate.schema_duplicate", Severity: "error", Meaning: "A mapped resource declares the same schema path more than once.", LikelyCause: "Mapping metadata was generated or edited with duplicate path entries.", Repair: "Keep one schema entry per path."},
		{Code: "validate.schema_invalid", Severity: "error", Meaning: "A mapped resource schema entry is malformed.", LikelyCause: "A schema path is empty or otherwise unusable.", Repair: "Provide a non-empty schema path."},
		{Code: "validate.sensitive_path_unredacted", Severity: "error", Meaning: "A sensitive mapped path is not covered by redaction metadata.", LikelyCause: "Schema or response binding metadata marks a path sensitive, but resource/profile redaction omits it.", Repair: "Add the path to resource or profile redaction metadata before persisting artifacts."},
		{Code: "validate.type_invalid", Severity: "error", Meaning: "A mapped attribute does not match the declared scalar or collection type.", LikelyCause: "The desired value type differs from the API-source or mapping metadata type.", Repair: "Correct the desired value type or update the mapping schema if the API contract changed."},
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
