// Package uuid provides UUID generation functions for starkite.
package uuid

import (
	"crypto/rand"
	"fmt"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "uuid"

// Module implements UUID generation functions.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "uuid provides UUID generation: v4"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = &starlarkstruct.Module{
			Name: string(ModuleName),
			Members: starlark.StringDict{
				"v4": starlark.NewBuiltin("uuid.v4", m.v4),
			},
		}
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

func (m *Module) v4(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("uuid.v4 takes no arguments")
	}

	// Generate UUID v4
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		return nil, err
	}

	// Set version (4) and variant (RFC 4122)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return starlark.String(fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])), nil
}
