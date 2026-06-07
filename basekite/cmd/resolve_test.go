package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRunTarget(t *testing.T) {
	dir := t.TempDir()

	// Loose script file.
	loose := filepath.Join(dir, "loose.star")
	if err := os.WriteFile(loose, []byte("print('hi')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Executable module directory: mod.yaml + main.star.
	mod := filepath.Join(dir, "mymod")
	if err := os.MkdirAll(mod, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(mod, "mod.yaml"), []byte("name: mymod\n"), 0o644)
	os.WriteFile(filepath.Join(mod, "main.star"), []byte("def main(): pass\n"), 0o644)

	// Directory missing the manifest.
	notMod := filepath.Join(dir, "notmod")
	os.MkdirAll(notMod, 0o755)
	os.WriteFile(filepath.Join(notMod, "main.star"), []byte("def main(): pass\n"), 0o644)

	// Module directory missing the main.star entry.
	noEntry := filepath.Join(dir, "noentry")
	os.MkdirAll(noEntry, 0o755)
	os.WriteFile(filepath.Join(noEntry, "mod.yaml"), []byte("name: noentry\n"), 0o644)

	t.Run("loose file", func(t *testing.T) {
		entry, isModule, err := resolveRunTarget(loose)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry != loose || isModule {
			t.Errorf("got (%q, %v), want (%q, false)", entry, isModule, loose)
		}
	})

	t.Run("directory module", func(t *testing.T) {
		entry, isModule, err := resolveRunTarget(mod)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(mod, "main.star")
		if entry != want || !isModule {
			t.Errorf("got (%q, %v), want (%q, true)", entry, isModule, want)
		}
	})

	t.Run("directory missing manifest errors", func(t *testing.T) {
		if _, _, err := resolveRunTarget(notMod); err == nil {
			t.Error("expected error for directory without mod.yaml")
		}
	})

	t.Run("module missing main.star errors", func(t *testing.T) {
		if _, _, err := resolveRunTarget(noEntry); err == nil {
			t.Error("expected error for module without main.star")
		}
	})

	t.Run("nonexistent path errors", func(t *testing.T) {
		if _, _, err := resolveRunTarget(filepath.Join(dir, "nope")); err == nil {
			t.Error("expected error for nonexistent path")
		}
	})
}
