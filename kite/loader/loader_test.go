package loader

import (
	"testing"

	"github.com/project-starkite/starkite/libkite"
)

// TestNewAllRegistry_NoCollisions is the CI guard for edition-namespace
// disjointness. Any future PR that adds a module with a name already used by
// another edition (e.g. registering "k8s" in both cloud and ai), or that
// makes a module export a top-level key already exported by a different
// module, fails this test before reaching the binary.
//
// Strict mode in NewAllRegistry causes:
//   - module-name collision  → panic during composition (recovered here)
//   - export-key collision   → error from LoadAll
//   - global-alias collision → error from LoadAll
func TestNewAllRegistry_NoCollisions(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("NewAllRegistry panicked — module-name collision detected: %v", rec)
		}
	}()

	r := NewAllRegistry(&libkite.ModuleConfig{})

	if _, err := r.LoadAll(); err != nil {
		t.Fatalf("LoadAll returned error — export or alias collision detected: %v", err)
	}
}

// TestNewAllRegistry_ContainsAllExpectedModules sanity-checks that every
// edition's modules survive the composition. If any of these go missing,
// either a loader stopped registering it or strict mode caught a
// regression that the previous test didn't already surface.
func TestNewAllRegistry_ContainsAllExpectedModules(t *testing.T) {
	r := NewAllRegistry(&libkite.ModuleConfig{})

	expected := []libkite.ModuleName{
		// base (sample — not exhaustive)
		"os", "fs", "http", "ssh", "json", "yaml", "log", "runtime", "vars", "test",
		// cloud
		"k8s",
		// ai (genai module exports under the Starlark name "ai")
		"ai", "mcp",
	}

	for _, name := range expected {
		if _, ok := r.Get(name); !ok {
			t.Errorf("expected module %q to be registered in the all-edition registry", name)
		}
	}
}
