package modules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	argsmod "github.com/project-starkite/starkite/libkite/modules/args"
	b64 "github.com/project-starkite/starkite/libkite/modules/base64"
	concurmod "github.com/project-starkite/starkite/libkite/modules/concur"
	containersmod "github.com/project-starkite/starkite/libkite/modules/containers"
	csvmod "github.com/project-starkite/starkite/libkite/modules/csv"
	fleetmod "github.com/project-starkite/starkite/libkite/modules/fleet"
	fmtmod "github.com/project-starkite/starkite/libkite/modules/fmt"
	"github.com/project-starkite/starkite/libkite/modules/fs"
	gzipmod "github.com/project-starkite/starkite/libkite/modules/gzip"
	hashmod "github.com/project-starkite/starkite/libkite/modules/hash"
	"github.com/project-starkite/starkite/libkite/modules/http"
	invmod "github.com/project-starkite/starkite/libkite/modules/inventory"
	iomod "github.com/project-starkite/starkite/libkite/modules/io"
	jsonmod "github.com/project-starkite/starkite/libkite/modules/json"
	logmod "github.com/project-starkite/starkite/libkite/modules/log"
	osmod "github.com/project-starkite/starkite/libkite/modules/os"
	regexmod "github.com/project-starkite/starkite/libkite/modules/regexp"
	retrymod "github.com/project-starkite/starkite/libkite/modules/retry"
	runtimemod "github.com/project-starkite/starkite/libkite/modules/runtime"
	sbxmod "github.com/project-starkite/starkite/libkite/modules/sandbox"
	sqlmod "github.com/project-starkite/starkite/libkite/modules/sql"
	"github.com/project-starkite/starkite/libkite/modules/ssh"
	stringsmod "github.com/project-starkite/starkite/libkite/modules/strings"
	tablemod "github.com/project-starkite/starkite/libkite/modules/table"
	tmplmod "github.com/project-starkite/starkite/libkite/modules/template"
	testmod "github.com/project-starkite/starkite/libkite/modules/test"
	timemod "github.com/project-starkite/starkite/libkite/modules/time"
	uuidmod "github.com/project-starkite/starkite/libkite/modules/uuid"
	varsmod "github.com/project-starkite/starkite/libkite/modules/vars"
	yamlmod "github.com/project-starkite/starkite/libkite/modules/yaml"
	zipmod "github.com/project-starkite/starkite/libkite/modules/zip"
)

// =============================================================================
// Attr dispatch tests — verify try_ methods are accessible as *starlark.Builtin
// =============================================================================

func loadModule(t *testing.T, mod interface {
	Load(*libkite.ModuleConfig) (starlark.StringDict, error)
}, key string) starlark.HasAttrs {
	t.Helper()
	exports, err := mod.Load(&libkite.ModuleConfig{})
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

	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(tmpFile, []byte("test content"), 0o644); err != nil {
		t.Fatal(err)
	}

	pathFn, err := mod.Attr("path")
	if err != nil || pathFn == nil {
		t.Fatalf("fs.path not found: err=%v", err)
	}
	pathObj, err := starlark.Call(thread, pathFn, starlark.Tuple{starlark.String(tmpFile)}, nil)
	if err != nil {
		t.Fatalf("fs.path(%q) error: %v", tmpFile, err)
	}
	pathHA := pathObj.(starlark.HasAttrs)

	fn, err := pathHA.Attr("try_read_text")
	if err != nil || fn == nil {
		t.Fatalf("try_read_text not found on Path: err=%v", err)
	}

	val, err := starlark.Call(thread, fn, nil, nil)
	if err != nil {
		t.Fatalf("try_read_text call error: %v", err)
	}

	r, ok := val.(*libkite.Result)
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
	if !ok || string(s) != "test content" {
		t.Fatalf("expected 'test content', got %v", value)
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
	pathObj, err := starlark.Call(thread, pathFn, starlark.Tuple{starlark.String("/nonexistent/xyz/missing.txt")}, nil)
	if err != nil {
		t.Fatalf("fs.path('/nonexistent/xyz/missing.txt') error: %v", err)
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

	r, ok := val.(*libkite.Result)
	if !ok {
		t.Fatalf("expected *Result, got %T", val)
	}

	okAttr, _ := r.Attr("ok")
	if okAttr != starlark.False {
		t.Fatal("expected ok=False for nonexistent file")
	}

	errAttr, _ := r.Attr("error")
	errStr := string(errAttr.(starlark.String))
	if errStr == "" {
		t.Fatal("expected non-empty error string")
	}
}

func TestOSTryExecSuccess(t *testing.T) {
	mod := loadModule(t, osmod.New(), "os")
	thread := trustedThread()

	fn, err := mod.Attr("try_exec")
	if err != nil || fn == nil {
		t.Fatalf("try_exec not found: err=%v", err)
	}

	val, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("hostname")}, nil)
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
	if len(string(stdout.(starlark.String))) == 0 {
		t.Fatal("expected non-empty stdout from hostname")
	}
}

