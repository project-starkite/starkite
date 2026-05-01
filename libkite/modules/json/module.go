// Package json provides JSON file reading/writing for starkite.
package json

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "json"

// Module implements JSON file reading/writing with builder pattern.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "json provides JSON file reading/writing: file, from"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = libkite.NewTryModule(string(ModuleName), starlark.StringDict{
			"file":   starlark.NewBuiltin("json.file", m.fileFactory),
			"source": starlark.NewBuiltin("json.source", m.sourceFactory),
			"encode": starlark.NewBuiltin("json.encode", m.encode),
			"decode": starlark.NewBuiltin("json.decode", m.decode),
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// fileFactory creates a JsonFile anchored at a path.
// Usage: json.file(path)
func (m *Module) fileFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("json.file: expected 1 argument (path), got %d", len(args))
	}
	path, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("json.file: path must be a string, got %s", args[0].Type())
	}
	return &JsonFile{path: path, thread: thread, config: m.config}, nil
}

// sourceFactory creates a Writer from any Starlark value.
// Usage: json.source(data)
func (m *Module) sourceFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("json.source: expected 1 argument (data), got %d", len(args))
	}
	return &Writer{data: args[0], thread: thread, config: m.config}, nil
}

// encode encodes a Starlark value to a JSON string.
// Usage: json.encode(value) -> string
func (m *Module) encode(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("json.encode: expected 1 argument, got %d", len(args))
	}

	var goVal any
	if err := startype.Starlark(args[0]).Go(&goVal); err != nil {
		return nil, fmt.Errorf("json.encode: %w", err)
	}

	goVal = convertForJSON(goVal)

	output, err := json.Marshal(goVal)
	if err != nil {
		return nil, fmt.Errorf("json.encode: %w", err)
	}
	return starlark.String(output), nil
}

// decode decodes a JSON string/bytes into a Starlark value.
// Usage: json.decode(s) -> any
func (m *Module) decode(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("json.decode: expected 1 argument, got %d", len(args))
	}
	var data []byte
	switch v := args[0].(type) {
	case starlark.String:
		data = []byte(string(v))
	case starlark.Bytes:
		data = []byte(string(v))
	default:
		return nil, fmt.Errorf("json.decode: argument must be string or bytes, got %s", args[0].Type())
	}

	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("json.decode: %w", err)
	}

	var result starlark.Value
	if err := startype.Go(v).Starlark(&result); err != nil {
		return nil, fmt.Errorf("json.decode: %w", err)
	}
	return result, nil
}
