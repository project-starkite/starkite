// Package loader provides cloud edition module registration.
// It registers all base modules via the base loader, then adds cloud-specific modules.
package loader

import (
	"github.com/project-starkite/starkite/cloudkite/modules/k8s"
	"github.com/project-starkite/starkite/libkite"
	baseloader "github.com/project-starkite/starkite/libkite/loader"
)

// RegisterCloudModules registers cloud-specific modules on an existing registry.
func RegisterCloudModules(r *libkite.Registry) {
	r.Register(k8s.New())
}

// NewCloudRegistry creates a new registry with all base and cloud modules registered.
func NewCloudRegistry(config *libkite.ModuleConfig) *libkite.Registry {
	r := libkite.NewRegistry(config)
	baseloader.RegisterAll(r)
	RegisterCloudModules(r)
	return r
}
