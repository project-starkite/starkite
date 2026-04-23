package mcp

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

func TestLoad_RegistersBuiltins(t *testing.T) {
	m := New()
	globals, err := m.Load(&starbase.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	mcpMod, ok := globals["mcp"]
	if !ok {
		t.Fatalf("expected 'mcp' in globals")
	}
	for _, name := range []string{"serve", "connect"} {
		val, err := mcpMod.(starlark.HasAttrs).Attr(name)
		if err != nil || val == nil {
			t.Errorf("mcp.%s missing: err=%v val=%v", name, err, val)
		}
	}
}

func TestServe_RejectsPositional(t *testing.T) {
	m := New()
	globals, _ := m.Load(&starbase.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star", `mcp.serve("x")`, globals)
	if err == nil || !strings.Contains(err.Error(), "keyword") {
		t.Errorf("expected kwargs-only error, got %v", err)
	}
}

func TestServe_RequiresName(t *testing.T) {
	m := New()
	globals, _ := m.Load(&starbase.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star", `mcp.serve()`, globals)
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected name-required error, got %v", err)
	}
}
