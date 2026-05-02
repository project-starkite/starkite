package modules_test

import (
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/modules/fs"
	"github.com/project-starkite/starkite/libkite/modules/http"
	osmod "github.com/project-starkite/starkite/libkite/modules/os"
)

// loadFSPath returns a fs.path() builtin and a Starlark thread with the given
// permission rules applied.
func loadFSPath(t *testing.T, allow, deny []string, def libkite.PermissionDefault) (*starlark.Thread, *starlark.Builtin) {
	t.Helper()
	thread := &starlark.Thread{Name: "test"}
	checker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   allow,
		Deny:    deny,
		Default: def,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker: %v", err)
	}
	libkite.SetPermissions(thread, checker)

	mod := &fs.Module{}
	exports, err := mod.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("fs.Load: %v", err)
	}
	pathFn, _ := exports["fs"].(starlark.HasAttrs).Attr("path")
	return thread, pathFn.(*starlark.Builtin)
}

func mkPath(t *testing.T, thread *starlark.Thread, pathFn *starlark.Builtin, p string) starlark.HasAttrs {
	t.Helper()
	v, err := pathFn.CallInternal(thread, starlark.Tuple{starlark.String(p)}, nil)
	if err != nil {
		t.Fatalf("fs.path(%q): %v", p, err)
	}
	return v.(starlark.HasAttrs)
}

func callMethod(t *testing.T, thread *starlark.Thread, obj starlark.HasAttrs, name string, args ...starlark.Value) error {
	t.Helper()
	fn, err := obj.Attr(name)
	if err != nil || fn == nil {
		t.Fatalf("method %s not found: %v", name, err)
	}
	_, err = fn.(*starlark.Builtin).CallInternal(thread, starlark.Tuple(args), nil)
	return err
}

// TestCategoryRule_FSReadAllowsAllReadFunctions: a category rule grants all
// read-class functions but not writes.
func TestCategoryRule_FSReadAllowsAllReadFunctions(t *testing.T) {
	thread, pathFn := loadFSPath(t, []string{"fs.read"}, nil, libkite.DefaultDeny)
	p := mkPath(t, thread, pathFn, "/tmp/permissions-test")

	// Read-class methods should not produce permission errors (the underlying
	// I/O may still fail since the file may not exist).
	for _, fn := range []string{"exists", "is_file", "is_dir"} {
		err := callMethod(t, thread, p, fn)
		if libkite.IsPermissionError(err) {
			t.Errorf("fs.read should allow %s, got permission error: %v", fn, err)
		}
	}

	// A write-class method should be blocked.
	err := callMethod(t, thread, p, "write_text", starlark.String("data"))
	if !libkite.IsPermissionError(err) {
		t.Errorf("fs.read should NOT allow write_text; got: %v", err)
	}
}

// TestCategoryRule_FunctionListNarrows: when a function list is given, only
// the listed functions in that category are allowed.
func TestCategoryRule_FunctionListNarrows(t *testing.T) {
	thread, pathFn := loadFSPath(t, []string{"fs.read(exists:*)"}, nil, libkite.DefaultDeny)
	p := mkPath(t, thread, pathFn, "/tmp/permissions-test")

	// exists is allowed.
	if err := callMethod(t, thread, p, "exists"); libkite.IsPermissionError(err) {
		t.Errorf("exists should be allowed, got: %v", err)
	}

	// read_text is in the same category but not in the function list.
	if err := callMethod(t, thread, p, "read_text"); !libkite.IsPermissionError(err) {
		t.Errorf("read_text should be denied (not in function list), got: %v", err)
	}
}

// TestCategoryRule_ResourceScopesPath: a $CWD-scoped rule allows operations
// inside the cwd and denies them outside.
func TestCategoryRule_ResourceScopesPath(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	thread, pathFn := loadFSPath(t, []string{"fs.read($CWD/**)"}, nil, libkite.DefaultDeny)

	insidePath := filepath.Join(cwd, "subdir/some_file")
	pIn := mkPath(t, thread, pathFn, insidePath)
	if err := callMethod(t, thread, pIn, "exists"); libkite.IsPermissionError(err) {
		t.Errorf("exists on path inside $CWD should be allowed, got: %v", err)
	}

	pOut := mkPath(t, thread, pathFn, "/etc/passwd")
	if err := callMethod(t, thread, pOut, "exists"); !libkite.IsPermissionError(err) {
		t.Errorf("exists on /etc/passwd should be denied (outside $CWD), got: %v", err)
	}
}

// TestCategoryRule_DenyPrecedence: deny rules take precedence over broader
// category-level allows. Note path.read_text() calls Check with function
// name "read_file" (Starlark method name and internal Check arg differ).
func TestCategoryRule_DenyPrecedence(t *testing.T) {
	thread, pathFn := loadFSPath(t,
		[]string{"fs.read"},
		[]string{"fs.read(read_file:*)"},
		libkite.DefaultDeny,
	)
	p := mkPath(t, thread, pathFn, "/tmp/permissions-test")

	// exists (in category, not in deny list) → allowed (file may not exist;
	// only checking permission outcome).
	if err := callMethod(t, thread, p, "exists"); libkite.IsPermissionError(err) {
		t.Errorf("exists should be allowed, got: %v", err)
	}

	// read_text (in category, function read_file in deny list) → blocked.
	if err := callMethod(t, thread, p, "read_text"); !libkite.IsPermissionError(err) {
		t.Errorf("read_text should be denied, got: %v", err)
	}
}

