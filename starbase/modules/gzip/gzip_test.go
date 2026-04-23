package gzip

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

func newTestSource(data []byte) *Source {
	return &Source{data: data, thread: &starlark.Thread{Name: "test"}, config: &starbase.ModuleConfig{}}
}

func newTestGzipFile(path string) *GzipFile {
	return &GzipFile{path: path, thread: &starlark.Thread{Name: "test"}, config: &starbase.ModuleConfig{}}
}

func newTestModule() *Module {
	return New()
}

// ============================================================================
// Source Value interface tests
// ============================================================================

func TestSourceString(t *testing.T) {
	s := newTestSource([]byte("hello"))
	if got := s.String(); got != "gzip.source(5 bytes)" {
		t.Errorf("String() = %q, want %q", got, "gzip.source(5 bytes)")
	}
}

func TestSourceType(t *testing.T) {
	s := newTestSource([]byte("x"))
	if got := s.Type(); got != "gzip.source" {
		t.Errorf("Type() = %q, want %q", got, "gzip.source")
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
// Source compress method
// ============================================================================

func TestSourceCompress(t *testing.T) {
	s := newTestSource([]byte("Hello, World!"))
	v, err := callMethod(s, "compress")
	if err != nil {
		t.Fatal(err)
	}
	compressed, ok := v.(starlark.Bytes)
	if !ok {
		t.Fatalf("compress should return Bytes, got %s", v.Type())
	}
	if len(compressed) == 0 {
		t.Error("compressed data should not be empty")
	}

	// Verify it's valid gzip
	r, err := gzip.NewReader(bytes.NewReader([]byte(string(compressed))))
	if err != nil {
		t.Fatalf("compressed data is not valid gzip: %v", err)
	}
	defer r.Close()
	content, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "Hello, World!" {
		t.Errorf("decompressed = %q, want %q", string(content), "Hello, World!")
	}
}

func TestSourceCompressLevel(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 1000)
	s := newTestSource(data)

	v1, err := callMethodKw(s, "compress", nil, kw("level", starlark.MakeInt(1)))
	if err != nil {
		t.Fatal(err)
	}
	v9, err := callMethodKw(s, "compress", nil, kw("level", starlark.MakeInt(9)))
	if err != nil {
		t.Fatal(err)
	}

	c1 := v1.(starlark.Bytes)
	c9 := v9.(starlark.Bytes)
	if len(c9) > len(c1) {
		t.Errorf("level 9 (%d) should be <= level 1 (%d)", len(c9), len(c1))
	}
}

func TestSourceCompressKwargLevel(t *testing.T) {
	s := newTestSource([]byte("test"))
	v, err := callMethodKw(s, "compress", nil, kw("level", starlark.MakeInt(9)))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.(starlark.Bytes); !ok {
		t.Fatalf("compress with kwarg level should return Bytes, got %s", v.Type())
	}
}

func TestSourceCompressInvalidLevel(t *testing.T) {
	s := newTestSource([]byte("test"))
	_, err := callMethodKw(s, "compress", nil, kw("level", starlark.MakeInt(99)))
	if err == nil {
		t.Error("compress with level=99 should fail")
	}
}

func TestSourceCompressEmpty(t *testing.T) {
	s := newTestSource([]byte{})
	v, err := callMethod(s, "compress")
	if err != nil {
		t.Fatal(err)
	}
	compressed := v.(starlark.Bytes)
	if len(compressed) == 0 {
		t.Error("even empty data produces gzip output")
	}
}

// ============================================================================
// Source compress with dest parameter
// ============================================================================

func TestSourceCompressWithDest(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "output.gz")

	s := newTestSource([]byte("hello dest"))
	v, err := callMethodKw(s, "compress", nil, kw("dest", starlark.String(dest)))
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.None {
		t.Errorf("compress with dest should return None, got %s", v.Type())
	}

	// Verify file was written and contains valid gzip
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	content, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello dest" {
		t.Errorf("decompressed = %q, want %q", string(content), "hello dest")
	}
}

func TestSourceCompressWithDestPositional(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "output.gz")

	s := newTestSource([]byte("positional dest"))
	v, err := callMethod(s, "compress", starlark.String(dest))
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.None {
		t.Errorf("compress with positional dest should return None, got %s", v.Type())
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	content, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "positional dest" {
		t.Errorf("decompressed = %q, want %q", string(content), "positional dest")
	}
}

