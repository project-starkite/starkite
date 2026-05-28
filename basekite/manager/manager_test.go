package manager

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
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
		expectedStarlark := filepath.Join(expected, "starlark")
		if mgr.StarlarkDir() != expectedStarlark {
			t.Errorf("StarlarkDir() = %q, want %q", mgr.StarlarkDir(), expectedStarlark)
		}
		expectedWasm := filepath.Join(expected, "wasm")
		if mgr.WasmDir() != expectedWasm {
			t.Errorf("WasmDir() = %q, want %q", mgr.WasmDir(), expectedWasm)
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

		// Root, starlark, and wasm directories should be created
		for _, dir := range []string{customDir, mgr.StarlarkDir(), mgr.WasmDir()} {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				t.Errorf("directory %q was not created", dir)
			}
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
		// Create a fake starlark module
		moduleDir := filepath.Join(mgr.StarlarkDir(), "test-module")
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("failed to create module dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(moduleDir, "main.star"), []byte("# test"), 0644); err != nil {
			t.Fatalf("failed to create main.star: %v", err)
		}

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
		// Create a fake starlark module
		moduleDir := filepath.Join(mgr.StarlarkDir(), "my-module")
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("failed to create module dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(moduleDir, "main.star"), []byte("# test"), 0644); err != nil {
			t.Fatalf("failed to create main.star: %v", err)
		}

		info, err := mgr.Get("my-module")
		if err != nil {
			t.Fatalf("Get() failed: %v", err)
		}
		if info.Name != "my-module" {
			t.Errorf("expected name 'my-module', got %q", info.Name)
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
		moduleDir := filepath.Join(mgr.StarlarkDir(), "remove-me")
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("failed to create module dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(moduleDir, "main.star"), []byte("# test"), 0644); err != nil {
			t.Fatalf("failed to create main.star: %v", err)
		}

		err := mgr.Remove("remove-me")
		if err != nil {
			t.Fatalf("Remove() failed: %v", err)
		}

		// Verify it's gone
		if _, err := os.Stat(moduleDir); !os.IsNotExist(err) {
			t.Error("module directory still exists after removal")
		}
	})
}

