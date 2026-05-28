package manager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseWasmManifest_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "module.yaml")
	content := `
name: echo
version: 1.0.0
description: Echo plugin
wasm: echo.wasm
min_starkite: "0.5.0"
functions:
  - name: echo
    params:
      - name: input
        type: string
    returns: string
  - name: add
    params:
      - name: a
        type: int
      - name: b
        type: int
    returns: int
permissions:
  - log
`
	os.WriteFile(path, []byte(content), 0644)

	m, err := parseWasmManifest(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "echo" {
		t.Errorf("Name = %q, want %q", m.Name, "echo")
	}
	if m.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", m.Version, "1.0.0")
	}
	if m.Wasm != "echo.wasm" {
		t.Errorf("Wasm = %q, want %q", m.Wasm, "echo.wasm")
	}
	if len(m.Functions) != 2 {
		t.Fatalf("Functions count = %d, want 2", len(m.Functions))
	}
	if m.Functions[0].Name != "echo" {
		t.Errorf("Functions[0].Name = %q, want %q", m.Functions[0].Name, "echo")
	}
	if len(m.Permissions) != 1 || m.Permissions[0] != "log" {
		t.Errorf("Permissions = %v, want [log]", m.Permissions)
	}
}

func TestParseWasmManifest_MissingFile(t *testing.T) {
	_, err := parseWasmManifest("/nonexistent/module.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseWasmManifest_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "module.yaml")
	os.WriteFile(path, []byte("{{{{not yaml"), 0644)

	_, err := parseWasmManifest(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParseWasmManifest_MissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "module.yaml")
	os.WriteFile(path, []byte("version: 1.0\nwasm: a.wasm\n"), 0644)

	_, err := parseWasmManifest(path)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseWasmManifest_MissingVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "module.yaml")
	os.WriteFile(path, []byte("name: x\nwasm: a.wasm\n"), 0644)

	_, err := parseWasmManifest(path)
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestParseWasmManifest_MissingWasm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "module.yaml")
	os.WriteFile(path, []byte("name: x\nversion: 1.0\n"), 0644)

	_, err := parseWasmManifest(path)
	if err == nil {
		t.Fatal("expected error for missing wasm field")
	}
}
