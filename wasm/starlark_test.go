package wasm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/modules/test"
)

// TestStarlarkEndToEnd boots a full libkite Runtime with the echo WASM plugin
// registered, then runs the echo_test.star Starlark script through ExecuteTests.
// This exercises every moving part: Runtime -> Registry -> WasmModule -> Extism
// -> JSON marshaling -> WASM guest -> JSON unmarshaling -> Starlark value.
func TestStarlarkEndToEnd(t *testing.T) {
	wasmPath := filepath.Join("testdata", "echo", "echo.wasm")
	if _, err := os.Stat(wasmPath); os.IsNotExist(err) {
		t.Skip("echo.wasm not found; skipping end-to-end Starlark test")
	}

	// Read the Starlark test script
	testScript, err := os.ReadFile(filepath.Join("testdata", "echo", "echo_test.star"))
	if err != nil {
		t.Fatalf("read test script: %v", err)
	}

	// Parse the echo plugin manifest
	manifest := mustParseManifest(t, filepath.Join("testdata", "echo", "module.yaml"))

	// Build a registry with just the echo WASM module
	moduleConfig := &libkite.ModuleConfig{}
	registry := libkite.NewRegistry(moduleConfig)

	discovered := &DiscoveredPlugin{
		Manifest: manifest,
		WasmPath: wasmPath,
	}
	wasmModule := NewWasmModule(discovered)
	registry.Register(wasmModule)
	registry.Register(test.New())

	// Create a full Runtime with the registry
	config := &libkite.Config{
		ScriptPath:  "echo_test.star",
		Registry:    registry,
		Permissions: libkite.TrustedPermissions(),
	}

	rt, err := libkite.New(config)
	if err != nil {
		t.Fatalf("libkite.New: %v", err)
	}
	defer rt.Close()

	// Run all test_* functions in the Starlark script
	results, err := rt.ExecuteTests(context.Background(), string(testScript))
	if err != nil {
		t.Fatalf("ExecuteTests: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("no test results; expected test_* functions in echo_test.star")
	}

	// Report each test result
	passed, failed, skipped := 0, 0, 0
	for _, r := range results {
		switch {
		case r.Skipped:
			skipped++
			t.Logf("  SKIP  %s (%v)", r.Name, r.Duration)
		case r.Passed:
			passed++
			t.Logf("  PASS  %s (%v)", r.Name, r.Duration)
		default:
			failed++
			t.Errorf("  FAIL  %s: %v (%v)", r.Name, r.Error, r.Duration)
		}
	}

	t.Logf("Starlark tests: %d passed, %d failed, %d skipped (total: %d)", passed, failed, skipped, len(results))
}
