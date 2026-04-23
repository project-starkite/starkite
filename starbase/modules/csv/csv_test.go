package csv

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

func newTestCsvFile(path string) *CsvFile {
	return &CsvFile{path: path, thread: &starlark.Thread{Name: "test"}, config: &starbase.ModuleConfig{}}
}

func newTestWriter(data *starlark.List) *Writer {
	return &Writer{data: data, thread: &starlark.Thread{Name: "test"}, config: &starbase.ModuleConfig{}}
}

func newTestModule() *Module {
	return New()
}

// ============================================================================
// CsvFile Value interface tests
// ============================================================================

func TestCsvFileString(t *testing.T) {
	f := newTestCsvFile("/tmp/data.csv")
	if got := f.String(); got != `csv.file("/tmp/data.csv")` {
		t.Errorf("String() = %q, want %q", got, `csv.file("/tmp/data.csv")`)
	}
}

func TestCsvFileType(t *testing.T) {
	f := newTestCsvFile("/tmp/data.csv")
	if got := f.Type(); got != "csv.file" {
		t.Errorf("Type() = %q, want %q", got, "csv.file")
	}
}

func TestCsvFileTruth(t *testing.T) {
	if !bool(newTestCsvFile("/tmp/data.csv").Truth()) {
		t.Error("non-empty path should be truthy")
	}
	if bool(newTestCsvFile("").Truth()) {
		t.Error("empty path should be falsy")
	}
}

func TestCsvFileHash(t *testing.T) {
	f := newTestCsvFile("/tmp/data.csv")
	_, err := f.Hash()
	if err == nil {
		t.Error("Hash() should return error (unhashable)")
	}
}

func TestCsvFilePathProperty(t *testing.T) {
	f := newTestCsvFile("/tmp/data.csv")
	v, err := f.Attr("path")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	if got != "/tmp/data.csv" {
		t.Errorf("path = %q, want %q", got, "/tmp/data.csv")
	}
}

func TestCsvFileAttrNames(t *testing.T) {
	f := newTestCsvFile("/tmp/data.csv")
	names := f.AttrNames()
	required := []string{"path", "read", "try_read"}
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
// CsvFile read methods
// ============================================================================

func TestCsvFileReadBasic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	os.WriteFile(path, []byte("a,b\n1,2\n"), 0644)

	f := newTestCsvFile(path)
	v, err := callFileMethod(f, "read")
	if err != nil {
		t.Fatal(err)
	}
	list := v.(*starlark.List)
	if list.Len() != 2 {
		t.Fatalf("expected 2 rows, got %d", list.Len())
	}
}

func TestCsvFileReadHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	os.WriteFile(path, []byte("name,value\nfoo,1\nbar,2\n"), 0644)

	f := newTestCsvFile(path)
	v, err := callFileMethodKwargs(f, "read", nil, []starlark.Tuple{kw("header", starlark.True)})
	if err != nil {
		t.Fatal(err)
	}
	list := v.(*starlark.List)
	if list.Len() != 2 {
		t.Fatalf("expected 2 records, got %d", list.Len())
	}
	dict := list.Index(0).(*starlark.Dict)
	nameVal, _, _ := dict.Get(starlark.String("name"))
	if string(nameVal.(starlark.String)) != "foo" {
		t.Errorf("name = %v, want 'foo'", nameVal)
	}
}

func TestCsvFileReadMissing(t *testing.T) {
	f := newTestCsvFile("/tmp/nonexistent_csv_test_file.csv")
	_, err := callFileMethod(f, "read")
	if err == nil {
		t.Error("read on missing file should fail")
	}
}

func TestCsvFileDryRun(t *testing.T) {
	f := &CsvFile{
		path:   "/tmp/nonexistent.csv",
		thread: &starlark.Thread{Name: "test"},
		config: &starbase.ModuleConfig{DryRun: true},
	}
	v, err := callFileMethod(f, "read")
	if err != nil {
		t.Fatal(err)
	}
	list := v.(*starlark.List)
	if list.Len() != 0 {
		t.Errorf("DryRun read should return empty list, got %d rows", list.Len())
	}
}

// ============================================================================
// CsvFile try_ dispatch
// ============================================================================

func TestCsvFileTryReadSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	os.WriteFile(path, []byte("a,b\n1,2\n"), 0644)

	f := newTestCsvFile(path)
	v, _ := f.Attr("try_read")
	result, err := starlark.Call(f.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*starbase.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.True {
		t.Error("try_read should succeed")
	}
}

