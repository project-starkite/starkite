package runtime

import (
	goruntime "runtime"
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"go.starlark.net/starlark"
)

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
		t.Fatal("expected nil aliases for runtime module")
	}
}

func TestRuntimeFunctions(t *testing.T) {
	m := New()
	dict, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	modVal := dict[string(ModuleName)]
	thread := &starlark.Thread{Name: "test"}

	// platform
	val, err := m.platform(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("platform() error: %v", err)
	}
	if s, ok := val.(starlark.String); !ok || string(s) != goruntime.GOOS {
		t.Fatalf("platform() = %v, want %s", val, goruntime.GOOS)
	}

	// arch
	val, err = m.arch(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("arch() error: %v", err)
	}
	if s, ok := val.(starlark.String); !ok || string(s) != goruntime.GOARCH {
		t.Fatalf("arch() = %v, want %s", val, goruntime.GOARCH)
	}

	// cpu_count
	val, err = m.cpuCount(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("cpuCount() error: %v", err)
	}
	if n, ok := val.(starlark.Int); !ok {
		t.Fatalf("expected Int, got %T", val)
	} else {
		num, _ := n.Int64()
		if num < 1 {
			t.Fatalf("cpuCount() = %d, want >= 1", num)
		}
	}

	// version
	val, err = m.version(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("version() error: %v", err)
	}
	if s, ok := val.(starlark.String); !ok || string(s) != Version {
		t.Fatalf("version() = %v, want %s", val, Version)
	}

	// go_version
	val, err = m.goVersion(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("goVersion() error: %v", err)
	}
	if s, ok := val.(starlark.String); !ok || string(s) != goruntime.Version() {
		t.Fatalf("goVersion() = %v, want %s", val, goruntime.Version())
	}

	// uname
	val, err = m.uname(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("uname() error: %v", err)
	}
	if dictVal, ok := val.(*starlark.Dict); !ok || dictVal.Len() == 0 {
		t.Fatalf("uname() returned invalid dict: %v", val)
	}

	// Verify try_ members are registered on module
	hasAttrs := modVal.(starlark.HasAttrs)
	for _, fn := range []string{"platform", "arch", "cpu_count", "uname", "version", "go_version"} {
		attr, err := hasAttrs.Attr("try_" + fn)
		if err != nil || attr == nil {
			t.Fatalf("expected try_%s on module", fn)
		}
	}
}

func TestRuntimeArgRejectionAndTry(t *testing.T) {
	m := New()
	dict, _ := m.Load(&libkite.ModuleConfig{})
	modVal := dict[string(ModuleName)].(starlark.HasAttrs)
	thread := &starlark.Thread{Name: "test"}

	// Direct call with unexpected args should fail
	_, err := m.platform(thread, nil, starlark.Tuple{starlark.String("extra")}, nil)
	if err == nil {
		t.Fatal("expected error when passing args to platform()")
	}

	// try_platform with extra args should return Result(ok=False)
	tryPlatVal, _ := modVal.Attr("try_platform")
	fn := tryPlatVal.(*starlark.Builtin)
	resVal, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("extra")}, nil)
	if err != nil {
		t.Fatalf("try_platform should not return Go error: %v", err)
	}
	res, ok := resVal.(*libkite.Result)
	if !ok || bool(res.Truth()) {
		t.Fatalf("expected Result(ok=False), got %v", resVal)
	}

	// try_platform with no args should return Result(ok=True)
	resVal, err = starlark.Call(thread, fn, nil, nil)
	if err != nil {
		t.Fatalf("try_platform call error: %v", err)
	}
	res, ok = resVal.(*libkite.Result)
	if !ok || !bool(res.Truth()) {
		t.Fatalf("expected Result(ok=True), got %v", resVal)
	}
}
