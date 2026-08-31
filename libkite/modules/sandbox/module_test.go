package sandbox

import (
	"runtime"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func newTestThread() *starlark.Thread {
	reg := libkite.NewRegistry(&libkite.ModuleConfig{})
	reg.Register(New())
	rt, err := libkite.NewTrusted(nil, libkite.WithRegistry(reg))
	if err != nil {
		panic(err)
	}
	return rt.NewThread("test")
}

func TestSandboxModule_LoadAndMetadata(t *testing.T) {
	m := New()
	if m.Name() != ModuleName {
		t.Errorf("m.Name() = %s, want %s", m.Name(), ModuleName)
	}

	dict, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	modVal, ok := dict[string(ModuleName)]
	if !ok {
		t.Fatalf("module %q not found in dict", ModuleName)
	}

	hasAttrs, ok := modVal.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("module value does not implement HasAttrs: %T", modVal)
	}

	for _, attr := range []string{"config", "run_script", "list_drivers", "default_driver"} {
		v, err := hasAttrs.Attr(attr)
		if err != nil || v == nil {
			t.Errorf("missing module member %q", attr)
		}
	}
}

func TestSandboxModule_ListAndDefaultDrivers(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sandbox execution drivers only available on linux")
	}
	m := New()
	dict, _ := m.Load(&libkite.ModuleConfig{})
	modVal := dict[string(ModuleName)].(starlark.HasAttrs)

	thread := newTestThread()

	// 1. list_drivers
	listFn, _ := modVal.Attr("list_drivers")
	listRes, err := starlark.Call(thread, listFn.(starlark.Callable), nil, nil)
	if err != nil {
		t.Fatalf("list_drivers() error: %v", err)
	}
	listVal, ok := listRes.(*starlark.List)
	if !ok {
		t.Fatalf("list_drivers() expected *starlark.List, got %T", listRes)
	}
	if listVal.Len() == 0 {
		t.Error("list_drivers() returned empty list")
	}

	// 2. default_driver
	defFn, _ := modVal.Attr("default_driver")
	defRes, err := starlark.Call(thread, defFn.(starlark.Callable), nil, nil)
	if err != nil {
		t.Fatalf("default_driver() error: %v", err)
	}
	defStr, ok := starlark.AsString(defRes)
	if !ok || defStr == "" {
		t.Errorf("default_driver() returned invalid string: %v", defRes)
	}
	t.Logf("Detected default driver: %s", defStr)
}

func TestSandboxModule_ConfigAndBoxExec(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sandbox execution drivers only available on linux")
	}
	m := New()
	dict, _ := m.Load(&libkite.ModuleConfig{})
	modVal := dict[string(ModuleName)].(starlark.HasAttrs)
	thread := newTestThread()

	configFn, _ := modVal.Attr("config")

	// Call sandbox.config(driver="default", memory="512MB", network="none")
	kwargs := []starlark.Tuple{
		{starlark.String("driver"), starlark.String("default")},
		{starlark.String("memory"), starlark.String("512MB")},
		{starlark.String("network"), starlark.String("none")},
		{starlark.String("mounts"), starlark.NewList([]starlark.Value{
			starlark.String("/tmp:/tmp:ro"),
		})},
	}

	boxVal, err := starlark.Call(thread, configFn.(starlark.Callable), nil, kwargs)
	if err != nil {
		t.Fatalf("sandbox.config() error: %v", err)
	}

	box, ok := boxVal.(*Sandbox)
	if !ok {
		t.Fatalf("sandbox.config() expected *Sandbox, got %T", boxVal)
	}

	if box.maxMemoryMB != 512 {
		t.Errorf("box.maxMemoryMB = %d, want 512", box.maxMemoryMB)
	}

	// Execute command via box.exec("echo", ["starlark-sandbox-test"])
	execFn, err := box.Attr("exec")
	if err != nil || execFn == nil {
		t.Fatalf("box.Attr('exec') error: %v", err)
	}

	execArgs := starlark.Tuple{
		starlark.String("echo"),
		starlark.NewList([]starlark.Value{starlark.String("starlark-sandbox-test")}),
	}

	resVal, err := starlark.Call(thread, execFn.(starlark.Callable), execArgs, nil)
	if err != nil {
		t.Fatalf("box.exec() error: %v", err)
	}

	res, ok := resVal.(*BoxExecResult)
	if !ok {
		t.Fatalf("box.exec() expected *BoxExecResult, got %T", resVal)
	}

	if !res.isOK() {
		if res.exitCode == 125 && (strings.Contains(res.errMsg, "no matching manifest for windows") || runtime.GOOS == "windows") {
			t.Skipf("sandbox execution skipped: host container driver cannot run Linux images on Windows: %s", res.errMsg)
		}
		t.Errorf("box.exec() failed: exit_code=%d, error=%s", res.exitCode, res.errMsg)
	}

	if !strings.Contains(res.stdout, "starlark-sandbox-test") {
		t.Errorf("stdout = %q, want 'starlark-sandbox-test'", res.stdout)
	}
}

func TestParseMemory(t *testing.T) {
	tests := []struct {
		input starlark.Value
		want  int64
	}{
		{starlark.MakeInt(256), 256},
		{starlark.String("512MB"), 512},
		{starlark.String("512m"), 512},
		{starlark.String("1GB"), 1024},
		{starlark.String("2G"), 2048},
		{starlark.String("100"), 100},
	}

	for _, tt := range tests {
		got, err := parseMemory(tt.input)
		if err != nil {
			t.Errorf("parseMemory(%v) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseMemory(%v) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