func TestCsvFileTryReadFailure(t *testing.T) {
	f := newTestCsvFile("/tmp/nonexistent_csv_test_file.csv")
	v, _ := f.Attr("try_read")
	result, err := starlark.Call(f.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*starbase.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.False {
		t.Error("try_read on missing file should have ok=False")
	}
}

// ============================================================================
// Writer Value interface tests
// ============================================================================

func TestWriterString(t *testing.T) {
	w := newTestWriter(starlark.NewList([]starlark.Value{starlark.NewList([]starlark.Value{starlark.String("a")})}))
	if got := w.String(); got != "csv.writer(1 rows)" {
		t.Errorf("String() = %q, want %q", got, "csv.writer(1 rows)")
	}
}

func TestWriterType(t *testing.T) {
	w := newTestWriter(starlark.NewList(nil))
	if got := w.Type(); got != "csv.writer" {
		t.Errorf("Type() = %q, want %q", got, "csv.writer")
	}
}

func TestWriterDataProperty(t *testing.T) {
	data := starlark.NewList([]starlark.Value{starlark.String("x")})
	w := newTestWriter(data)
	v, err := w.Attr("data")
	if err != nil {
		t.Fatal(err)
	}
	if v != data {
		t.Error("data should return the original list")
	}
}

func TestWriterAttrNames(t *testing.T) {
	w := newTestWriter(starlark.NewList(nil))
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

func TestWriterWriteFileListOfLists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")

	data := starlark.NewList([]starlark.Value{
		starlark.NewList([]starlark.Value{starlark.String("a"), starlark.String("b")}),
		starlark.NewList([]starlark.Value{starlark.String("1"), starlark.String("2")}),
	})
	w := newTestWriter(data)
	_, err := callWriterMethod(w, "write_file", starlark.String(path))
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if s != "a,b\n1,2\n" {
		t.Errorf("file content = %q, want %q", s, "a,b\n1,2\n")
	}
}

func TestWriterWriteFileListOfDicts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")

	dict := starlark.NewDict(2)
	dict.SetKey(starlark.String("name"), starlark.String("foo"))
	dict.SetKey(starlark.String("value"), starlark.String("1"))

	data := starlark.NewList([]starlark.Value{dict})
	w := newTestWriter(data)
	_, err := callWriterMethod(w, "write_file", starlark.String(path))
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if len(s) == 0 {
		t.Error("file content should not be empty")
	}
}

func TestWriterDryRun(t *testing.T) {
	w := &Writer{
		data:   starlark.NewList([]starlark.Value{starlark.NewList([]starlark.Value{starlark.String("a")})}),
		thread: &starlark.Thread{Name: "test"},
		config: &starbase.ModuleConfig{DryRun: true},
	}
	v, err := callWriterMethod(w, "write_file", starlark.String("/tmp/nonexistent.csv"))
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
	m.config = &starbase.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	v, err := m.fileFactory(thread, nil, starlark.Tuple{starlark.String("/tmp/test.csv")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cf, ok := v.(*CsvFile)
	if !ok {
		t.Fatalf("fileFactory should return *CsvFile, got %T", v)
	}
	if cf.path != "/tmp/test.csv" {
		t.Errorf("path = %q, want %q", cf.path, "/tmp/test.csv")
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
	m.config = &starbase.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	list := starlark.NewList([]starlark.Value{starlark.String("a")})
	v, err := m.sourceFactory(thread, nil, starlark.Tuple{list}, nil)
	if err != nil {
		t.Fatal(err)
	}
	w, ok := v.(*Writer)
	if !ok {
		t.Fatalf("sourceFactory should return *Writer, got %T", v)
	}
	if w.data != list {
		t.Error("writer data should match input")
	}
}

func TestModuleSourceFactoryBadType(t *testing.T) {
	m := newTestModule()
	m.config = &starbase.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	_, err := m.sourceFactory(thread, nil, starlark.Tuple{starlark.String("not a list")}, nil)
	if err == nil {
		t.Error("sourceFactory with string should fail")
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
	if got := m.Name(); got != "csv" {
		t.Errorf("Name() = %q, want %q", got, "csv")
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
	dict, err := m.Load(&starbase.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := dict["csv"]; !ok {
		t.Error("Load should return dict with 'csv' key")
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

func callFileMethod(f *CsvFile, name string, args ...starlark.Value) (starlark.Value, error) {
	method, err := f.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("csv.file has no attribute %q", name)
	}
	return starlark.Call(f.thread, method, args, nil)
}

func callFileMethodKwargs(f *CsvFile, name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	method, err := f.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("csv.file has no attribute %q", name)
	}
	return starlark.Call(f.thread, method, args, kwargs)
}

func callWriterMethod(w *Writer, name string, args ...starlark.Value) (starlark.Value, error) {
	method, err := w.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("csv.writer has no attribute %q", name)
	}
	return starlark.Call(w.thread, method, args, nil)
}

func kw(name string, val starlark.Value) starlark.Tuple {
	return starlark.Tuple{starlark.String(name), val}
}
