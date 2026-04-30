package starbase

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

// fakeModule is a minimal Module implementation for registry tests.
type fakeModule struct {
	name    ModuleName
	exports starlark.StringDict
	aliases starlark.StringDict
}

func (f *fakeModule) Name() ModuleName        { return f.name }
func (f *fakeModule) Description() string     { return "fake module for tests" }
func (f *fakeModule) FactoryMethod() string   { return "" }
func (f *fakeModule) Aliases() starlark.StringDict { return f.aliases }
func (f *fakeModule) Load(_ *ModuleConfig) (starlark.StringDict, error) {
	return f.exports, nil
}

func TestRegistry_StrictMode_PanicsOnDuplicateModule(t *testing.T) {
	r := NewRegistry(&ModuleConfig{})
	r.SetStrict(true)
	r.Register(&fakeModule{name: "foo"})

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("expected panic on duplicate module name")
		}
		msg, ok := rec.(error)
		if !ok || !strings.Contains(msg.Error(), `"foo"`) {
			t.Fatalf("expected panic message naming %q, got %v", "foo", rec)
		}
	}()
	r.Register(&fakeModule{name: "foo"})
}

func TestRegistry_StrictMode_ErrorsOnDuplicateExport(t *testing.T) {
	r := NewRegistry(&ModuleConfig{})
	r.SetStrict(true)
	r.Register(&fakeModule{
		name:    "a",
		exports: starlark.StringDict{"shared": starlark.None},
	})
	r.Register(&fakeModule{
		name:    "b",
		exports: starlark.StringDict{"shared": starlark.None},
	})

	_, err := r.LoadAll()
	if err == nil {
		t.Fatal("expected duplicate-export error from LoadAll")
	}
	if !strings.Contains(err.Error(), "duplicate export") || !strings.Contains(err.Error(), `"shared"`) {
		t.Fatalf("expected error mentioning duplicate export of 'shared', got: %v", err)
	}
}

func TestRegistry_StrictMode_ErrorsOnDuplicateAlias(t *testing.T) {
	r := NewRegistry(&ModuleConfig{})
	r.SetStrict(true)
	r.Register(&fakeModule{
		name:    "a",
		aliases: starlark.StringDict{"shared_alias": starlark.None},
	})
	r.Register(&fakeModule{
		name:    "b",
		aliases: starlark.StringDict{"shared_alias": starlark.None},
	})

	_, err := r.LoadAll()
	if err == nil {
		t.Fatal("expected duplicate-alias error from LoadAll")
	}
	if !strings.Contains(err.Error(), "duplicate global alias") || !strings.Contains(err.Error(), `"shared_alias"`) {
		t.Fatalf("expected error mentioning duplicate alias 'shared_alias', got: %v", err)
	}
}

func TestRegistry_LenientMode_NoErrorOnCollision(t *testing.T) {
	// Default mode: collisions silently overwrite — preserves the behavior
	// the lean editions rely on. This test fails if someone makes strict the default.
	r := NewRegistry(&ModuleConfig{})
	r.Register(&fakeModule{
		name:    "foo",
		exports: starlark.StringDict{"shared": starlark.None},
		aliases: starlark.StringDict{"shared_alias": starlark.None},
	})
	// Re-registering the same name in lenient mode does not panic.
	r.Register(&fakeModule{
		name:    "foo",
		exports: starlark.StringDict{"shared": starlark.None},
		aliases: starlark.StringDict{"shared_alias": starlark.None},
	})
	if _, err := r.LoadAll(); err != nil {
		t.Fatalf("LoadAll should not error in lenient mode: %v", err)
	}
}
