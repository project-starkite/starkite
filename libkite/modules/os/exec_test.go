package osmod

import (
	"bytes"
	"io"
	"runtime"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	iomod "github.com/project-starkite/starkite/libkite/modules/io"
)

func TestDirectProcessExecution(t *testing.T) {
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

type trackingReader struct {
	io.Reader
	closed bool
}

func (tr *trackingReader) Close() error {
	tr.closed = true
	return nil
}

type trackingWriter struct {
	io.Writer
	closed bool
}

func (tw *trackingWriter) Close() error {
	tw.closed = true
	return nil
}

func TestProcessExecutionStreaming(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on windows")
	}

	type testContext struct {
		reader *trackingReader
		writer *trackingWriter
		buf    *bytes.Buffer
	}

	tests := []struct {
		name        string
		script      string
		permissions *libkite.PermissionConfig
		setup       func() (starlark.Tuple, *testContext)
		wantResult  string
		wantErr     string
		verify      func(t *testing.T, ctx *testContext)
	}{
		{
			name: "exec with string input",
			script: `
def test():
    return os.exec("cat", input="hello from string")
`,
			permissions: libkite.AllowAllPermissions(),
			wantResult:  "hello from string",
		},
		{
			name: "exec with bytes input",
			script: `
def test():
    return os.exec("cat", input=bytes("hello from bytes"))
`,
			permissions: libkite.AllowAllPermissions(),
			wantResult:  "hello from bytes",
		},
		{
			name: "exec with reader input and writer output",
			script: `
def test(r, w):
    res = os.exec("cat", input=r, output=w)
    return res
`,
			permissions: libkite.AllowAllPermissions(),
			setup: func() (starlark.Tuple, *testContext) {
				tr := &trackingReader{Reader: strings.NewReader("hello from stream")}
				var buf bytes.Buffer
				tw := &trackingWriter{Writer: &buf}
				rVal := iomod.NewReader(tr, "test_input")
				wVal := iomod.NewWriter(tw, "test_output")
				return starlark.Tuple{rVal, wVal}, &testContext{reader: tr, writer: tw, buf: &buf}
			},
			wantResult: "",
			verify: func(t *testing.T, ctx *testContext) {
				if got := ctx.buf.String(); got != "hello from stream" {
					t.Errorf("writer output = %q, want %q", got, "hello from stream")
				}
				if !ctx.reader.closed {
					t.Error("expected reader to be automatically closed")
				}
				if !ctx.writer.closed {
					t.Error("expected writer to be automatically closed")
				}
			},
		},
		{
			name: "try_exec with reader input and writer output",
			script: `
def test(r, w):
    res = os.try_exec("cat", input=r, output=w)
    if not res.ok:
        return "error: " + res.error
    return "ok"
`,
			permissions: libkite.AllowAllPermissions(),
			setup: func() (starlark.Tuple, *testContext) {
				tr := &trackingReader{Reader: strings.NewReader("try_exec stream")}
				var buf bytes.Buffer
				tw := &trackingWriter{Writer: &buf}
				rVal := iomod.NewReader(tr, "test_input")
				wVal := iomod.NewWriter(tw, "test_output")
				return starlark.Tuple{rVal, wVal}, &testContext{reader: tr, writer: tw, buf: &buf}
			},
			wantResult: "ok",
			verify: func(t *testing.T, ctx *testContext) {
				if got := ctx.buf.String(); got != "try_exec stream" {
					t.Errorf("writer output = %q, want %q", got, "try_exec stream")
				}
				if !ctx.reader.closed {
					t.Error("expected reader to be automatically closed")
				}
				if !ctx.writer.closed {
					t.Error("expected writer to be automatically closed")
				}
			},
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

			var args starlark.Tuple
			var ctx *testContext
			if tc.setup != nil {
				args, ctx = tc.setup()
			}

			res, err := starlark.Call(thread, testFn, args, nil)
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

				if tc.verify != nil {
					tc.verify(t, ctx)
				}
			}
		})
	}
}
