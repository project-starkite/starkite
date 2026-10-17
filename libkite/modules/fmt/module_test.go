package fmtmod

import (
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"go.starlark.net/starlark"
)

func TestModuleMetadata(t *testing.T) {
	m := New()
	if m.Name() != ModuleName {
		t.Fatalf("expected module name %s, got %s", ModuleName, m.Name())
	}
	if m.Description() == "" {
		t.Fatal("expected non-empty description")
	}
	if m.FactoryMethod() != "" {
		t.Fatalf("expected empty factory method, got %q", m.FactoryMethod())
	}
}

func TestLoadAndAliases(t *testing.T) {
	m := New()
	dict, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	modVal, ok := dict[string(ModuleName)]
	if !ok {
		t.Fatalf("module %s not found in dict", ModuleName)
	}
	hasAttrs, ok := modVal.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("expected HasAttrs, got %T", modVal)
	}

	for _, name := range []string{"printf", "println", "sprintf", "try_printf", "try_println", "try_sprintf"} {
		attr, err := hasAttrs.Attr(name)
		if err != nil || attr == nil {
			t.Fatalf("expected attr %q on module, err: %v", name, err)
		}
	}

	aliases := m.Aliases()
	for _, name := range []string{"printf", "println", "sprintf"} {
		if _, ok := aliases[name]; !ok {
			t.Fatalf("expected alias %q in Aliases()", name)
		}
	}
}

func TestSprintf(t *testing.T) {
	m := New()
	_, _ = m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}

	// Empty args
	val, err := m.sprintf(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("sprintf() empty failed: %v", err)
	}
	if str, ok := val.(starlark.String); !ok || string(str) != "" {
		t.Fatalf("expected empty string, got %v", val)
	}

	// Single format string without verbs
	val, err = m.sprintf(thread, nil, starlark.Tuple{starlark.String("hello world")}, nil)
	if err != nil {
		t.Fatalf("sprintf() single arg failed: %v", err)
	}
	if str, ok := val.(starlark.String); !ok || string(str) != "hello world" {
		t.Fatalf("expected 'hello world', got %v", val)
	}

	// Format with multiple args (%s, %d, %v)
	args := starlark.Tuple{
		starlark.String("name=%s count=%d items=%v"),
		starlark.String("kite"),
		starlark.MakeInt(42),
		starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2)}),
	}
	val, err = m.sprintf(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("sprintf() multi-arg failed: %v", err)
	}
	want := "name=kite count=42 items=[1 2]"
	if str, ok := val.(starlark.String); !ok || string(str) != want {
		t.Fatalf("expected %q, got %q", want, val)
	}

	// Non-string format should return error
	_, err = m.sprintf(thread, nil, starlark.Tuple{starlark.MakeInt(123)}, nil)
	if err == nil {
		t.Fatal("expected error when format is not a string, got nil")
	}
}

func TestPrintfAndPrintln(t *testing.T) {
	m := New()
	_, _ = m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}

	// printf empty
	val, err := m.printf(thread, nil, nil, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("printf empty failed: val=%v, err=%v", val, err)
	}

	// printf formatted
	val, err = m.printf(thread, nil, starlark.Tuple{starlark.String("log: %d\n"), starlark.MakeInt(10)}, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("printf formatted failed: val=%v, err=%v", val, err)
	}

	// println
	val, err = m.println(thread, nil, starlark.Tuple{starlark.String("line"), starlark.MakeInt(1)}, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("println failed: val=%v, err=%v", val, err)
	}
}

func TestTrySprintf(t *testing.T) {
	m := New()
	dict, _ := m.Load(&libkite.ModuleConfig{})
	modVal := dict[string(ModuleName)].(starlark.HasAttrs)
	thread := &starlark.Thread{Name: "test"}

	trySprintfVal, err := modVal.Attr("try_sprintf")
	if err != nil || trySprintfVal == nil {
		t.Fatalf("try_sprintf not found: %v", err)
	}
	fn := trySprintfVal.(*starlark.Builtin)

	// Valid invocation -> Result(ok=True, value=...)
	resVal, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("num=%d"), starlark.MakeInt(7)}, nil)
	if err != nil {
		t.Fatalf("try_sprintf call error: %v", err)
	}
	res, ok := resVal.(*libkite.Result)
	if !ok || !bool(res.Truth()) {
		t.Fatalf("expected Result(ok=True), got %v", resVal)
	}

	// Invalid invocation (non-string format) -> Result(ok=False, error=...)
	resVal, err = starlark.Call(thread, fn, starlark.Tuple{starlark.MakeInt(999)}, nil)
	if err != nil {
		t.Fatalf("try_sprintf should not return Go error: %v", err)
	}
	res, ok = resVal.(*libkite.Result)
	if !ok || bool(res.Truth()) {
		t.Fatalf("expected Result(ok=False), got %v", resVal)
	}
	if res.ErrorString() == "" {
		t.Fatal("expected non-empty error string in Result")
	}
}
