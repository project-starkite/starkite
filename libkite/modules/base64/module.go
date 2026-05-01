// Package base64 provides base64 encoding/decoding functions for starkite.
package base64

import (
	"fmt"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "base64"

// Module implements base64 encoding/decoding with a builder/source pattern.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "base64 provides base64 encoding/decoding: file, text, bytes"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = libkite.NewTryModule(string(ModuleName), starlark.StringDict{
			"file":  starlark.NewBuiltin("base64.file", m.fileFactory),
			"text":  starlark.NewBuiltin("base64.text", m.textFactory),
			"bytes": starlark.NewBuiltin("base64.bytes", m.bytesFactory),
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// fileFactory creates a Base64File anchored at a path.
// Usage: base64.file(path)
func (m *Module) fileFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path string `name:"path" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	return &Base64File{path: p.Path, thread: thread, config: m.config}, nil
}

// textFactory creates a Source from a string.
// Usage: base64.text(s)
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
// Usage: base64.bytes(data)
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
		return nil, fmt.Errorf("base64.bytes: argument must be bytes or string, got %s", p.Data.Type())
	}
	return &Source{data: data, thread: thread, config: m.config}, nil
}
