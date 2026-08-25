package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/project-starkite/starkite/basekite/manager"
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

	t.Run("bare .star file requires a path prefix", func(t *testing.T) {
		_, _, err := resolveRunTarget("loose.star")
		if err == nil {
			t.Fatal("expected error for bare .star reference")
		}
		if got := err.Error(); !strings.Contains(got, "./loose.star") {
			t.Errorf("error should hint the ./ form, got: %v", got)
		}
	})

	t.Run("bare single segment is not runnable", func(t *testing.T) {
		if _, _, err := resolveRunTarget("mymod"); err == nil {
			t.Error("expected error for bare single-segment reference")
		}
	})

	t.Run("windows backslash path prefix", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("windows path prefix test only applies on windows")
		}
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = os.Chdir(cwd)
		}()
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		entry, isModule, err := resolveRunTarget(".\\loose.star")
		if err != nil {
			t.Fatalf("unexpected error resolving .\\loose.star: %v", err)
		}
		if entry != ".\\loose.star" || isModule {
			t.Errorf("got (%q, %v), want (\".\\\\loose.star\", false)", entry, isModule)
		}
	})
}

func TestResolveRunTargetInstalledRevisions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cacheRoot := filepath.Join(home, ".starkite", "modules")

	mgr, err := manager.New(cacheRoot)
	if err != nil {
		t.Fatalf("manager.New: %v", err)
	}
	src := filepath.Join(t.TempDir(), "src")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "mod.yaml"), []byte("namespace: acme\nname: tool\nversion: 0.1.0\n"), 0o644)
	os.WriteFile(filepath.Join(src, "main.star"), []byte("def main():\n    pass\n"), 0o644)

	first, err := mgr.Install(src, manager.InstallOptions{})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	os.WriteFile(filepath.Join(src, "main.star"), []byte("def main():\n    print('v2')\n"), 0o644)
	updated, err := mgr.Update("acme/tool")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	future := time.Now().Add(time.Hour)
	os.Chtimes(updated.Path, future, future)

	t.Run("bare reference selects newest", func(t *testing.T) {
		entry, isModule, err := resolveRunTarget("acme/tool")
		if err != nil || !isModule {
			t.Fatalf("resolve acme/tool: %v (isModule=%v)", err, isModule)
		}
		if entry != filepath.Join(updated.Path, "main.star") {
			t.Errorf("entry = %q, want newest revision %q", entry, updated.Path)
		}
	})

	t.Run("@rev pins an exact revision", func(t *testing.T) {
		entry, _, err := resolveRunTarget("acme/tool@" + first.Rev)
		if err != nil {
			t.Fatalf("resolve acme/tool@%s: %v", first.Rev, err)
		}
		if entry != filepath.Join(first.Path, "main.star") {
			t.Errorf("entry = %q, want pinned revision %q", entry, first.Path)
		}
	})

	t.Run("unknown @rev errors", func(t *testing.T) {
		if _, _, err := resolveRunTarget("acme/tool@deadbeef"); err == nil {
			t.Error("expected error for unknown revision")
		}
	})

	t.Run("uninstalled reference errors", func(t *testing.T) {
		if _, _, err := resolveRunTarget("acme/missing"); err == nil {
			t.Error("expected error for uninstalled module")
		}
	})
}
