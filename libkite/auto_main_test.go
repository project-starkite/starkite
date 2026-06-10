package libkite

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

// autoMainRT builds a trusted runtime whose print output is captured in out and
// whose session diagnostics are captured in logBuf (when non-nil).
func autoMainRT(t *testing.T, entryPoint string, out, logBuf *bytes.Buffer) *Runtime {
	t.Helper()
	cfg := &Config{
		EntryPoint: entryPoint,
		Print: func(_ *starlark.Thread, msg string) {
			out.WriteString(msg + "\n")
		},
	}
	if logBuf != nil {
		cfg.Logger = slog.New(slog.NewTextHandler(logBuf, nil))
	}
	rt, err := NewTrusted(cfg)
	if err != nil {
		t.Fatalf("NewTrusted: %v", err)
	}
	t.Cleanup(rt.Close)
	return rt
}

func TestExecute_AutoInvokesMain(t *testing.T) {
	var out bytes.Buffer
	rt := autoMainRT(t, "main", &out, nil)

	err := rt.Execute(context.Background(), `
def main():
    print("MARKER")
`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if n := strings.Count(out.String(), "MARKER"); n != 1 {
		t.Errorf("main() ran %d times, want 1\noutput: %q", n, out.String())
	}
}

func TestExecute_SkipsAndLogsWhenMainCalledExplicitly(t *testing.T) {
	var out, logBuf bytes.Buffer
	rt := autoMainRT(t, "main", &out, &logBuf)

	err := rt.Execute(context.Background(), `
def main():
    print("MARKER")

main()
`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if n := strings.Count(out.String(), "MARKER"); n != 1 {
		t.Errorf("main() ran %d times, want 1 (tolerant skip must avoid double-run)\noutput: %q", n, out.String())
	}
	if !strings.Contains(logBuf.String(), "skipping automatic entry-point invocation") {
		t.Errorf("skip not logged to session log; got: %q", logBuf.String())
	}
}

func TestExecute_NoMain_RunsAsUsual(t *testing.T) {
	var out bytes.Buffer
	rt := autoMainRT(t, "main", &out, nil)

	err := rt.Execute(context.Background(), `print("TOPLEVEL")`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "TOPLEVEL" {
		t.Errorf("output = %q, want %q", got, "TOPLEVEL")
	}
}

func TestExecute_MainNotCallable_NotInvoked(t *testing.T) {
	var out bytes.Buffer
	rt := autoMainRT(t, "main", &out, nil)

	// A non-callable binding named main must not trip auto-invocation.
	err := rt.Execute(context.Background(), `
main = 42
print("TOPLEVEL")
`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "TOPLEVEL" {
		t.Errorf("output = %q, want %q", got, "TOPLEVEL")
	}
}

func TestExecute_EmptyEntryPoint_NoAutoInvoke(t *testing.T) {
	var out bytes.Buffer
	rt := autoMainRT(t, "", &out, nil) // auto-invocation disabled

	err := rt.Execute(context.Background(), `
def main():
    print("MARKER")
`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out.String(), "MARKER") {
		t.Errorf("main() was invoked with EntryPoint disabled; output: %q", out.String())
	}
}

func TestExecute_AutoInvokeMainError_Propagates(t *testing.T) {
	var out bytes.Buffer
	rt := autoMainRT(t, "main", &out, nil)

	err := rt.Execute(context.Background(), `
def main():
    fail("boom")
`)
	if err == nil {
		t.Fatal("expected error from failing main(), got nil")
	}
}

func TestExecute_LoadedModuleMainNotAutoInvoked(t *testing.T) {
	dir := t.TempDir()
	modPath := filepath.Join(dir, "mod.star")
	if err := os.WriteFile(modPath, []byte(`
def main():
    print("MODULE_MAIN")

def helper():
    print("HELPER")
`), 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}
	entryPath := filepath.Join(dir, "entry.star")
	// A single-file module exports a struct under its basename; access members
	// through it (mod.helper), not as bare imported names.
	entrySrc := `
load("./mod.star", "mod")
mod.helper()
`
	if err := os.WriteFile(entryPath, []byte(entrySrc), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}

	var out bytes.Buffer
	cfg := &Config{
		EntryPoint: "main",
		ScriptPath: entryPath,
		Print: func(_ *starlark.Thread, msg string) {
			out.WriteString(msg + "\n")
		},
	}
	rt, err := NewTrusted(cfg)
	if err != nil {
		t.Fatalf("NewTrusted: %v", err)
	}
	t.Cleanup(rt.Close)

	if err := rt.Execute(context.Background(), entrySrc); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "HELPER") {
		t.Errorf("entry did not run helper(); output: %q", out.String())
	}
	if strings.Contains(out.String(), "MODULE_MAIN") {
		t.Errorf("loaded module's main() was auto-invoked; output: %q", out.String())
	}
}
