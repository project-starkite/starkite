// Package fmtmod provides formatting functions for starkite.
// Named fmtmod to avoid conflict with Go's fmt package.
package fmtmod

import (
	"fmt"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "fmt"

// Module implements formatting functions.
type Module struct {
	once    sync.Once
	module  starlark.Value
	aliases starlark.StringDict
	config  *libkite.ModuleConfig
}

func New() *Module {
	return &Module{}
}

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "fmt provides formatting functions: printf, println, sprintf"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config

		members := starlark.StringDict{
			"printf":  starlark.NewBuiltin("fmt.printf", m.printf),
			"println": starlark.NewBuiltin("fmt.println", m.println),
			"sprintf": starlark.NewBuiltin("fmt.sprintf", m.sprintf),
		}

		m.module = libkite.NewTryModule(string(ModuleName), members)

		// Create global aliases
		m.aliases = starlark.StringDict{
			"printf":  starlark.NewBuiltin("printf", m.printf),
			"println": starlark.NewBuiltin("println", m.println),
			"sprintf": starlark.NewBuiltin("sprintf", m.sprintf),
		}
	})

	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict {
	return m.aliases
}

func (m *Module) FactoryMethod() string { return "" }

// printf prints formatted output to stdout.
func (m *Module) printf(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) == 0 {
		return starlark.None, nil
	}

	format, err := formatString(args)
	if err != nil {
		return nil, err
	}

	fmt.Print(format)
	return starlark.None, nil
}

// println prints values separated by spaces with a trailing newline.
func (m *Module) println(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	goArgs := make([]interface{}, len(args))
	for i, arg := range args {
		var goVal any
		if err := startype.Starlark(arg).Go(&goVal); err != nil {
			goArgs[i] = arg.String()
			continue
		}
		goArgs[i] = goVal
	}
	fmt.Println(goArgs...)
	return starlark.None, nil
}

// sprintf returns a formatted string.
func (m *Module) sprintf(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) == 0 {
		return starlark.String(""), nil
	}

	format, err := formatString(args)
	if err != nil {
		return nil, err
	}

	return starlark.String(format), nil
}

// formatString formats a string using Go's fmt.Sprintf.
func formatString(args starlark.Tuple) (string, error) {
	if len(args) == 0 {
		return "", nil
	}

	format, ok := starlark.AsString(args[0])
	if !ok {
		return "", fmt.Errorf("format must be a string")
	}

	if len(args) == 1 {
		return format, nil
	}

	// Convert remaining args to Go values
	goArgs := make([]interface{}, len(args)-1)
	for i, arg := range args[1:] {
		var goVal any
		if err := startype.Starlark(arg).Go(&goVal); err != nil {
			goArgs[i] = arg.String()
			continue
		}
		goArgs[i] = goVal
	}

	return fmt.Sprintf(format, goArgs...), nil
}
