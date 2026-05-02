package loader_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/loader"
)

// newRuntimeWithModules builds a Runtime with the full loader registry.
func newRuntimeWithModules(t *testing.T, perms *libkite.PermissionConfig) *libkite.Runtime {
	t.Helper()
	reg := loader.NewDefaultRegistry(&libkite.ModuleConfig{})
	rt, err := libkite.New(&libkite.Config{
		Registry:    reg,
		Permissions: perms,
	})
	if err != nil {
		t.Fatalf("libkite.New: %v", err)
	}
	t.Cleanup(rt.Close)
	return rt
}

func TestRuntime_Call_UsesStarbaseModule(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	rt := newRuntimeWithModules(t, libkite.AllowAllPermissions())
	if err := rt.ExecuteRepl(context.Background(), `
def check(url):
    return http.url(url).get(timeout="5s").status_code
`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	v, err := rt.Call(context.Background(), "check", nil, map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	got, ok := v.(starlark.Int)
	if !ok {
		t.Fatalf("return type = %T, want starlark.Int", v)
	}
	if n, _ := got.Int64(); n != 200 {
		t.Errorf("got %d, want 200", n)
	}
}

func TestRuntime_Eval_UsesStarbaseModule(t *testing.T) {
	rt := newRuntimeWithModules(t, libkite.AllowAllPermissions())
	v, err := rt.Eval(context.Background(), `exists("/tmp")`)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if _, ok := v.(starlark.Bool); !ok {
		t.Errorf("return type = %T, want starlark.Bool", v)
	}
}

func TestRuntime_Call_RespectsPermissions(t *testing.T) {
	rt := newRuntimeWithModules(t, libkite.StrictPermissions())

	if err := rt.ExecuteRepl(context.Background(), `
def hit():
    return http.url("http://127.0.0.1:1").get(timeout="1s").status_code
`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	_, err := rt.Call(context.Background(), "hit", nil, nil)
	if err == nil {
		t.Fatal("want permission error, got nil")
	}
	if !libkite.IsPermissionError(err) &&
		!strings.Contains(err.Error(), "permission") &&
		!strings.Contains(err.Error(), "denied") {
		t.Errorf("error not a permission error: %v", err)
	}
}
