package osmod

import (
	"runtime"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func TestBifurcatedProcessExecution(t *testing.T) {
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
			name: "direct exec with list of arguments",
			script: `
def test():
    return os.exec("echo", ["hello", "world"])
`,
			permissions: libkite.AllowAllPermissions(),
			wantResult:  "hello world",
		},
		{
			name: "direct exec fallback whitespace splitting",
			script: `
def test():
    return os.exec("echo hello world")
`,
			permissions: libkite.AllowAllPermissions(),
			wantResult:  "hello world",
		},
		{
			name: "shell execution allowed under allow-all-shell",
			script: `
def test():
    return os.shell("echo hello $VAR", env={"VAR": "world"})
`,
			permissions: libkite.AllowAllShellPermissions(),
			wantResult:  "hello world",
		},
		{
			name: "shell execution denied under allow-all",
			script: `
def test():
    return os.shell("echo hello world")
`,
			permissions: libkite.AllowAllPermissions(),
			wantErr:     "blocked by deny rule: os.shell",
		},
		{
			name: "try_shell execution allowed under allow-all-shell",
			script: `
def test():
    res = os.try_shell("echo hello world")
    return res.stdout
`,
			permissions: libkite.AllowAllShellPermissions(),
			wantResult:  "hello world",
		},
		{
			name: "try_shell execution denied under allow-all",
			script: `
def test():
    return os.try_shell("echo hello world")
`,
			permissions: libkite.AllowAllPermissions(),
			wantErr:     "blocked by deny rule: os.shell",
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
