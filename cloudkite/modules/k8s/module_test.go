package k8s

import (
	"sort"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/project-starkite/starkite/starbase"
)

func TestModuleName(t *testing.T) {
	m := New()
	if m.Name() != "k8s" {
		t.Errorf("Name() = %q, want %q", m.Name(), "k8s")
	}
}

func TestModuleFactoryMethod(t *testing.T) {
	m := New()
	if m.FactoryMethod() != "config" {
		t.Errorf("FactoryMethod() = %q, want %q", m.FactoryMethod(), "config")
	}
}

func TestModuleLoad(t *testing.T) {
	m := New()
	config := &starbase.ModuleConfig{}

	result, err := m.Load(config)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	k8sModule, ok := result["k8s"]
	if !ok {
		t.Fatal("Load() did not return 'k8s' key")
	}
	if k8sModule == nil {
		t.Fatal("k8s module is nil")
	}
}

func TestModuleLoadIdempotent(t *testing.T) {
	m := New()
	config := &starbase.ModuleConfig{}

	result1, _ := m.Load(config)
	result2, _ := m.Load(config)

	if result1["k8s"] != result2["k8s"] {
		t.Error("Load() should return the same module on subsequent calls")
	}
}

func TestModuleDescription(t *testing.T) {
	m := New()
	desc := m.Description()
	if desc == "" {
		t.Error("Description() returned empty string")
	}
}

func TestModuleAliases(t *testing.T) {
	m := New()
	if m.Aliases() != nil {
		t.Error("Aliases() should return nil")
	}
}

func loadK8sModule(t *testing.T) starlark.HasAttrs {
	t.Helper()
	m := New()
	result, err := m.Load(&starbase.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	return result["k8s"].(starlark.HasAttrs)
}

// TestModuleTryAttr verifies that try_ variants of builtin methods resolve
// to *starlark.Builtin from the loaded module.
func TestModuleTryAttr(t *testing.T) {
	mod := loadK8sModule(t)

	tryMethods := []string{
		"try_get", "try_apply", "try_deploy", "try_list",
		"try_delete", "try_scale", "try_logs", "try_exec",
		"try_config", "try_yaml",
	}

	for _, name := range tryMethods {
		v, err := mod.Attr(name)
		if err != nil {
			t.Errorf("Attr(%q) error: %v", name, err)
			continue
		}
		if v == nil {
			t.Errorf("Attr(%q) returned nil", name)
			continue
		}
		if _, ok := v.(*starlark.Builtin); !ok {
			t.Errorf("Attr(%q) returned %T, want *starlark.Builtin", name, v)
		}
	}
}

// TestModuleTryAttrNames verifies that AttrNames includes try_ variants for
// builtins but NOT for non-builtins like obj (starlarkstruct.Module).
func TestModuleTryAttrNames(t *testing.T) {
	mod := loadK8sModule(t)
	names := mod.AttrNames()

	if !sort.StringsAreSorted(names) {
		t.Fatal("AttrNames() is not sorted")
	}

	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	// obj is a starlarkstruct.Module, NOT a builtin — should not get try_obj
	if nameSet["try_obj"] {
		t.Error("AttrNames() should not contain try_obj (obj is not a builtin)")
	}
	if !nameSet["obj"] {
		t.Error("AttrNames() should contain obj")
	}

	// config and yaml are builtins — should have try_ variants
	for _, name := range []string{"config", "yaml", "get", "apply"} {
		if !nameSet[name] {
			t.Errorf("AttrNames() missing %q", name)
		}
		tryName := "try_" + name
		if !nameSet[tryName] {
			t.Errorf("AttrNames() missing %q", tryName)
		}
	}
}

// TestModuleTryNonexistent verifies that try_ of a nonexistent method returns (nil, nil).
func TestModuleTryNonexistent(t *testing.T) {
	mod := loadK8sModule(t)

	v, err := mod.Attr("try_nonexistent")
	if err != nil {
		t.Fatalf("Attr(\"try_nonexistent\") error: %v", err)
	}
	if v != nil {
		t.Fatalf("Attr(\"try_nonexistent\") = %v, want nil", v)
	}
}

// TestModuleDirectAttrUnchanged verifies that direct method lookups still work:
// get, apply, config return *starlark.Builtin, obj returns *starlarkstruct.Module.
func TestModuleDirectAttrUnchanged(t *testing.T) {
	mod := loadK8sModule(t)

	// Builtins
	for _, name := range []string{"get", "apply", "config", "yaml"} {
		v, err := mod.Attr(name)
		if err != nil {
			t.Errorf("Attr(%q) error: %v", name, err)
			continue
		}
		if v == nil {
			t.Errorf("Attr(%q) returned nil", name)
			continue
		}
		if _, ok := v.(*starlark.Builtin); !ok {
			t.Errorf("Attr(%q) returned %T, want *starlark.Builtin", name, v)
		}
	}

	// obj should be starlarkstruct.Module
	v, err := mod.Attr("obj")
	if err != nil {
		t.Fatalf("Attr(\"obj\") error: %v", err)
	}
	if _, ok := v.(*starlarkstruct.Module); !ok {
		t.Fatalf("Attr(\"obj\") returned %T, want *starlarkstruct.Module", v)
	}
}