func TestTryWithPermissionsDenied(t *testing.T) {
	mod := loadModule(t, &fs.Module{}, "fs")

	// Create a sandboxed thread that denies fs operations
	thread := &starlark.Thread{Name: "test"}
	checker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   []string{"strings.*"},
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker error: %v", err)
	}
	libkite.SetPermissions(thread, checker)

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

	r, ok := val.(*libkite.Result)
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
	r := val.(*libkite.Result)
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
	r = val.(*libkite.Result)
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

	r, ok := val.(*libkite.Result)
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

	r := val.(*libkite.Result)
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

	r := val.(*libkite.Result)
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

	r := val.(*libkite.Result)
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

	r := val.(*libkite.Result)
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

	r := val.(*libkite.Result)
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

	r := val.(*libkite.Result)
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

	r := val.(*libkite.Result)
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

	r := val.(*libkite.Result)
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
// Comprehensive registry try_ contract verification (Phase 1, v0.9.5)
// =============================================================================

func TestAllStdlibModulesTryAttr(t *testing.T) {
	// 1. fmt
	fmtM := loadModule(t, fmtmod.New(), "fmt")
	assertTryAttrs(t, fmtM, "fmt", []string{"sprintf", "printf", "println"})

	// 2. runtime
	rtM := loadModule(t, runtimemod.New(), "runtime")
	assertTryAttrs(t, rtM, "runtime", []string{"platform", "arch", "cpu_count", "uname", "version", "go_version"})

	// 3. strings
	strM := loadModule(t, stringsmod.New(), "strings")
	assertTryAttrs(t, strM, "strings", []string{
		"ljust", "rjust", "center", "cut", "equal", "has_any", "quote", "unquote",
	})

	// 4. time
	timeM := loadModule(t, timemod.New(), "time")
	assertTryAttrs(t, timeM, "time", []string{
		"now", "sleep", "parse", "format", "since", "until", "duration",
	})

	// 5. uuid
	uuidM := loadModule(t, uuidmod.New(), "uuid")
	assertTryAttrs(t, uuidM, "uuid", []string{"v4"})

	// 6. vars
	varsM := loadModule(t, varsmod.New(), "vars")
	assertTryAttrs(t, varsM, "vars", []string{
		"var_str", "var_int", "var_bool", "var_float", "var_names", "var_list", "var_dict",
	})

	// 7. retry
	retryM := loadModule(t, retrymod.New(), "retry")
	assertTryAttrs(t, retryM, "retry", []string{"do", "with_backoff"})

	// 8. table
	tableM := loadModule(t, tablemod.New(), "table")
	assertTryAttrs(t, tableM, "table", []string{"new", "print"})

	// 9. log
	logM := loadModule(t, logmod.New(), "log")
	assertTryAttrs(t, logM, "log", []string{
		"debug", "info", "warn", "error", "set_level", "set_format", "set_output", "logger",
	})

	// 10. inventory
	invM := loadModule(t, invmod.New(), "inventory")
	assertTryAttrs(t, invM, "inventory", []string{
		"file", "list", "filter", "group_by", "merge", "addresses",
	})

	// 11. sandbox
	sbxM := loadModule(t, sbxmod.New(), "sandbox")
	assertTryAttrs(t, sbxM, "sandbox", []string{
		"config", "run_script", "list_drivers", "default_driver",
	})

	// 12. test
	testM := loadModule(t, testmod.New(), "test")
	assertTryAttrs(t, testM, "test", []string{
		"skip", "fail", "assert", "assert_equal", "assert_not_equal", "assert_contains", "assert_true", "assert_false",
	})

	// 13. concur
	concurM := loadModule(t, concurmod.New(), "concur")
	assertTryAttrs(t, concurM, "concur", []string{"map", "each", "exec"})

	// 14. regexp
	regexM := loadModule(t, regexmod.New(), "regexp")
	assertTryAttrs(t, regexM, "regexp", []string{"match", "find", "find_all", "replace", "split", "compile"})

	// 15. template
	tmplM := loadModule(t, tmplmod.New(), "template")
	assertTryAttrs(t, tmplM, "template", []string{"text", "file", "bytes", "render"})

	// 16. hash
	hashM := loadModule(t, hashmod.New(), "hash")
	assertTryAttrs(t, hashM, "hash", []string{"file", "text", "bytes"})

	// 17. gzip
	gzipM := loadModule(t, gzipmod.New(), "gzip")
	assertTryAttrs(t, gzipM, "gzip", []string{"file", "text", "bytes"})

	// 18. zip
	zipM := loadModule(t, zipmod.New(), "zip")
	assertTryAttrs(t, zipM, "zip", []string{"file"})

	// 19. sql
	sqlM := loadModule(t, sqlmod.New(), "sql")
	assertTryAttrs(t, sqlM, "sql", []string{"open", "stmt"})

	// 20. fleet
	fleetM := loadModule(t, fleetmod.New(), "fleet")
	assertTryAttrs(t, fleetM, "fleet", []string{"new", "file", "hosts_file", "host_file"})

	// 21. containers
	containersM := loadModule(t, containersmod.New(), "containers")
	assertTryAttrs(t, containersM, "containers", []string{
		"config", "run", "exec", "stop", "delete", "remove",
		"image_pull", "image_build", "image_list", "image_inspect", "image_remove",
		"inspect", "list",
	})

	// 22. args
	argsM := loadModule(t, argsmod.New(), "args")
	assertTryAttrs(t, argsM, "args", []string{"string", "int", "bool", "float", "list", "positional", "parse"})
}

