package diagnostic

import (
	"testing"

	"github.com/OpenUdon/ramen/internal/ansibleconvert"
)

func TestCatalogV1StableAndSorted(t *testing.T) {
	catalog := CatalogV1()
	if catalog.Version != CatalogVersion || len(catalog.Entries) == 0 {
		t.Fatalf("catalog = %#v", catalog)
	}
	for i, entry := range catalog.Entries {
		if entry.Code == "" || entry.Severity == "" || entry.Meaning == "" || entry.Repair == "" {
			t.Fatalf("incomplete entry: %#v", entry)
		}
		if i > 0 && catalog.Entries[i-1].Code > entry.Code {
			t.Fatalf("catalog is not sorted: %s before %s", catalog.Entries[i-1].Code, entry.Code)
		}
	}
}

func TestCatalogV1IncludesAnsibleConvertCodes(t *testing.T) {
	catalog := CatalogV1()
	entries := map[string]bool{}
	for _, entry := range catalog.Entries {
		entries[entry.Code] = true
	}
	for _, code := range []string{
		ansibleconvert.CodeJinjaUnsupported,
		ansibleconvert.CodeModuleUnknown,
		ansibleconvert.CodeArgspecViolation,
		ansibleconvert.CodeNoLogLiteral,
		ansibleconvert.CodeAlwaysUnsupported,
		ansibleconvert.CodeRescueTodo,
		ansibleconvert.CodeDynamicInclude,
		ansibleconvert.CodeDirectiveTodo,
		ansibleconvert.CodeDelegateUnsupported,
		ansibleconvert.CodeHostsRuntimeOwned,
		ansibleconvert.CodeHandlerUnnotified,
		ansibleconvert.CodePlaybookShape,
		ansibleconvert.CodeStaticResolution,
		ansibleconvert.CodeVariableConflict,
	} {
		if !entries[code] {
			t.Fatalf("diagnostic catalog missing Ansible converter code %q", code)
		}
	}
}
