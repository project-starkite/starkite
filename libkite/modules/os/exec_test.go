package osmod

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func TestExecProcessExecution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on windows")
	}

	tests := []struct {
		name        string
		script      string
		permissions *libkite.PermissionConfig
		wantErr     string
		wantResult  string
	}{
		{
			name: "exec allowed under allow-all",
			script: `
def test():
    return os.exec("echo hello")
`,
			permissions: libkite.AllowAllPermissions(),
			wantResult:  "hello",
		},
		{
			name: "exec denied under explicit deny",
			script: `
def test():
    return os.exec("echo hello")
`,
			permissions: libkite.AllowPermissions("os.exec"),
			wantErr:     "blocked by deny rule: os.exec",
		},
		{
			name: "exec single string with double quotes",
			script: `
def test():
    return os.exec('echo "hello world"')
`,
			permissions: libkite.AllowAllPermissions(),
			wantResult:  "hello world",
		},
		{
			name: "exec single string with single quotes",
			script: `
def test():
    return os.exec("echo '{\"k\": \"v\"}'")
`,
			permissions: libkite.AllowAllPermissions(),
			wantResult:  `{"k": "v"}`,
		},
		{
			name: "exec single string with escaped spaces",
			script: `
def test():
    return os.exec("echo hello\\ world")
`,
			permissions: libkite.AllowAllPermissions(),
			wantResult:  "hello world",
		},
		{
			name: "exec with explicit argument list",
			script: `
def test():
    return os.exec("echo", ["hello", "world"])
`,
			permissions: libkite.AllowAllPermissions(),
			wantResult:  "hello world",
		},
		{
			name: "try_exec allowed under allow-all",
			script: `
def test():
    res = os.try_exec("echo hello")
    return res.stdout
`,
			permissions: libkite.AllowAllPermissions(),
			wantResult:  "hello",
		},
		{
			name: "try_exec denied under explicit deny",
			script: `
def test():
    return os.try_exec("echo hello")
`,
			permissions: libkite.AllowPermissions("os.exec"),
			wantErr:     "blocked by deny rule: os.exec",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := libkite.New(&libkite.Config{
				Permissions: tc.permissions,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Close()

			osMod := New()
			dict, err := osMod.Load(nil)
			if err != nil {
				t.Fatal(err)
			}

			predeclared := starlark.StringDict{
				"os": dict["os"],
			}

			thread := rt.NewThread("test-thread")
			globals, err := starlark.ExecFile(thread, "test.star", tc.script, predeclared)
			if err != nil {
				t.Fatal(err)
			}

			testFn, ok := globals["test"]
			if !ok {
				t.Fatal("test function not found")
			}

			res, err := starlark.Call(thread, testFn, nil, nil)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("got error %q, want it to contain %q", err.Error(), tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				gotStr, ok := starlark.AsString(res)
				if !ok {
					t.Fatalf("expected string result, got %s", res.Type())
				}
				got := strings.TrimSpace(gotStr)
				want := strings.TrimSpace(tc.wantResult)
				if got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			}
		})
	}
}

func TestIsolatedEnvReadWrite(t *testing.T) {
	t.Setenv("HOST_SECRET_CANARY", "leaked_secret")

	mod := NewWithConfig(&libkite.ModuleConfig{
		Env: map[string]string{
			"ALLOWED_VAR": "safe_value",
		},
	})
	_, err := mod.Load(&libkite.ModuleConfig{
		Env: map[string]string{
			"ALLOWED_VAR": "safe_value",
		},
	})
	if err != nil {
		t.Fatalf("failed to load module: %v", err)
	}

	thread := &starlark.Thread{Name: "test"}
	aliases := mod.Aliases()
	envFn := aliases["env"].(starlark.Callable)
	setenvFn := aliases["setenv"].(starlark.Callable)

	// 1. Host canary must not be leaked
	canaryVal, err := starlark.Call(thread, envFn, starlark.Tuple{starlark.String("HOST_SECRET_CANARY")}, nil)
	if err != nil {
		t.Fatalf("unexpected error calling os.env: %v", err)
	}
	if canaryVal.(starlark.String) != "" {
		t.Errorf("expected empty string for scrubbed host var, got %q", canaryVal)
	}

	// 2. Allowed var is accessible
	allowedVal, err := starlark.Call(thread, envFn, starlark.Tuple{starlark.String("ALLOWED_VAR")}, nil)
	if err != nil {
		t.Fatalf("unexpected error calling os.env: %v", err)
	}
	if allowedVal.(starlark.String) != "safe_value" {
		t.Errorf("expected safe_value, got %q", allowedVal)
	}

	// 3. os.setenv updates module table without mutating host process
	_, err = starlark.Call(thread, setenvFn, starlark.Tuple{starlark.String("NEW_VAR"), starlark.String("runtime_val")}, nil)
	if err != nil {
		t.Fatalf("unexpected error calling os.setenv: %v", err)
	}

	newVal, err := starlark.Call(thread, envFn, starlark.Tuple{starlark.String("NEW_VAR")}, nil)
	if err != nil {
		t.Fatalf("unexpected error calling os.env: %v", err)
	}
	if newVal.(starlark.String) != "runtime_val" {
		t.Errorf("expected runtime_val, got %q", newVal)
	}

	// Host process environment must NOT be polluted
	if hostVal := os.Getenv("NEW_VAR"); hostVal != "" {
		t.Errorf("expected host process env to not contain NEW_VAR, got %q", hostVal)
	}
}

func TestIsolatedEnvSubprocess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on windows")
	}

	t.Setenv("HOST_SECRET_CANARY", "leaked_secret")

	thread := &starlark.Thread{Name: "test"}
	checker, err := libkite.NewPermissionChecker(libkite.AllowAllPermissions())
	if err != nil {
		t.Fatalf("failed to create checker: %v", err)
	}
	libkite.SetPermissions(thread, checker)

	mod := NewWithConfig(&libkite.ModuleConfig{
		Env: map[string]string{
			"ALLOWED_VAR": "subprocess_safe",
			"PATH":        os.Getenv("PATH"),
		},
	})
	_, err = mod.Load(&libkite.ModuleConfig{
		Env: map[string]string{
			"ALLOWED_VAR": "subprocess_safe",
			"PATH":        os.Getenv("PATH"),
		},
	})
	if err != nil {
		t.Fatalf("failed to load module: %v", err)
	}

	execFn := mod.Aliases()["exec"].(starlark.Callable)
	res, err := starlark.Call(thread, execFn, starlark.Tuple{starlark.String("env")}, nil)
	if err != nil {
		t.Fatalf("os.exec failed: %v", err)
	}

	output := res.(starlark.String).GoString()

	if !strings.Contains(output, "ALLOWED_VAR=subprocess_safe") {
		t.Errorf("expected output to contain ALLOWED_VAR=subprocess_safe, got:\n%s", output)
	}
	if strings.Contains(output, "HOST_SECRET_CANARY=leaked_secret") {
		t.Errorf("detected host canary leak in child process environment:\n%s", output)
	}
}
