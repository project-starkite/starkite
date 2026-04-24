package modules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
	b64 "github.com/project-starkite/starkite/starbase/modules/base64"
	csvmod "github.com/project-starkite/starkite/starbase/modules/csv"
	"github.com/project-starkite/starkite/starbase/modules/fs"
	"github.com/project-starkite/starkite/starbase/modules/http"
	iomod "github.com/project-starkite/starkite/starbase/modules/io"
	jsonmod "github.com/project-starkite/starkite/starbase/modules/json"
	osmod "github.com/project-starkite/starkite/starbase/modules/os"
	"github.com/project-starkite/starkite/starbase/modules/ssh"
	yamlmod "github.com/project-starkite/starkite/starbase/modules/yaml"
)

// =============================================================================
// Attr dispatch tests — verify try_ methods are accessible as *starlark.Builtin
// =============================================================================

func loadModule(t *testing.T, mod interface {
	Load(*starbase.ModuleConfig) (starlark.StringDict, error)
}, key string) starlark.HasAttrs {
	t.Helper()
	exports, err := mod.Load(&starbase.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	val, ok := exports[key]
	if !ok {
		t.Fatalf("Load() missing key %q", key)
	}
	ha, ok := val.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("module %q is %T, want starlark.HasAttrs", key, val)
	}
	return ha
}

func assertTryAttrs(t *testing.T, mod starlark.HasAttrs, moduleName string, methods []string) {
	t.Helper()
	for _, name := range methods {
		tryName := "try_" + name
		v, err := mod.Attr(tryName)
		if err != nil {
			t.Errorf("%s.Attr(%q) error: %v", moduleName, tryName, err)
			continue
		}
		if v == nil {
			t.Errorf("%s.Attr(%q) returned nil", moduleName, tryName)
			continue
		}
		if _, ok := v.(*starlark.Builtin); !ok {
			t.Errorf("%s.Attr(%q) returned %T, want *starlark.Builtin", moduleName, tryName, v)
		}
	}
}

func TestFSTryAttr(t *testing.T) {
	mod := loadModule(t, &fs.Module{}, "fs")
	thread := trustedThread()

	// Get path factory and create a Path object
	pathFn, err := mod.Attr("path")
	if err != nil || pathFn == nil {
		t.Fatalf("fs.path not found: err=%v", err)
	}
	pathObj, err := starlark.Call(thread, pathFn, starlark.Tuple{starlark.String("/tmp")}, nil)
	if err != nil {
		t.Fatalf("fs.path('/tmp') error: %v", err)
	}
	pathHA := pathObj.(starlark.HasAttrs)

	// Verify try_ variants exist on Path object
	assertTryAttrs(t, pathHA, "fs.path", []string{
		"read_text", "write_text", "exists", "mkdir", "remove",
		"glob", "touch", "listdir",
	})
}

func TestOSTryAttr(t *testing.T) {
	mod := loadModule(t, osmod.New(), "os")
	assertTryAttrs(t, mod, "os", []string{
		"exec", "env", "setenv", "cwd", "chdir",
		"hostname", "which", "username",
	})
}

func TestHTTPTryAttr(t *testing.T) {
	mod := loadModule(t, http.New(), "http")
	assertTryAttrs(t, mod, "http", []string{
		"url", "config", "server", "serve",
	})
}

func TestIOTryAttr(t *testing.T) {
	mod := loadModule(t, iomod.New(), "io")
	assertTryAttrs(t, mod, "io", []string{
		"confirm", "prompt",
	})
}

func TestSSHTryAttr(t *testing.T) {
	mod := loadModule(t, ssh.New(), "ssh")
	assertTryAttrs(t, mod, "ssh", []string{
		"config",
	})
}

// =============================================================================
// Execution tests — call try_ functions with a thread, verify Result wrapping
// =============================================================================

func trustedThread() *starlark.Thread {
	thread := &starlark.Thread{Name: "test"}
	// No permissions set = trusted mode (nil checker)
	return thread
}

