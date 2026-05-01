package zip

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func newTestArchive(path string) *Archive {
	return &Archive{
		path:   path,
		thread: &starlark.Thread{Name: "test"},
		config: &libkite.ModuleConfig{},
	}
}

func newTestModule() *Module {
	return New()
}

// createTestZip creates a zip file with the given entries at path.
func createTestZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, content := range entries {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write([]byte(content))
	}
	w.Close()
}

// ============================================================================
// Archive Value interface tests
// ============================================================================

func TestArchiveString(t *testing.T) {
	a := newTestArchive("/tmp/test.zip")
	if got := a.String(); got != `zip.file("/tmp/test.zip")` {
		t.Errorf("String() = %q, want %q", got, `zip.file("/tmp/test.zip")`)
	}
}

func TestArchiveType(t *testing.T) {
	a := newTestArchive("/tmp/test.zip")
	if got := a.Type(); got != "zip.archive" {
		t.Errorf("Type() = %q, want %q", got, "zip.archive")
	}
}

func TestArchiveTruth(t *testing.T) {
	if !bool(newTestArchive("/tmp/test.zip").Truth()) {
		t.Error("non-empty path should be truthy")
	}
	if bool(newTestArchive("").Truth()) {
		t.Error("empty path should be falsy")
	}
}

func TestArchiveHash(t *testing.T) {
	a := newTestArchive("/tmp/test.zip")
	_, err := a.Hash()
	if err == nil {
		t.Error("Hash() should return error (unhashable)")
	}
}

func TestArchiveFreeze(t *testing.T) {
	a := newTestArchive("/tmp/test.zip")
	a.Freeze() // should not panic
}

func TestArchiveAttrNames(t *testing.T) {
	a := newTestArchive("/tmp/test.zip")
	names := a.AttrNames()
	required := []string{
		"namelist", "read", "read_all", "write", "write_all",
		"try_namelist", "try_read", "try_read_all", "try_write", "try_write_all",
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
	// Check sorted
	sorted := make([]string, len(names))
	copy(sorted, names)
	sort.Strings(sorted)
	for i := range names {
		if names[i] != sorted[i] {
			t.Error("AttrNames should be sorted")
			break
		}
	}
}

// ============================================================================
// namelist
// ============================================================================

func TestNamelist(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"a.txt": "aaa",
		"b.go":  "bbb",
		"c.txt": "ccc",
	})

	a := newTestArchive(zipPath)
	v, err := callArchiveMethod(a, "namelist")
	if err != nil {
		t.Fatal(err)
	}
	list := v.(*starlark.List)
	if list.Len() != 3 {
		t.Errorf("namelist length = %d, want 3", list.Len())
	}
}

func TestNamelistWithMatch(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"a.txt": "aaa",
		"b.go":  "bbb",
		"c.txt": "ccc",
	})

	a := newTestArchive(zipPath)
	v, err := callArchiveMethodKw(a, "namelist", nil, kw("match", starlark.String("*.txt")))
	if err != nil {
		t.Fatal(err)
	}
	list := v.(*starlark.List)
	if list.Len() != 2 {
		t.Errorf("namelist(match=*.txt) length = %d, want 2", list.Len())
	}
}

// ============================================================================
// read
// ============================================================================

func TestRead(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"hello.txt": "Hello World",
	})

	a := newTestArchive(zipPath)
	v, err := callArchiveMethod(a, "read", starlark.String("hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	b, ok := v.(starlark.Bytes)
	if !ok {
		t.Fatalf("read should return Bytes, got %s", v.Type())
	}
	if string(b) != "Hello World" {
		t.Errorf("read = %q, want %q", string(b), "Hello World")
	}
}

func TestReadMissing(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	createTestZip(t, zipPath, map[string]string{"a.txt": "aaa"})

	a := newTestArchive(zipPath)
	_, err := callArchiveMethod(a, "read", starlark.String("nonexistent.txt"))
	if err == nil {
		t.Error("read missing entry should fail")
	}
}

// ============================================================================
// read_all
// ============================================================================

func TestReadAll(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"a.txt": "aaa",
		"b.txt": "bbb",
	})

	a := newTestArchive(zipPath)
	v, err := callArchiveMethod(a, "read_all")
	if err != nil {
		t.Fatal(err)
	}
	dict := v.(*starlark.Dict)
	if dict.Len() != 2 {
		t.Errorf("read_all length = %d, want 2", dict.Len())
	}
}

func TestReadAllWithMatch(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"a.txt": "aaa",
		"b.go":  "bbb",
		"c.txt": "ccc",
	})

	a := newTestArchive(zipPath)
	v, err := callArchiveMethodKw(a, "read_all", nil, kw("match", starlark.String("*.txt")))
	if err != nil {
		t.Fatal(err)
	}
	dict := v.(*starlark.Dict)
	if dict.Len() != 2 {
		t.Errorf("read_all(match=*.txt) length = %d, want 2", dict.Len())
	}
}

