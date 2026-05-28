package diagnostic

import "testing"

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
