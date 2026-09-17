package osmod

import (
	"runtime"
	"strings"
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

func TestShellExecution_ExecAndTryExec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell test skipped on windows")
	}

	m, thread := newTestOSModule(t)

	val, err := m.shell(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("os.shell() failed: %v", err)
	}
	sh := val.(*Shell)

	// 1. Basic exec
	execAttr, err := sh.Attr("exec")
	if err != nil {
		t.Fatalf("Attr('exec') failed: %v", err)
	}
	execFn := execAttr.(*starlark.Builtin)

	outVal, err := execFn.CallInternal(thread, starlark.Tuple{starlark.String("echo 'hello from shell'")}, nil)
	if err != nil {
		t.Fatalf("sh.exec() failed: %v", err)
	}
	if outStr := outVal.(starlark.String).GoString(); outStr != "hello from shell\n" {
		t.Errorf("sh.exec output = %q, expected 'hello from shell\\n'", outStr)
	}

	// 2. Pipelines and redirection
	outVal, err = execFn.CallInternal(thread, starlark.Tuple{starlark.String("printf 'alpha\nbeta\ngamma' | grep beta")}, nil)
	if err != nil {
		t.Fatalf("sh.exec() pipeline failed: %v", err)
	}
	if outStr := outVal.(starlark.String).GoString(); outStr != "beta\n" {
		t.Errorf("sh.exec pipeline output = %q, expected 'beta\\n'", outStr)
	}

	// 3. Non-zero exit in exec raises error
	_, err = execFn.CallInternal(thread, starlark.Tuple{starlark.String("exit 7")}, nil)
	if err == nil {
		t.Errorf("expected error on non-zero exit")
	}

	// 4. Basic try_exec success
	tryExecAttr, err := sh.Attr("try_exec")
	if err != nil {
		t.Fatalf("Attr('try_exec') failed: %v", err)
	}
	tryExecFn := tryExecAttr.(*starlark.Builtin)

	resVal, err := tryExecFn.CallInternal(thread, starlark.Tuple{starlark.String("echo success")}, nil)
	if err != nil {
		t.Fatalf("sh.try_exec() returned Go error: %v", err)
	}
	res := resVal.(*ExecResult)
	if !res.isOK() || res.exitCode != 0 {
		t.Errorf("try_exec expected ok=true, code=0, got %v", res)
	}
	if res.stdout != "success\n" {
		t.Errorf("try_exec stdout = %q, expected 'success\\n'", res.stdout)
	}

	// 5. try_exec on non-zero exit captures code and ok=False without error
	resVal, err = tryExecFn.CallInternal(thread, starlark.Tuple{starlark.String("echo 'failed message' >&2; exit 42")}, nil)
	if err != nil {
		t.Fatalf("sh.try_exec() should not return Go error on non-zero exit: %v", err)
	}
	res = resVal.(*ExecResult)
	if res.isOK() {
		t.Errorf("try_exec on exit 42 should have ok=False")
	}
	if res.exitCode != 42 {
		t.Errorf("try_exec expected exitCode=42, got %d", res.exitCode)
	}
	if res.stderr != "failed message\n" {
		t.Errorf("try_exec stderr = %q, expected 'failed message\\n'", res.stderr)
	}
}

func TestShellExecution_EnvAndCwdOverrides(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell test skipped on windows")
	}

	m, thread := newTestOSModule(t)

	boundEnv := starlark.NewDict(2)
	boundEnv.SetKey(starlark.String("VAR1"), starlark.String("bound1"))
	boundEnv.SetKey(starlark.String("VAR2"), starlark.String("bound2"))

	val, err := m.shell(thread, nil, nil, []starlark.Tuple{
		{starlark.String("cwd"), starlark.String("/tmp")},
		{starlark.String("env"), boundEnv},
	})
	if err != nil {
		t.Fatalf("os.shell() failed: %v", err)
	}
	sh := val.(*Shell)

	execAttr, _ := sh.Attr("exec")
	execFn := execAttr.(*starlark.Builtin)

	// 1. Inherits bound env and cwd
	outVal, err := execFn.CallInternal(thread, starlark.Tuple{starlark.String("echo $VAR1 $VAR2 $(pwd)")}, nil)
	if err != nil {
		t.Fatalf("sh.exec() failed: %v", err)
	}
	outStr := outVal.(starlark.String).GoString()
	if !strings.HasPrefix(outStr, "bound1 bound2") {
		t.Errorf("expected output to start with 'bound1 bound2', got %q", outStr)
	}

	// 2. Per-call overrides merge into and override bound options
	callEnv := starlark.NewDict(2)
	callEnv.SetKey(starlark.String("VAR2"), starlark.String("overridden2"))
	callEnv.SetKey(starlark.String("VAR3"), starlark.String("call3"))

	outVal, err = execFn.CallInternal(thread, starlark.Tuple{starlark.String("echo $VAR1 $VAR2 $VAR3")}, []starlark.Tuple{
		{starlark.String("env"), callEnv},
	})
	if err != nil {
		t.Fatalf("sh.exec() with call env failed: %v", err)
	}
	if outStr = outVal.(starlark.String).GoString(); outStr != "bound1 overridden2 call3\n" {
		t.Errorf("expected 'bound1 overridden2 call3\\n', got %q", outStr)
	}
}

