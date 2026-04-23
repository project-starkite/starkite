package wasm

import (
	"log"
	"os"
	"path/filepath"
)

// DiscoveredPlugin holds the parsed manifest and resolved paths for a plugin.
type DiscoveredPlugin struct {
	Manifest     *PluginManifest
	WasmPath     string // absolute path to .wasm file
	ManifestPath string // absolute path to module.yaml
}

// DefaultPluginDir returns the default plugin directory (~/.starkite/modules/wasm/).
func DefaultPluginDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".starkite", "modules", "wasm")
}

// Discover scans pluginDir for subdirectories containing module.yaml files.
// Each valid subdirectory becomes a DiscoveredPlugin. Invalid plugins are
// logged but do not stop discovery.
func Discover(pluginDir string) ([]*DiscoveredPlugin, error) {
	if pluginDir == "" {
		pluginDir = DefaultPluginDir()
	}

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no plugins directory is fine
		}
		return nil, err
	}

	var plugins []*DiscoveredPlugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(pluginDir, entry.Name(), "module.yaml")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		manifest, err := ParseManifest(manifestPath)
		if err != nil {
			log.Printf("wasm: skipping plugin %q: %v", entry.Name(), err)
			continue
		}

		wasmPath := filepath.Join(pluginDir, entry.Name(), manifest.Wasm)
		if _, err := os.Stat(wasmPath); os.IsNotExist(err) {
			log.Printf("wasm: skipping plugin %q: wasm file %q not found", entry.Name(), manifest.Wasm)
			continue
		}

		plugins = append(plugins, &DiscoveredPlugin{
			Manifest:     manifest,
			WasmPath:     wasmPath,
			ManifestPath: manifestPath,
		})
	}

	return plugins, nil
}
