package osmod

import (
	"runtime"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func newTestOSModule(t *testing.T) (*Module, *starlark.Thread) {
	t.Helper()
	rt, err := libkite.New(&libkite.Config{
		Permissions: libkite.AllowAllPermissions(),
	})
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}
	t.Cleanup(func() { rt.Close() })

	m := New()
	_, err = m.Load(nil)
	if err != nil {
		t.Fatalf("failed to load os module: %v", err)
	}
	thread := rt.NewThread("test-thread")
	return m, thread
}

func TestShellConstructor_Defaults(t *testing.T) {
	m, thread := newTestOSModule(t)

	val, err := m.shell(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("os.shell() failed: %v", err)
	}

	sh, ok := val.(*Shell)
	if !ok {
		t.Fatalf("expected *Shell, got %T", val)
	}

	if runtime.GOOS == "windows" {
		if sh.flag != "/c" {
			t.Errorf("expected default flag '/c' on Windows, got %q", sh.flag)
		}
	} else {
		if sh.command != "/bin/sh" {
			t.Errorf("expected default command '/bin/sh', got %q", sh.command)
		}
		if sh.flag != "-c" {
			t.Errorf("expected default flag '-c', got %q", sh.flag)
		}
	}

	if sh.timeout != 60*time.Second {
		t.Errorf("expected default timeout 60s, got %v", sh.timeout)
	}
	if sh.cwd != "" {
		t.Errorf("expected empty default cwd, got %q", sh.cwd)
	}

	// Test Attr access
	cmdAttr, err := sh.Attr("command")
	if err != nil || cmdAttr.(starlark.String) != starlark.String(sh.command) {
		t.Errorf("Attr('command') = %v, %v; expected %s", cmdAttr, err, sh.command)
	}

	flagAttr, err := sh.Attr("flag")
	if err != nil || flagAttr.(starlark.String) != starlark.String(sh.flag) {
		t.Errorf("Attr('flag') = %v, %v; expected %s", flagAttr, err, sh.flag)
	}

	timeoutAttr, err := sh.Attr("timeout")
	if err != nil || timeoutAttr.(starlark.String) != starlark.String("60s") {
		t.Errorf("Attr('timeout') = %v, %v; expected 60s", timeoutAttr, err)
	}

	cwdAttr, err := sh.Attr("cwd")
	if err != nil || cwdAttr.(starlark.String) != starlark.String("") {
		t.Errorf("Attr('cwd') = %v, %v; expected empty string", cwdAttr, err)
	}
}

func TestShellConstructor_AutoDetectFlag(t *testing.T) {
	m, thread := newTestOSModule(t)

	tests := []struct {
		cmd          string
		expectedFlag string
	}{
		{"/bin/bash", "-c"},
		{"/bin/zsh", "-c"},
		{"cmd.exe", "/c"},
		{"C:\\Windows\\System32\\cmd.exe", "/c"},
		{"pwsh", "-Command"},
		{"powershell.exe", "-Command"},
		{"/usr/local/bin/pwsh", "-Command"},
	}

	for _, tt := range tests {
		val, err := m.shell(thread, nil, starlark.Tuple{starlark.String(tt.cmd)}, nil)
		if err != nil {
			t.Fatalf("os.shell(%q) failed: %v", tt.cmd, err)
		}
		sh := val.(*Shell)
		if sh.flag != tt.expectedFlag {
			t.Errorf("os.shell(%q).flag = %q; expected %q", tt.cmd, sh.flag, tt.expectedFlag)
		}
	}
}

func TestShellConstructor_CustomOptions(t *testing.T) {
	m, thread := newTestOSModule(t)

	envDict := starlark.NewDict(2)
	envDict.SetKey(starlark.String("FOO"), starlark.String("bar"))
	envDict.SetKey(starlark.String("KEY"), starlark.String("val"))

	kwargs := []starlark.Tuple{
		{starlark.String("command"), starlark.String("/bin/bash")},
		{starlark.String("flag"), starlark.String("-l")},
		{starlark.String("cwd"), starlark.String("/tmp")},
		{starlark.String("timeout"), starlark.String("30s")},
		{starlark.String("env"), envDict},
	}

	val, err := m.shell(thread, nil, nil, kwargs)
	if err != nil {
		t.Fatalf("os.shell() with options failed: %v", err)
	}

	sh := val.(*Shell)
	if sh.command != "/bin/bash" {
		t.Errorf("command = %q; expected '/bin/bash'", sh.command)
	}
	if sh.flag != "-l" {
		t.Errorf("flag = %q; expected '-l'", sh.flag)
	}
	if sh.cwd != "/tmp" {
		t.Errorf("cwd = %q; expected '/tmp'", sh.cwd)
	}
	if sh.timeout != 30*time.Second {
		t.Errorf("timeout = %v; expected 30s", sh.timeout)
	}
	if sh.env["FOO"] != "bar" || sh.env["KEY"] != "val" {
		t.Errorf("env = %v; expected FOO=bar, KEY=val", sh.env)
	}
}

func TestShellConstructor_Errors(t *testing.T) {
	m, thread := newTestOSModule(t)

	// Too many positional arguments
	_, err := m.shell(thread, nil, starlark.Tuple{starlark.String("a"), starlark.String("b")}, nil)
	if err == nil {
		t.Errorf("expected error for multiple positional args")
	}

	// Duplicate command
	_, err = m.shell(thread, nil, starlark.Tuple{starlark.String("sh")}, []starlark.Tuple{
		{starlark.String("command"), starlark.String("bash")},
	})
	if err == nil {
		t.Errorf("expected error for duplicate command argument")
	}

	// Invalid timeout
	_, err = m.shell(thread, nil, nil, []starlark.Tuple{
		{starlark.String("timeout"), starlark.String("invalid")},
	})
	if err == nil {
		t.Errorf("expected error for invalid timeout string")
	}

	// Non-dict env
	_, err = m.shell(thread, nil, nil, []starlark.Tuple{
		{starlark.String("env"), starlark.String("not-a-dict")},
	})
	if err == nil {
		t.Errorf("expected error for non-dict env")
	}

	// Unknown keyword argument
	_, err = m.shell(thread, nil, nil, []starlark.Tuple{
		{starlark.String("bogus"), starlark.String("val")},
	})
	if err == nil {
		t.Errorf("expected error for unknown keyword argument")
	}
}
