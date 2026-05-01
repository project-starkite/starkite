// Package loader provides ai edition module registration.
// It registers all base modules via the base loader, then adds ai-specific modules.
package loader

import (
	stdlog "log"

	"github.com/project-starkite/starkite/ai/modules/genai"
	"github.com/project-starkite/starkite/ai/modules/mcp"
	"github.com/project-starkite/starkite/libkite"
	baseloader "github.com/project-starkite/starkite/libkite/loader"
	"github.com/project-starkite/starkite/wasm"
)

// RegisterAIModules registers ai-specific modules on an existing registry.
func RegisterAIModules(r *libkite.Registry) {
	r.Register(genai.New())
	r.Register(mcp.New())
	// agent module will be registered here in Phase 3
}

// NewAIRegistry creates a new registry with all base and ai modules registered.
func NewAIRegistry(config *libkite.ModuleConfig) *libkite.Registry {
	r := libkite.NewRegistry(config)
	baseloader.RegisterAll(r)
	RegisterAIModules(r)
	if err := wasm.RegisterPlugins(r, ""); err != nil {
		stdlog.Printf("wasm: plugin discovery error: %v", err)
	}
	return r
}
