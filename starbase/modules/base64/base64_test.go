package base64

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
)

func newTestSource(data []byte) *Source {
	return &Source{data: data, thread: &starlark.Thread{Name: "test"}, config: &starbase.ModuleConfig{}}
}

func newTestBase64File(path string) *Base64File {
	return &Base64File{path: path, thread: &starlark.Thread{Name: "test"}, config: &starbase.ModuleConfig{}}
}

func newTestModule() *Module {
	return New()
}

// ============================================================================
// Source Value interface tests
// ============================================================================

func TestSourceString(t *testing.T) {
	s := newTestSource([]byte("hello"))
	if got := s.String(); got != "base64.source(5 bytes)" {
		t.Errorf("String() = %q, want %q", got, "base64.source(5 bytes)")
	}
}

func TestSourceType(t *testing.T) {
	s := newTestSource([]byte("x"))
	if got := s.Type(); got != "base64.source" {
		t.Errorf("Type() = %q, want %q", got, "base64.source")
	}
}

func TestSourceTruth(t *testing.T) {
	tests := []struct {
		data []byte
		want bool
	}{
		{[]byte("hello"), true},
		{[]byte{}, false},
		{nil, false},
	}
	for _, tt := range tests {
		s := newTestSource(tt.data)
		if got := bool(s.Truth()); got != tt.want {
			t.Errorf("Truth(%v) = %v, want %v", tt.data, got, tt.want)
		}
	}
}

func TestSourceHash(t *testing.T) {
	s := newTestSource([]byte("hello"))
	_, err := s.Hash()
	if err == nil {
		t.Error("Hash() should return error (unhashable)")
	}
}

func TestSourceFreeze(t *testing.T) {
	s := newTestSource([]byte("hello"))
	s.Freeze() // should not panic
}

// ============================================================================
// Source properties
// ============================================================================

func TestSourceDataProperty(t *testing.T) {
	s := newTestSource([]byte("hello"))
	v, err := s.Attr("data")
	if err != nil {
		t.Fatal(err)
	}
	b, ok := v.(starlark.Bytes)
	if !ok {
		t.Fatalf("data should be Bytes, got %s", v.Type())
	}
	if string(b) != "hello" {
		t.Errorf("data = %q, want %q", string(b), "hello")
	}
}

// ============================================================================
// Source encode/decode methods
// ============================================================================

func TestSourceEncode(t *testing.T) {
	s := newTestSource([]byte("hello"))
	v, err := callSourceMethod(s, "encode")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	want := "aGVsbG8="
	if got != want {
		t.Errorf("encode = %q, want %q", got, want)
	}
}

func TestSourceDecode(t *testing.T) {
	s := newTestSource([]byte("aGVsbG8="))
	v, err := callSourceMethod(s, "decode")
	if err != nil {
		t.Fatal(err)
	}
	b, ok := v.(starlark.Bytes)
	if !ok {
		t.Fatalf("decode should return Bytes, got %s", v.Type())
	}
	if string(b) != "hello" {
		t.Errorf("decode = %q, want %q", string(b), "hello")
	}
}

func TestSourceDecodeReturnsBytes(t *testing.T) {
	s := newTestSource([]byte("aGVsbG8="))
	v, err := callSourceMethod(s, "decode")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.(starlark.Bytes); !ok {
		t.Errorf("decode should return starlark.Bytes, got %T", v)
	}
}

func TestSourceEncodeURL(t *testing.T) {
	s := newTestSource([]byte("hello"))
	v, err := callSourceMethod(s, "encode_url")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	want := "aGVsbG8="
	if got != want {
		t.Errorf("encode_url = %q, want %q", got, want)
	}
}

func TestSourceDecodeURL(t *testing.T) {
	s := newTestSource([]byte("aGVsbG8="))
	v, err := callSourceMethod(s, "decode_url")
	if err != nil {
		t.Fatal(err)
	}
	b, ok := v.(starlark.Bytes)
	if !ok {
		t.Fatalf("decode_url should return Bytes, got %s", v.Type())
	}
	if string(b) != "hello" {
		t.Errorf("decode_url = %q, want %q", string(b), "hello")
	}
}

func TestSourceEncodeEmpty(t *testing.T) {
	s := newTestSource([]byte(""))
	v, err := callSourceMethod(s, "encode")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	if got != "" {
		t.Errorf("encode('') = %q, want %q", got, "")
	}
}

