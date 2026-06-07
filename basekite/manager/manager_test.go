package manager

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestParseSource(t *testing.T) {
	tests := []struct {
		source      string
		wantRepo    string
		wantVersion string
	}{
		{
			source:      "github.com/user/repo",
			wantRepo:    "github.com/user/repo",
			wantVersion: "",
		},
		{
			source:      "github.com/user/repo@v1.0.0",
			wantRepo:    "github.com/user/repo",
			wantVersion: "v1.0.0",
		},
		{
			source:      "github.com/user/repo@main",
			wantRepo:    "github.com/user/repo",
			wantVersion: "main",
		},
		{
			source:      "github.com/user/repo@abc1234",
			wantRepo:    "github.com/user/repo",
			wantVersion: "abc1234",
		},
		{
			source:      "git@github.com:user/repo.git",
			wantRepo:    "git@github.com:user/repo.git",
			wantVersion: "",
		},
		{
			source:      "git@github.com:user/repo.git@v2.0.0",
			wantRepo:    "git@github.com:user/repo.git",
			wantVersion: "v2.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			repo, version := ParseSource(tt.source)
			if repo != tt.wantRepo {
				t.Errorf("ParseSource(%q) repo = %q, want %q", tt.source, repo, tt.wantRepo)
			}
			if version != tt.wantVersion {
				t.Errorf("ParseSource(%q) version = %q, want %q", tt.source, version, tt.wantVersion)
			}
		})
	}
}

func TestInferModuleName(t *testing.T) {
	tests := []struct {
		repo string
		want string
	}{
		{"github.com/user/starkite-helm", "starkite-helm"},
		{"github.com/user/helm", "helm"},
		{"git@github.com:user/repo.git", "repo"},
		{"https://github.com/user/mymodule.git", "mymodule"},
		{"gitlab.com/org/subgroup/module", "module"},
		{"simple-name", "simple-name"},
	}

	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			got := InferModuleName(tt.repo)
			if got != tt.want {
				t.Errorf("InferModuleName(%q) = %q, want %q", tt.repo, got, tt.want)
			}
		})
	}
}

func TestInferNamespaceName(t *testing.T) {
	tests := []struct {
		repo          string
		wantNamespace string
		wantName      string
	}{
		{"github.com/user/starkite-helm", "user", "starkite-helm"},
		{"git@github.com:user/repo.git", "user", "repo"},
		{"https://github.com/user/mymodule.git", "user", "mymodule"},
		{"gitlab.com/org/subgroup/module", "subgroup", "module"},
		{"git.internal.example/team/repo", "team", "repo"},
		{"file:///path/to/repo", "to", "repo"},
		{"simple-name", "", "simple-name"},
	}

	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			ns, name := InferNamespaceName(tt.repo)
			if ns != tt.wantNamespace || name != tt.wantName {
				t.Errorf("InferNamespaceName(%q) = (%q, %q), want (%q, %q)",
					tt.repo, ns, name, tt.wantNamespace, tt.wantName)
			}
		})
	}
}

// writeStarlarkModule creates an installed-layout starlark module fixture at
// <modulesDir>/<namespace>/<name>@<rev> with a mod.yaml and a main.star.
func writeStarlarkModule(t *testing.T, mgr *Manager, namespace, name string) string {
	t.Helper()
	dir := filepath.Join(mgr.ModulesDir(), namespace, name+"@testrev0001")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create module dir: %v", err)
	}
	manifest := "namespace: " + namespace + "\nname: " + name + "\nversion: 0.1.0\n"
	if err := os.WriteFile(filepath.Join(dir, "mod.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write mod.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.star"), []byte("def main(): pass\n"), 0o644); err != nil {
		t.Fatalf("write main.star: %v", err)
	}
	return dir
}

