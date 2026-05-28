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
		{Code: "plan.prevent_destroy", Severity: "error", Meaning: "A planned delete or replace violates prevent_destroy.", LikelyCause: "The resource lifecycle blocks destructive action.", Repair: "Remove the destructive control or intentionally change lifecycle metadata."},
		{Code: "project.operation_missing", Severity: "error", Meaning: "A required native project operation role is absent.", LikelyCause: "The resource lacks create, read, update, delete, or import metadata for the requested action.", Repair: "Add the missing operation role with source and operation IDs."},
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