func TestFSTryReadSuccess(t *testing.T) {
	mod := loadModule(t, &fs.Module{}, "fs")
	thread := trustedThread()

	// Create a Path object for /etc/hostname
	pathFn, err := mod.Attr("path")
	if err != nil || pathFn == nil {
		t.Fatalf("fs.path not found: err=%v", err)
	}
	pathObj, err := starlark.Call(thread, pathFn, starlark.Tuple{starlark.String("/etc/hosts")}, nil)
	if err != nil {
		t.Fatalf("fs.path('/etc/hosts') error: %v", err)
	}
	pathHA := pathObj.(starlark.HasAttrs)

	fn, err := pathHA.Attr("try_read_text")
	if err != nil || fn == nil {
		t.Fatalf("try_read_text not found on Path: err=%v", err)
	}

	// Read /etc/hosts — should exist on Linux and macOS
	val, err := starlark.Call(thread, fn, nil, nil)
	if err != nil {
		t.Fatalf("try_read_text call error: %v", err)
	}

	r, ok := val.(*starbase.Result)
	if !ok {
		t.Fatalf("expected *Result, got %T", val)
	}

	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.True {
		errAttr, _ := r.Attr("error")
		t.Fatalf("expected ok=True, got ok=False error=%v", errAttr)
	}

	value, _ := r.Attr("value")
	s, ok := value.(starlark.String)
	if !ok || len(string(s)) == 0 {
		t.Fatalf("expected non-empty string value, got %v", value)
	}
}

func TestFSTryReadFailure(t *testing.T) {
	mod := loadModule(t, &fs.Module{}, "fs")
	thread := trustedThread()

	// Create a Path object for a nonexistent file
	pathFn, err := mod.Attr("path")
	if err != nil || pathFn == nil {
		t.Fatalf("fs.path not found: err=%v", err)
	}
	pathObj, err := starlark.Call(thread, pathFn, starlark.Tuple{starlark.String("/nonexistent/xyz")}, nil)
	if err != nil {
		t.Fatalf("fs.path('/nonexistent/xyz') error: %v", err)
	}
	pathHA := pathObj.(starlark.HasAttrs)

	fn, err := pathHA.Attr("try_read_text")
	if err != nil || fn == nil {
		t.Fatalf("try_read_text not found on Path: err=%v", err)
	}

	val, err := starlark.Call(thread, fn, nil, nil)
	if err != nil {
		t.Fatalf("try_read_text should not return Go error, got: %v", err)
	}

	r, ok := val.(*starbase.Result)
	if !ok {
		t.Fatalf("expected *Result, got %T", val)
	}

	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.False {
		t.Fatal("expected ok=False for nonexistent file")
	}

	errAttr, _ := r.Attr("error")
	errStr := string(errAttr.(starlark.String))
	if !strings.Contains(errStr, "no such file") {
		t.Fatalf("error = %q, want to contain 'no such file'", errStr)
	}
}

func TestOSTryExecSuccess(t *testing.T) {
	mod := loadModule(t, osmod.New(), "os")
	thread := trustedThread()

	fn, err := mod.Attr("try_exec")
	if err != nil || fn == nil {
		t.Fatalf("try_exec not found: err=%v", err)
	}

	val, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("echo hello")}, nil)
	if err != nil {
		t.Fatalf("try_exec call error: %v", err)
	}

	// try_exec returns ExecResult directly (flat, not wrapped in Result)
	r, ok := val.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("expected HasAttrs, got %T", val)
	}

	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.True {
		errAttr, _ := r.Attr("error")
		t.Fatalf("expected ok=True, got ok=False error=%v", errAttr)
	}

	// ExecResult has stdout directly (no .value nesting)
	stdout, _ := r.Attr("stdout")
	if stdout == nil {
		t.Fatal("expected non-nil stdout")
	}
	if !strings.Contains(string(stdout.(starlark.String)), "hello") {
		t.Fatalf("stdout = %q, expected to contain 'hello'", stdout)
	}
}

