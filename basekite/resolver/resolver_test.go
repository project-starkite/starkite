package resolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/project-starkite/starkite/basekite/manager"
	"github.com/project-starkite/starkite/libkite"
)

// writeModuleSource creates a module source directory with a mod.yaml (declaring
// the given identity and dependencies) and a main.star entry file.
func writeModuleSource(t *testing.T, dir, namespace, name string, deps map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	var manifest strings.Builder
	manifest.WriteString("namespace: " + namespace + "\nname: " + name + "\nversion: 0.1.0\n")
	if len(deps) > 0 {
		manifest.WriteString("dependencies:\n")
		for id, src := range deps {
			manifest.WriteString("  " + id + ": " + src + "\n")
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "mod.yaml"), []byte(manifest.String()), 0o644); err != nil {
		t.Fatalf("write mod.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.star"), []byte("def main():\n    pass\n"), 0o644); err != nil {
		t.Fatalf("write main.star: %v", err)
	}
}

func newResolver(t *testing.T) (*Resolver, string) {
	t.Helper()
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	mgr, err := manager.New(cacheRoot)
	if err != nil {
		t.Fatalf("manager.New: %v", err)
	}
	return New(mgr), cacheRoot
}

func TestSyncResolvesTransitiveClosure(t *testing.T) {
	root := t.TempDir()
	srcLeaf := filepath.Join(root, "src-leaf")
	srcMid := filepath.Join(root, "src-mid")
	srcApp := filepath.Join(root, "src-app")

	writeModuleSource(t, srcLeaf, "acme", "leaf", nil)
	writeModuleSource(t, srcMid, "acme", "mid", map[string]string{"acme/leaf": srcLeaf})
	writeModuleSource(t, srcApp, "acme", "app", map[string]string{"acme/mid": srcMid})

	r, cacheRoot := newResolver(t)

	lock, err := r.Sync(srcApp)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// The closure includes the direct and transitive dependency, not the app.
	for _, id := range []string{"acme/mid", "acme/leaf"} {
		locked, ok := lock.Modules[id]
		if !ok {
			t.Fatalf("closure missing %q; got %v", id, keys(lock.Modules))
		}
		if locked.Rev == "" || locked.Hash == "" {
			t.Errorf("%q has empty rev/hash: %+v", id, locked)
		}
		if !isDir(filepath.Join(cacheRoot, "acme", filepath.Base(id)+"@"+locked.Rev)) {
			t.Errorf("%q not stored in cache at rev %s", id, locked.Rev)
		}
	}
	if _, ok := lock.Modules["acme/app"]; ok {
		t.Error("closure should not contain the root module itself")
	}

	// mod.lock is written beside the app's mod.yaml.
	if _, err := os.Stat(filepath.Join(srcApp, libkite.LockFile)); err != nil {
		t.Errorf("mod.lock not written: %v", err)
	}
}

func TestSyncIncrementalReuse(t *testing.T) {
	root := t.TempDir()
	srcLeaf := filepath.Join(root, "src-leaf")
	srcApp := filepath.Join(root, "src-app")
	writeModuleSource(t, srcLeaf, "acme", "leaf", nil)
	writeModuleSource(t, srcApp, "acme", "app", map[string]string{"acme/leaf": srcLeaf})

	r, _ := newResolver(t)

	first, err := r.Sync(srcApp)
	if err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	second, err := r.Sync(srcApp)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	if first.Modules["acme/leaf"] != second.Modules["acme/leaf"] {
		t.Errorf("incremental resolve changed the locked entry: %+v vs %+v",
			first.Modules["acme/leaf"], second.Modules["acme/leaf"])
	}
}

func TestResolveIdentityMismatch(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src-leaf")
	app := filepath.Join(root, "src-app")
	// The source declares acme/leaf, but the app declares the dependency under a
	// different identity.
	writeModuleSource(t, src, "acme", "leaf", nil)
	writeModuleSource(t, app, "acme", "app", map[string]string{"acme/wrong": src})

	r, _ := newResolver(t)

	if _, err := r.Sync(app); err == nil {
		t.Fatal("expected identity mismatch error, got nil")
	}
}

func TestSyncLooseResolvesFromCache(t *testing.T) {
	root := t.TempDir()
	srcLeaf := filepath.Join(root, "src-leaf")
	writeModuleSource(t, srcLeaf, "acme", "leaf", nil)

	r, _ := newResolver(t)
	if _, err := r.mgr.Install(srcLeaf, manager.InstallOptions{}); err != nil {
		t.Fatalf("install leaf: %v", err)
	}

	// A loose script that loads the installed module plus things that must be
	// ignored: a built-in (bare name), a relative module, and a .star path.
	scriptDir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	script := filepath.Join(scriptDir, "deploy.star")
	body := `load("acme/leaf", "leaf")
load("json", "json")
load("./helpers", "helpers")
load("./util.star", "util")

def main():
    pass
`
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	lock, err := r.SyncLoose(script)
	if err != nil {
		t.Fatalf("SyncLoose: %v", err)
	}
	if len(lock.Modules) != 1 {
		t.Fatalf("expected only the installed ref locked, got %v", keys(lock.Modules))
	}
	if _, ok := lock.Modules["acme/leaf"]; !ok {
		t.Errorf("acme/leaf not locked; got %v", keys(lock.Modules))
	}
	if _, err := os.Stat(filepath.Join(scriptDir, libkite.LockFile)); err != nil {
		t.Errorf("mod.lock not written beside script: %v", err)
	}
}

func TestSyncLooseUninstalledIsError(t *testing.T) {
	root := t.TempDir()
	scriptDir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	script := filepath.Join(scriptDir, "deploy.star")
	if err := os.WriteFile(script, []byte("load(\"acme/missing\", \"missing\")\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	r, _ := newResolver(t)
	if _, err := r.SyncLoose(script); err == nil {
		t.Fatal("expected error for an uninstalled dependency, got nil")
	}
}

func TestVerifyCached(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	writeModuleSource(t, src, "acme", "leaf", nil)

	cacheRoot := filepath.Join(t.TempDir(), "cache")
	mgr, err := manager.New(cacheRoot)
	if err != nil {
		t.Fatalf("manager.New: %v", err)
	}
	info, err := mgr.Install(src, manager.InstallOptions{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if err := verifyCached(info.Path, info.Hash); err != nil {
		t.Fatalf("intact cached tree should verify: %v", err)
	}

	// Tamper with a tracked file; verification must fail.
	if err := os.WriteFile(filepath.Join(info.Path, "main.star"), []byte("def main():\n    print('x')\n"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if err := verifyCached(info.Path, info.Hash); err == nil {
		t.Fatal("tampered cached tree should fail verification")
	}
}

func keys(m map[string]libkite.LockedModule) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
