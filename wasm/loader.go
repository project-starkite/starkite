package wasm

import (
	"log"

	"github.com/project-starkite/starkite/libkite"
)

// RegisterPlugins discovers WASM plugins and registers them with the registry.
// If pluginDir is empty, DefaultPluginDir() is used.
// Existing modules with the same name are not overwritten.
// Individual plugin errors are logged but do not abort registration.
func RegisterPlugins(registry *libkite.Registry, pluginDir string) error {
	plugins, err := Discover(pluginDir)
	if err != nil {
		return err
	}

	for _, p := range plugins {
		name := libkite.ModuleName(p.Manifest.Name)

		// Skip if a module with the same name is already registered
		if _, exists := registry.Get(name); exists {
			log.Printf("wasm: skipping plugin %q: module name already registered", p.Manifest.Name)
			continue
		}

		module := NewWasmModule(p)
		registry.Register(module)
	}

	return nil
}