func TestTryWithPermissionsDenied(t *testing.T) {
	mod := loadModule(t, &fs.Module{}, "fs")

	// Create a sandboxed thread that denies fs operations
	thread := &starlark.Thread{Name: "test"}
	checker, err := starbase.NewPermissionChecker(&starbase.PermissionConfig{
		Allow:   []string{"strings.*"},
		Default: starbase.DefaultDeny,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker error: %v", err)
	}
	starbase.SetPermissions(thread, checker)

	// Create a Path object and get try_read_text from it
	pathFn, err := mod.Attr("path")
	if err != nil || pathFn == nil {
		t.Fatalf("fs.path not found: err=%v", err)
	}
	pathObj, err := starlark.Call(thread, pathFn, starlark.Tuple{starlark.String("/etc/hostname")}, nil)
	if err != nil {
		t.Fatalf("fs.path('/etc/hostname') error: %v", err)
	}
	pathHA := pathObj.(starlark.HasAttrs)

	fn, err := pathHA.Attr("try_read_text")
	if err != nil || fn == nil {
		t.Fatalf("try_read_text not found on Path: err=%v", err)
	}

	// Call try_read_text with denied permissions — should get Result(ok=False) not a Go error
	val, err := starlark.Call(thread, fn, nil, nil)
	if err != nil {
		t.Fatalf("try_ should never propagate Go error, got: %v", err)
	}

	r, ok := val.(*starbase.Result)
	if !ok {
		t.Fatalf("expected *Result, got %T", val)
	}

	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.False {
		t.Fatal("expected ok=False when permissions deny access")
	}

	errAttr, _ := r.Attr("error")
	errStr := string(errAttr.(starlark.String))
	if !strings.Contains(errStr, "permission") {
		t.Fatalf("error = %q, want to contain 'permission'", errStr)
	}
}

func TestFSTryMkdirAndRemove(t *testing.T) {
	mod := loadModule(t, &fs.Module{}, "fs")
	thread := trustedThread()

	// Create a temp dir path
	tmpDir := os.TempDir() + "/starkite_try_test_dir"

	// Clean up in case of previous failed run
	os.Remove(tmpDir)

	// Get path factory
	pathFn, err := mod.Attr("path")
	if err != nil || pathFn == nil {
		t.Fatalf("fs.path not found: err=%v", err)
	}

	// Create Path object for tmpDir
	pathObj, err := starlark.Call(thread, pathFn, starlark.Tuple{starlark.String(tmpDir)}, nil)
	if err != nil {
		t.Fatalf("fs.path('%s') error: %v", tmpDir, err)
	}
	pathHA := pathObj.(starlark.HasAttrs)

	// try_mkdir
	mkdirFn, _ := pathHA.Attr("try_mkdir")
	val, err := starlark.Call(thread, mkdirFn, nil, nil)
	if err != nil {
		t.Fatalf("try_mkdir call error: %v", err)
	}
	r := val.(*starbase.Result)
	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.True {
		errAttr, _ := r.Attr("error")
		t.Fatalf("try_mkdir expected ok=True, got error=%v", errAttr)
	}

	// Verify dir exists
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Fatal("directory was not created")
	}

	// try_remove
	removeFn, _ := pathHA.Attr("try_remove")
	val, err = starlark.Call(thread, removeFn, nil, nil)
	if err != nil {
		t.Fatalf("try_remove call error: %v", err)
	}
	r = val.(*starbase.Result)
	okAttr, _ = r.Attr("ok")
	if okAttr != starlark.True {
		errAttr, _ := r.Attr("error")
		t.Fatalf("try_remove expected ok=True, got error=%v", errAttr)
	}

	// Verify dir removed
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Fatal("directory was not removed")
	}
}

// =============================================================================
// base64 module try_ tests (factory-based: try_file, try_text, try_bytes)
// =============================================================================

func TestBase64TryAttr(t *testing.T) {
	mod := loadModule(t, b64.New(), "base64")
	assertTryAttrs(t, mod, "base64", []string{
		"file", "text", "bytes",
	})
}

func TestBase64TryTextSuccess(t *testing.T) {
	mod := loadModule(t, b64.New(), "base64")
	thread := trustedThread()

	fn, err := mod.Attr("try_text")
	if err != nil || fn == nil {
		t.Fatalf("try_text not found: err=%v", err)
	}

	val, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("hello")}, nil)
	if err != nil {
		t.Fatalf("try_text call error: %v", err)
	}

	r, ok := val.(*starbase.Result)
	if !ok {
		t.Fatalf("expected *Result, got %T", val)
	}

	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.True {
		errAttr, _ := r.Attr("error")
		t.Fatalf("expected ok=True, got error=%v", errAttr)
	}

	// Value should be a base64.source object
	value, _ := r.Attr("value")
	ha, ok := value.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("expected HasAttrs (base64.source), got %T", value)
	}

	// Verify the source can encode
	encodeFn, err := ha.Attr("encode")
	if err != nil || encodeFn == nil {
		t.Fatalf("source missing encode method: err=%v", err)
	}
	encoded, err := starlark.Call(thread, encodeFn, nil, nil)
	if err != nil {
		t.Fatalf("encode call error: %v", err)
	}
	if string(encoded.(starlark.String)) != "aGVsbG8=" {
		t.Fatalf("expected 'aGVsbG8=', got %v", encoded)
	}
}

