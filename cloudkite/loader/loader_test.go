package loader

import (
	"testing"

	"github.com/project-starkite/starkite/libkite"
)

func TestNewCloudRegistry(t *testing.T) {
	config := &libkite.ModuleConfig{}
	r := NewCloudRegistry(config)
	if r == nil {
		t.Fatal("NewCloudRegistry returned nil")
	}

	// Cloud module
	if _, ok := r.Get("k8s"); !ok {
		t.Fatal("expected 'k8s' module in cloud registry")
	}

	// Base modules
	for _, name := range []string{"os", "fs", "http", "json", "yaml", "fmt", "time", "strings"} {
		if _, ok := r.Get(libkite.ModuleName(name)); !ok {
			t.Fatalf("expected base module %q in cloud registry", name)
		}
	}
}

func TestRegisterCloudModules(t *testing.T) {
	r := libkite.NewRegistry(nil)
	RegisterCloudModules(r)
	if _, ok := r.Get("k8s"); !ok {
		t.Fatal("expected 'k8s' module registered via RegisterCloudModules")
	}
}
