package manager

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// WasmManifest is a lightweight representation of a WASM module's module.yaml.
// It avoids importing pkg/libkite/wasm (which pulls in Extism/starlark deps).
type WasmManifest struct {
	Name        string             `yaml:"name"`
	Version     string             `yaml:"version"`
	Description string             `yaml:"description"`
	Wasm        string             `yaml:"wasm"`
	MinStarkite string             `yaml:"min_starkite"`
	Functions   []WasmFunctionInfo `yaml:"functions"`
	Permissions []string           `yaml:"permissions"`
}

// WasmFunctionInfo describes a function exported by a WASM module.
type WasmFunctionInfo struct {
	Name    string          `yaml:"name"`
	Params  []WasmParamInfo `yaml:"params"`
	Returns string          `yaml:"returns"`
}

// WasmParamInfo describes a function parameter.
type WasmParamInfo struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

// parseWasmManifest reads and parses a WASM module.yaml file.
func parseWasmManifest(path string) (*WasmManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m WasmManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	if m.Name == "" {
		return nil, fmt.Errorf("manifest missing required field: name")
	}
	if m.Version == "" {
		return nil, fmt.Errorf("manifest missing required field: version")
	}
	if m.Wasm == "" {
		return nil, fmt.Errorf("manifest missing required field: wasm")
	}

	return &m, nil
}
