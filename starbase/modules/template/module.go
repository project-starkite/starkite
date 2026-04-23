// Package template provides text/template rendering functions for starkite.
package template

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"text/template"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

const ModuleName starbase.ModuleName = "template"

// Module implements template rendering functions.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *starbase.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "template provides text/template rendering: text, file, bytes, render"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = starbase.NewTryModule(string(ModuleName), starlark.StringDict{
			// Object API factories
			"text":  starlark.NewBuiltin("template.text", m.textFactory),
			"file":  starlark.NewBuiltin("template.file", m.fileFactory),
			"bytes": starlark.NewBuiltin("template.bytes", m.bytesFactory),

			// Legacy convenience function (backwards compatible)
			"render": starlark.NewBuiltin("template.render", m.render),
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// textFactory creates a Template from a string.
// Usage: template.text(s, delims=("<%", "%>"))
func (m *Module) textFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("template.text: takes exactly 1 argument (template string)")
	}
	s, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("template.text: argument must be a string, got %s", args[0].Type())
	}
	left, right, err := extractDelims(kwargs)
	if err != nil {
		return nil, fmt.Errorf("template.text: %w", err)
	}
	return parseTemplate(s, left, right, thread, m.config)
}

// fileFactory creates a Template by reading a file.
// Usage: template.file(path, delims=("<%", "%>"))
func (m *Module) fileFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("template.file: takes exactly 1 argument (path)")
	}
	path, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("template.file: argument must be a string, got %s", args[0].Type())
	}
	if err := starbase.Check(thread, "fs", "read_file", path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	left, right, err := extractDelims(kwargs)
	if err != nil {
		return nil, fmt.Errorf("template.file: %w", err)
	}
	return parseTemplate(string(data), left, right, thread, m.config)
}

// bytesFactory creates a Template from bytes.
// Usage: template.bytes(b, delims=("<%", "%>"))
func (m *Module) bytesFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("template.bytes: takes exactly 1 argument (bytes or string)")
	}
	var s string
	switch v := args[0].(type) {
	case starlark.Bytes:
		s = string(v)
	case starlark.String:
		s = string(v)
	default:
		return nil, fmt.Errorf("template.bytes: argument must be bytes or string, got %s", args[0].Type())
	}
	left, right, err := extractDelims(kwargs)
	if err != nil {
		return nil, fmt.Errorf("template.bytes: %w", err)
	}
	return parseTemplate(s, left, right, thread, m.config)
}

// render is the legacy convenience function for backwards compatibility.
// Usage: template.render(template_string, data)
func (m *Module) render(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("render: expected 2 arguments (template, data)")
	}
	tmplStr, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("render: first argument must be a string, got %s", args[0].Type())
	}

	tmpl, err := template.New("tmpl").Parse(tmplStr)
	if err != nil {
		return nil, err
	}

	var goData any
	if err := startype.Starlark(args[1]).Go(&goData); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, goData); err != nil {
		return nil, err
	}
	return starlark.String(buf.String()), nil
}
