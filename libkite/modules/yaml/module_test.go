package yaml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// mockDictConvertible implements startype.DictConvertible for testing.
type mockDictConvertible struct {
	dict *starlark.Dict
}

func (m *mockDictConvertible) String() string         { return "<mock>" }
func (m *mockDictConvertible) Type() string           { return "mock" }
func (m *mockDictConvertible) Freeze()                {}
func (m *mockDictConvertible) Truth() starlark.Bool   { return starlark.True }
func (m *mockDictConvertible) Hash() (uint32, error)  { return 0, nil }
func (m *mockDictConvertible) ToDict() *starlark.Dict { return m.dict }

func newMock(kvs ...any) *mockDictConvertible {
	d := starlark.NewDict(len(kvs) / 2)
	for i := 0; i < len(kvs); i += 2 {
		k := starlark.String(kvs[i].(string))
		var v starlark.Value
		switch val := kvs[i+1].(type) {
		case string:
			v = starlark.String(val)
		case int:
			v = starlark.MakeInt(val)
		case starlark.Value:
			v = val
		}
		d.SetKey(k, v)
	}
	return &mockDictConvertible{dict: d}
}

func newTestModule(t *testing.T) *Module {
	t.Helper()
	m := New()
	_, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func newTestYamlFile(path string) *YamlFile {
	return &YamlFile{path: path, thread: &starlark.Thread{Name: "test"}, config: &libkite.ModuleConfig{}}
}

func newTestWriter(data starlark.Value) *Writer {
	return &Writer{data: data, thread: &starlark.Thread{Name: "test"}, config: &libkite.ModuleConfig{}}
}

// ============================================================================
// YamlFile Value interface tests
// ============================================================================

func TestYamlFileString(t *testing.T) {
	f := newTestYamlFile("/tmp/config.yaml")
	if got := f.String(); got != `yaml.file("/tmp/config.yaml")` {
		t.Errorf("String() = %q, want %q", got, `yaml.file("/tmp/config.yaml")`)
	}
}

func TestYamlFileType(t *testing.T) {
	f := newTestYamlFile("/tmp/config.yaml")
	if got := f.Type(); got != "yaml.file" {
		t.Errorf("Type() = %q, want %q", got, "yaml.file")
	}
}

func TestYamlFileTruth(t *testing.T) {
	if !bool(newTestYamlFile("/tmp/config.yaml").Truth()) {
		t.Error("non-empty path should be truthy")
	}
	if bool(newTestYamlFile("").Truth()) {
		t.Error("empty path should be falsy")
	}
}

func TestYamlFileHash(t *testing.T) {
	f := newTestYamlFile("/tmp/config.yaml")
	_, err := f.Hash()
	if err == nil {
		t.Error("Hash() should return error (unhashable)")
	}
}

func TestYamlFilePathProperty(t *testing.T) {
	f := newTestYamlFile("/tmp/config.yaml")
	v, err := f.Attr("path")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	if got != "/tmp/config.yaml" {
		t.Errorf("path = %q, want %q", got, "/tmp/config.yaml")
	}
}

func TestYamlFileAttrNames(t *testing.T) {
	f := newTestYamlFile("/tmp/config.yaml")
	names := f.AttrNames()
	required := []string{"path", "decode", "decode_all", "try_decode", "try_decode_all"}
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
// YamlFile decode methods
// ============================================================================

func TestYamlFileDecode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	os.WriteFile(path, []byte("name: crsh\nversion: 1\n"), 0644)

	f := newTestYamlFile(path)
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

func TestYamlFileDecodeAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.yaml")
	os.WriteFile(path, []byte("name: first\n---\nname: second\n"), 0644)

	f := newTestYamlFile(path)
	v, err := callFileMethod(f, "decode_all")
	if err != nil {
		t.Fatal(err)
	}
	list, ok := v.(*starlark.List)
	if !ok {
		t.Fatalf("expected list, got %s", v.Type())
	}
	if list.Len() != 2 {
		t.Fatalf("expected 2 documents, got %d", list.Len())
	}
}

func TestYamlFileDecodeMissing(t *testing.T) {
	f := newTestYamlFile("/tmp/nonexistent_yaml_test_file.yaml")
	_, err := callFileMethod(f, "decode")
	if err == nil {
		t.Error("decode on missing file should fail")
	}
}

func TestYamlFileDryRun(t *testing.T) {
	f := &YamlFile{
		path:   "/tmp/nonexistent.yaml",
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

	v, err = callFileMethod(f, "decode_all")
	if err != nil {
		t.Fatal(err)
	}
	list, ok := v.(*starlark.List)
	if !ok || list.Len() != 0 {
		t.Errorf("DryRun decode_all should return empty list, got %v", v)
	}
}

// ============================================================================
// YamlFile try_ dispatch
// ============================================================================

func TestYamlFileTryDecodeSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	os.WriteFile(path, []byte("key: value\n"), 0644)

	f := newTestYamlFile(path)
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

func TestYamlFileTryDecodeFailure(t *testing.T) {
	f := newTestYamlFile("/tmp/nonexistent_yaml_test_file.yaml")
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
	if got := w.String(); got != "yaml.writer(string)" {
		t.Errorf("String() = %q, want %q", got, "yaml.writer(string)")
	}
}

func TestWriterType(t *testing.T) {
	w := newTestWriter(starlark.String("hello"))
	if got := w.Type(); got != "yaml.writer" {
		t.Errorf("Type() = %q, want %q", got, "yaml.writer")
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

func TestWriterWriteFileSingleDoc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")

	dict := starlark.NewDict(1)
	dict.SetKey(starlark.String("key"), starlark.String("value"))
	w := newTestWriter(dict)

	_, err := callWriterMethod(w, "write_file", starlark.String(path))
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "key: value") {
		t.Errorf("file content = %q, expected to contain 'key: value'", string(content))
	}
}

func TestWriterWriteFileMultiDoc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.yaml")

	dict1 := starlark.NewDict(1)
	dict1.SetKey(starlark.String("name"), starlark.String("first"))
	dict2 := starlark.NewDict(1)
	dict2.SetKey(starlark.String("name"), starlark.String("second"))

	list := starlark.NewList([]starlark.Value{dict1, dict2})
	w := newTestWriter(list)

	_, err := callWriterMethod(w, "write_file", starlark.String(path))
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if !strings.Contains(s, "---") {
		t.Errorf("multi-doc should contain '---' separator, got:\n%s", s)
	}
	if !strings.Contains(s, "name: first") {
		t.Errorf("should contain 'name: first', got:\n%s", s)
	}
	if !strings.Contains(s, "name: second") {
		t.Errorf("should contain 'name: second', got:\n%s", s)
	}
}

func TestWriterWriteFileDictConvertible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mock.yaml")

	mock := newMock("kind", "Deployment", "name", "web")
	w := newTestWriter(mock)

	_, err := callWriterMethod(w, "write_file", starlark.String(path))
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if !strings.Contains(s, "kind: Deployment") {
		t.Errorf("expected 'kind: Deployment' in yaml, got:\n%s", s)
	}
	if !strings.Contains(s, "name: web") {
		t.Errorf("expected 'name: web' in yaml, got:\n%s", s)
	}
}

