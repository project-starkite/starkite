//go:build lsp

package lsp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/M31-Labs/starlsp"

	"github.com/project-starkite/starkite/libkite"
)

func TestResolveInstalledModuleWithAndWithoutLock(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	v1Dir := filepath.Join(fakeHome, ".starkite", "modules", "acme", "tool@v1.0.0")
	v2Dir := filepath.Join(fakeHome, ".starkite", "modules", "acme", "tool@v2.0.0")
	if err := os.MkdirAll(v1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(v2Dir, 0o755); err != nil {
		t.Fatal(err)
	}

	v1Entry := filepath.Join(v1Dir, libkite.EntryFile)
	v2Entry := filepath.Join(v2Dir, libkite.EntryFile)
	if err := os.WriteFile(v1Entry, []byte("def run(): pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v2Entry, []byte("def run(): pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := host(t).(starlsp.LoadResolver)

	// Test 1: Caller with mod.lock pinning acme/tool to v2.0.0
	pinnedDir := t.TempDir()
	lock := &libkite.Lock{
		Version: 1,
		Modules: map[string]libkite.LockedModule{
			"acme/tool": {Rev: "v2.0.0"},
		},
	}
	if err := lock.Save(pinnedDir); err != nil {
		t.Fatalf("lock.Save: %v", err)
	}

	got, ok := h.ResolveLoad(starlsp.LoadRequest{
		Target: "acme/tool",
		From:   filepath.Join(pinnedDir, "main.star"),
	})
	if !ok {
		t.Fatalf("ResolveLoad failed for pinned module")
	}
	if got != v2Entry {
		t.Errorf("got %q, want pinned %q", got, v2Entry)
	}

	// Test 2: Caller without lockfile accepts installed revision
	unpinnedDir := t.TempDir()
	gotUnpinned, ok := h.ResolveLoad(starlsp.LoadRequest{
		Target: "acme/tool",
		From:   filepath.Join(unpinnedDir, "main.star"),
	})
	if !ok {
		t.Fatalf("ResolveLoad failed for unpinned module")
	}
	if gotUnpinned != v1Entry && gotUnpinned != v2Entry {
		t.Errorf("got %q, want either installed revision", gotUnpinned)
	}

	// Test 3: Unknown module returns false
	if _, ok := h.ResolveLoad(starlsp.LoadRequest{
		Target: "acme/unknown",
		From:   filepath.Join(unpinnedDir, "main.star"),
	}); ok {
		t.Error("expected unknown module in acme namespace to fail resolution")
	}

	// Test 4: Unknown namespace returns false
	if _, ok := h.ResolveLoad(starlsp.LoadRequest{
		Target: "nonexistent/tool",
		From:   filepath.Join(unpinnedDir, "main.star"),
	}); ok {
		t.Error("expected non-existent namespace to fail resolution")
	}
}
