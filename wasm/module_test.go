package wasm

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/project-starkite/starkite/libkite"
)

func TestWasmModule_Name(t *testing.T) {
	m := &WasmModule{
		manifest: &PluginManifest{Name: "testplugin"},
	}
	if m.Name() != "testplugin" {
		t.Errorf("Name() = %q, want %q", m.Name(), "testplugin")
	}
}

func TestWasmModule_Description(t *testing.T) {
	m := &WasmModule{
		manifest: &PluginManifest{Description: "A test plugin"},
	}
	if m.Description() != "A test plugin" {
		t.Errorf("Description() = %q, want %q", m.Description(), "A test plugin")
	}
}

func TestWasmModule_Aliases(t *testing.T) {
	m := &WasmModule{manifest: &PluginManifest{}}
	if m.Aliases() != nil {
		t.Error("Aliases() should return nil")
	}
}

func TestWasmModule_FactoryMethod(t *testing.T) {
	m := &WasmModule{manifest: &PluginManifest{}}
	if m.FactoryMethod() != "" {
		t.Error("FactoryMethod() should return empty string")
	}
}

func TestWasmModule_Load_MissingWasm(t *testing.T) {
	m := &WasmModule{
		manifest: &PluginManifest{
			Name:    "bad",
			Version: "1.0",
			Wasm:    "nonexistent.wasm",
			Functions: []FunctionManifest{
				{Name: "f", Returns: "string"},
			},
		},
		wasmPath: "/nonexistent/path/bad.wasm",
	}

	_, err := m.Load(&libkite.ModuleConfig{})
	if err == nil {
		t.Fatal("expected error for missing wasm file")
	}
	if !IsWasmError(err) {
		t.Errorf("expected WasmError, got %T: %v", err, err)
	}
}

func TestNewWasmModule(t *testing.T) {
	discovered := &DiscoveredPlugin{
		Manifest: &PluginManifest{
			Name:        "test",
			Version:     "1.0",
			Description: "test plugin",
			Wasm:        "test.wasm",
			Functions: []FunctionManifest{
				{Name: "hello", Returns: "string"},
			},
		},
		WasmPath:     "/path/to/test.wasm",
		ManifestPath: "/path/to/module.yaml",
	}

	m := NewWasmModule(discovered)
	if m.Name() != "test" {
		t.Errorf("Name() = %q, want %q", m.Name(), "test")
	}
	if m.wasmPath != "/path/to/test.wasm" {
		t.Errorf("wasmPath = %q, want %q", m.wasmPath, "/path/to/test.wasm")
	}
}