func TestReadAllWithFiles(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"a.txt": "aaa",
		"b.txt": "bbb",
		"c.txt": "ccc",
	})

	filesList := starlark.NewList([]starlark.Value{starlark.String("a.txt"), starlark.String("c.txt")})
	a := newTestArchive(zipPath)
	v, err := callArchiveMethodKw(a, "read_all", nil, kw("files", filesList))
	if err != nil {
		t.Fatal(err)
	}
	dict := v.(*starlark.Dict)
	if dict.Len() != 2 {
		t.Errorf("read_all(files=[a,c]) length = %d, want 2", dict.Len())
	}
}

func TestReadAllMatchAndFilesMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	createTestZip(t, zipPath, map[string]string{"a.txt": "aaa"})

	filesList := starlark.NewList([]starlark.Value{starlark.String("a.txt")})
	a := newTestArchive(zipPath)
	_, err := callArchiveMethodKw(a, "read_all", nil,
		kw("match", starlark.String("*.txt")),
		kw("files", filesList),
	)
	if err == nil {
		t.Error("read_all with both match and files should fail")
	}
}

// ============================================================================
// write
// ============================================================================

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.txt")
	os.WriteFile(sourcePath, []byte("source content"), 0644)

	zipPath := filepath.Join(dir, "output.zip")
	a := newTestArchive(zipPath)
	_, err := callArchiveMethod(a, "write", starlark.String(sourcePath))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the archive
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) != 1 {
		t.Fatalf("archive should have 1 entry, got %d", len(reader.File))
	}
	if reader.File[0].Name != "source.txt" {
		t.Errorf("entry name = %q, want %q", reader.File[0].Name, "source.txt")
	}
}

func TestWriteWithNameOverride(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.txt")
	os.WriteFile(sourcePath, []byte("content"), 0644)

	zipPath := filepath.Join(dir, "output.zip")
	a := newTestArchive(zipPath)
	_, err := callArchiveMethodKw(a, "write", starlark.Tuple{starlark.String(sourcePath)},
		kw("name", starlark.String("custom.txt")),
	)
	if err != nil {
		t.Fatal(err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if reader.File[0].Name != "custom.txt" {
		t.Errorf("entry name = %q, want %q", reader.File[0].Name, "custom.txt")
	}
}

// ============================================================================
// write_all
// ============================================================================

func TestWriteAllWithFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		os.WriteFile(filepath.Join(dir, name), []byte("content of "+name), 0644)
	}

	zipPath := filepath.Join(dir, "output.zip")
	filesList := starlark.NewList([]starlark.Value{
		starlark.String(filepath.Join(dir, "a.txt")),
		starlark.String(filepath.Join(dir, "b.txt")),
	})

	a := newTestArchive(zipPath)
	_, err := callArchiveMethodKw(a, "write_all", nil, kw("files", filesList))
	if err != nil {
		t.Fatal(err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) != 2 {
		t.Fatalf("archive should have 2 entries, got %d", len(reader.File))
	}
}

func TestWriteAllWithMatch(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.go", "c.txt"} {
		os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644)
	}

	zipPath := filepath.Join(dir, "output.zip")
	a := newTestArchive(zipPath)
	_, err := callArchiveMethodKw(a, "write_all", nil, kw("match", starlark.String(filepath.Join(dir, "*.txt"))))
	if err != nil {
		t.Fatal(err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) != 2 {
		t.Fatalf("archive should have 2 .txt entries, got %d", len(reader.File))
	}
}