func TestFindEntryPoint(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := New(tmpDir)

	t.Run("main.star", func(t *testing.T) {
		moduleDir := filepath.Join(tmpDir, "mod1")
		os.MkdirAll(moduleDir, 0755)
		os.WriteFile(filepath.Join(moduleDir, "main.star"), []byte("# main"), 0644)

		entryPoint := mgr.findEntryPoint(moduleDir, "mod1")
		if entryPoint != filepath.Join(moduleDir, "main.star") {
			t.Errorf("expected main.star, got %q", entryPoint)
		}
	})

	t.Run("name.star", func(t *testing.T) {
		moduleDir := filepath.Join(tmpDir, "mod2")
		os.MkdirAll(moduleDir, 0755)
		os.WriteFile(filepath.Join(moduleDir, "mod2.star"), []byte("# mod2"), 0644)

		entryPoint := mgr.findEntryPoint(moduleDir, "mod2")
		if entryPoint != filepath.Join(moduleDir, "mod2.star") {
			t.Errorf("expected mod2.star, got %q", entryPoint)
		}
	})

	t.Run("any .star file", func(t *testing.T) {
		moduleDir := filepath.Join(tmpDir, "mod3")
		os.MkdirAll(moduleDir, 0755)
		os.WriteFile(filepath.Join(moduleDir, "helper.star"), []byte("# helper"), 0644)

		entryPoint := mgr.findEntryPoint(moduleDir, "mod3")
		if entryPoint != filepath.Join(moduleDir, "helper.star") {
			t.Errorf("expected helper.star, got %q", entryPoint)
		}
	})

	t.Run("no .star file", func(t *testing.T) {
		moduleDir := filepath.Join(tmpDir, "mod4")
		os.MkdirAll(moduleDir, 0755)
		os.WriteFile(filepath.Join(moduleDir, "README.md"), []byte("# readme"), 0644)

		entryPoint := mgr.findEntryPoint(moduleDir, "mod4")
		if entryPoint != "" {
			t.Errorf("expected empty string, got %q", entryPoint)
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

// --- WASM module tests ---

// createFakeWasmModule creates a fake WASM module directory with module.yaml and a .wasm file.
func createFakeWasmModule(t *testing.T, dir, name, version string, functions []string, permissions []string) {
	t.Helper()
	moduleDir := filepath.Join(dir, name)
	os.MkdirAll(moduleDir, 0755)

	// Build functions YAML
	var funcYAML string
	for _, fn := range functions {
		funcYAML += "  - name: " + fn + "\n    returns: string\n"
	}

	// Build permissions YAML
	var permYAML string
	if len(permissions) > 0 {
		permYAML = "permissions:\n"
		for _, p := range permissions {
			permYAML += "  - " + p + "\n"
		}
	}

	manifest := "name: " + name + "\nversion: " + version + "\nwasm: " + name + ".wasm\nfunctions:\n" + funcYAML + permYAML
	os.WriteFile(filepath.Join(moduleDir, "module.yaml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(moduleDir, name+".wasm"), []byte("fake-wasm-binary"), 0644)
}

func TestManagerListIncludesWasm(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Create a starlark module
	starlarkDir := filepath.Join(mgr.StarlarkDir(), "helm")
	os.MkdirAll(starlarkDir, 0755)
	os.WriteFile(filepath.Join(starlarkDir, "main.star"), []byte("# helm"), 0644)

	// Create a WASM module
	createFakeWasmModule(t, mgr.WasmDir(), "echo", "1.0.0", []string{"echo", "add"}, []string{"log"})

	modules, err := mgr.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(modules))
	}

	// Sort by name for predictable ordering
	sort.Slice(modules, func(i, j int) bool { return modules[i].Name < modules[j].Name })

	if modules[0].Name != "echo" || modules[0].Type != "wasm" {
		t.Errorf("expected echo/wasm, got %s/%s", modules[0].Name, modules[0].Type)
	}
	if modules[1].Name != "helm" || modules[1].Type != "starlark" {
		t.Errorf("expected helm/starlark, got %s/%s", modules[1].Name, modules[1].Type)
	}
}

func TestManagerGetWasm(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	createFakeWasmModule(t, mgr.WasmDir(), "echo", "1.0.0", []string{"echo", "add"}, []string{"log"})

	info, err := mgr.Get("echo")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if info.Type != "wasm" {
		t.Errorf("expected type 'wasm', got %q", info.Type)
	}
	if info.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", info.Version)
	}
	if info.WasmFile != "echo.wasm" {
		t.Errorf("expected WasmFile 'echo.wasm', got %q", info.WasmFile)
	}
	if len(info.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(info.Functions))
	}
	if info.Functions[0] != "echo" || info.Functions[1] != "add" {
		t.Errorf("unexpected functions: %v", info.Functions)
	}
	if len(info.Permissions) != 1 || info.Permissions[0] != "log" {
		t.Errorf("unexpected permissions: %v", info.Permissions)
	}
}

func TestManagerRemoveWasm(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	createFakeWasmModule(t, mgr.WasmDir(), "echo", "1.0.0", []string{"echo"}, nil)

	err = mgr.Remove("echo")
	if err != nil {
		t.Fatalf("Remove() failed: %v", err)
	}

	// Verify it's gone
	wasmPath := filepath.Join(mgr.WasmDir(), "echo")
	if _, err := os.Stat(wasmPath); !os.IsNotExist(err) {
		t.Error("WASM module directory still exists after removal")
	}
}

func TestInstallWasmFromLocal(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Create a source directory with WASM module
	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0755)
	manifest := "name: mymod\nversion: 2.0.0\nwasm: mymod.wasm\nfunctions:\n  - name: run\n    returns: string\npermissions:\n  - exec\n"
	os.WriteFile(filepath.Join(sourceDir, "module.yaml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(sourceDir, "mymod.wasm"), []byte("fake-wasm"), 0644)

	info, err := mgr.InstallWasm(sourceDir, InstallOptions{})
	if err != nil {
		t.Fatalf("InstallWasm() failed: %v", err)
	}
	if info.Name != "mymod" {
		t.Errorf("expected name 'mymod', got %q", info.Name)
	}
	if info.Type != "wasm" {
		t.Errorf("expected type 'wasm', got %q", info.Type)
	}
	if info.Version != "2.0.0" {
		t.Errorf("expected version '2.0.0', got %q", info.Version)
	}

	// Verify files were copied
	destDir := filepath.Join(mgr.WasmDir(), "mymod")
	if !fileExists(filepath.Join(destDir, "module.yaml")) {
		t.Error("module.yaml not copied to destination")
	}
	if !fileExists(filepath.Join(destDir, "mymod.wasm")) {
		t.Error("mymod.wasm not copied to destination")
	}
}

func TestInstallWasmFromLocalForce(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Create source
	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0755)
	manifest := "name: mymod\nversion: 1.0.0\nwasm: mymod.wasm\nfunctions:\n  - name: run\n    returns: string\n"
	os.WriteFile(filepath.Join(sourceDir, "module.yaml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(sourceDir, "mymod.wasm"), []byte("v1"), 0644)

	// Install first time
	_, err = mgr.InstallWasm(sourceDir, InstallOptions{})
	if err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	// Install again without force should fail
	_, err = mgr.InstallWasm(sourceDir, InstallOptions{})
	if err == nil {
		t.Fatal("expected error for duplicate install without force")
	}

	// Install with force should succeed
	_, err = mgr.InstallWasm(sourceDir, InstallOptions{Force: true})
	if err != nil {
		t.Fatalf("force install failed: %v", err)
	}
}

func TestInstallWasmMissingManifest(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Create a source directory without module.yaml
	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0755)
	os.WriteFile(filepath.Join(sourceDir, "something.wasm"), []byte("fake"), 0644)

	_, err = mgr.InstallWasm(sourceDir, InstallOptions{})
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestInstallWasmMissingWasmFile(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Create a source directory with manifest but no .wasm file
	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0755)
	manifest := "name: mymod\nversion: 1.0.0\nwasm: mymod.wasm\nfunctions:\n  - name: run\n    returns: string\n"
	os.WriteFile(filepath.Join(sourceDir, "module.yaml"), []byte(manifest), 0644)

	_, err = mgr.InstallWasm(sourceDir, InstallOptions{})
	if err == nil {
		t.Fatal("expected error for missing WASM file")
	}
}

func TestUpdateWasmReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	createFakeWasmModule(t, mgr.WasmDir(), "echo", "1.0.0", []string{"echo"}, nil)

	_, err = mgr.Update("echo")
	if err == nil {
		t.Fatal("expected error for WASM module update")
	}
	if err.Error() != "WASM modules cannot be updated; reinstall with --force" {
		t.Errorf("unexpected error message: %v", err)
	}
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
		{"file.wasm", true},
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
