// Package fleet provides compute resource fleet management for starkite.
package fleet

import (
	"fmt"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	fleetpkg "github.com/project-starkite/starkite/libkite/fleet"
)

const ModuleName libkite.ModuleName = "fleet"

// Module implements compute fleet management.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

// New creates a new Fleet module instance.
func New() *Module { return &Module{} }

// Name returns the module identifier.
func (m *Module) Name() libkite.ModuleName { return ModuleName }

// Description returns the module documentation.
func (m *Module) Description() string {
	return "fleet provides compute resource fleet management: file, from"
}

// Load initializes the module and its built-in functions.
func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		members := starlark.StringDict{
			"file":        starlark.NewBuiltin("fleet.file", m.file),
			"new":         starlark.NewBuiltin("fleet.new", m.fromSource),
			"from_source": starlark.NewBuiltin("fleet.from_source", m.fromSource),
			"of":          starlark.NewBuiltin("fleet.of", m.fromSource),
		}
		m.module = libkite.NewTryModule(string(ModuleName), members)
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

// Aliases returns global aliases for the module.
func (m *Module) Aliases() starlark.StringDict { return nil }

// FactoryMethod returns the primary constructor if any.
func (m *Module) FactoryMethod() string { return "new" }

// file loads a fleet from a static YAML or JSON file.
// Usage: fleet.file("path/to/hosts.yaml")
func (m *Module) file(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path string `name:"path" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "fs", "read", "read_file", p.Path); err != nil {
		return nil, err
	}

	return fleetpkg.FromFile(p.Path)
}

// fromSource constructs a fleet from runtime sources (list, callable function, dict, or JSON string).
// Usage: fleet.new([{"name": "n1", "address": "1.1.1.1"}]) OR fleet.new(discover_fn)
func (m *Module) fromSource(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("%s expects at least 1 argument (source: list, callable, dict, or JSON string)", fn.Name())
	}
	return fleetpkg.FromSource(thread, args[0])
}
