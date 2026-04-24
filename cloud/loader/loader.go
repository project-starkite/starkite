// Package loader provides cloud edition module registration.
// It registers all base modules via the base loader, then adds cloud-specific modules.
package loader

import (
	stdlog "log"

	"github.com/project-starkite/starkite/cloud/modules/k8s"
	"github.com/project-starkite/starkite/starbase"
	baseloader "github.com/project-starkite/starkite/starbase/loader"
	"github.com/project-starkite/starkite/wasm"
)

// RegisterCloudModules registers cloud-specific modules on an existing registry.
func RegisterCloudModules(r *starbase.Registry) {
	r.Register(k8s.New())
}

// NewCloudRegistry creates a new registry with all base and cloud modules registered.
func NewCloudRegistry(config *starbase.ModuleConfig) *starbase.Registry {
	r := starbase.NewRegistry(config)
	baseloader.RegisterAll(r)
	RegisterCloudModules(r)
	if err := wasm.RegisterPlugins(r, ""); err != nil {
		stdlog.Printf("wasm: plugin discovery error: %v", err)
	}
	return r
}
