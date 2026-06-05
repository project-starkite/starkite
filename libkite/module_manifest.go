package libkite

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ManifestFile is the required manifest at the root of every module directory.
const ManifestFile = "module.yaml"

// ModuleManifest describes a module. Every module is a directory containing a
// module.yaml plus one or more .star files; the manifest is the single source
// of truth for the module's identity, independent of where the module lives.
type ModuleManifest struct {
	// Namespace and Name together identify the module as "namespace/name".
	// Namespace is optional in the manifest when it can be resolved from the
	// install source; Name is always required.
	Namespace   string `yaml:"namespace,omitempty"`
	Name        string `yaml:"name"`
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`

	// Entry is the .star file loaded first; its directory's other .star files
	// merge their public symbols into the module. Defaults to "main.star".
	Entry string `yaml:"entry,omitempty"`

	// Permissions lists the capability rules the module's code may use, in the
	// same grammar as a permission profile (e.g. "http.client(api.slack.com:*)").
	// It can only narrow the runtime ceiling, never widen it.
	Permissions []string `yaml:"permissions,omitempty"`

	// MinStarkite is the minimum starkite version the module requires.
	MinStarkite string `yaml:"min_starkite,omitempty"`
}

// EntryFile returns the manifest's entry .star filename, defaulting to main.star.
func (m *ModuleManifest) EntryFile() string {
	if m.Entry == "" {
		return "main.star"
	}
	return m.Entry
}

// QualifiedName returns the module's "namespace/name" identity, or just "name"
// when no namespace is set.
func (m *ModuleManifest) QualifiedName() string {
	if m.Namespace == "" {
		return m.Name
	}
	return m.Namespace + "/" + m.Name
}

// LoadModuleManifest reads and validates the module.yaml in dirPath. A module
// directory without a valid manifest is not a module.
func LoadModuleManifest(dirPath string) (*ModuleManifest, error) {
	manifestPath := filepath.Join(dirPath, ManifestFile)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("module %q missing %s", filepath.Base(dirPath), ManifestFile)
		}
		return nil, fmt.Errorf("reading %s: %w", ManifestFile, err)
	}

	var m ModuleManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", ManifestFile, err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("%s missing required field: name", ManifestFile)
	}

	entry := filepath.Join(dirPath, m.EntryFile())
	if !fileExists(entry) {
		return nil, fmt.Errorf("module %q entry file %q not found", m.Name, m.EntryFile())
	}

	return &m, nil
}