func TestWriteAllWithBaseDir(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "src")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "main.go"), []byte("package main"), 0644)

	zipPath := filepath.Join(dir, "output.zip")
	filesList := starlark.NewList([]starlark.Value{
		starlark.String(filepath.Join(subDir, "main.go")),
	})

	a := newTestArchive(zipPath)
	_, err := callArchiveMethodKw(a, "write_all", nil,
		kw("files", filesList),
		kw("base_dir", starlark.String(dir)),
	)
	if err != nil {
		t.Fatal(err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if reader.File[0].Name != filepath.Join("src", "main.go") {
		t.Errorf("entry name = %q, want %q", reader.File[0].Name, filepath.Join("src", "main.go"))
	}
}

func TestWriteAllMatchAndFilesMutuallyExclusive(t *testing.T) {
	a := newTestArchive("/tmp/test.zip")
	filesList := starlark.NewList([]starlark.Value{starlark.String("a.txt")})
	_, err := callArchiveMethodKw(a, "write_all", nil,
		kw("match", starlark.String("*.txt")),
		kw("files", filesList),
	)
	if err == nil {
		t.Error("write_all with both match and files should fail")
	}
}

func TestWriteAllMustSpecifySource(t *testing.T) {
	a := newTestArchive("/tmp/test.zip")
	_, err := callArchiveMethodKw(a, "write_all", nil)
	if err == nil {
		t.Error("write_all with no match or files should fail")
	}
}

// ============================================================================
// try_ dispatch
// ============================================================================

func TestTryReadSuccess(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	createTestZip(t, zipPath, map[string]string{"a.txt": "aaa"})

	a := newTestArchive(zipPath)
	v, err := a.Attr("try_read")
	if err != nil || v == nil {
		t.Fatal("try_read should return a builtin")
	}

	result, err := starlark.Call(a.thread, v, starlark.Tuple{starlark.String("a.txt")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*libkite.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.True {
		t.Error("try_read on existing entry should have ok=True")
	}
}

func TestTryReadFailure(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	createTestZip(t, zipPath, map[string]string{"a.txt": "aaa"})

	a := newTestArchive(zipPath)
	v, _ := a.Attr("try_read")
	result, err := starlark.Call(a.thread, v, starlark.Tuple{starlark.String("missing.txt")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*libkite.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.False {
		t.Error("try_read on missing entry should have ok=False")
	}
}

func TestTryUnknownMethod(t *testing.T) {
	a := newTestArchive("/tmp/test.zip")
	v, err := a.Attr("try_nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Error("try_nonexistent should return nil")
	}
}

// ============================================================================
// DryRun
// ============================================================================

func TestWriteDryRun(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.txt")
	os.WriteFile(sourcePath, []byte("content"), 0644)

	zipPath := filepath.Join(dir, "output.zip")
	a := &Archive{
		path:   zipPath,
		thread: &starlark.Thread{Name: "test"},
		config: &libkite.ModuleConfig{DryRun: true},
	}

	_, err := callArchiveMethod(a, "write", starlark.String(sourcePath))
	if err != nil {
		t.Fatal(err)
	}

	// File should not be created
	if _, err := os.Stat(zipPath); err == nil {
		t.Error("write in DryRun should not create file")
	}
}

// ============================================================================
// Module factory
// ============================================================================

func TestModuleFileFactory(t *testing.T) {
	m := newTestModule()
	m.config = &libkite.ModuleConfig{}
	thread := &starlark.Thread{Name: "test"}

	v, err := m.fileFactory(thread, nil, starlark.Tuple{starlark.String("/tmp/test.zip")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, ok := v.(*Archive)
	if !ok {
		t.Fatalf("fileFactory should return *Archive, got %T", v)
	}
	if a.path != "/tmp/test.zip" {
		t.Errorf("archive path = %q, want %q", a.path, "/tmp/test.zip")
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

func TestModuleFileFactoryWrongType(t *testing.T) {
	m := newTestModule()
	thread := &starlark.Thread{Name: "test"}
	_, err := m.fileFactory(thread, nil, starlark.Tuple{starlark.MakeInt(42)}, nil)
	if err == nil {
		t.Error("fileFactory with int should fail")
	}
}

func TestModuleName(t *testing.T) {
	m := newTestModule()
	if got := m.Name(); got != "zip" {
		t.Errorf("Name() = %q, want %q", got, "zip")
	}
}

func TestModuleDescription(t *testing.T) {
	m := newTestModule()
	if m.Description() == "" {
		t.Error("Description should not be empty")
	}
}

func TestModuleLoad(t *testing.T) {
	m := newTestModule()
	dict, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := dict["zip"]; !ok {
		t.Error("Load should return dict with 'zip' key")
	}
}

// ============================================================================
// Round-trip: write then read
// ============================================================================

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "data.txt")
	os.WriteFile(sourcePath, []byte("round trip data"), 0644)

	zipPath := filepath.Join(dir, "roundtrip.zip")

	// Write
	wa := newTestArchive(zipPath)
	_, err := callArchiveMethod(wa, "write", starlark.String(sourcePath))
	if err != nil {
		t.Fatal(err)
	}

	// Read back
	ra := newTestArchive(zipPath)
	v, err := callArchiveMethod(ra, "read", starlark.String("data.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(v.(starlark.Bytes)) != "round trip data" {
		t.Errorf("round-trip = %q, want %q", v, "round trip data")
	}
}

// ============================================================================
// Test helpers
// ============================================================================

func callArchiveMethod(a *Archive, name string, args ...starlark.Value) (starlark.Value, error) {
	method, err := a.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("zip.archive has no attribute %q", name)
	}
	return starlark.Call(a.thread, method, args, nil)
}

func callArchiveMethodKw(a *Archive, name string, args starlark.Tuple, kwargs ...starlark.Tuple) (starlark.Value, error) {
	method, err := a.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("zip.archive has no attribute %q", name)
	}
	return starlark.Call(a.thread, method, args, kwargs)
}

func kw(name string, val starlark.Value) starlark.Tuple {
	return starlark.Tuple{starlark.String(name), val}
}
