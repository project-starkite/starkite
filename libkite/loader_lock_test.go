package libkite

import (
	"os"
	"path/filepath"
	"testing"
)

// writeCacheModule creates a version-addressed cache module under
// <home>/.starkite/modules/<ns>/<name>@<rev> and returns its path.
func writeCacheModule(t *testing.T, home, ns, name, rev string) string {
	t.Helper()
	dir := filepath.Join(home, ".starkite", "modules", ns, name+"@"+rev)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir cache module: %v", err)
	}
	os.WriteFile(filepath.Join(dir, ManifestFile), []byte("namespace: "+ns+"\nname: "+name+"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, EntryFile), []byte("def greet():\n    return \""+rev+"\"\n"), 0o644)
	return dir
}

// entryWithLock creates an entry-module directory with a mod.lock pinning the
// given identities, and returns the path to its entry file.
func entryWithLock(t *testing.T, modules map[string]LockedModule) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir entry: %v", err)
	}
	os.WriteFile(filepath.Join(dir, EntryFile), []byte("def main():\n    pass\n"), 0o644)
	if modules != nil {
		if err := (&Lock{Modules: modules}).Save(dir); err != nil {
			t.Fatalf("save lock: %v", err)
		}
	}
	return filepath.Join(dir, EntryFile)
}

func TestResolveInstalledModuleLockAware(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// Two revisions of the same module coexist in the cache.
	revA := writeCacheModule(t, home, "acme", "leaf", "aaaa1111")
	writeCacheModule(t, home, "acme", "leaf", "bbbb2222")

	t.Run("lock pins the revision over a newer cached one", func(t *testing.T) {
		entry := entryWithLock(t, map[string]LockedModule{
			"acme/leaf": {Source: "x", Rev: "aaaa1111", Hash: "sha256:x"},
		})
		rt := &Runtime{config: &Config{ScriptPath: entry}}
		got, err := rt.resolveInstalledModule("acme/leaf")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got != revA {
			t.Errorf("got %q, want locked revision %q", got, revA)
		}
	})

	t.Run("no lock entry is ambiguous with multiple revisions", func(t *testing.T) {
		entry := entryWithLock(t, nil) // no mod.lock
		rt := &Runtime{config: &Config{ScriptPath: entry}}
		if _, err := rt.resolveInstalledModule("acme/leaf"); err == nil {
			t.Error("expected ambiguity error without a lock pin")
		}
	})

	t.Run("locked revision missing from cache errors", func(t *testing.T) {
		entry := entryWithLock(t, map[string]LockedModule{
			"acme/leaf": {Rev: "ffff9999"},
		})
		rt := &Runtime{config: &Config{ScriptPath: entry}}
		if _, err := rt.resolveInstalledModule("acme/leaf"); err == nil {
			t.Error("expected error for a locked-but-uninstalled revision")
		}
	})

	t.Run("single revision resolves without a lock", func(t *testing.T) {
		home2 := t.TempDir()
		t.Setenv("HOME", home2)
		t.Setenv("USERPROFILE", home2)
		only := writeCacheModule(t, home2, "acme", "solo", "c0ffee00")
		entry := entryWithLock(t, nil)
		rt := &Runtime{config: &Config{ScriptPath: entry}}
		got, err := rt.resolveInstalledModule("acme/solo")
		if err != nil || got != only {
			t.Fatalf("single-rev resolve = %q (err %v), want %q", got, err, only)
		}
	})
}
