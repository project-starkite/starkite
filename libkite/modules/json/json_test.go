package json

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func newTestJsonFile(path string) *JsonFile {
	return &JsonFile{path: path, thread: &starlark.Thread{Name: "test"}, config: &libkite.ModuleConfig{}}
}

func newTestWriter(data starlark.Value) *Writer {
	return &Writer{data: data, thread: &starlark.Thread{Name: "test"}, config: &libkite.ModuleConfig{}}
}

func newTestModule() *Module {
	return New()
}

// ============================================================================
// JsonFile Value interface tests
// ============================================================================

func TestJsonFileString(t *testing.T) {
	f := newTestJsonFile("/tmp/config.json")
	if got := f.String(); got != `json.file("/tmp/config.json")` {
		t.Errorf("String() = %q, want %q", got, `json.file("/tmp/config.json")`)
	}
}

func TestJsonFileType(t *testing.T) {
	f := newTestJsonFile("/tmp/config.json")
	if got := f.Type(); got != "json.file" {
		t.Errorf("Type() = %q, want %q", got, "json.file")
	}
}

func TestJsonFileTruth(t *testing.T) {
	if !bool(newTestJsonFile("/tmp/config.json").Truth()) {
		t.Error("non-empty path should be truthy")
	}
	if bool(newTestJsonFile("").Truth()) {
		t.Error("empty path should be falsy")
	}
}

func TestJsonFileHash(t *testing.T) {
	f := newTestJsonFile("/tmp/config.json")
	_, err := f.Hash()
	if err == nil {
		t.Error("Hash() should return error (unhashable)")
	}
}

func TestJsonFilePathProperty(t *testing.T) {
	f := newTestJsonFile("/tmp/config.json")
	v, err := f.Attr("path")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	if got != "/tmp/config.json" {
		t.Errorf("path = %q, want %q", got, "/tmp/config.json")
	}
}

func TestJsonFileAttrNames(t *testing.T) {
	f := newTestJsonFile("/tmp/config.json")
	names := f.AttrNames()
	required := []string{"path", "decode", "try_decode"}
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, r := range required {
		if !nameSet[r] {
			t.Errorf("AttrNames missing %q", r)
		}
	}
}

// ============================================================================
// JsonFile decode method
// ============================================================================

func TestJsonFileDecode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	os.WriteFile(path, []byte(`{"name":"crsh","count":5}`), 0644)

	f := newTestJsonFile(path)
	v, err := callFileMethod(f, "decode")
	if err != nil {
		t.Fatal(err)
	}
	dict, ok := v.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected dict, got %s", v.Type())
	}
	nameVal, _, _ := dict.Get(starlark.String("name"))
	if string(nameVal.(starlark.String)) != "crsh" {
		t.Errorf("name = %v, want 'crsh'", nameVal)
	}
}

func TestJsonFileDecodeMissing(t *testing.T) {
	f := newTestJsonFile("/tmp/nonexistent_json_test_file.json")
	_, err := callFileMethod(f, "decode")
	if err == nil {
		t.Error("decode on missing file should fail")
	}
}

func TestJsonFileDryRun(t *testing.T) {
	f := &JsonFile{
		path:   "/tmp/nonexistent.json",
		thread: &starlark.Thread{Name: "test"},
		config: &libkite.ModuleConfig{DryRun: true},
	}
	v, err := callFileMethod(f, "decode")
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.None {
		t.Errorf("DryRun decode should return None, got %v", v)
	}
}

// ============================================================================
// JsonFile try_ dispatch
// ============================================================================

func TestJsonFileTryDecodeSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	os.WriteFile(path, []byte(`{"key":"value"}`), 0644)

	f := newTestJsonFile(path)
	v, _ := f.Attr("try_decode")
	result, err := starlark.Call(f.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*libkite.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.True {
		t.Error("try_decode should succeed")
	}
}

func TestJsonFileTryDecodeFailure(t *testing.T) {
	f := newTestJsonFile("/tmp/nonexistent_json_test_file.json")
	v, _ := f.Attr("try_decode")
	result, err := starlark.Call(f.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*libkite.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.False {
		t.Error("try_decode on missing file should have ok=False")
	}
}

// ============================================================================
// Writer Value interface tests
// ============================================================================

func TestWriterString(t *testing.T) {
	w := newTestWriter(starlark.String("hello"))
	if got := w.String(); got != "json.writer(string)" {
		t.Errorf("String() = %q, want %q", got, "json.writer(string)")
	}
}

func TestWriterType(t *testing.T) {
	w := newTestWriter(starlark.String("hello"))
	if got := w.Type(); got != "json.writer" {
		t.Errorf("Type() = %q, want %q", got, "json.writer")
	}
}

func TestWriterDataProperty(t *testing.T) {
	data := starlark.String("hello")
	w := newTestWriter(data)
	v, err := w.Attr("data")
	if err != nil {
		t.Fatal(err)
	}
	if v != data {
		t.Errorf("data = %v, want %v", v, data)
	}
}

func TestWriterAttrNames(t *testing.T) {
	w := newTestWriter(starlark.String("x"))
	names := w.AttrNames()
	required := []string{"data", "write_file", "try_write_file"}
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, r := range required {
		if !nameSet[r] {
			t.Errorf("AttrNames missing %q", r)
		}
	}
}

// ============================================================================
// Writer write_file methods
// ============================================================================

func TestWriterWriteFileDict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	dict := starlark.NewDict(1)
	dict.SetKey(starlark.String("name"), starlark.String("crsh"))
	w := newTestWriter(dict)

	_, err := callWriterMethod(w, "write_file", starlark.String(path))
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if !strings.Contains(s, `"name"`) || !strings.Contains(s, `"crsh"`) {
		t.Errorf("file content = %q, expected to contain name and crsh", s)
	}
}