func TestBase64TryBytesSuccess(t *testing.T) {
	mod := loadModule(t, b64.New(), "base64")
	thread := trustedThread()

	fn, _ := mod.Attr("try_bytes")
	val, err := starlark.Call(thread, fn, starlark.Tuple{starlark.Bytes("hello")}, nil)
	if err != nil {
		t.Fatalf("try_bytes with bytes error: %v", err)
	}

	r := val.(*starbase.Result)
	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.True {
		errAttr, _ := r.Attr("error")
		t.Fatalf("expected ok=True, got error=%v", errAttr)
	}

	// Value should be a base64.source object
	value, _ := r.Attr("value")
	ha, ok := value.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("expected HasAttrs (base64.source), got %T", value)
	}

	// Verify the source can encode
	encodeFn, _ := ha.Attr("encode")
	encoded, err := starlark.Call(thread, encodeFn, nil, nil)
	if err != nil {
		t.Fatalf("encode call error: %v", err)
	}
	if string(encoded.(starlark.String)) != "aGVsbG8=" {
		t.Fatalf("expected 'aGVsbG8=', got %v", encoded)
	}
}

func TestBase64TryFileSuccess(t *testing.T) {
	mod := loadModule(t, b64.New(), "base64")
	thread := trustedThread()

	fn, _ := mod.Attr("try_file")
	val, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("/tmp/test.txt")}, nil)
	if err != nil {
		t.Fatalf("try_file call error: %v", err)
	}

	r := val.(*starbase.Result)
	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.True {
		errAttr, _ := r.Attr("error")
		t.Fatalf("expected ok=True, got error=%v", errAttr)
	}

	// Value should be a base64.file object
	value, _ := r.Attr("value")
	if value.Type() != "base64.file" {
		t.Fatalf("expected base64.file, got %s", value.Type())
	}
}

// =============================================================================
// csv module try_ tests (factory-based: try_file, try_source)
// =============================================================================

func TestCSVTryAttr(t *testing.T) {
	mod := loadModule(t, csvmod.New(), "csv")
	assertTryAttrs(t, mod, "csv", []string{
		"file", "source",
	})
}

func TestCSVTryFileSuccess(t *testing.T) {
	mod := loadModule(t, csvmod.New(), "csv")
	thread := trustedThread()

	fn, _ := mod.Attr("try_file")
	val, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("/tmp/test.csv")}, nil)
	if err != nil {
		t.Fatalf("try_file call error: %v", err)
	}

	r := val.(*starbase.Result)
	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.True {
		errAttr, _ := r.Attr("error")
		t.Fatalf("expected ok=True, got error=%v", errAttr)
	}

	value, _ := r.Attr("value")
	if value.Type() != "csv.file" {
		t.Fatalf("expected csv.file, got %s", value.Type())
	}
}

func TestCSVTrySourceSuccess(t *testing.T) {
	mod := loadModule(t, csvmod.New(), "csv")
	thread := trustedThread()

	fn, _ := mod.Attr("try_source")
	list := starlark.NewList([]starlark.Value{
		starlark.NewList([]starlark.Value{starlark.String("a"), starlark.String("b")}),
	})
	val, err := starlark.Call(thread, fn, starlark.Tuple{list}, nil)
	if err != nil {
		t.Fatalf("try_source call error: %v", err)
	}

	r := val.(*starbase.Result)
	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.True {
		errAttr, _ := r.Attr("error")
		t.Fatalf("expected ok=True, got error=%v", errAttr)
	}

	value, _ := r.Attr("value")
	if value.Type() != "csv.writer" {
		t.Fatalf("expected csv.writer, got %s", value.Type())
	}
}

