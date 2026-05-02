package wasm

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// PluginManifest describes a WASM plugin's metadata and function signatures.
type PluginManifest struct {
	Name        string             `yaml:"name"`
	Version     string             `yaml:"version"`
	Description string             `yaml:"description"`
	Wasm        string             `yaml:"wasm"`         // filename relative to manifest dir
	MinStarkite string             `yaml:"min_starkite"` // minimum starkite version
	Functions   []FunctionManifest `yaml:"functions"`
	Permissions []string           `yaml:"permissions"` // host fn groups: "log", "exec", etc.
}

// FunctionManifest describes a single function exported by a WASM plugin.
type FunctionManifest struct {
	Name    string          `yaml:"name"`   // Starlark-visible name
	Export  string          `yaml:"export"` // WASM export name (defaults to Name)
	Params  []ParamManifest `yaml:"params"`
	Returns string          `yaml:"returns"` // "string", "int", "float", "bool", "dict", "list", "none"
}

// ParamManifest describes a function parameter.
type ParamManifest struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`     // "string", "int", "float", "bool", "dict", "list"
	Required *bool  `yaml:"required"` // defaults to true if nil
}

// IsRequired returns whether the parameter is required (defaults to true).
func (p *ParamManifest) IsRequired() bool {
	if p.Required == nil {
		return true
	}
	return *p.Required
}

// ExportName returns the WASM export name, defaulting to the function name.
func (f *FunctionManifest) ExportName() string {
	if f.Export != "" {
		return f.Export
	}
	return f.Name
}

// validTypes is the set of valid type names for params and return types.
var validTypes = map[string]bool{
	"string": true,
	"int":    true,
	"float":  true,
	"bool":   true,
	"dict":   true,
	"list":   true,
	"none":   true,
}

// ParseManifest reads and parses a module.yaml file.
func ParseManifest(path string) (*PluginManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ManifestError{Path: path, Message: fmt.Sprintf("read error: %v", err)}
	}

	var manifest PluginManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, &ManifestError{Path: path, Message: fmt.Sprintf("parse error: %v", err)}
	}

	if err := manifest.Validate(); err != nil {
		return nil, &ManifestError{Path: path, Message: err.Error()}
	}

	return &manifest, nil
}

// Validate checks that the manifest has all required fields and valid types.
func (m *PluginManifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}
	if m.Wasm == "" {
		return fmt.Errorf("wasm filename is required")
	}
	if len(m.Functions) == 0 {
		return fmt.Errorf("at least one function is required")
	}

	for i, fn := range m.Functions {
		if fn.Name == "" {
			return fmt.Errorf("function[%d]: name is required", i)
		}
		if fn.Returns != "" && !validTypes[fn.Returns] {
			return fmt.Errorf("function %q: invalid return type %q", fn.Name, fn.Returns)
		}
		for j, p := range fn.Params {
			if p.Name == "" {
				return fmt.Errorf("function %q param[%d]: name is required", fn.Name, j)
			}
			if p.Type == "" {
				return fmt.Errorf("function %q param %q: type is required", fn.Name, p.Name)
			}
			if !validTypes[p.Type] {
				return fmt.Errorf("function %q param %q: invalid type %q", fn.Name, p.Name, p.Type)
			}
		}
	}

	return nil
}
