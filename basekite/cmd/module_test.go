package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/project-starkite/starkite/basekite/manager"
)

// writeLocalModuleSource creates a local module source directory for install.
func writeLocalModuleSource(t *testing.T, ns, name, body string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "src-"+name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "mod.yaml"), []byte("namespace: "+ns+"\nname: "+name+"\nversion: 0.1.0\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.star"), []byte(body), 0o644)
	return dir
}

// TestModuleCommandsEndToEnd drives install → list → info → verify → update →
// remove through the actual command handlers, with an isolated module cache via
// $HOME so the real user cache is untouched.
func TestModuleCommandsEndToEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)        // unix: os.UserHomeDir()
	t.Setenv("USERPROFILE", home) // windows
	cacheRoot := filepath.Join(home, ".starkite", "modules")

	src := writeLocalModuleSource(t, "acme", "tool", "def greet():\n    return 1\n")

	// reset install flags between runs
	moduleInstallAs, moduleInstallForce = "acme/tool", false
	t.Cleanup(func() { moduleInstallAs, moduleInstallForce = "", false })

	t.Run("install", func(t *testing.T) {
		if err := runModuleInstall(nil, []string{src}); err != nil {
			t.Fatalf("install: %v", err)
		}
		mgr, _ := manager.New(cacheRoot)
		revs, _ := mgr.Revisions("acme/tool")
		if len(revs) != 1 {
			t.Fatalf("expected 1 installed revision, got %d", len(revs))
		}
	})

	t.Run("list", func(t *testing.T) {
		if err := runModuleList(nil, nil); err != nil {
			t.Errorf("list: %v", err)
		}
	})

	t.Run("info", func(t *testing.T) {
		if err := runModuleInfo(nil, []string{"acme/tool"}); err != nil {
			t.Errorf("info: %v", err)
		}
	})

	t.Run("verify", func(t *testing.T) {
		if err := runModuleVerify(nil, []string{"acme/tool"}); err != nil {
			t.Errorf("verify: %v", err)
		}
	})

	t.Run("update adds a revision", func(t *testing.T) {
		os.WriteFile(filepath.Join(src, "main.star"), []byte("def greet():\n    return 2\n"), 0o644)
		if err := runModuleUpdate(nil, []string{"acme/tool"}); err != nil {
			t.Fatalf("update: %v", err)
		}
		mgr, _ := manager.New(cacheRoot)
		revs, _ := mgr.Revisions("acme/tool")
		if len(revs) != 2 {
			t.Fatalf("expected 2 revisions after update, got %d", len(revs))
		}
	})

	t.Run("info tolerates multiple revisions", func(t *testing.T) {
		if err := runModuleInfo(nil, []string{"acme/tool"}); err != nil {
			t.Errorf("info with 2 revisions should not error: %v", err)
		}
	})

	t.Run("verify checks all revisions", func(t *testing.T) {
		if err := runModuleVerify(nil, nil); err != nil {
			t.Errorf("verify all: %v", err)
		}
	})

	t.Run("remove", func(t *testing.T) {
		if err := runModuleRemove(nil, []string{"acme/tool"}); err != nil {
			t.Fatalf("remove: %v", err)
		}
		mgr, _ := manager.New(cacheRoot)
		if revs, _ := mgr.Revisions("acme/tool"); len(revs) != 0 {
			t.Errorf("expected 0 revisions after remove, got %d", len(revs))
		}
	})

	t.Run("info after remove errors", func(t *testing.T) {
		if err := runModuleInfo(nil, []string{"acme/tool"}); err == nil {
			t.Error("info on a removed module should error")
		}
	})
}