func TestWriterDryRun(t *testing.T) {
	w := &Writer{
		data:   starlark.String("hello"),
		thread: &starlark.Thread{Name: "test"},
		config: &libkite.ModuleConfig{DryRun: true},
	}
	v, err := callWriterMethod(w, "write_file", starlark.String("/tmp/nonexistent.yaml"))
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
	m := newTestModule(t)
	thread := &starlark.Thread{Name: "test"}

	v, err := m.fileFactory(thread, nil, starlark.Tuple{starlark.String("/tmp/test.yaml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	yf, ok := v.(*YamlFile)
	if !ok {
		t.Fatalf("fileFactory should return *YamlFile, got %T", v)
	}
	if yf.path != "/tmp/test.yaml" {
		t.Errorf("path = %q, want %q", yf.path, "/tmp/test.yaml")
	}
}

func TestModuleFileFactoryWrongArgCount(t *testing.T) {
	m := newTestModule(t)
	thread := &starlark.Thread{Name: "test"}

	_, err := m.fileFactory(thread, nil, starlark.Tuple{}, nil)
	if err == nil {
		t.Error("fileFactory with 0 args should fail")
	}
}

func TestModuleSourceFactory(t *testing.T) {
	m := newTestModule(t)
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
	m := newTestModule(t)
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
	m := New()
	if got := m.Name(); got != "yaml" {
		t.Errorf("Name() = %q, want %q", got, "yaml")
	}
}

func TestModuleDescription(t *testing.T) {
	m := New()
	desc := m.Description()
	if desc == "" {
		t.Error("Description should not be empty")
	}
}

func TestModuleLoad(t *testing.T) {
	m := New()
	dict, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := dict["yaml"]; !ok {
		t.Error("Load should return dict with 'yaml' key")
	}
}

func TestModuleAliases(t *testing.T) {
	m := New()
	if m.Aliases() != nil {
		t.Error("Aliases should return nil")
	}
}

func TestModuleFactoryMethod(t *testing.T) {
	m := New()
	if m.FactoryMethod() != "" {
		t.Error("FactoryMethod should return empty string")
	}
}

// ============================================================================
// Test helpers
// ============================================================================

func callFileMethod(f *YamlFile, name string, args ...starlark.Value) (starlark.Value, error) {
	method, err := f.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, nil
	}
	return starlark.Call(f.thread, method, args, nil)
}

func callWriterMethod(w *Writer, name string, args ...starlark.Value) (starlark.Value, error) {
	method, err := w.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, nil
	}
	return starlark.Call(w.thread, method, args, nil)
}