func TestWasmModule_Close_BeforeLoad(t *testing.T) {
	m := &WasmModule{manifest: &PluginManifest{Name: "test"}}
	// Close before Load should not panic
	if err := m.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

// loadEchoModule is a helper that loads the echo WASM module or skips the test.
func loadEchoModule(t *testing.T) *WasmModule {
	t.Helper()
	wasmPath := filepath.Join("testdata", "echo", "echo.wasm")
	if _, err := os.Stat(wasmPath); os.IsNotExist(err) {
		t.Skip("echo.wasm not found; skipping integration test")
	}

	discovered := &DiscoveredPlugin{
		Manifest: mustParseManifest(t, filepath.Join("testdata", "echo", "module.yaml")),
		WasmPath: wasmPath,
	}

	m := NewWasmModule(discovered)
	config := &libkite.ModuleConfig{}
	if _, err := m.Load(config); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	return m
}

// TestWasmModule_Integration tests loading a real WASM binary.
func TestWasmModule_Integration(t *testing.T) {
	m := loadEchoModule(t)
	defer m.Close()

	exports, _ := m.Load(&libkite.ModuleConfig{})
	if _, ok := exports["echo"]; !ok {
		t.Fatal("expected 'echo' in exports")
	}
}

// TestWasmModule_CallEcho calls the echo function and verifies the result.
func TestWasmModule_CallEcho(t *testing.T) {
	m := loadEchoModule(t)
	defer m.Close()

	thread := &starlark.Thread{Name: "test"}
	echoFn := m.module.(*starlarkstruct.Module).Members["echo"].(*starlark.Builtin)

	result, err := starlark.Call(thread, echoFn, starlark.Tuple{starlark.String("hello world")}, nil)
	if err != nil {
		t.Fatalf("echo() error: %v", err)
	}

	s, ok := result.(starlark.String)
	if !ok {
		t.Fatalf("echo() returned %T, want String", result)
	}
	if string(s) != "hello world" {
		t.Errorf("echo() = %q, want %q", string(s), "hello world")
	}
}

// TestWasmModule_CallAdd calls the add function and verifies the result.
func TestWasmModule_CallAdd(t *testing.T) {
	m := loadEchoModule(t)
	defer m.Close()

	thread := &starlark.Thread{Name: "test"}
	addFn := m.module.(*starlarkstruct.Module).Members["add"].(*starlark.Builtin)

	result, err := starlark.Call(thread, addFn,
		starlark.Tuple{starlark.MakeInt(17), starlark.MakeInt(25)}, nil)
	if err != nil {
		t.Fatalf("add() error: %v", err)
	}

	i, ok := result.(starlark.Int)
	if !ok {
		t.Fatalf("add() returned %T, want Int", result)
	}
	val, _ := i.Int64()
	if val != 42 {
		t.Errorf("add(17, 25) = %d, want 42", val)
	}
}

// TestWasmModule_CallAddKwargs calls add with keyword arguments.
func TestWasmModule_CallAddKwargs(t *testing.T) {
	m := loadEchoModule(t)
	defer m.Close()

	thread := &starlark.Thread{Name: "test"}
	addFn := m.module.(*starlarkstruct.Module).Members["add"].(*starlark.Builtin)

	kwargs := []starlark.Tuple{
		{starlark.String("b"), starlark.MakeInt(100)},
		{starlark.String("a"), starlark.MakeInt(23)},
	}
	result, err := starlark.Call(thread, addFn, nil, kwargs)
	if err != nil {
		t.Fatalf("add() error: %v", err)
	}

	i, ok := result.(starlark.Int)
	if !ok {
		t.Fatalf("add() returned %T, want Int", result)
	}
	val, _ := i.Int64()
	if val != 123 {
		t.Errorf("add(a=23, b=100) = %d, want 123", val)
	}
}

// TestWasmModule_CallEchoEmpty tests echo with an empty string.
func TestWasmModule_CallEchoEmpty(t *testing.T) {
	m := loadEchoModule(t)
	defer m.Close()

	thread := &starlark.Thread{Name: "test"}
	echoFn := m.module.(*starlarkstruct.Module).Members["echo"].(*starlark.Builtin)

	result, err := starlark.Call(thread, echoFn, starlark.Tuple{starlark.String("")}, nil)
	if err != nil {
		t.Fatalf("echo() error: %v", err)
	}

	s, ok := result.(starlark.String)
	if !ok {
		t.Fatalf("echo() returned %T, want String", result)
	}
	if string(s) != "" {
		t.Errorf("echo('') = %q, want %q", string(s), "")
	}
}

// TestWasmModule_MissingArg tests that calling with missing required args fails.
func TestWasmModule_MissingArg(t *testing.T) {
	m := loadEchoModule(t)
	defer m.Close()

	thread := &starlark.Thread{Name: "test"}
	addFn := m.module.(*starlarkstruct.Module).Members["add"].(*starlark.Builtin)

	// Only one arg when two are required
	_, err := starlark.Call(thread, addFn, starlark.Tuple{starlark.MakeInt(1)}, nil)
	if err == nil {
		t.Fatal("expected error for missing required arg")
	}
}

// TestWasmModule_Concurrent tests that concurrent calls produce correct results.
func TestWasmModule_Concurrent(t *testing.T) {
	m := loadEchoModule(t)
	defer m.Close()

	addFn := m.module.(*starlarkstruct.Module).Members["add"].(*starlark.Builtin)

	const goroutines = 10
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			thread := &starlark.Thread{Name: "test"}
			result, err := starlark.Call(thread, addFn,
				starlark.Tuple{starlark.MakeInt(n), starlark.MakeInt(n)}, nil)
			if err != nil {
				errs <- err
				return
			}
			val, _ := result.(starlark.Int).Int64()
			if val != int64(2*n) {
				errs <- fmt.Errorf("add(%d, %d) = %d, want %d", n, n, val, 2*n)
				return
			}
			errs <- nil
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent call error: %v", err)
		}
	}
}

func mustParseManifest(t *testing.T, path string) *PluginManifest {
	t.Helper()
	m, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("ParseManifest(%q) error: %v", path, err)
	}
	return m
}