func TestShellExecution_Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell test skipped on windows")
	}

	// Create runtime with explicit deny on os.exec
	rt, err := libkite.New(&libkite.Config{
		Permissions: libkite.AllowPermissions("os.exec"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	m := New()
	if _, err := m.Load(nil); err != nil {
		t.Fatal(err)
	}
	thread := rt.NewThread("test-deny-thread")

	val, err := m.shell(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("os.shell() construction should not fail even when exec denied: %v", err)
	}
	sh := val.(*Shell)

	execAttr, _ := sh.Attr("exec")
	execFn := execAttr.(*starlark.Builtin)

	// Calling exec must be denied by the permission checker
	_, err = execFn.CallInternal(thread, starlark.Tuple{starlark.String("echo denied")}, nil)
	if err == nil {
		t.Fatalf("expected permission denial on sh.exec()")
	}
	if !strings.Contains(err.Error(), "blocked by deny rule: os.exec") {
		t.Errorf("expected 'blocked by deny rule: os.exec', got %v", err)
	}
}

func TestShellFactory_Presets(t *testing.T) {
	m, thread := newTestOSModule(t)

	tests := []struct {
		name         string
		factory      func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error)
		expectedCmd  string
		expectedFlag string
	}{
		{"sh", m.sh, "/bin/sh", "-c"},
		{"bash", m.bash, "/bin/bash", "-c"},
		{"zsh", m.zsh, "/bin/zsh", "-c"},
		{"cmdexe", m.cmdexe, "cmd.exe", "/c"},
		{"powershell", m.powershell, resolvePowerShellCommand(), "-Command"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := starlark.NewBuiltin("os."+tt.name, tt.factory)
			val, err := tt.factory(thread, b, nil, nil)
			if err != nil {
				t.Fatalf("%s() failed: %v", tt.name, err)
			}
			sh, ok := val.(*Shell)
			if !ok {
				t.Fatalf("expected *Shell, got %T", val)
			}
			if sh.command != tt.expectedCmd {
				t.Errorf("%s.command = %q; expected %q", tt.name, sh.command, tt.expectedCmd)
			}
			if sh.flag != tt.expectedFlag {
				t.Errorf("%s.flag = %q; expected %q", tt.name, sh.flag, tt.expectedFlag)
			}
		})
	}
}

func TestShellFactory_OptionsAndOverrides(t *testing.T) {
	m, thread := newTestOSModule(t)

	envDict := starlark.NewDict(1)
	envDict.SetKey(starlark.String("ENV_VAR"), starlark.String("test"))

	kwargs := []starlark.Tuple{
		{starlark.String("cwd"), starlark.String("/custom/dir")},
		{starlark.String("timeout"), starlark.String("15s")},
		{starlark.String("env"), envDict},
		{starlark.String("flag"), starlark.String("-l")},
	}

	b := starlark.NewBuiltin("os.bash", m.bash)
	val, err := m.bash(thread, b, nil, kwargs)
	if err != nil {
		t.Fatalf("os.bash() with kwargs failed: %v", err)
	}

	sh := val.(*Shell)
	if sh.command != "/bin/bash" {
		t.Errorf("expected command '/bin/bash', got %q", sh.command)
	}
	if sh.flag != "-l" {
		t.Errorf("expected overridden flag '-l', got %q", sh.flag)
	}
	if sh.cwd != "/custom/dir" {
		t.Errorf("expected cwd '/custom/dir', got %q", sh.cwd)
	}
	if sh.timeout != 15*time.Second {
		t.Errorf("expected timeout 15s, got %v", sh.timeout)
	}
	if sh.env["ENV_VAR"] != "test" {
		t.Errorf("expected env ENV_VAR=test, got %v", sh.env)
	}
}

func TestShellFactory_PositionalArgsRejected(t *testing.T) {
	m, thread := newTestOSModule(t)

	b := starlark.NewBuiltin("os.sh", m.sh)
	_, err := m.sh(thread, b, starlark.Tuple{starlark.String("echo hello")}, nil)
	if err == nil {
		t.Fatal("expected error when passing positional arguments to factory shortcut")
	}
	if !strings.Contains(err.Error(), "takes no positional arguments") {
		t.Errorf("expected 'takes no positional arguments' error, got: %v", err)
	}
}

func TestShellFactory_ModuleRegistration(t *testing.T) {
	m, _ := newTestOSModule(t)

	hasAttrs, ok := m.module.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("expected m.module to implement starlark.HasAttrs, got %T", m.module)
	}

	shortcuts := []string{"shell", "sh", "bash", "zsh", "cmdexe", "powershell"}
	for _, name := range shortcuts {
		val, err := hasAttrs.Attr(name)
		if err != nil || val == nil {
			t.Errorf("module.Attr(%q) not found or err: %v", name, err)
		}
		tryVal, err := hasAttrs.Attr("try_" + name)
		if err != nil || tryVal == nil {
			t.Errorf("module.Attr(try_%s) not found or err: %v", name, err)
		}
		if _, ok := m.aliases[name]; !ok {
			t.Errorf("alias %q not registered in module aliases", name)
		}
	}
}
