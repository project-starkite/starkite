// Package loader provides all-edition module registration.
// It composes the base, cloud, and ai loaders into a single registry.
package loader

import (
	stdlog "log"

	ailoader "github.com/project-starkite/starkite/aikite/loader"
	cloudloader "github.com/project-starkite/starkite/cloudkite/loader"
	"github.com/project-starkite/starkite/starbase"
	baseloader "github.com/project-starkite/starkite/starbase/loader"
	"github.com/project-starkite/starkite/wasm"
)

// NewAllRegistry creates a registry with base + cloud + ai modules registered.
// Strict mode is enabled so any module-name, export-key, or global-alias
// collision across editions surfaces immediately at startup instead of
// silently overwriting.
func NewAllRegistry(config *starbase.ModuleConfig) *starbase.Registry {
	r := starbase.NewRegistry(config)
	r.SetStrict(true)
	baseloader.RegisterAll(r)
	cloudloader.RegisterCloudModules(r)
	ailoader.RegisterAIModules(r)
	if err := wasm.RegisterPlugins(r, ""); err != nil {
		stdlog.Printf("wasm: plugin discovery error: %v", err)
	}
	return r
}
