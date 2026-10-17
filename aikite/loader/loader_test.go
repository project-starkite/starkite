package loader

import (
	"testing"

	"github.com/project-starkite/starkite/libkite"
)

func TestNewAIRegistry(t *testing.T) {
	config := &libkite.ModuleConfig{}
	r := NewAIRegistry(config)
	if r == nil {
		t.Fatal("NewAIRegistry returned nil")
	}

	// AI module
	if _, ok := r.Get("mcp"); !ok {
		t.Fatal("expected 'mcp' module in AI registry")
	}

	// Base modules
	for _, name := range []string{"os", "fs", "http", "json", "yaml", "fmt", "time", "strings"} {
		if _, ok := r.Get(libkite.ModuleName(name)); !ok {
			t.Fatalf("expected base module %q in AI registry", name)
		}
	}
}

func TestRegisterAIModules(t *testing.T) {
	r := libkite.NewRegistry(nil)
	RegisterAIModules(r)
	if _, ok := r.Get("mcp"); !ok {
		t.Fatal("expected 'mcp' module registered via RegisterAIModules")
	}
}
