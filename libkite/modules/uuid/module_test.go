package uuid

import (
	"regexp"
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"go.starlark.net/starlark"
)

var uuidV4Regex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestModuleMetadata(t *testing.T) {
	m := New()
	if m.Name() != ModuleName {
		t.Fatalf("expected name %s, got %s", ModuleName, m.Name())
	}
	if m.Description() == "" {
		t.Fatal("expected non-empty description")
	}
	if m.FactoryMethod() != "" {
		t.Fatalf("expected empty factory method, got %q", m.FactoryMethod())
	}
	if m.Aliases() != nil {
		t.Fatal("expected nil aliases")
	}
}

func TestUUIDV4FormatAndUniqueness(t *testing.T) {
	m := New()
	_, _ = m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}

	val1, err := m.v4(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("v4() error: %v", err)
	}
	s1, ok := val1.(starlark.String)
	if !ok {
		t.Fatalf("expected starlark.String, got %T", val1)
	}
	if !uuidV4Regex.MatchString(string(s1)) {
		t.Fatalf("UUID %q does not match RFC 4122 v4 format", s1)
	}

	val2, err := m.v4(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("v4() second call error: %v", err)
	}
	s2 := val2.(starlark.String)
	if string(s1) == string(s2) {
		t.Fatalf("two UUID v4 calls produced identical result: %q", s1)
	}
}

func TestUUIDV4ArgRejectionAndTry(t *testing.T) {
	m := New()
	dict, _ := m.Load(&libkite.ModuleConfig{})
	modVal := dict[string(ModuleName)].(starlark.HasAttrs)
	thread := &starlark.Thread{Name: "test"}

	// Direct call with argument should fail
	_, err := m.v4(thread, nil, starlark.Tuple{starlark.String("extra")}, nil)
	if err == nil {
		t.Fatal("v4() with args should fail")
	}

	// try_v4 with args returns Result(ok=False)
	tryVal, err := modVal.Attr("try_v4")
	if err != nil || tryVal == nil {
		t.Fatalf("try_v4 not found on module")
	}
	fn := tryVal.(*starlark.Builtin)

	resVal, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("extra")}, nil)
	if err != nil {
		t.Fatalf("try_v4 should not return Go error: %v", err)
	}
	res := resVal.(*libkite.Result)
	if bool(res.Truth()) {
		t.Fatalf("expected Result(ok=False), got %v", res)
	}

	// try_v4 without args returns Result(ok=True)
	resVal, err = starlark.Call(thread, fn, nil, nil)
	if err != nil {
		t.Fatalf("try_v4 call error: %v", err)
	}
	res = resVal.(*libkite.Result)
	if !bool(res.Truth()) {
		t.Fatalf("expected Result(ok=True), got %v", res)
	}
	v, _ := res.Attr("value")
	if !uuidV4Regex.MatchString(string(v.(starlark.String))) {
		t.Fatalf("try_v4 produced invalid UUID: %v", v)
	}
}