func TestSourceDecodeEmpty(t *testing.T) {
	s := newTestSource([]byte(""))
	v, err := callSourceMethod(s, "decode")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.Bytes))
	if got != "" {
		t.Errorf("decode('') = %q, want %q", got, "")
	}
}

func TestSourceDecodeInvalid(t *testing.T) {
	s := newTestSource([]byte("!!!invalid!!!"))
	_, err := callSourceMethod(s, "decode")
	if err == nil {
		t.Error("decode of invalid base64 should fail")
	}
}

func TestSourceDecodeURLInvalid(t *testing.T) {
	s := newTestSource([]byte("!!!invalid!!!"))
	_, err := callSourceMethod(s, "decode_url")
	if err == nil {
		t.Error("decode_url of invalid base64 should fail")
	}
}

func TestSourceKnownValues(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Man", "TWFu"},
		{"Ma", "TWE="},
		{"M", "TQ=="},
	}
	for _, tt := range tests {
		s := newTestSource([]byte(tt.input))
		v, err := callSourceMethod(s, "encode")
		if err != nil {
			t.Fatal(err)
		}
		got := string(v.(starlark.String))
		if got != tt.want {
			t.Errorf("encode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSourceMethodNoArgs(t *testing.T) {
	s := newTestSource([]byte("hello"))
	_, err := callSourceMethodArgs(s, "encode", starlark.Tuple{starlark.String("extra")}, nil)
	if err == nil {
		t.Error("encode with args should fail")
	}
}

// ============================================================================
// Source try_ dispatch
// ============================================================================

func TestSourceTryEncode(t *testing.T) {
	s := newTestSource([]byte("hello"))
	v, err := s.Attr("try_encode")
	if err != nil {
		t.Fatal(err)
	}
	if v == nil {
		t.Fatal("try_encode should return a builtin")
	}

	result, err := starlark.Call(s.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := result.(*starbase.Result)
	if !ok {
		t.Fatalf("try_encode should return Result, got %T", result)
	}
	okVal, _ := r.Attr("ok")
	if okVal != starlark.True {
		t.Error("try_encode should have ok=True")
	}
	val, _ := r.Attr("value")
	if string(val.(starlark.String)) != "aGVsbG8=" {
		t.Errorf("try_encode value = %q", val)
	}
}

func TestSourceTryDecodeSuccess(t *testing.T) {
	s := newTestSource([]byte("aGVsbG8="))
	v, _ := s.Attr("try_decode")
	result, err := starlark.Call(s.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*starbase.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.True {
		t.Error("try_decode should have ok=True")
	}
	val, _ := r.Attr("value")
	if string(val.(starlark.Bytes)) != "hello" {
		t.Errorf("try_decode value = %q", val)
	}
}

func TestSourceTryDecodeFailure(t *testing.T) {
	s := newTestSource([]byte("!!!invalid!!!"))
	v, _ := s.Attr("try_decode")
	result, err := starlark.Call(s.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*starbase.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.False {
		t.Error("try_decode on invalid input should have ok=False")
	}
	errVal, _ := r.Attr("error")
	errStr, _ := starlark.AsString(errVal)
	if errStr == "" {
		t.Error("try_decode error should not be empty")
	}
}

func TestSourceTryUnknownMethod(t *testing.T) {
	s := newTestSource([]byte("x"))
	v, err := s.Attr("try_nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Error("try_nonexistent should return nil")
	}
}

// ============================================================================
// Source AttrNames
// ============================================================================

func TestSourceAttrNames(t *testing.T) {
	s := newTestSource([]byte("x"))
	names := s.AttrNames()
	required := []string{
		"data",
		"decode", "decode_url", "encode", "encode_url",
		"try_decode", "try_decode_url", "try_encode", "try_encode_url",
	}
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
// Base64File Value interface tests
// ============================================================================

func TestBase64FileString(t *testing.T) {
	f := newTestBase64File("/tmp/data.txt")
	if got := f.String(); got != `base64.file("/tmp/data.txt")` {
		t.Errorf("String() = %q, want %q", got, `base64.file("/tmp/data.txt")`)
	}
}

func TestBase64FileType(t *testing.T) {
	f := newTestBase64File("/tmp/data.txt")
	if got := f.Type(); got != "base64.file" {
		t.Errorf("Type() = %q, want %q", got, "base64.file")
	}
}

func TestBase64FileTruth(t *testing.T) {
	if !bool(newTestBase64File("/tmp/data.txt").Truth()) {
		t.Error("non-empty path should be truthy")
	}
	if bool(newTestBase64File("").Truth()) {
		t.Error("empty path should be falsy")
	}
}

func TestBase64FileHash(t *testing.T) {
	f := newTestBase64File("/tmp/data.txt")
	_, err := f.Hash()
	if err == nil {
		t.Error("Hash() should return error (unhashable)")
	}
}

func TestBase64FilePathProperty(t *testing.T) {
	f := newTestBase64File("/tmp/data.txt")
	v, err := f.Attr("path")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	if got != "/tmp/data.txt" {
		t.Errorf("path = %q, want %q", got, "/tmp/data.txt")
	}
}

func TestBase64FileAttrNames(t *testing.T) {
	f := newTestBase64File("/tmp/data.txt")
	names := f.AttrNames()
	required := []string{
		"path",
		"decode", "decode_url", "encode", "encode_url",
		"try_decode", "try_decode_url", "try_encode", "try_encode_url",
	}
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
// Base64File encode/decode methods
// ============================================================================

func TestBase64FileEncode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	f := newTestBase64File(path)
	v, err := callFileMethod(f, "encode")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	want := "aGVsbG8="
	if got != want {
		t.Errorf("encode = %q, want %q", got, want)
	}
}

func TestBase64FileDecode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "encoded.txt")
	os.WriteFile(path, []byte("aGVsbG8="), 0644)

	f := newTestBase64File(path)
	v, err := callFileMethod(f, "decode")
	if err != nil {
		t.Fatal(err)
	}
	b, ok := v.(starlark.Bytes)
	if !ok {
		t.Fatalf("decode should return Bytes, got %s", v.Type())
	}
	if string(b) != "hello" {
		t.Errorf("decode = %q, want %q", string(b), "hello")
	}
}

func TestBase64FileEncodeURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	f := newTestBase64File(path)
	v, err := callFileMethod(f, "encode_url")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	want := "aGVsbG8="
	if got != want {
		t.Errorf("encode_url = %q, want %q", got, want)
	}
}

func TestBase64FileDecodeURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "encoded.txt")
	os.WriteFile(path, []byte("aGVsbG8="), 0644)

	f := newTestBase64File(path)
	v, err := callFileMethod(f, "decode_url")
	if err != nil {
		t.Fatal(err)
	}
	b, ok := v.(starlark.Bytes)
	if !ok {
		t.Fatalf("decode_url should return Bytes, got %s", v.Type())
	}
	if string(b) != "hello" {
		t.Errorf("decode_url = %q, want %q", string(b), "hello")
	}
}

func TestBase64FileMissing(t *testing.T) {
	f := newTestBase64File("/tmp/nonexistent_base64_test_file.txt")
	_, err := callFileMethod(f, "encode")
	if err == nil {
		t.Error("encode on missing file should fail")
	}
}

func TestBase64FileDecodeInvalidContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.txt")
	os.WriteFile(path, []byte("!!!invalid!!!"), 0644)

	f := newTestBase64File(path)
	_, err := callFileMethod(f, "decode")
	if err == nil {
		t.Error("decode of file with invalid base64 should fail")
	}
}