func TestManagerNew(t *testing.T) {
	t.Run("default directory", func(t *testing.T) {
		mgr, err := New("")
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".starkite", "modules")
		if mgr.ModulesDir() != expected {
			t.Errorf("ModulesDir() = %q, want %q", mgr.ModulesDir(), expected)
		}
	})

	t.Run("custom directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		customDir := filepath.Join(tmpDir, "custom-modules")

		mgr, err := New(customDir)
		if err != nil {
			t.Fatalf("New(%q) failed: %v", customDir, err)
		}

		if mgr.ModulesDir() != customDir {
			t.Errorf("ModulesDir() = %q, want %q", mgr.ModulesDir(), customDir)
		}

		// The modules root should be created.
		if _, err := os.Stat(mgr.ModulesDir()); os.IsNotExist(err) {
			t.Errorf("directory %q was not created", mgr.ModulesDir())
		}
	})
}

func TestManagerList(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	t.Run("empty list", func(t *testing.T) {
		modules, err := mgr.List()
		if err != nil {
			t.Fatalf("List() failed: %v", err)
		}
		if len(modules) != 0 {
			t.Errorf("expected empty list, got %d modules", len(modules))
		}
	})

	t.Run("with starlark modules", func(t *testing.T) {
		writeStarlarkModule(t, mgr, "acme", "test-module")

		modules, err := mgr.List()
		if err != nil {
			t.Fatalf("List() failed: %v", err)
		}
		if len(modules) != 1 {
			t.Fatalf("expected 1 module, got %d", len(modules))
		}
		if modules[0].Name != "test-module" {
			t.Errorf("expected module name 'test-module', got %q", modules[0].Name)
		}
		if modules[0].Namespace != "acme" {
			t.Errorf("expected namespace 'acme', got %q", modules[0].Namespace)
		}
		if modules[0].Type != "starlark" {
			t.Errorf("expected type 'starlark', got %q", modules[0].Type)
		}
	})
}

func TestManagerGet(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	t.Run("not found", func(t *testing.T) {
		_, err := mgr.Get("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent module")
		}
	})

	t.Run("found starlark", func(t *testing.T) {
		moduleDir := writeStarlarkModule(t, mgr, "acme", "my-module")

		info, err := mgr.Get("acme/my-module")
		if err != nil {
			t.Fatalf("Get() failed: %v", err)
		}
		if info.Name != "my-module" {
			t.Errorf("expected name 'my-module', got %q", info.Name)
		}
		if info.Namespace != "acme" {
			t.Errorf("expected namespace 'acme', got %q", info.Namespace)
		}
		if info.Type != "starlark" {
			t.Errorf("expected type 'starlark', got %q", info.Type)
		}
		if info.EntryPoint != filepath.Join(moduleDir, "main.star") {
			t.Errorf("unexpected entry point: %q", info.EntryPoint)
		}
	})
}

func TestManagerRemove(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	t.Run("not found", func(t *testing.T) {
		err := mgr.Remove("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent module")
		}
	})

	t.Run("remove starlark", func(t *testing.T) {
		moduleDir := writeStarlarkModule(t, mgr, "acme", "remove-me")

		err := mgr.Remove("acme/remove-me")
		if err != nil {
			t.Fatalf("Remove() failed: %v", err)
		}

		// Verify it's gone
		if _, err := os.Stat(moduleDir); !os.IsNotExist(err) {
			t.Error("module directory still exists after removal")
		}
	})
}

// writeLocalSource creates a local module source directory (not in the cache)
// suitable for mgr.Install.
func writeLocalSource(t *testing.T, ns, name, body string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "mod.yaml"), []byte("namespace: "+ns+"\nname: "+name+"\nversion: 0.1.0\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.star"), []byte(body), 0o644)
	return dir
}

