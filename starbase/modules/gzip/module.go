// Package gzip provides gzip compression/decompression functions for starkite.
package gzip

import (
	"fmt"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

const ModuleName starbase.ModuleName = "gzip"

// Module implements gzip compression/decompression functions.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *starbase.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "gzip provides compression/decompression: file, text, bytes"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = starbase.NewTryModule(string(ModuleName), starlark.StringDict{
			"file":  starlark.NewBuiltin("gzip.file", m.fileFactory),
			"text":  starlark.NewBuiltin("gzip.text", m.textFactory),
			"bytes": starlark.NewBuiltin("gzip.bytes", m.bytesFactory),
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// fileFactory creates a GzipFile anchored at a path.
// Usage: gzip.file(path)
func (m *Module) fileFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path string `name:"path" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	return &GzipFile{path: p.Path, thread: thread, config: m.config}, nil
}

// textFactory creates a Source from a string.
// Usage: gzip.text(s)
func (m *Module) textFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		S string `name:"s" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	return &Source{data: []byte(p.S), thread: thread, config: m.config}, nil
}

// bytesFactory creates a Source from bytes or string.
// Usage: gzip.bytes(data)
func (m *Module) bytesFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Data starlark.Value `name:"data" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	var data []byte
	switch v := p.Data.(type) {
	case starlark.Bytes:
		data = []byte(string(v))
	case starlark.String:
		data = []byte(string(v))
	default:
		return nil, fmt.Errorf("gzip.bytes: argument must be bytes or string, got %s", p.Data.Type())
	}
	return &Source{data: data, thread: thread, config: m.config}, nil
}