// TestCategoryRule_MultipleCategoriesSameModule: independent category rules
// for the same module compose correctly. Uses Check() directly (rather than
// invoking exec-class Starlark builtins) to avoid an unrelated nil-deref in
// os.runCmd when the test environment lacks a configured timeout.
func TestCategoryRule_MultipleCategoriesSameModule(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}
	checker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   []string{"os.env", "os.exec"},
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker: %v", err)
	}
	libkite.SetPermissions(thread, checker)

	// env category: env + setenv should both pass.
	if err := libkite.Check(thread, "os", "env", "env", "HOME"); err != nil {
		t.Errorf("os.env.env should be allowed: %v", err)
	}
	if err := libkite.Check(thread, "os", "env", "setenv", "X"); err != nil {
		t.Errorf("os.env.setenv should be allowed (env category granted): %v", err)
	}

	// exec category: exec + which should both pass.
	if err := libkite.Check(thread, "os", "exec", "exec", "ls"); err != nil {
		t.Errorf("os.exec.exec should be allowed: %v", err)
	}
	if err := libkite.Check(thread, "os", "exec", "which", "ls"); err != nil {
		t.Errorf("os.exec.which should be allowed (exec category granted): %v", err)
	}

	// process category: chdir not granted → blocked.
	if err := libkite.Check(thread, "os", "process", "chdir", "/tmp"); err == nil {
		t.Error("os.process.chdir should be blocked (process category not granted)")
	}

	// Also verify through the Starlark layer for env (which doesn't fork).
	mod := &osmod.Module{}
	mod.Load(&libkite.ModuleConfig{})
	envFn := mod.Aliases()["env"].(*starlark.Builtin)
	if _, err := envFn.CallInternal(thread, starlark.Tuple{starlark.String("HOME")}, nil); libkite.IsPermissionError(err) {
		t.Errorf("os.env via Starlark should be allowed: %v", err)
	}
}

// TestCategoryRule_HTTPClientVsServer: http.client and http.server are
// distinct categories that can be granted independently.
func TestCategoryRule_HTTPClientVsServer(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}
	checker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   []string{"http.client"},
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker: %v", err)
	}
	libkite.SetPermissions(thread, checker)

	mod := &http.Module{}
	exports, err := mod.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("http.Load: %v", err)
	}
	httpVal := exports["http"].(starlark.HasAttrs)

	// http.config (client category) → allowed
	configFn, _ := httpVal.Attr("config")
	if _, err := configFn.(*starlark.Builtin).CallInternal(thread, starlark.Tuple{}, nil); libkite.IsPermissionError(err) {
		t.Errorf("http.config should be allowed under http.client: %v", err)
	}

	// http.server (server category) → blocked
	serverFn, _ := httpVal.Attr("server")
	_, err = serverFn.(*starlark.Builtin).CallInternal(thread, starlark.Tuple{}, nil)
	if !libkite.IsPermissionError(err) {
		t.Errorf("http.server should be denied (server category not granted): %v", err)
	}
}

// TestCategoryRule_TrustedAllowsEverything: the trusted profile leaves no
// gated operation blocked across modules.
func TestCategoryRule_TrustedAllowsEverything(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}
	checker, _ := libkite.NewPermissionChecker(libkite.TrustedPermissions())
	libkite.SetPermissions(thread, checker)

	for _, c := range []struct{ mod, cat, fn string }{
		{"fs", "read", "read_file"},
		{"fs", "write", "write"},
		{"fs", "delete", "remove"},
		{"os", "exec", "exec"},
		{"http", "client", "get"},
		{"http", "server", "serve"},
		{"k8s", "read", "read"},
		{"ai", "generate", "generate"},
	} {
		if err := libkite.Check(thread, c.mod, c.cat, c.fn, "/anywhere"); err != nil {
			t.Errorf("trusted should allow %s.%s.%s: %v", c.mod, c.cat, c.fn, err)
		}
	}
}

// TestCategoryRule_StrictBlocksEverything: the strict profile blocks every
// gated category in every gated module.
func TestCategoryRule_StrictBlocksEverything(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}
	checker, _ := libkite.NewPermissionChecker(libkite.StrictPermissions())
	libkite.SetPermissions(thread, checker)

	for _, c := range []struct{ mod, cat, fn string }{
		{"fs", "read", "read_file"},
		{"fs", "write", "write"},
		{"fs", "delete", "remove"},
		{"os", "exec", "exec"},
		{"os", "env", "env"},
		{"os", "process", "chdir"},
		{"http", "client", "get"},
		{"http", "server", "serve"},
		{"ssh", "connect", "exec"},
		{"ssh", "transfer", "upload"},
		{"k8s", "read", "read"},
		{"k8s", "write", "write"},
		{"k8s", "exec", "exec"},
		{"k8s", "config", "config"},
		{"ai", "generate", "generate"},
		{"mcp", "client", "connect"},
		{"mcp", "server", "serve"},
		{"io", "prompt", "prompt"},
	} {
		if err := libkite.Check(thread, c.mod, c.cat, c.fn, ""); err == nil {
			t.Errorf("strict should block %s.%s.%s", c.mod, c.cat, c.fn)
		}
	}
}
