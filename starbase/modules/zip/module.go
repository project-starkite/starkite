// Package zip provides ZIP archive reading/writing functions for starkite.
package zip

import (
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

const ModuleName starbase.ModuleName = "zip"

// Module implements ZIP archive functions.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *starbase.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "zip provides ZIP archive operations: file"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = starbase.NewTryModule(string(ModuleName), starlark.StringDict{
			"file": starlark.NewBuiltin("zip.file", m.fileFactory),
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// fileFactory creates an Archive anchored at a path.
// Usage: zip.file("archive.zip")
func (m *Module) fileFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path string `name:"path" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	return &Archive{path: p.Path, thread: thread, config: m.config}, nil
}
