package wasm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	plugins, err := Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestDiscover_NonexistentDir(t *testing.T) {
	plugins, err := Discover("/nonexistent/path/plugins")
	if err != nil {
		t.Fatal("expected nil error for nonexistent dir")
	}
	if plugins != nil {
		t.Errorf("expected nil plugins, got %v", plugins)
	}
}

func TestDiscover_ValidPlugin(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "myplugin")
	os.Mkdir(pluginDir, 0755)

	// Write manifest
	manifest := `
name: myplugin
version: 1.0.0
description: test plugin
wasm: myplugin.wasm
functions:
  - name: greet
    params:
      - name: name
        type: string
    returns: string
`
	os.WriteFile(filepath.Join(pluginDir, "module.yaml"), []byte(manifest), 0644)

	// Create fake .wasm file
	os.WriteFile(filepath.Join(pluginDir, "myplugin.wasm"), []byte("fake-wasm"), 0644)

	plugins, err := Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}

	p := plugins[0]
	if p.Manifest.Name != "myplugin" {
		t.Errorf("name = %q, want %q", p.Manifest.Name, "myplugin")
	}
	if !filepath.IsAbs(p.WasmPath) || filepath.Base(p.WasmPath) != "myplugin.wasm" {
		t.Errorf("wasmPath = %q, expected absolute path ending in myplugin.wasm", p.WasmPath)
	}
}

func TestDiscover_MissingManifest(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "nomanifest"), 0755)

	plugins, err := Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins for missing manifest, got %d", len(plugins))
	}
}

func TestDiscover_MissingWasm(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "broken")
	os.Mkdir(pluginDir, 0755)

	manifest := `
name: broken
version: 1.0.0
wasm: missing.wasm
functions:
  - name: f
    returns: string
`
	os.WriteFile(filepath.Join(pluginDir, "module.yaml"), []byte(manifest), 0644)

	plugins, err := Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins for missing wasm, got %d", len(plugins))
	}
}

func TestDiscover_InvalidManifest(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "invalid")
	os.Mkdir(pluginDir, 0755)

	// Missing required fields
	os.WriteFile(filepath.Join(pluginDir, "module.yaml"), []byte("name: invalid\n"), 0644)

	plugins, err := Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins for invalid manifest, got %d", len(plugins))
	}
}

func TestDiscover_SkipsFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a file (not a directory)
	os.WriteFile(filepath.Join(dir, "notadir.yaml"), []byte("test"), 0644)

	plugins, err := Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestDiscover_MultiplePlugins(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"alpha", "beta"} {
		pluginDir := filepath.Join(dir, name)
		os.Mkdir(pluginDir, 0755)

		manifest := "name: " + name + "\nversion: 1.0.0\nwasm: " + name + ".wasm\nfunctions:\n  - name: f\n    returns: string\n"
		os.WriteFile(filepath.Join(pluginDir, "module.yaml"), []byte(manifest), 0644)
		os.WriteFile(filepath.Join(pluginDir, name+".wasm"), []byte("fake"), 0644)
	}

	plugins, err := Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 2 {
		t.Errorf("expected 2 plugins, got %d", len(plugins))
	}
}
