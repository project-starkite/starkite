package gzip

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

// Source is a Starlark value wrapping data for gzip compression/decompression.
type Source struct {
	data   []byte
	thread *starlark.Thread
	config *starbase.ModuleConfig
}

var (
	_ starlark.Value    = (*Source)(nil)
	_ starlark.HasAttrs = (*Source)(nil)
)

func (s *Source) String() string {
	n := len(s.data)
	return fmt.Sprintf("gzip.source(%d bytes)", n)
}
func (s *Source) Type() string          { return "gzip.source" }
func (s *Source) Freeze()               {} // immutable
func (s *Source) Truth() starlark.Bool  { return starlark.Bool(len(s.data) > 0) }
func (s *Source) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: gzip.source") }

// Attr implements starlark.HasAttrs.
func (s *Source) Attr(name string) (starlark.Value, error) {
	// try_ dispatch
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := s.methodBuiltin(base); method != nil {
			return starbase.TryWrap("gzip.source."+name, method), nil
		}
		return nil, nil
	}

	// Properties
	if name == "data" {
		return starlark.Bytes(s.data), nil
	}

	// Methods
	if method := s.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (s *Source) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "compress":
		return starlark.NewBuiltin("gzip.source.compress", s.compressMethod)
	case "decompress":
		return starlark.NewBuiltin("gzip.source.decompress", s.decompressMethod)
	}
	return nil
}

// AttrNames implements starlark.HasAttrs.
func (s *Source) AttrNames() []string {
	names := []string{
		"data", "compress", "decompress",
		"try_compress", "try_decompress",
	}
	sort.Strings(names)
	return names
}

func (s *Source) isDryRun() bool {
	return s.config != nil && s.config.DryRun
}

// compressMethod compresses the source data.
// Usage: source.compress(dest="", level=-1)
// If dest is given, writes to file and returns None; otherwise returns bytes.
// Note: level is kwarg-only (no position tag).
func (s *Source) compressMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Dest  string `name:"dest" position:"0"`
		Level int    `name:"level"`
	}
	p.Level = gzip.DefaultCompression
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	level := p.Level
	if level < gzip.HuffmanOnly || level > gzip.BestCompression {
		return nil, fmt.Errorf("gzip.compress: invalid compression level %d (must be -2 to 9)", level)
	}

	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		return nil, fmt.Errorf("gzip.compress: %w", err)
	}
	if _, err := w.Write(s.data); err != nil {
		w.Close()
		return nil, fmt.Errorf("gzip.compress: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gzip.compress: %w", err)
	}

	if p.Dest != "" {
		if err := starbase.Check(s.thread, "fs", "write", p.Dest); err != nil {
			return nil, err
		}
		if s.isDryRun() {
			return starlark.None, nil
		}
		if err := os.WriteFile(p.Dest, buf.Bytes(), 0644); err != nil {
			return nil, fmt.Errorf("gzip.compress: %w", err)
		}
		return starlark.None, nil
	}

	return starlark.Bytes(buf.String()), nil
}

// decompressMethod decompresses the source data.
// Usage: source.decompress(dest="")
// If dest is given, writes to file and returns None; otherwise returns bytes.
func (s *Source) decompressMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Dest string `name:"dest" position:"0"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	r, err := gzip.NewReader(bytes.NewReader(s.data))
	if err != nil {
		return nil, fmt.Errorf("gzip.decompress: %w", err)
	}
	defer r.Close()

	content, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("gzip.decompress: %w", err)
	}

	if p.Dest != "" {
		if err := starbase.Check(s.thread, "fs", "write", p.Dest); err != nil {
			return nil, err
		}
		if s.isDryRun() {
			return starlark.None, nil
		}
		if err := os.WriteFile(p.Dest, content, 0644); err != nil {
			return nil, fmt.Errorf("gzip.decompress: %w", err)
		}
		return starlark.None, nil
	}

	return starlark.Bytes(content), nil
}