func TestBase64FileMethodNoArgs(t *testing.T) {
	f := newTestBase64File("/tmp/test.txt")
	_, err := callFileMethodArgs(f, "encode", starlark.Tuple{starlark.String("extra")}, nil)
	if err == nil {
		t.Error("encode with args should fail")
	}
}

// ============================================================================
// Base64File DryRun
// ============================================================================

func TestBase64FileDryRunEncode(t *testing.T) {
	f := &Base64File{
		path:   "/tmp/nonexistent_file.txt",
		thread: &starlark.Thread{Name: "test"},
		config: &starbase.ModuleConfig{DryRun: true},
	}
	v, err := callFileMethod(f, "encode")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	if got != "" {
		t.Errorf("DryRun encode = %q, want empty string", got)
	}
}

func TestBase64FileDryRunDecode(t *testing.T) {
	f := &Base64File{
		path:   "/tmp/nonexistent_file.txt",
		thread: &starlark.Thread{Name: "test"},
		config: &starbase.ModuleConfig{DryRun: true},
	}
	v, err := callFileMethod(f, "decode")
	if err != nil {
		t.Fatal(err)
	}
	b, ok := v.(starlark.Bytes)
	if !ok {
		t.Fatalf("DryRun decode should return Bytes, got %T", v)
	}
	if string(b) != "" {
		t.Errorf("DryRun decode = %q, want empty bytes", string(b))
	}
}

// ============================================================================
// Base64File try_ dispatch
// ============================================================================

