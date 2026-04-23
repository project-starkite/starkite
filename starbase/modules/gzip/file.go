package gzip

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

// GzipFile is a Starlark value representing a .gz file path.
// Methods perform file-to-file compression/decompression.
type GzipFile struct {
	path   string
	thread *starlark.Thread
	config *starbase.ModuleConfig
}

var (
	_ starlark.Value    = (*GzipFile)(nil)
	_ starlark.HasAttrs = (*GzipFile)(nil)
)

func (f *GzipFile) String() string        { return fmt.Sprintf("gzip.file(%q)", f.path) }
func (f *GzipFile) Type() string          { return "gzip.file" }
func (f *GzipFile) Freeze()               {} // immutable
func (f *GzipFile) Truth() starlark.Bool  { return starlark.Bool(f.path != "") }
func (f *GzipFile) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: gzip.file") }

func (f *GzipFile) Attr(name string) (starlark.Value, error) {
	// try_ dispatch
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := f.methodBuiltin(base); method != nil {
			return starbase.TryWrap("gzip.file."+name, method), nil
		}
		return nil, nil
	}

	if method := f.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (f *GzipFile) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "compress":
		return starlark.NewBuiltin("gzip.file.compress", f.compressMethod)
	case "decompress":
		return starlark.NewBuiltin("gzip.file.decompress", f.decompressMethod)
	}
	return nil
}

func (f *GzipFile) AttrNames() []string {
	names := []string{
		"compress", "decompress",
		"try_compress", "try_decompress",
	}
	sort.Strings(names)
	return names
}

func (f *GzipFile) isDryRun() bool {
	return f.config != nil && f.config.DryRun
}

// compressMethod reads source file and writes compressed data to f.path.
// Usage: gzip.file("data.gz").compress("source.txt", level=-1)
func (f *GzipFile) compressMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Source string `name:"source" position:"0" required:"true"`
		Level  int    `name:"level"`
	}
	p.Level = gzip.DefaultCompression
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if p.Level < gzip.HuffmanOnly || p.Level > gzip.BestCompression {
		return nil, fmt.Errorf("gzip.file.compress: invalid compression level %d (must be -2 to 9)", p.Level)
	}

	if err := starbase.Check(f.thread, "fs", "read_file", p.Source); err != nil {
		return nil, err
	}
	if err := starbase.Check(f.thread, "fs", "write", f.path); err != nil {
		return nil, err
	}

	if f.isDryRun() {
		return starlark.None, nil
	}

	data, err := os.ReadFile(p.Source)
	if err != nil {
		return nil, fmt.Errorf("gzip.file.compress: %w", err)
	}

	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, p.Level)
	if err != nil {
		return nil, fmt.Errorf("gzip.file.compress: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		w.Close()
		return nil, fmt.Errorf("gzip.file.compress: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gzip.file.compress: %w", err)
	}

	if err := os.WriteFile(f.path, buf.Bytes(), 0644); err != nil {
		return nil, fmt.Errorf("gzip.file.compress: %w", err)
	}
	return starlark.None, nil
}

// decompressMethod reads f.path and writes decompressed data to dest.
// Usage: gzip.file("data.gz").decompress("output.txt") or gzip.file("data.gz").decompress()
func (f *GzipFile) decompressMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Dest string `name:"dest" position:"0"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	dest := p.Dest
	// Auto-derive dest by stripping .gz suffix
	if dest == "" {
		ext := filepath.Ext(f.path)
		if ext != ".gz" {
			return nil, fmt.Errorf("gzip.file.decompress: cannot derive output path (no .gz suffix); specify destination explicitly")
		}
		dest = strings.TrimSuffix(f.path, ".gz")
	}

	if err := starbase.Check(f.thread, "fs", "read_file", f.path); err != nil {
		return nil, err
	}
	if err := starbase.Check(f.thread, "fs", "write", dest); err != nil {
		return nil, err
	}

	if f.isDryRun() {
		return starlark.None, nil
	}

	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("gzip.file.decompress: %w", err)
	}

	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip.file.decompress: %w", err)
	}
	defer r.Close()

	content, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("gzip.file.decompress: %w", err)
	}

	if err := os.WriteFile(dest, content, 0644); err != nil {
		return nil, fmt.Errorf("gzip.file.decompress: %w", err)
	}
	return starlark.None, nil
}
