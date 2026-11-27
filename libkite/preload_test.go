package libkite

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

func TestResolveStaticLoadClosure_LocalAndTransitive(t *testing.T) {
	tempDir := t.TempDir()

	subDir := filepath.Join(tempDir, "sub")
	libDir := filepath.Join(tempDir, "lib")
	pkgDir := filepath.Join(tempDir, "pkg")
	for _, d := range []string{subDir, libDir, pkgDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	mainPath := filepath.Join(tempDir, "main.star")
	aPath := filepath.Join(subDir, "a.star")
	bPath := filepath.Join(libDir, "b.star")
	pkgManifest := filepath.Join(pkgDir, ManifestFile)
	pkgEntry := filepath.Join(pkgDir, EntryFile)

	if err := os.WriteFile(mainPath, []byte(`
load("./sub/a.star", "a")
load("./pkg", "pkg")
result = a.val_a + "-" + pkg.pkg_val
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(aPath, []byte(`
load("../lib/b.star", "b")
val_a = b.val_b + "-a"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(bPath, []byte(`
val_b = "b"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(pkgManifest, []byte("name: pkg\nversion: 1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(pkgEntry, []byte(`
pkg_val = "from_pkg"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	closure, err := ResolveStaticLoadClosure(mainPath, tempDir, nil)
	if err != nil {
		t.Fatalf("ResolveStaticLoadClosure: %v", err)
	}

	for _, reqFile := range []string{mainPath, aPath, bPath, pkgManifest, pkgEntry} {
		clean := filepath.Clean(reqFile)
		if _, ok := closure.Files[clean]; !ok {
			t.Errorf("closure.Files missing %q", clean)
		}
		if !slices.Contains(closure.ReadPaths, clean) {
			t.Errorf("closure.ReadPaths missing %q", clean)
		}
	}

	cleanPkgDir := filepath.Clean(pkgDir)
	if _, ok := closure.ModuleDirs[cleanPkgDir]; !ok {
		t.Errorf("closure.ModuleDirs missing %q", cleanPkgDir)
	}
	if !slices.Contains(closure.ReadPaths, cleanPkgDir) {
		t.Errorf("closure.ReadPaths missing module dir %q", cleanPkgDir)
	}
}

func TestRuntime_ExecuteWithPreloadedClosureAfterFilesRemoved(t *testing.T) {
	tempDir := t.TempDir()

	mainPath := filepath.Join(tempDir, "main.star")
	helperPath := filepath.Join(tempDir, "helper.star")

	if err := os.WriteFile(mainPath, []byte(`
load("./helper.star", "helper")

def main():
    print("result=" + str(helper.answer))
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(helperPath, []byte("answer = 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	closure, err := ResolveStaticLoadClosure(mainPath, tempDir, nil)
	if err != nil {
		t.Fatalf("ResolveStaticLoadClosure: %v", err)
	}

	// Delete helper.star from disk to simulate kernel sandbox denying filesystem read
	if err := os.Remove(helperPath); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	cfg := &Config{
		ScriptPath: mainPath,
		WorkDir:    tempDir,
		Preloaded:  closure,
		EntryPoint: "main",
		Print: func(thread *starlark.Thread, msg string) {
			buf.WriteString(msg + "\n")
		},
	}

	rt, err := New(cfg)
	if err != nil {
		t.Fatalf("New(cfg): %v", err)
	}
	defer rt.Close()

	mainContent, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := rt.Execute(context.Background(), string(mainContent)); err != nil {
		t.Fatalf("rt.Execute failed despite preloaded closure: %v", err)
	}

	if !strings.Contains(buf.String(), "result=42") {
		t.Errorf("expected output to contain 'result=42', got: %q", buf.String())
	}

	// Verify that without preloaded closure, loading the removed file fails
	cfgNoPreload := &Config{
		ScriptPath: mainPath,
		WorkDir:    tempDir,
		EntryPoint: "main",
	}
	rtNoPreload, err := New(cfgNoPreload)
	if err != nil {
		t.Fatalf("New(cfgNoPreload): %v", err)
	}
	defer rtNoPreload.Close()

	if err := rtNoPreload.Execute(context.Background(), string(mainContent)); err == nil {
		t.Errorf("expected execution without preloaded closure to fail since helper.star was deleted")
	}
}

func TestResolveStaticLoadClosure_BlocksTraversalSymlinksAndBadLockRev(t *testing.T) {
	parentDir := t.TempDir()
	workDir := filepath.Join(parentDir, "workspace")
	outsideDir := filepath.Join(parentDir, "outside")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}

	outsideStar := filepath.Join(outsideDir, "secret.star")
	if err := os.WriteFile(outsideStar, []byte("secret = 99\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	escapeLink := filepath.Join(workDir, "escape")
	if err := os.Symlink(outsideDir, escapeLink); err == nil {
		mainSymlink := filepath.Join(workDir, "main_symlink.star")
		if err := os.WriteFile(mainSymlink, []byte("load(\"./escape/secret.star\", \"secret\")\nload(\"../outside/secret.star\", \"secret\")\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		closure, err := ResolveStaticLoadClosure(mainSymlink, workDir, nil)
		if err != nil {
			t.Fatalf("ResolveStaticLoadClosure: %v", err)
		}
		if _, ok := closure.Files[outsideStar]; ok {
			t.Errorf("outside file should not be preloaded via traversal or symlink")
		}
		for _, p := range closure.ReadPaths {
			if strings.Contains(p, "outside") || strings.Contains(p, "escape") {
				t.Errorf("ReadPaths must not include escaped path %q", p)
			}
		}
	}

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	badRevLockDir := t.TempDir()
	badMain := filepath.Join(badRevLockDir, "main.star")
	if err := os.WriteFile(badMain, []byte("load(\"acme/tool\", \"tool\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	badLock := &Lock{
		Version: 1,
		Modules: map[string]LockedModule{
			"acme/tool": {Rev: "../../etc"},
		},
	}
	if err := badLock.Save(badRevLockDir); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveStaticLoadClosure(badMain, badRevLockDir, nil); err == nil {
		t.Error("expected traversal rev in mod.lock to fail ResolveStaticLoadClosure")
	}

	hashDir := t.TempDir()
	modCacheDir := filepath.Join(fakeHome, ".starkite", "modules", "acme", "tool@v1")
	if err := os.MkdirAll(modCacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modCacheDir, ManifestFile), []byte("namespace: acme\nname: tool\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modCacheDir, EntryFile), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tamperMain := filepath.Join(hashDir, "main.star")
	if err := os.WriteFile(tamperMain, []byte("load(\"acme/tool\", \"tool\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tamperLock := &Lock{
		Version: 1,
		Modules: map[string]LockedModule{
			"acme/tool": {Rev: "v1", Hash: "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
		},
	}
	if err := tamperLock.Save(hashDir); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveStaticLoadClosure(tamperMain, hashDir, nil); err == nil {
		t.Error("expected hash mismatch in mod.lock to fail ResolveStaticLoadClosure")
	}
}
