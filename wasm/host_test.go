package wasm

import (
	"testing"

	"github.com/project-starkite/starkite/starbase"
	"go.starlark.net/starlark"
)

func TestHostContext_Build_AllPermissions(t *testing.T) {
	ctx := &HostContext{
		moduleName: "testmod",
	}
	funcs := ctx.Build([]string{"log", "env", "var", "exec", "read_file", "write_file", "http"})

	// log is always included + 6 explicit permissions
	if len(funcs) != 7 {
		t.Errorf("expected 7 host functions, got %d", len(funcs))
	}
}

func TestHostContext_Build_NoPermissions(t *testing.T) {
	ctx := &HostContext{
		moduleName: "testmod",
	}
	funcs := ctx.Build(nil)

	// Only log (always available)
	if len(funcs) != 1 {
		t.Errorf("expected 1 host function (log), got %d", len(funcs))
	}
}

func TestHostContext_Build_PartialPermissions(t *testing.T) {
	ctx := &HostContext{
		moduleName: "testmod",
	}
	funcs := ctx.Build([]string{"env", "read_file"})

	// log (always) + env + read_file = 3
	if len(funcs) != 3 {
		t.Errorf("expected 3 host functions, got %d", len(funcs))
	}
}

func TestHostContext_CheckPermission_NoThread(t *testing.T) {
	ctx := &HostContext{
		moduleName: "testmod",
		thread:     nil,
	}

	// No thread = trusted mode, should allow
	if err := ctx.checkPermission("env", "PATH"); err != nil {
		t.Errorf("expected nil error with no thread, got: %v", err)
	}
}

func TestHostContext_CheckPermission_WithChecker(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}

	checker, err := starbase.NewPermissionChecker(&starbase.PermissionConfig{
		Deny:    []string{"testmod.exec"},
		Default: starbase.DefaultAllow,
	})
	if err != nil {
		t.Fatalf("failed to create checker: %v", err)
	}
	starbase.SetPermissions(thread, checker)

	ctx := &HostContext{
		moduleName: "testmod",
		thread:     thread,
	}

	// env should be allowed (not denied)
	if err := ctx.checkPermission("env", "PATH"); err != nil {
		t.Errorf("expected env to be allowed, got: %v", err)
	}

	// exec should be denied
	if err := ctx.checkPermission("exec", "ls"); err == nil {
		t.Error("expected exec to be denied")
	}
}
