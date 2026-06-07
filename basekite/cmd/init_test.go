package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInitScaffold(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-module")

	// runInit reads package-level flag vars; set them explicitly.
	initName, initTemplate, listTemplates = "", "basic", false
	t.Cleanup(func() { initName, initTemplate, listTemplates = "", "basic", false })

	if err := runInit(nil, []string{dir}); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	for _, name := range []string{"mod.yaml", "main.star", "mod.lock", "README.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to be created: %v", name, err)
		}
	}

	manifest, err := os.ReadFile(filepath.Join(dir, "mod.yaml"))
	if err != nil {
		t.Fatalf("read mod.yaml: %v", err)
	}
	if !strings.Contains(string(manifest), "name: my-module") {
		t.Errorf("mod.yaml should default name to the directory: %s", manifest)
	}

	lock, err := os.ReadFile(filepath.Join(dir, "mod.lock"))
	if err != nil {
		t.Fatalf("read mod.lock: %v", err)
	}
	if !strings.Contains(string(lock), "version: 1") {
		t.Errorf("mod.lock should be a valid empty lock: %s", lock)
	}
}

func TestRunInitExplicitIdentity(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	initName, initTemplate, listTemplates = "acme/widget", "basic", false
	t.Cleanup(func() { initName, initTemplate, listTemplates = "", "basic", false })

	if err := runInit(nil, []string{dir}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	manifest, _ := os.ReadFile(filepath.Join(dir, "mod.yaml"))
	if !strings.Contains(string(manifest), "namespace: acme") || !strings.Contains(string(manifest), "name: widget") {
		t.Errorf("mod.yaml should reflect --name acme/widget: %s", manifest)
	}
}

func TestRunInitUnknownTemplate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x")
	initName, initTemplate, listTemplates = "", "bogus", false
	t.Cleanup(func() { initName, initTemplate, listTemplates = "", "basic", false })

	if err := runInit(nil, []string{dir}); err == nil {
		t.Fatal("expected error for unknown template")
	}
}

func TestModuleIdentity(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		dir     string
		wantNS  string
		wantNam string
	}{
		{"ns/name flag", "acme/widget", "/tmp/whatever", "acme", "widget"},
		{"bare name flag", "widget", "/tmp/whatever", "", "widget"},
		{"default from dir", "", "/tmp/My Module", "", "my-module"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, name := moduleIdentity(tt.flag, tt.dir)
			if ns != tt.wantNS || name != tt.wantNam {
				t.Errorf("moduleIdentity(%q,%q) = (%q,%q), want (%q,%q)", tt.flag, tt.dir, ns, name, tt.wantNS, tt.wantNam)
			}
		})
	}
}
