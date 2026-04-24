// Package mcp provides a Starlark module for the Model Context Protocol.
//
// Slice 2.0 (skeleton): registers ai.mcp.* builtins as stubs. Slices 2.1+
// fill in mcp.serve() and mcp.connect() using the official Go MCP SDK.
package mcp

import (
	"sync"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
)

// ModuleName is the Starlark namespace for this module: `mcp`.
const ModuleName starbase.ModuleName = "mcp"

type Module struct {
	loadOnce sync.Once
	module   starlark.Value
	mcfg     *starbase.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() starbase.ModuleName    { return ModuleName }
func (m *Module) Description() string          { return "mcp provides Model Context Protocol server + client" }
func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

func (m *Module) Load(mcfg *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.loadOnce.Do(func() {
		m.mcfg = mcfg
		m.module = starbase.NewTryModule(string(ModuleName), starlark.StringDict{
			"serve":   starlark.NewBuiltin("mcp.serve", m.serveBuiltin),
			"connect": starlark.NewBuiltin("mcp.connect", m.connectBuiltin),
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}