// ============================================================================
// Source decompress method
// ============================================================================

func TestSourceDecompress(t *testing.T) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte("test data"))
	w.Close()

	s := newTestSource(buf.Bytes())
	v, err := callMethod(s, "decompress")
	if err != nil {
		t.Fatal(err)
	}
	result, ok := v.(starlark.Bytes)
	if !ok {
		t.Fatalf("decompress should return Bytes, got %s", v.Type())
	}
	if string(result) != "test data" {
		t.Errorf("decompress = %q, want %q", string(result), "test data")
	}
}

func TestSourceDecompressInvalid(t *testing.T) {
	s := newTestSource([]byte("not gzip data"))
	_, err := callMethod(s, "decompress")
	if err == nil {
		t.Error("decompress on non-gzip data should fail")
	}
}

// ============================================================================
// Source decompress with dest parameter
// ============================================================================

func TestSourceDecompressWithDest(t *testing.T) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte("decompress to file"))
	w.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "output.txt")

	s := newTestSource(buf.Bytes())
	v, err := callMethodKw(s, "decompress", nil, kw("dest", starlark.String(dest)))
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.None {
		t.Errorf("decompress with dest should return None, got %s", v.Type())
	}

	content, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "decompress to file" {
		t.Errorf("file content = %q, want %q", string(content), "decompress to file")
	}
}

func TestSourceDecompressWithDestPositional(t *testing.T) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte("positional decompress"))
	w.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "output.txt")

	s := newTestSource(buf.Bytes())
	v, err := callMethod(s, "decompress", starlark.String(dest))
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.None {
		t.Errorf("decompress with positional dest should return None, got %s", v.Type())
	}

	content, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "positional decompress" {
		t.Errorf("file content = %q, want %q", string(content), "positional decompress")
	}
}

// ============================================================================
// Source compress/decompress round-trip
// ============================================================================

func TestSourceRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"simple text", []byte("Hello, World!")},
		{"empty", []byte{}},
		{"binary", []byte{0x00, 0x01, 0x02, 0xff}},
		{"large", bytes.Repeat([]byte("abcdefgh"), 10000)},
		{"multiline", []byte("line1\nline2\nline3\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newTestSource(tt.data)
			compressed, err := callMethod(src, "compress")
			if err != nil {
				t.Fatal(err)
			}

			src2 := newTestSource([]byte(string(compressed.(starlark.Bytes))))
			decompressed, err := callMethod(src2, "decompress")
			if err != nil {
				t.Fatal(err)
			}

			got := []byte(string(decompressed.(starlark.Bytes)))
			if !bytes.Equal(got, tt.data) {
				t.Errorf("round-trip failed: got %d bytes, want %d bytes", len(got), len(tt.data))
			}
		})
	}
}

// ============================================================================
// Source try_ dispatch
// ============================================================================

