package loader_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/loader"
)

// runEntry executes entrySrc as the entry script at entryPath (so relative
// load() resolves against its directory) under the given runtime permission.
func runEntry(t *testing.T, perms *libkite.PermissionConfig, entryPath, entrySrc string) error {
	t.Helper()
	rt, err := libkite.New(&libkite.Config{
		Registry:    loader.NewDefaultRegistry(&libkite.ModuleConfig{}),
		Permissions: perms,
		ScriptPath:  entryPath,
	})
	if err != nil {
		t.Fatalf("libkite.New: %v", err)
	}
	t.Cleanup(rt.Close)
	return rt.Execute(context.Background(), entrySrc)
}

// writeModuleAndEntry writes a single-file module `mod.star` plus an entry
// script that loads it, and returns the entry path and source.
func writeModuleAndEntry(t *testing.T, modSrc, entrySrc string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mod.star"), []byte(modSrc), 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}
	entryPath := filepath.Join(dir, "entry.star")
	if err := os.WriteFile(entryPath, []byte(entrySrc), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	return entryPath, entrySrc
}

// A loaded module is bound by the runtime permission: a capability the runtime
// profile withholds (os.exec under allow-fs) is denied inside the module.
func TestLoadedModule_BoundByRuntimePermission(t *testing.T) {
	entryPath, entry := writeModuleAndEntry(t,
		"def reach():\n    return exec(\"echo hi\")\n",
		"load(\"mod.star\", \"mod\")\nmod.reach()\n",
	)
	err := runEntry(t, libkite.AllowFSPermissions(), entryPath, entry)
	if err == nil {
		t.Fatal("expected permission denial from loaded module, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "permission denied") || !strings.Contains(msg, "os.exec") {
		t.Errorf("expected os.exec denial; got %q", msg)
	}
}

// The denial is attributed to the loaded module that triggered it.
func TestLoadedModule_DenialAttributed(t *testing.T) {
	entryPath, entry := writeModuleAndEntry(t,
		"def reach():\n    return exec(\"echo hi\")\n",
		"load(\"mod.star\", \"mod\")\nmod.reach()\n",
	)
	err := runEntry(t, libkite.AllowFSPermissions(), entryPath, entry)
	if err == nil {
		t.Fatal("expected denial, got nil")
	}
	if !strings.Contains(err.Error(), `module "mod"`) {
		t.Errorf("denial not attributed to loaded module; got %q", err.Error())
	}
}

// A denial in the entry script itself carries no module attribution.
func TestEntryScriptDenial_NoModuleAttribution(t *testing.T) {
	dir := t.TempDir()
	entryPath := filepath.Join(dir, "entry.star")
	entry := "exec(\"echo hi\")\n"
	if err := os.WriteFile(entryPath, []byte(entry), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	err := runEntry(t, libkite.AllowFSPermissions(), entryPath, entry)
	if err == nil {
		t.Fatal("expected denial, got nil")
	}
	if strings.Contains(err.Error(), "module \"") {
		t.Errorf("entry-script denial must not be attributed to a module; got %q", err.Error())
	}
}

// A loaded module may use any capability the runtime permission grants.
func TestLoadedModule_AllowedWithinRuntimePermission(t *testing.T) {
	entryPath, entry := writeModuleAndEntry(t,
		"def reach():\n    return exec(\"echo hi\").strip()\n",
		"load(\"mod.star\", \"mod\")\nresult = mod.reach()\n",
	)
	if err := runEntry(t, libkite.AllowAllPermissions(), entryPath, entry); err != nil {
		t.Fatalf("module call should be allowed under allow-all: %v", err)
	}
}
