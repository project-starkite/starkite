// Package csv provides CSV file reading/writing for starkite.
package csv

import (
	"fmt"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "csv"

// Module implements CSV file reading/writing with builder pattern.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "csv provides CSV file reading/writing: file, from"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = libkite.NewTryModule(string(ModuleName), starlark.StringDict{
			"file":   starlark.NewBuiltin("csv.file", m.fileFactory),
			"source": starlark.NewBuiltin("csv.source", m.sourceFactory),
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// fileFactory creates a CsvFile anchored at a path.
// Usage: csv.file(path)
func (m *Module) fileFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path string `name:"path" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	return &CsvFile{path: p.Path, thread: thread, config: m.config}, nil
}

// sourceFactory creates a Writer from a list of data.
// Usage: csv.source(data)
func (m *Module) sourceFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("csv.source: missing required argument: data")
	}
	if len(args) > 1 {
		return nil, fmt.Errorf("csv.source: expected 1 argument, got %d", len(args))
	}
	list, ok := args[0].(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("csv.source: data must be a list, got %s", args[0].Type())
	}
	return &Writer{data: list, thread: thread, config: m.config}, nil
}

// stringValue converts a Starlark value to string.
func stringValue(v starlark.Value) string {
	if s, ok := starlark.AsString(v); ok {
		return s
	}
	return v.String()
}
