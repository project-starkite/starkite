package libkite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseModuleManifest_WithSandbox(t *testing.T) {
	manifestYAML := `
namespace: myorg
name: testmod
version: 1.0.0
description: A test module with sandbox requirements
sandbox:
  driver: auto
  network: host
  timeout: 5m
  max_memory: 512MB
  paths:
    read:
      - .
    write:
      - ./dist
  exec:
    allow:
      - git
      - go
    deny:
      - rm
  calls:
    allow:
      - "os.*"
    deny:
      - "os.exit"
`
	m, err := ParseModuleManifest([]byte(manifestYAML), "mod.yaml")
	if err != nil {
		t.Fatalf("ParseModuleManifest: %v", err)
	}

	if m.Namespace != "myorg" || m.Name != "testmod" {
		t.Errorf("got Namespace=%q Name=%q, want myorg/testmod", m.Namespace, m.Name)
	}
	if m.Sandbox == nil {
		t.Fatal("expected non-nil Sandbox spec")
	}
	if m.Sandbox.Driver != "auto" {
		t.Errorf("Driver = %q, want auto", m.Sandbox.Driver)
	}
	if m.Sandbox.Timeout != "5m" {
		t.Errorf("Timeout = %q, want 5m", m.Sandbox.Timeout)
	}
	if len(m.Sandbox.Paths.Read) != 1 || m.Sandbox.Paths.Read[0] != "." {
		t.Errorf("Paths.Read = %v, want [.]", m.Sandbox.Paths.Read)
	}
	if len(m.Sandbox.Paths.Write) != 1 || m.Sandbox.Paths.Write[0] != "./dist" {
		t.Errorf("Paths.Write = %v, want [./dist]", m.Sandbox.Paths.Write)
	}
	if len(m.Sandbox.Exec.Allow) != 2 {
		t.Errorf("Exec.Allow = %v, want [git go]", m.Sandbox.Exec.Allow)
	}
	if len(m.Sandbox.Calls.Deny) != 1 || m.Sandbox.Calls.Deny[0] != "os.exit" {
		t.Errorf("Calls.Deny = %v, want [os.exit]", m.Sandbox.Calls.Deny)
	}
}

func TestParseModuleManifest_InvalidSandbox(t *testing.T) {
	badTimeout := `
name: badtimeout
sandbox:
  timeout: invalid-duration
`
	if _, err := ParseModuleManifest([]byte(badTimeout), "mod.yaml"); err == nil || !strings.Contains(err.Error(), "invalid timeout") {
		t.Errorf("expected timeout validation error, got: %v", err)
	}

	badNet := `
name: badnet
sandbox:
  network: invalid-network-mode
`
	if _, err := ParseModuleManifest([]byte(badNet), "mod.yaml"); err == nil || !strings.Contains(err.Error(), "unknown network mode") {
		t.Errorf("expected network mode validation error, got: %v", err)
	}
}

func TestLoadModuleManifest(t *testing.T) {
	dir := t.TempDir()
	manifest := `
name: validmod
sandbox:
  network: loopback
`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, EntryFile), []byte("def main(): pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadModuleManifest(dir)
	if err != nil {
		t.Fatalf("LoadModuleManifest: %v", err)
	}
	if m.Name != "validmod" {
		t.Errorf("Name = %q, want validmod", m.Name)
	}
	if m.Sandbox == nil || m.Sandbox.Network != "loopback" {
		t.Errorf("expected Sandbox.Network = loopback, got %+v", m.Sandbox)
	}
}