func TestWriterWriteFileIndent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pretty.json")

	dict := starlark.NewDict(1)
	dict.SetKey(starlark.String("a"), starlark.MakeInt(1))
	w := newTestWriter(dict)

	_, err := callWriterMethodKwargs(w, "write_file",
		starlark.Tuple{starlark.String(path)},
		[]starlark.Tuple{kw("indent", starlark.String("  "))})
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if !strings.Contains(s, "\n") {
		t.Errorf("pretty-printed JSON should have newlines, got: %q", s)
	}
	if !strings.Contains(s, "  ") {
		t.Errorf("pretty-printed JSON should have indentation, got: %q", s)
	}
}

func TestWriterDryRun(t *testing.T) {
	w := &Writer{
		data:   starlark.String("hello"),
		thread: &starlark.Thread{Name: "test"},
		config: &libkite.ModuleConfig{DryRun: true},
	}
	v, err := callWriterMethod(w, "write_file", starlark.String("/tmp/nonexistent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.None {
		t.Errorf("DryRun write_file should return None, got %v", v)
	}
}

// ============================================================================
// Module factory tests
// ============================================================================

func TestModuleFileFactory(t *testing.T) {
	m := newTestModule()
	m.config = &libkite.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	v, err := m.fileFactory(thread, nil, starlark.Tuple{starlark.String("/tmp/test.json")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	jf, ok := v.(*JsonFile)
	if !ok {
		t.Fatalf("fileFactory should return *JsonFile, got %T", v)
	}
	if jf.path != "/tmp/test.json" {
		t.Errorf("path = %q, want %q", jf.path, "/tmp/test.json")
	}
}

func TestModuleFileFactoryWrongArgCount(t *testing.T) {
	m := newTestModule()
	thread := &starlark.Thread{Name: "test"}

	_, err := m.fileFactory(thread, nil, starlark.Tuple{}, nil)
	if err == nil {
		t.Error("fileFactory with 0 args should fail")
	}
}

func TestModuleSourceFactory(t *testing.T) {
	m := newTestModule()
	m.config = &libkite.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	dict := starlark.NewDict(1)
	dict.SetKey(starlark.String("key"), starlark.String("val"))
	v, err := m.sourceFactory(thread, nil, starlark.Tuple{dict}, nil)
	if err != nil {
		t.Fatal(err)
	}
	w, ok := v.(*Writer)
	if !ok {
		t.Fatalf("sourceFactory should return *Writer, got %T", v)
	}
	if w.data != dict {
		t.Error("writer data should match input")
	}
}

func TestModuleSourceFactoryWrongArgCount(t *testing.T) {
	m := newTestModule()
	thread := &starlark.Thread{Name: "test"}

	_, err := m.sourceFactory(thread, nil, starlark.Tuple{}, nil)
	if err == nil {
		t.Error("sourceFactory with 0 args should fail")
	}
}

// ============================================================================
// Module interface
// ============================================================================

func TestModuleName(t *testing.T) {
	m := newTestModule()
	if got := m.Name(); got != "json" {
		t.Errorf("Name() = %q, want %q", got, "json")
	}
}

func TestModuleDescription(t *testing.T) {
	m := newTestModule()
	desc := m.Description()
	if desc == "" {
		t.Error("Description should not be empty")
	}
}

func TestModuleLoad(t *testing.T) {
	m := newTestModule()
	dict, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := dict["json"]; !ok {
		t.Error("Load should return dict with 'json' key")
	}
}

func TestModuleAliases(t *testing.T) {
	m := newTestModule()
	if m.Aliases() != nil {
		t.Error("Aliases should return nil")
	}
}

func TestModuleFactoryMethod(t *testing.T) {
	m := newTestModule()
	if m.FactoryMethod() != "" {
		t.Error("FactoryMethod should return empty string")
	}
}

// ============================================================================
// Test helpers
// ============================================================================

func callFileMethod(f *JsonFile, name string, args ...starlark.Value) (starlark.Value, error) {
	method, err := f.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("json.file has no attribute %q", name)
	}
	return starlark.Call(f.thread, method, args, nil)
}

func callWriterMethod(w *Writer, name string, args ...starlark.Value) (starlark.Value, error) {
	method, err := w.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("json.writer has no attribute %q", name)
	}
	return starlark.Call(w.thread, method, args, nil)
}

func callWriterMethodKwargs(w *Writer, name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	method, err := w.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("json.writer has no attribute %q", name)
	}
	return starlark.Call(w.thread, method, args, kwargs)
}

func kw(name string, val starlark.Value) starlark.Tuple {
	return starlark.Tuple{starlark.String(name), val}
}
