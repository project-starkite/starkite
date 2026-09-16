package args

import (
	"sync"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "args"

// Module implements the args module for declarative CLI argument and flag parsing.
type Module struct {
	once    sync.Once
	module  starlark.Value
	aliases starlark.StringDict
	config  *libkite.ModuleConfig
}

// New creates a new instance of the args module.
func New() *Module {
	return &Module{}
}

// Name returns the module name.
func (m *Module) Name() libkite.ModuleName { return ModuleName }

// Description returns a description of the args module.
func (m *Module) Description() string {
	return "args provides declarative command-line argument and flag parsing: string, int, bool, float, list, positional, parse"
}

// Load initializes the args module and its exports.
func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config

		members := starlark.StringDict{
			"string":     starlark.NewBuiltin("args.string", m.buildString),
			"int":        starlark.NewBuiltin("args.int", m.buildInt),
			"bool":       starlark.NewBuiltin("args.bool", m.buildBool),
			"float":      starlark.NewBuiltin("args.float", m.buildFloat),
			"list":       starlark.NewBuiltin("args.list", m.buildList),
			"positional": starlark.NewBuiltin("args.positional", m.buildPositional),
			"parse":      starlark.NewBuiltin("args.parse", m.parseStub),
		}

		m.module = libkite.NewTryModule(string(ModuleName), members)
	})

	return starlark.StringDict{string(ModuleName): m.module}, nil
}

// Aliases returns global aliases. The args module namespace itself is sufficient.
func (m *Module) Aliases() starlark.StringDict {
	return nil
}

// FactoryMethod returns empty string as args is not a factory module.
func (m *Module) FactoryMethod() string { return "" }
