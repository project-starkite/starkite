package hash

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func newTestSource(data []byte) *Source {
	return &Source{data: data, thread: &starlark.Thread{Name: "test"}, config: &libkite.ModuleConfig{}}
}

func newTestHashFile(path string) *HashFile {
	return &HashFile{path: path, thread: &starlark.Thread{Name: "test"}, config: &libkite.ModuleConfig{}}
}

func newTestModule() *Module {
	return New()
}

// ============================================================================
// Source Value interface tests
// ============================================================================

func TestSourceString(t *testing.T) {
	s := newTestSource([]byte("hello"))
	if got := s.String(); got != "hash.source(5 bytes)" {
		t.Errorf("String() = %q, want %q", got, "hash.source(5 bytes)")
	}
}

func TestSourceType(t *testing.T) {
	s := newTestSource([]byte("x"))
	if got := s.Type(); got != "hash.source" {
		t.Errorf("Type() = %q, want %q", got, "hash.source")
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
// Source hash methods
// ============================================================================

func TestSourceMD5(t *testing.T) {
	s := newTestSource([]byte("hello"))
	v, err := callSourceMethod(s, "md5")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	want := "5d41402abc4b2a76b9719d911017c592"
	if got != want {
		t.Errorf("md5 = %q, want %q", got, want)
	}
}

func TestSourceSHA1(t *testing.T) {
	s := newTestSource([]byte("hello"))
	v, err := callSourceMethod(s, "sha1")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	want := "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
	if got != want {
		t.Errorf("sha1 = %q, want %q", got, want)
	}
}

func TestSourceSHA256(t *testing.T) {
	s := newTestSource([]byte("hello"))
	v, err := callSourceMethod(s, "sha256")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("sha256 = %q, want %q", got, want)
	}
}

func TestSourceSHA512(t *testing.T) {
	s := newTestSource([]byte("hello"))
	v, err := callSourceMethod(s, "sha512")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	if len(got) != 128 {
		t.Errorf("sha512 length = %d, want 128", len(got))
	}
}

func TestSourceMD5Empty(t *testing.T) {
	s := newTestSource([]byte(""))
	v, err := callSourceMethod(s, "md5")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	want := "d41d8cd98f00b204e9800998ecf8427e"
	if got != want {
		t.Errorf("md5('') = %q, want %q", got, want)
	}
}

func TestSourceHashMethodNoArgs(t *testing.T) {
	s := newTestSource([]byte("hello"))
	_, err := callSourceMethodArgs(s, "md5", starlark.Tuple{starlark.String("extra")}, nil)
	if err == nil {
		t.Error("md5 with args should fail")
	}
}

// ============================================================================
// Source try_ dispatch
// ============================================================================

func TestSourceTryMD5(t *testing.T) {
	s := newTestSource([]byte("hello"))
	v, err := s.Attr("try_md5")
	if err != nil {
		t.Fatal(err)
	}
	if v == nil {
		t.Fatal("try_md5 should return a builtin")
	}

	result, err := starlark.Call(s.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := result.(*libkite.Result)
	if !ok {
		t.Fatalf("try_md5 should return Result, got %T", result)
	}
	okVal, _ := r.Attr("ok")
	if okVal != starlark.True {
		t.Error("try_md5 should have ok=True")
	}
	val, _ := r.Attr("value")
	if string(val.(starlark.String)) != "5d41402abc4b2a76b9719d911017c592" {
		t.Errorf("try_md5 value = %q", val)
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
	required := []string{"data", "md5", "sha1", "sha256", "sha512", "try_md5", "try_sha1", "try_sha256", "try_sha512"}
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
// HashFile Value interface tests
// ============================================================================

func TestHashFileString(t *testing.T) {
	f := newTestHashFile("/tmp/data.txt")
	if got := f.String(); got != `hash.file("/tmp/data.txt")` {
		t.Errorf("String() = %q, want %q", got, `hash.file("/tmp/data.txt")`)
	}
}

func TestHashFileType(t *testing.T) {
	f := newTestHashFile("/tmp/data.txt")
	if got := f.Type(); got != "hash.file" {
		t.Errorf("Type() = %q, want %q", got, "hash.file")
	}
}

func TestHashFileTruth(t *testing.T) {
	if !bool(newTestHashFile("/tmp/data.txt").Truth()) {
		t.Error("non-empty path should be truthy")
	}
	if bool(newTestHashFile("").Truth()) {
		t.Error("empty path should be falsy")
	}
}

func TestHashFileHash(t *testing.T) {
	f := newTestHashFile("/tmp/data.txt")
	_, err := f.Hash()
	if err == nil {
		t.Error("Hash() should return error (unhashable)")
	}
}

func TestHashFilePathProperty(t *testing.T) {
	f := newTestHashFile("/tmp/data.txt")
	v, err := f.Attr("path")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	if got != "/tmp/data.txt" {
		t.Errorf("path = %q, want %q", got, "/tmp/data.txt")
	}
}

func TestHashFileAttrNames(t *testing.T) {
	f := newTestHashFile("/tmp/data.txt")
	names := f.AttrNames()
	required := []string{"path", "md5", "sha1", "sha256", "sha512", "try_md5", "try_sha1", "try_sha256", "try_sha512"}
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
// HashFile hash methods
// ============================================================================

func TestHashFileSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	f := newTestHashFile(path)
	v, err := callFileMethod(f, "sha256")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("sha256 = %q, want %q", got, want)
	}
}

func TestHashFileMD5(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	f := newTestHashFile(path)
	v, err := callFileMethod(f, "md5")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	want := "5d41402abc4b2a76b9719d911017c592"
	if got != want {
		t.Errorf("md5 = %q, want %q", got, want)
	}
}

func TestHashFileMissing(t *testing.T) {
	f := newTestHashFile("/tmp/nonexistent_hash_test_file.txt")
	_, err := callFileMethod(f, "sha256")
	if err == nil {
		t.Error("sha256 on missing file should fail")
	}
}

func TestHashFileMethodNoArgs(t *testing.T) {
	f := newTestHashFile("/tmp/test.txt")
	_, err := callFileMethodArgs(f, "sha256", starlark.Tuple{starlark.String("extra")}, nil)
	if err == nil {
		t.Error("sha256 with args should fail")
	}
}

// ============================================================================
// HashFile DryRun
// ============================================================================

func TestHashFileDryRun(t *testing.T) {
	f := &HashFile{
		path:   "/tmp/nonexistent_file.txt",
		thread: &starlark.Thread{Name: "test"},
		config: &libkite.ModuleConfig{DryRun: true},
	}
	v, err := callFileMethod(f, "sha256")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.(starlark.String))
	if len(got) != 64 {
		t.Errorf("DryRun sha256 length = %d, want 64", len(got))
	}
	want := "0000000000000000000000000000000000000000000000000000000000000000"
	if got != want {
		t.Errorf("DryRun sha256 = %q, want all zeros", got)
	}
}

// ============================================================================
// HashFile try_ dispatch
// ============================================================================

func TestHashFileTrySHA256Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	f := newTestHashFile(path)
	v, _ := f.Attr("try_sha256")
	result, err := starlark.Call(f.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*libkite.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.True {
		t.Error("try_sha256 should succeed")
	}
}

func TestHashFileTrySHA256Failure(t *testing.T) {
	f := newTestHashFile("/tmp/nonexistent_hash_test_file.txt")
	v, _ := f.Attr("try_sha256")
	result, err := starlark.Call(f.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*libkite.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.False {
		t.Error("try_sha256 on missing file should have ok=False")
	}
	errVal, _ := r.Attr("error")
	errStr, _ := starlark.AsString(errVal)
	if errStr == "" {
		t.Error("try_sha256 error should not be empty")
	}
}

// ============================================================================
// Module factory tests
// ============================================================================

func TestModuleTextFactory(t *testing.T) {
	m := newTestModule()
	m.config = &libkite.ModuleConfig{}
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
	m.config = &libkite.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	_, err := m.textFactory(thread, nil, starlark.Tuple{starlark.MakeInt(42)}, nil)
	if err == nil {
		t.Error("textFactory with int should fail")
	}
}

func TestModuleBytesFactory(t *testing.T) {
	m := newTestModule()
	m.config = &libkite.ModuleConfig{}
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
	m.config = &libkite.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	_, err := m.bytesFactory(thread, nil, starlark.Tuple{starlark.MakeInt(42)}, nil)
	if err == nil {
		t.Error("bytesFactory with int should fail")
	}
}

func TestModuleFileFactory(t *testing.T) {
	m := newTestModule()
	m.config = &libkite.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	v, err := m.fileFactory(thread, nil, starlark.Tuple{starlark.String("/tmp/test.txt")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	hf, ok := v.(*HashFile)
	if !ok {
		t.Fatalf("fileFactory should return *HashFile, got %T", v)
	}
	if hf.path != "/tmp/test.txt" {
		t.Errorf("path = %q, want %q", hf.path, "/tmp/test.txt")
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
	if got := m.Name(); got != "hash" {
		t.Errorf("Name() = %q, want %q", got, "hash")
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
	if _, ok := dict["hash"]; !ok {
		t.Error("Load should return dict with 'hash' key")
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
		return nil, fmt.Errorf("hash.source has no attribute %q", name)
	}
	return starlark.Call(s.thread, method, args, nil)
}

func callSourceMethodArgs(s *Source, name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	method, err := s.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("hash.source has no attribute %q", name)
	}
	return starlark.Call(s.thread, method, args, kwargs)
}

func callFileMethod(f *HashFile, name string, args ...starlark.Value) (starlark.Value, error) {
	method, err := f.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("hash.file has no attribute %q", name)
	}
	return starlark.Call(f.thread, method, args, nil)
}

func callFileMethodArgs(f *HashFile, name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	method, err := f.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("hash.file has no attribute %q", name)
	}
	return starlark.Call(f.thread, method, args, kwargs)
}

func kw(name string, val starlark.Value) starlark.Tuple {
	return starlark.Tuple{starlark.String(name), val}
}
