package libkite

import (
	"context"
	"strings"
	"testing"
)

// RequireEntryPoint makes a missing entry point a hard error (a module run),
// while a loose script (RequireEntryPoint=false) runs top-level code without one.
func TestExecute_RequireEntryPoint(t *testing.T) {
	const library = "def helper():\n    return 1\n"
	const executable = "def main():\n    pass\n"

	t.Run("module without main errors", func(t *testing.T) {
		rt, err := New(&Config{EntryPoint: "main", RequireEntryPoint: true, ScriptPath: "lib.star"})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		t.Cleanup(rt.Close)
		err = rt.Execute(context.Background(), library)
		if err == nil {
			t.Fatal("expected library error, got nil")
		}
		if !strings.Contains(err.Error(), "library") {
			t.Errorf("error should mention 'library'; got %q", err.Error())
		}
	})

	t.Run("module with main runs", func(t *testing.T) {
		rt, err := New(&Config{EntryPoint: "main", RequireEntryPoint: true, ScriptPath: "main.star"})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		t.Cleanup(rt.Close)
		if err := rt.Execute(context.Background(), executable); err != nil {
			t.Errorf("executable module should run: %v", err)
		}
	})

	t.Run("loose script without main is tolerated", func(t *testing.T) {
		rt, err := New(&Config{EntryPoint: "main", RequireEntryPoint: false, ScriptPath: "loose.star"})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		t.Cleanup(rt.Close)
		if err := rt.Execute(context.Background(), library); err != nil {
			t.Errorf("loose script should run without main: %v", err)
		}
	})
}