func TestManagerInstall(t *testing.T) {
	mgr, err := New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	src := writeLocalSource(t, "acme", "tool", "def main():\n    pass\n")

	info, err := mgr.Install(src, InstallOptions{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if info.Namespace != "acme" || info.Name != "tool" {
		t.Errorf("identity = %s/%s, want acme/tool", info.Namespace, info.Name)
	}
	if info.Rev == "" || info.Hash == "" {
		t.Errorf("expected non-empty rev and hash, got rev=%q hash=%q", info.Rev, info.Hash)
	}
	if filepath.Base(info.Path) != "tool@"+info.Rev {
		t.Errorf("install path %q should be version-addressed as tool@%s", info.Path, info.Rev)
	}

	// The receipt records the same hash.
	prov, err := ReadProvenance(info.Path)
	if err != nil || prov == nil {
		t.Fatalf("ReadProvenance: %v (prov=%v)", err, prov)
	}
	if prov.Hash != info.Hash || prov.Rev != info.Rev {
		t.Errorf("receipt rev/hash %s/%s != info %s/%s", prov.Rev, prov.Hash, info.Rev, info.Hash)
	}

	// Reinstalling identical content is idempotent: same revision, no error.
	info2, err := mgr.Install(src, InstallOptions{})
	if err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if info2.Rev != info.Rev {
		t.Errorf("idempotent reinstall produced a new rev: %s vs %s", info2.Rev, info.Rev)
	}
}

func TestManagerUpdateAndRevisions(t *testing.T) {
	mgr, err := New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	src := writeLocalSource(t, "acme", "tool", "def main():\n    pass\n")

	first, err := mgr.Install(src, InstallOptions{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if revs, err := mgr.Revisions("acme/tool"); err != nil || len(revs) != 1 {
		t.Fatalf("Revisions after install = %d (err %v), want 1", len(revs), err)
	}

	// Change the source and update — a new revision is added alongside the first.
	os.WriteFile(filepath.Join(src, "main.star"), []byte("def main():\n    print('v2')\n"), 0o644)
	updated, err := mgr.Update("acme/tool")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Rev == first.Rev {
		t.Error("update of changed source should produce a new revision")
	}

	revs, err := mgr.Revisions("acme/tool")
	if err != nil || len(revs) != 2 {
		t.Fatalf("Revisions after update = %d (err %v), want 2", len(revs), err)
	}

	// With two revisions, Get is ambiguous but Verify checks both.
	if _, err := mgr.Get("acme/tool"); err == nil {
		t.Error("Get should error on multiple revisions")
	}
	results, err := mgr.Verify("acme/tool")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Verify checked %d revisions, want 2", len(results))
	}
	for _, r := range results {
		if !r.OK {
			t.Errorf("revision %s failed verify: %s", r.Rev, r.Reason)
		}
	}

	// Remove drops every revision.
	if err := mgr.Remove("acme/tool"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if revs, _ := mgr.Revisions("acme/tool"); len(revs) != 0 {
		t.Errorf("Revisions after remove = %d, want 0", len(revs))
	}
}

// initGitRepo creates a local git repository containing a module and returns its
// path and the short HEAD commit. Tests clone it over file:// to exercise the
// git install path offline.
func initGitRepo(t *testing.T, ns, name, body string) (repoDir, commit string) {
	t.Helper()
	repoDir = filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	os.WriteFile(filepath.Join(repoDir, "mod.yaml"), []byte("namespace: "+ns+"\nname: "+name+"\nversion: 0.1.0\n"), 0o644)
	os.WriteFile(filepath.Join(repoDir, "main.star"), []byte(body), 0o644)

	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	git("add", "-A")
	git("commit", "-m", "init")
	commit, err := GitGetCurrentCommit(repoDir)
	if err != nil {
		t.Fatalf("get commit: %v", err)
	}
	return repoDir, commit
}

func TestManagerInstallFromGit(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available")
	}
	repo, commit := initGitRepo(t, "acme", "leaf", "def main():\n    pass\n")
	mgr, err := New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	info, err := mgr.Install("file://"+repo, InstallOptions{})
	if err != nil {
		t.Fatalf("install from git: %v", err)
	}
	if info.Namespace != "acme" || info.Name != "leaf" {
		t.Errorf("identity = %s/%s, want acme/leaf", info.Namespace, info.Name)
	}
	if info.Rev != commit {
		t.Errorf("rev = %q, want commit %q", info.Rev, commit)
	}
	if filepath.Base(info.Path) != "leaf@"+commit {
		t.Errorf("cache dir %q should be leaf@%s", filepath.Base(info.Path), commit)
	}
	// The working .git directory must not be carried into the cache.
	if _, err := os.Stat(filepath.Join(info.Path, ".git")); !os.IsNotExist(err) {
		t.Error(".git was not stripped from the installed module")
	}
	if prov, _ := ReadProvenance(info.Path); prov == nil || prov.Source != "file://"+repo {
		t.Errorf("receipt source = %v, want file://%s", prov, repo)
	}
}

func TestManagerInstallFromGitTag(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available")
	}
	repo, _ := initGitRepo(t, "acme", "leaf", "def main():\n    pass\n")
	tag := exec.Command("git", "-C", repo, "tag", "v1.0.0")
	if out, err := tag.CombinedOutput(); err != nil {
		t.Fatalf("git tag: %v\n%s", err, out)
	}

	mgr, _ := New(filepath.Join(t.TempDir(), "cache"))
	info, err := mgr.Install("file://"+repo+"@v1.0.0", InstallOptions{})
	if err != nil {
		t.Fatalf("install from git tag: %v", err)
	}
	if info.Version != "v1.0.0" {
		t.Errorf("version = %q, want v1.0.0", info.Version)
	}
	if info.Rev == "" {
		t.Error("expected a commit rev for a tag install")
	}
}

func TestManagerInstallInvalid(t *testing.T) {
	mgr, err := New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mkdir := func(name string, files map[string]string) string {
		dir := filepath.Join(t.TempDir(), name)
		os.MkdirAll(dir, 0o755)
		for fn, content := range files {
			os.WriteFile(filepath.Join(dir, fn), []byte(content), 0o644)
		}
		return dir
	}

	tests := []struct {
		name   string
		source string
	}{
		{"missing mod.yaml", mkdir("a", map[string]string{"main.star": "def main(): pass\n"})},
		{"missing main.star", mkdir("b", map[string]string{"mod.yaml": "namespace: x\nname: b\n"})},
		{"mod.yaml missing name", mkdir("c", map[string]string{"mod.yaml": "version: 0.1.0\n", "main.star": "def main(): pass\n"})},
		{"nonexistent source", filepath.Join(t.TempDir(), "does-not-exist")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := mgr.Install(tt.source, InstallOptions{Name: "x/" + tt.name}); err == nil {
				t.Errorf("expected install of an invalid module to fail")
			}
		})
	}
}

func TestManagerResolve(t *testing.T) {
	mgr, _ := New(filepath.Join(t.TempDir(), "cache"))
	src := writeLocalSource(t, "acme", "tool", "def main():\n    pass\n")
	first, err := mgr.Install(src, InstallOptions{})
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	t.Run("single revision: newest and exact agree", func(t *testing.T) {
		got, err := mgr.Resolve("acme/tool", "")
		if err != nil || got.Rev != first.Rev {
			t.Fatalf("Resolve newest = %v (err %v), want %s", got, err, first.Rev)
		}
		if got, err := mgr.Resolve("acme/tool", first.Rev); err != nil || got.Rev != first.Rev {
			t.Fatalf("Resolve exact = %v (err %v)", got, err)
		}
		if got, err := mgr.Resolve("acme/tool", first.Rev[:6]); err != nil || got.Rev != first.Rev {
			t.Fatalf("Resolve prefix = %v (err %v)", got, err)
		}
	})

	// Update to a second revision.
	os.WriteFile(filepath.Join(src, "main.star"), []byte("def main():\n    print('v2')\n"), 0o644)
	updated, err := mgr.Update("acme/tool")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	// Make the updated revision unambiguously newest regardless of filesystem
	// mtime granularity.
	future := time.Now().Add(time.Hour)
	os.Chtimes(updated.Path, future, future)

	t.Run("newest follows the latest revision", func(t *testing.T) {
		got, err := mgr.Resolve("acme/tool", "")
		if err != nil || got.Rev != updated.Rev {
			t.Fatalf("Resolve newest = %v (err %v), want %s", got, err, updated.Rev)
		}
	})
	t.Run("exact still pins the older revision", func(t *testing.T) {
		got, err := mgr.Resolve("acme/tool", first.Rev)
		if err != nil || got.Rev != first.Rev {
			t.Fatalf("Resolve exact older = %v (err %v)", got, err)
		}
	})
	t.Run("unknown revision errors", func(t *testing.T) {
		if _, err := mgr.Resolve("acme/tool", "zzzzzz"); err == nil {
			t.Error("expected error for unknown revision")
		}
	})
	t.Run("not installed errors", func(t *testing.T) {
		if _, err := mgr.Resolve("acme/none", ""); err == nil {
			t.Error("expected error for uninstalled module")
		}
	})
}

func TestManagerVerify(t *testing.T) {
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	mgr, err := New(cacheRoot)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	os.WriteFile(filepath.Join(src, "mod.yaml"), []byte("namespace: acme\nname: tool\nversion: 0.1.0\n"), 0o644)
	os.WriteFile(filepath.Join(src, "main.star"), []byte("def main():\n    pass\n"), 0o644)

	info, err := mgr.Install(src, InstallOptions{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	t.Run("intact module verifies", func(t *testing.T) {
		results, err := mgr.Verify("acme/tool")
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if len(results) != 1 || !results[0].OK {
			t.Fatalf("expected one passing result, got %+v", results)
		}
	})

	t.Run("tampered module fails", func(t *testing.T) {
		// Modify a tracked file after install; the recorded hash no longer matches.
		if err := os.WriteFile(filepath.Join(info.Path, "main.star"), []byte("def main():\n    print('tampered')\n"), 0o644); err != nil {
			t.Fatalf("tamper: %v", err)
		}
		results, err := mgr.Verify("acme/tool")
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if len(results) != 1 || results[0].OK {
			t.Fatalf("expected failure for tampered module, got %+v", results)
		}
	})
}

func TestValidateModule(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := New(tmpDir)

	t.Run("valid: manifest + main.star", func(t *testing.T) {
		dir := filepath.Join(tmpDir, "ok")
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "mod.yaml"), []byte("name: ok\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "main.star"), []byte("# main"), 0o644)

		if err := mgr.validateModule(dir, "ok"); err != nil {
			t.Errorf("expected valid module, got error: %v", err)
		}
	})

	t.Run("missing mod.yaml", func(t *testing.T) {
		dir := filepath.Join(tmpDir, "no-manifest")
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "main.star"), []byte("# main"), 0o644)

		if err := mgr.validateModule(dir, "no-manifest"); err == nil {
			t.Error("expected error for missing mod.yaml")
		}
	})

	t.Run("missing main.star entry", func(t *testing.T) {
		dir := filepath.Join(tmpDir, "no-entry")
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "mod.yaml"), []byte("name: no-entry\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "helper.star"), []byte("# helper"), 0o644)

		if err := mgr.validateModule(dir, "no-entry"); err == nil {
			t.Error("expected error: entry must be main.star, not an arbitrary .star")
		}
	})
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("file exists", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "exists.txt")
		os.WriteFile(filePath, []byte("test"), 0644)

		if !fileExists(filePath) {
			t.Error("expected true for existing file")
		}
	})

	t.Run("file does not exist", func(t *testing.T) {
		if fileExists(filepath.Join(tmpDir, "nonexistent.txt")) {
			t.Error("expected false for nonexistent file")
		}
	})

	t.Run("directory", func(t *testing.T) {
		if fileExists(tmpDir) {
			t.Error("expected false for directory")
		}
	})
}

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{filepath.Join("/", "absolute", "path"), true},
		{"." + string(filepath.Separator) + "relative", true},
		{".." + string(filepath.Separator) + "parent", true},
		{"~" + string(filepath.Separator) + "home", true},
		{".", true},
		{"..", true},
		{"~", true},
		{"github.com/user/repo", false},
		{"user/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got := isLocalPath(tt.source)
			if got != tt.want {
				t.Errorf("isLocalPath(%q) = %v, want %v", tt.source, got, tt.want)
			}
		})
	}
}
