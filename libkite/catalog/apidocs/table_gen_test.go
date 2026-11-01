package apidocs

import (
	"testing"
)

func TestDocTableIntegrity(t *testing.T) {
	if len(Table) < 500 {
		t.Fatalf("expected at least 500 API doc table entries, got %d", len(Table))
	}

	modules := make(map[string]int)
	for _, entry := range Table {
		if entry.Module == "" {
			t.Errorf("entry has empty module: %+v", entry)
		}
		if entry.Name == "" {
			t.Errorf("entry has empty name: %+v", entry)
		}
		if entry.Signature == "" {
			t.Errorf("entry has empty signature: %+v", entry)
		}
		modules[entry.Module]++
	}

	for _, required := range []string{"fs", "http", "json", "yaml", "csv", "sql", "ssh", "k8s"} {
		if count := modules[required]; count == 0 {
			t.Errorf("required module %q has zero documentation entries in Table", required)
		}
	}
}