func TestSourceTryCompress(t *testing.T) {
	s := newTestSource([]byte("hello"))
	v, err := s.Attr("try_compress")
	if err != nil {
		t.Fatal(err)
	}
	if v == nil {
		t.Fatal("try_compress should return a builtin")
	}

	result, err := starlark.Call(s.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := result.(*starbase.Result)
	if !ok {
		t.Fatalf("try_compress should return Result, got %T", result)
	}
	okVal, _ := r.Attr("ok")
	if okVal != starlark.True {
		t.Error("try_compress on valid data should have ok=True")
	}
}

func TestSourceTryDecompressFailure(t *testing.T) {
	s := newTestSource([]byte("not gzip"))
	v, err := s.Attr("try_decompress")
	if err != nil {
		t.Fatal(err)
	}

	result, err := starlark.Call(s.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*starbase.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.False {
		t.Error("try_decompress on invalid data should have ok=False")
	}
	errVal, _ := r.Attr("error")
	errStr, _ := starlark.AsString(errVal)
	if errStr == "" {
		t.Error("try_decompress error should not be empty")
	}
}

func TestSourceTryDecompressSuccess(t *testing.T) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte("hello"))
	w.Close()

	s := newTestSource(buf.Bytes())
	v, _ := s.Attr("try_decompress")
	result, err := starlark.Call(s.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*starbase.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.True {
		t.Error("try_decompress should succeed on valid gzip")
	}
	val, _ := r.Attr("value")
	if string(val.(starlark.Bytes)) != "hello" {
		t.Errorf("try_decompress value = %q, want %q", val, "hello")
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
	required := []string{"data", "compress", "decompress", "try_compress", "try_decompress"}
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
// GzipFile Value interface tests
// ============================================================================

func TestGzipFileString(t *testing.T) {
	f := newTestGzipFile("/tmp/data.gz")
	if got := f.String(); got != `gzip.file("/tmp/data.gz")` {
		t.Errorf("String() = %q, want %q", got, `gzip.file("/tmp/data.gz")`)
	}
}

func TestGzipFileType(t *testing.T) {
	f := newTestGzipFile("/tmp/data.gz")
	if got := f.Type(); got != "gzip.file" {
		t.Errorf("Type() = %q, want %q", got, "gzip.file")
	}
}

func TestGzipFileTruth(t *testing.T) {
	if !bool(newTestGzipFile("/tmp/data.gz").Truth()) {
		t.Error("non-empty path should be truthy")
	}
	if bool(newTestGzipFile("").Truth()) {
		t.Error("empty path should be falsy")
	}
}

func TestGzipFileHash(t *testing.T) {
	f := newTestGzipFile("/tmp/data.gz")
	_, err := f.Hash()
	if err == nil {
		t.Error("Hash() should return error (unhashable)")
	}
}

func TestGzipFileAttrNames(t *testing.T) {
	f := newTestGzipFile("/tmp/data.gz")
	names := f.AttrNames()
	required := []string{"compress", "decompress", "try_compress", "try_decompress"}
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
// GzipFile compress/decompress
// ============================================================================

func TestGzipFileCompress(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	gz := filepath.Join(dir, "output.gz")
	os.WriteFile(src, []byte("compress me"), 0644)

	f := newTestGzipFile(gz)
	v, err := callGzipFileMethod(f, "compress", starlark.String(src))
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.None {
		t.Errorf("compress should return None, got %s", v.Type())
	}

	// Verify output
	data, err := os.ReadFile(gz)
	if err != nil {
		t.Fatal(err)
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	content, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "compress me" {
		t.Errorf("decompressed = %q, want %q", string(content), "compress me")
	}
}

func TestGzipFileCompressWithLevel(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	gz := filepath.Join(dir, "output.gz")
	os.WriteFile(src, []byte("level test"), 0644)

	f := newTestGzipFile(gz)
	_, err := callGzipFileMethodKw(f, "compress", starlark.Tuple{starlark.String(src)},
		kw("level", starlark.MakeInt(9)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(gz); err != nil {
		t.Error("output file should exist")
	}
}

func TestGzipFileDecompress(t *testing.T) {
	dir := t.TempDir()
	gz := filepath.Join(dir, "data.gz")
	out := filepath.Join(dir, "output.txt")

	// Create a gzip file
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte("decompress me"))
	w.Close()
	os.WriteFile(gz, buf.Bytes(), 0644)

	f := newTestGzipFile(gz)
	v, err := callGzipFileMethod(f, "decompress", starlark.String(out))
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.None {
		t.Errorf("decompress should return None, got %s", v.Type())
	}

	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "decompress me" {
		t.Errorf("output = %q, want %q", string(content), "decompress me")
	}
}

func TestGzipFileDecompressAutoName(t *testing.T) {
	dir := t.TempDir()
	gz := filepath.Join(dir, "data.gz")

	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte("auto name"))
	w.Close()
	os.WriteFile(gz, buf.Bytes(), 0644)

	f := newTestGzipFile(gz)
	_, err := callGzipFileMethod(f, "decompress")
	if err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(dir, "data")
	content, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("expected file %q not created: %v", expected, err)
	}
	if string(content) != "auto name" {
		t.Errorf("output = %q, want %q", string(content), "auto name")
	}
}

func TestGzipFileDecompressNoGzSuffix(t *testing.T) {
	f := newTestGzipFile("/tmp/data.bin")
	_, err := callGzipFileMethod(f, "decompress")
	if err == nil {
		t.Error("decompress without .gz suffix and no dest should fail")
	}
}

// ============================================================================
// GzipFile round-trip
// ============================================================================

func TestGzipFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "original.txt")
	gz := filepath.Join(dir, "data.gz")
	out := filepath.Join(dir, "restored.txt")

	os.WriteFile(src, []byte("round trip file data"), 0644)

	// Compress
	cf := newTestGzipFile(gz)
	_, err := callGzipFileMethod(cf, "compress", starlark.String(src))
	if err != nil {
		t.Fatal(err)
	}

	// Decompress
	df := newTestGzipFile(gz)
	_, err = callGzipFileMethod(df, "decompress", starlark.String(out))
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "round trip file data" {
		t.Errorf("round-trip = %q, want %q", string(content), "round trip file data")
	}
}

// ============================================================================
// GzipFile DryRun
// ============================================================================

func TestGzipFileCompressDryRun(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	gz := filepath.Join(dir, "output.gz")
	os.WriteFile(src, []byte("dry run"), 0644)

	f := &GzipFile{
		path:   gz,
		thread: &starlark.Thread{Name: "test"},
		config: &starbase.ModuleConfig{DryRun: true},
	}
	_, err := callGzipFileMethod(f, "compress", starlark.String(src))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(gz); err == nil {
		t.Error("compress in DryRun should not create file")
	}
}

// ============================================================================
// GzipFile try_ dispatch
// ============================================================================

func TestGzipFileTryCompressSuccess(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	gz := filepath.Join(dir, "output.gz")
	os.WriteFile(src, []byte("try compress"), 0644)

	f := newTestGzipFile(gz)
	v, _ := f.Attr("try_compress")
	result, err := starlark.Call(f.thread, v, starlark.Tuple{starlark.String(src)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*starbase.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.True {
		t.Error("try_compress should succeed")
	}
}

func TestGzipFileTryDecompressFailure(t *testing.T) {
	dir := t.TempDir()
	gz := filepath.Join(dir, "bad.gz")
	os.WriteFile(gz, []byte("not gzip"), 0644)

	f := newTestGzipFile(gz)
	v, _ := f.Attr("try_decompress")
	result, err := starlark.Call(f.thread, v, starlark.Tuple{starlark.String(filepath.Join(dir, "out.txt"))}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*starbase.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.False {
		t.Error("try_decompress on invalid data should have ok=False")
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

	v, err := m.fileFactory(thread, nil, starlark.Tuple{starlark.String("/tmp/test.gz")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	gf, ok := v.(*GzipFile)
	if !ok {
		t.Fatalf("fileFactory should return *GzipFile, got %T", v)
	}
	if gf.path != "/tmp/test.gz" {
		t.Errorf("path = %q, want %q", gf.path, "/tmp/test.gz")
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
	if got := m.Name(); got != "gzip" {
		t.Errorf("Name() = %q, want %q", got, "gzip")
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
	if _, ok := dict["gzip"]; !ok {
		t.Error("Load should return dict with 'gzip' key")
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

func callMethod(s *Source, name string, args ...starlark.Value) (starlark.Value, error) {
	method, err := s.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("gzip.source has no attribute %q", name)
	}
	return starlark.Call(s.thread, method, args, nil)
}

func callMethodKw(s *Source, name string, args starlark.Tuple, kwargs ...starlark.Tuple) (starlark.Value, error) {
	method, err := s.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("gzip.source has no attribute %q", name)
	}
	return starlark.Call(s.thread, method, args, kwargs)
}

func callGzipFileMethod(f *GzipFile, name string, args ...starlark.Value) (starlark.Value, error) {
	method, err := f.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("gzip.file has no attribute %q", name)
	}
	return starlark.Call(f.thread, method, args, nil)
}

func callGzipFileMethodKw(f *GzipFile, name string, args starlark.Tuple, kwargs ...starlark.Tuple) (starlark.Value, error) {
	method, err := f.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("gzip.file has no attribute %q", name)
	}
	return starlark.Call(f.thread, method, args, kwargs)
}

func kw(name string, val starlark.Value) starlark.Tuple {
	return starlark.Tuple{starlark.String(name), val}
}
