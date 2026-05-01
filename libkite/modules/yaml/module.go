// Package yaml provides YAML file reading/writing for starkite.
package yaml

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	goyaml "gopkg.in/yaml.v3"

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "yaml"

// Module implements YAML file reading/writing with builder pattern.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "yaml provides YAML file reading/writing: file, from"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = libkite.NewTryModule(string(ModuleName), starlark.StringDict{
			"file":       starlark.NewBuiltin("yaml.file", m.fileFactory),
			"source":     starlark.NewBuiltin("yaml.source", m.sourceFactory),
			"encode":     starlark.NewBuiltin("yaml.encode", m.encode),
			"encode_all": starlark.NewBuiltin("yaml.encode_all", m.encodeAll),
			"decode":     starlark.NewBuiltin("yaml.decode", m.decode),
			"decode_all": starlark.NewBuiltin("yaml.decode_all", m.decodeAll),
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// fileFactory creates a YamlFile anchored at a path.
// Usage: yaml.file(path)
func (m *Module) fileFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("yaml.file: expected 1 argument (path), got %d", len(args))
	}
	path, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("yaml.file: path must be a string, got %s", args[0].Type())
	}
	return &YamlFile{path: path, thread: thread, config: m.config}, nil
}

// sourceFactory creates a Writer from any Starlark value.
// Usage: yaml.source(data)
func (m *Module) sourceFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("yaml.source: expected 1 argument (data), got %d", len(args))
	}
	return &Writer{data: args[0], thread: thread, config: m.config}, nil
}

// encode encodes a Starlark value to a YAML string.
// Usage: yaml.encode(value) -> string
func (m *Module) encode(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("yaml.encode: expected 1 argument, got %d", len(args))
	}
	var goVal any
	if err := startype.Starlark(args[0]).Go(&goVal); err != nil {
		return nil, err
	}
	data, err := goyaml.Marshal(goVal)
	if err != nil {
		return nil, err
	}
	return starlark.String(data), nil
}

// encodeAll encodes a list of values to multi-document YAML.
// Usage: yaml.encode_all(list) -> string
func (m *Module) encodeAll(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("yaml.encode_all: expected 1 argument, got %d", len(args))
	}
	list, ok := args[0].(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("yaml.encode_all: expected list, got %s", args[0].Type())
	}
	var buf bytes.Buffer
	for i := 0; i < list.Len(); i++ {
		if i > 0 {
			buf.WriteString("---\n")
		}
		var goVal any
		if err := startype.Starlark(list.Index(i)).Go(&goVal); err != nil {
			return nil, fmt.Errorf("yaml.encode_all[%d]: %w", i, err)
		}
		data, err := goyaml.Marshal(goVal)
		if err != nil {
			return nil, fmt.Errorf("yaml.encode_all[%d]: %w", i, err)
		}
		buf.Write(data)
	}
	return starlark.String(buf.String()), nil
}

// decode decodes a YAML string/bytes into a Starlark value.
// Usage: yaml.decode(s) -> any
func (m *Module) decode(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	data, err := asBytes("yaml.decode", args)
	if err != nil {
		return nil, err
	}
	var v any
	if err := goyaml.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	var result starlark.Value
	if err := startype.Go(v).Starlark(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// decodeAll decodes multi-document YAML string/bytes into a list.
// Usage: yaml.decode_all(s) -> list
func (m *Module) decodeAll(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	data, err := asBytes("yaml.decode_all", args)
	if err != nil {
		return nil, err
	}
	decoder := goyaml.NewDecoder(bytes.NewReader(data))
	var results []starlark.Value
	for {
		var v any
		if err := decoder.Decode(&v); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("yaml.decode_all: %w", err)
		}
		var starVal starlark.Value
		if err := startype.Go(v).Starlark(&starVal); err != nil {
			return nil, fmt.Errorf("yaml.decode_all: %w", err)
		}
		results = append(results, starVal)
	}
	return starlark.NewList(results), nil
}

// asBytes extracts a byte slice from a starlark.String or starlark.Bytes argument.
func asBytes(fnName string, args starlark.Tuple) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%s: expected 1 argument, got %d", fnName, len(args))
	}
	switch v := args[0].(type) {
	case starlark.String:
		return []byte(string(v)), nil
	case starlark.Bytes:
		return []byte(string(v)), nil
	default:
		return nil, fmt.Errorf("%s: argument must be string or bytes, got %s", fnName, args[0].Type())
	}
}
