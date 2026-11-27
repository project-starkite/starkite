package libkite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"go.starlark.net/starlark"
)

func TestSignalNameNormalizationAndRegistration(t *testing.T) {
	rt := newTrustedRT(t)

	fn := starlark.NewBuiltin("sig_cb", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})
	rt.RegisterSignalHandler("SIGINT", fn)
	if !rt.HasSignalHandler("interrupt") || !rt.HasSignalHandler("int") || !rt.HasSignalHandler("SIGINT") {
		t.Fatalf("expected SIGINT/interrupt/int aliases to resolve to registered handler")
	}
	rt.UnregisterSignalHandler("interrupt")
	if rt.HasSignalHandler("SIGINT") {
		t.Fatalf("expected UnregisterSignalHandler to remove normalized entry")
	}
}

func TestExecute_SignalHandlerZeroAndOneArgAndExitCode(t *testing.T) {
	t.Run("zero arg handler via SIGINT", func(t *testing.T) {
		rt := newTrustedRT(t)
		var events []string
		var mu sync.Mutex
		rt.predecl["record_event"] = starlark.NewBuiltin("record_event", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			var s string
			if err := starlark.UnpackPositionalArgs("record_event", args, nil, 1, &s); err != nil {
				return nil, err
			}
			mu.Lock()
			events = append(events, s)
			mu.Unlock()
			return starlark.None, nil
		})
		rt.predecl["trigger_sigint"] = starlark.NewBuiltin("trigger_sigint", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			rt.handleSignal(syscall.SIGINT)
			return starlark.None, nil
		})

		err := rt.Execute(context.Background(), `
def on_int():
    record_event("handler0")

on_signal("SIGINT", on_int)
defer(lambda: record_event("defer"))
trigger_sigint()
`)
		exitErr, ok := errors.AsType[*ExitError](err)
		if !ok || exitErr.Code != ExitInterrupt {
			t.Fatalf("expected ExitError(%d), got %v", ExitInterrupt, err)
		}
		if len(events) != 2 || events[0] != "handler0" || events[1] != "defer" {
			t.Fatalf("unexpected teardown event sequence: %v", events)
		}
	})

	t.Run("one arg handler via SIGTERM", func(t *testing.T) {
		rt := newTrustedRT(t)
		var gotArg string
		rt.predecl["record_sig"] = starlark.NewBuiltin("record_sig", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			var s string
			if err := starlark.UnpackPositionalArgs("record_sig", args, nil, 1, &s); err != nil {
				return nil, err
			}
			gotArg = s
			return starlark.None, nil
		})
		rt.predecl["trigger_sigterm"] = starlark.NewBuiltin("trigger_sigterm", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			rt.handleSignal(syscall.SIGTERM)
			return starlark.None, nil
		})

		err := rt.Execute(context.Background(), `
def on_term(sig):
    record_sig(sig)

on_signal("SIGTERM", on_term)
trigger_sigterm()
`)
		exitErr, ok := errors.AsType[*ExitError](err)
		if !ok || exitErr.Code != ExitTerminate {
			t.Fatalf("expected ExitError(%d), got %v", ExitTerminate, err)
		}
		if gotArg != "SIGTERM" {
			t.Fatalf("expected signal arg %q, got %q", "SIGTERM", gotArg)
		}
	})
}

func TestLoader_SingleFileModuleRuntimeBindingAndCancel(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "mymod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "mod.yaml"), []byte("name: mymod\nversion: 0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "main.star"), []byte("register_mod_cleanup()\nval = 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleaned := false
	rt, err := NewTrusted(&Config{
		ScriptPath: filepath.Join(dir, "main.star"),
		WorkDir:    dir,
		Globals:    map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	rt.predecl["register_mod_cleanup"] = starlark.NewBuiltin("register_mod_cleanup", func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		if GetRuntime(thread) == nil {
			t.Error("expected GetRuntime(moduleThread) != nil during module init")
		}
		RegisterCleanup(thread, func() {
			cleaned = true
		})
		return starlark.None, nil
	})

	if err := rt.Execute(context.Background(), `load("./mymod", "mymod")`); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !cleaned {
		t.Errorf("expected RegisterCleanup called from loaded module to run at Execute teardown")
	}
}

func TestWrappedErrors_And_RegistryLoadAllCachedError(t *testing.T) {
	permErr := &PermissionError{Module: "fs", Category: "read", Function: "read_text", Reason: "denied"}
	wrapped := errors.Join(errors.New("outer"), permErr)
	if !IsPermissionError(wrapped) {
		t.Errorf("IsPermissionError should match wrapped PermissionError")
	}
	if !IsScriptError(errors.Join(errors.New("outer"), &ScriptError{Message: "bad"})) {
		t.Errorf("IsScriptError should match wrapped ScriptError")
	}
	if !IsExitError(errors.Join(errors.New("outer"), &ExitError{Code: 2})) {
		t.Errorf("IsExitError should match wrapped ExitError")
	}
	if !IsSkipError(errors.Join(errors.New("outer"), &SkipError{Reason: "skip"})) {
		t.Errorf("IsSkipError should match wrapped SkipError")
	}

	r := NewRegistry(&ModuleConfig{})
	r.SetStrict(true)
	r.Register(&fakeModule{name: "a", exports: starlark.StringDict{"dup": starlark.None}})
	r.Register(&fakeModule{name: "b", exports: starlark.StringDict{"dup": starlark.None}})
	if _, err1 := r.LoadAll(); err1 == nil {
		t.Fatal("expected initial LoadAll error")
	}
	if _, err2 := r.LoadAll(); err2 == nil {
		t.Fatal("expected subsequent LoadAll calls to return cached error")
	}
}

func TestConcurrentExecuteReplAndModuleLoad_NoRace(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "concurrent_mod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "mod.yaml"), []byte("name: concurrent_mod\nversion: 0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "main.star"), []byte("x = 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := NewTrusted(&Config{
		ScriptPath: filepath.Join(dir, "entry.star"),
		WorkDir:    dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			th := rt.NewThread("concurrent-load")
			if _, loadErr := th.Load(th, "./concurrent_mod"); loadErr != nil {
				t.Errorf("concurrent load failed: %v", loadErr)
			}
			if _, evalErr := rt.Eval(context.Background(), "1 + 1"); evalErr != nil {
				t.Errorf("concurrent Eval failed: %v", evalErr)
			}
		})
	}
	wg.Wait()
}