func TestDynamicValueTypesTryContract(t *testing.T) {
	thread := trustedThread()

	// 1. TimeValue & DurationValue
	timeM := loadModule(t, timemod.New(), "time")
	nowFn, err := timeM.Attr("now")
	if err != nil || nowFn == nil {
		t.Fatalf("time.now lookup failed: %v", err)
	}
	tvObj, err := starlark.Call(thread, nowFn, nil, nil)
	if err != nil {
		t.Fatalf("time.now() call failed: %v", err)
	}
	tv, ok := tvObj.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("expected HasAttrs for TimeValue, got %T", tvObj)
	}
	assertTryAttrs(t, tv, "TimeValue", []string{"add", "sub", "string"})

	// Check accessors
	for _, attr := range []string{"utc", "unix_milli", "nanosecond"} {
		v, err := tv.Attr(attr)
		if err != nil || v == nil {
			t.Errorf("TimeValue.%s missing: %v", attr, err)
		}
	}

	// Try_string call
	tryStrFn, _ := tv.Attr("try_string")
	resVal, err := starlark.Call(thread, tryStrFn, nil, nil)
	if err != nil {
		t.Fatalf("TimeValue.try_string error: %v", err)
	}
	res := resVal.(*libkite.Result)
	if okAttr, _ := res.Attr("ok"); okAttr != starlark.True {
		t.Fatalf("TimeValue.try_string ok != True")
	}

	// Try_add call (success & failure)
	tryAddFn, _ := tv.Attr("try_add")
	addResVal, _ := starlark.Call(thread, tryAddFn, starlark.Tuple{starlark.String("1h")}, nil)
	if okAttr, _ := addResVal.(*libkite.Result).Attr("ok"); okAttr != starlark.True {
		t.Fatalf("TimeValue.try_add('1h') ok != True")
	}
	badAddResVal, _ := starlark.Call(thread, tryAddFn, starlark.Tuple{starlark.String("invalid-duration")}, nil)
	if okAttr, _ := badAddResVal.(*libkite.Result).Attr("ok"); okAttr != starlark.False {
		t.Fatalf("TimeValue.try_add('invalid') ok != False")
	}

	// DurationValue
	durFn, _ := timeM.Attr("duration")
	durObj, err := starlark.Call(thread, durFn, starlark.Tuple{starlark.MakeInt(10)}, nil)
	if err != nil {
		t.Fatalf("time.duration(10) failed: %v", err)
	}
	dv := durObj.(starlark.HasAttrs)
	assertTryAttrs(t, dv, "DurationValue", []string{"string"})

	durTryStrFn, _ := dv.Attr("try_string")
	durResVal, err := starlark.Call(thread, durTryStrFn, nil, nil)
	if err != nil {
		t.Fatalf("DurationValue.try_string error: %v", err)
	}
	durRes := durResVal.(*libkite.Result)
	if okAttr, _ := durRes.Attr("ok"); okAttr != starlark.True {
		t.Fatalf("DurationValue.try_string ok != True")
	}
	if vAttr, _ := durRes.Attr("value"); vAttr != starlark.String("10s") {
		t.Fatalf("DurationValue.try_string value = %v, want '10s'", vAttr)
	}

	// 2. TableValue
	tableM := loadModule(t, tablemod.New(), "table")
	newTableFn, _ := tableM.Attr("new")
	tableObj, err := starlark.Call(thread, newTableFn, starlark.Tuple{
		starlark.NewList([]starlark.Value{starlark.String("C1"), starlark.String("C2")}),
	}, nil)
	if err != nil {
		t.Fatalf("table.new failed: %v", err)
	}
	tbl := tableObj.(starlark.HasAttrs)
	assertTryAttrs(t, tbl, "TableValue", []string{"add_row", "render"})

	// Call try_add_row with too many values -> failure Result
	tryAddRowFn, _ := tbl.Attr("try_add_row")
	rowFailRes, _ := starlark.Call(thread, tryAddRowFn, starlark.Tuple{
		starlark.String("a"), starlark.String("b"), starlark.String("c"),
	}, nil)
	if okAttr, _ := rowFailRes.(*libkite.Result).Attr("ok"); okAttr != starlark.False {
		t.Fatalf("TableValue.try_add_row(too many) ok != False")
	}

	// Call try_add_row with 1 value -> auto-padded, success Result
	rowOkRes, _ := starlark.Call(thread, tryAddRowFn, starlark.Tuple{starlark.String("a")}, nil)
	if okAttr, _ := rowOkRes.(*libkite.Result).Attr("ok"); okAttr != starlark.True {
		t.Fatalf("TableValue.try_add_row(padded) ok != True")
	}

	// 3. Logger
	logM := loadModule(t, logmod.New(), "log")
	loggerFn, err := logM.Attr("logger")
	if err != nil || loggerFn == nil {
		t.Fatalf("log.logger lookup failed: %v", err)
	}
	loggerObj, err := starlark.Call(thread, loggerFn, nil, nil)
	if err != nil {
		t.Fatalf("log.logger() failed: %v", err)
	}
	l := loggerObj.(starlark.HasAttrs)
	assertTryAttrs(t, l, "Logger", []string{"info", "warn", "error", "debug"})

	// 4. Sandbox
	sbxM := loadModule(t, sbxmod.New(), "sandbox")
	configFn, _ := sbxM.Attr("config")
	sbxObj, err := starlark.Call(thread, configFn, nil, nil)
	if err != nil {
		t.Fatalf("sandbox.config failed: %v", err)
	}
	sb := sbxObj.(starlark.HasAttrs)
	assertTryAttrs(t, sb, "Sandbox", []string{"exec"})
}