func TestBase64FileTryEncodeSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	f := newTestBase64File(path)
	v, _ := f.Attr("try_encode")
	result, err := starlark.Call(f.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*starbase.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.True {
		t.Error("try_encode should succeed")
	}
}

func TestBase64FileTryEncodeFailure(t *testing.T) {
	f := newTestBase64File("/tmp/nonexistent_base64_test_file.txt")
	v, _ := f.Attr("try_encode")
	result, err := starlark.Call(f.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*starbase.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.False {
		t.Error("try_encode on missing file should have ok=False")
	}
	errVal, _ := r.Attr("error")
	errStr, _ := starlark.AsString(errVal)
	if errStr == "" {
		t.Error("try_encode error should not be empty")
	}
}

// ============================================================================
// Module factory tests
// ============================================================================

func TestModuleTextFactory(t *testing.T) {
	m := newTestModule()
	m.config = &starbase.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	v, err := m.textFactory(thread, nil, starlark.Tuple{starlark.String("hello")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	src, ok := v.(*Source)
	if !ok {
		t.Fatalf("textFactory should return *Source, got %T", v)
	}
	if string(src.data) != "hello" {
		t.Errorf("source data = %q, want %q", string(src.data), "hello")
	}
}

func TestModuleTextFactoryWrongArgCount(t *testing.T) {
	m := newTestModule()
	thread := &starlark.Thread{Name: "test"}

	_, err := m.textFactory(thread, nil, starlark.Tuple{}, nil)
	if err == nil {
		t.Error("textFactory with 0 args should fail")
	}
}

func TestModuleTextFactoryWrongType(t *testing.T) {
	m := newTestModule()
	m.config = &starbase.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	_, err := m.textFactory(thread, nil, starlark.Tuple{starlark.MakeInt(42)}, nil)
	if err == nil {
		t.Error("textFactory with int should fail")
	}
}

func TestModuleBytesFactory(t *testing.T) {
	m := newTestModule()
	m.config = &starbase.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	// Test with Bytes
	v, err := m.bytesFactory(thread, nil, starlark.Tuple{starlark.Bytes("hello")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	src := v.(*Source)
	if string(src.data) != "hello" {
		t.Errorf("bytesFactory(Bytes) data = %q, want %q", string(src.data), "hello")
	}

	// Test with String
	v, err = m.bytesFactory(thread, nil, starlark.Tuple{starlark.String("world")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	src = v.(*Source)
	if string(src.data) != "world" {
		t.Errorf("bytesFactory(String) data = %q, want %q", string(src.data), "world")
	}
}

func TestModuleBytesFactoryWrongType(t *testing.T) {
	m := newTestModule()
	m.config = &starbase.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	_, err := m.bytesFactory(thread, nil, starlark.Tuple{starlark.MakeInt(42)}, nil)
	if err == nil {
		t.Error("bytesFactory with int should fail")
	}
}

func TestModuleFileFactory(t *testing.T) {
	m := newTestModule()
	m.config = &starbase.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	v, err := m.fileFactory(thread, nil, starlark.Tuple{starlark.String("/tmp/test.txt")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bf, ok := v.(*Base64File)
	if !ok {
		t.Fatalf("fileFactory should return *Base64File, got %T", v)
	}
	if bf.path != "/tmp/test.txt" {
		t.Errorf("path = %q, want %q", bf.path, "/tmp/test.txt")
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

// ============================================================================
// Module interface
// ============================================================================

func TestModuleName(t *testing.T) {
	m := newTestModule()
	if got := m.Name(); got != "base64" {
		t.Errorf("Name() = %q, want %q", got, "base64")
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
	if _, ok := dict["base64"]; !ok {
		t.Error("Load should return dict with 'base64' key")
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

func callSourceMethod(s *Source, name string, args ...starlark.Value) (starlark.Value, error) {
	method, err := s.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("base64.source has no attribute %q", name)
	}
	return starlark.Call(s.thread, method, args, nil)
}

func callSourceMethodArgs(s *Source, name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	method, err := s.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("base64.source has no attribute %q", name)
	}
	return starlark.Call(s.thread, method, args, kwargs)
}

func callFileMethod(f *Base64File, name string, args ...starlark.Value) (starlark.Value, error) {
	method, err := f.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("base64.file has no attribute %q", name)
	}
	return starlark.Call(f.thread, method, args, nil)
}

func callFileMethodArgs(f *Base64File, name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	method, err := f.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("base64.file has no attribute %q", name)
	}
	return starlark.Call(f.thread, method, args, kwargs)
}