func TestCSVTrySourceBadType(t *testing.T) {
	mod := loadModule(t, csvmod.New(), "csv")
	thread := trustedThread()

	fn, _ := mod.Attr("try_source")
	val, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("not a list")}, nil)
	if err != nil {
		t.Fatalf("try_source should not return Go error, got: %v", err)
	}

	r := val.(*starbase.Result)
	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.False {
		t.Fatal("expected ok=False for non-list input")
	}

	errAttr, _ := r.Attr("error")
	errStr := string(errAttr.(starlark.String))
	if !strings.Contains(errStr, "list") {
		t.Fatalf("error = %q, want to mention 'list'", errStr)
	}
}

func TestCSVTryFileReadSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	os.WriteFile(path, []byte("a,b\n1,2\n"), 0644)

	mod := loadModule(t, csvmod.New(), "csv")
	thread := trustedThread()

	// Get csv.file object
	fileFn, _ := mod.Attr("file")
	fileVal, err := starlark.Call(thread, fileFn, starlark.Tuple{starlark.String(path)}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Call read on the file object
	csvFile := fileVal.(starlark.HasAttrs)
	readFn, _ := csvFile.Attr("try_read")
	val, err := starlark.Call(thread, readFn, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	r := val.(*starbase.Result)
	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.True {
		errAttr, _ := r.Attr("error")
		t.Fatalf("expected ok=True, got error=%v", errAttr)
	}

	value, _ := r.Attr("value")
	list := value.(*starlark.List)
	if list.Len() != 2 {
		t.Fatalf("expected 2 rows, got %d", list.Len())
	}
}

// =============================================================================
// yaml module try_ tests (factory-based: try_file, try_source)
// =============================================================================

func TestYAMLTryAttr(t *testing.T) {
	mod := loadModule(t, yamlmod.New(), "yaml")
	assertTryAttrs(t, mod, "yaml", []string{
		"file", "source",
	})
}

func TestYAMLTryFileDecodeSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	os.WriteFile(path, []byte("key: value\n"), 0644)

	mod := loadModule(t, yamlmod.New(), "yaml")
	thread := trustedThread()

	fileFn, _ := mod.Attr("file")
	fileVal, err := starlark.Call(thread, fileFn, starlark.Tuple{starlark.String(path)}, nil)
	if err != nil {
		t.Fatal(err)
	}

	yamlFile := fileVal.(starlark.HasAttrs)
	decodeFn, _ := yamlFile.Attr("try_decode")
	val, err := starlark.Call(thread, decodeFn, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	r := val.(*starbase.Result)
	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.True {
		errAttr, _ := r.Attr("error")
		t.Fatalf("expected ok=True, got error=%v", errAttr)
	}

	value, _ := r.Attr("value")
	dict, ok := value.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected dict, got %T", value)
	}
	keyVal, _, _ := dict.Get(starlark.String("key"))
	if string(keyVal.(starlark.String)) != "value" {
		t.Fatalf("key = %v, want 'value'", keyVal)
	}
}

// =============================================================================
// json module try_ tests (factory-based: try_file, try_source)
// =============================================================================

func TestJSONTryAttr(t *testing.T) {
	mod := loadModule(t, jsonmod.New(), "json")
	assertTryAttrs(t, mod, "json", []string{
		"file", "source",
	})
}

func TestJSONTryFileDecodeSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	os.WriteFile(path, []byte(`{"key":"value"}`), 0644)

	mod := loadModule(t, jsonmod.New(), "json")
	thread := trustedThread()

	fileFn, _ := mod.Attr("file")
	fileVal, err := starlark.Call(thread, fileFn, starlark.Tuple{starlark.String(path)}, nil)
	if err != nil {
		t.Fatal(err)
	}

	jsonFile := fileVal.(starlark.HasAttrs)
	decodeFn, _ := jsonFile.Attr("try_decode")
	val, err := starlark.Call(thread, decodeFn, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	r := val.(*starbase.Result)
	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.True {
		errAttr, _ := r.Attr("error")
		t.Fatalf("expected ok=True, got error=%v", errAttr)
	}

	value, _ := r.Attr("value")
	dict, ok := value.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected dict, got %T", value)
	}
	keyVal, _, _ := dict.Get(starlark.String("key"))
	if string(keyVal.(starlark.String)) != "value" {
		t.Fatalf("key = %v, want 'value'", keyVal)
	}
}
