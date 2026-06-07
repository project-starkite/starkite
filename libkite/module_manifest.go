package libkite

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestFile is the required manifest at the root of every module directory.
const ManifestFile = "mod.yaml"

// SplitModuleRev splits a version-addressed cache directory name "<name>@<rev>"
// into its module name and revision. A name with no "@" yields an empty rev.
func SplitModuleRev(dirName string) (name, rev string) {
	if i := strings.LastIndex(dirName, "@"); i > 0 {
		return dirName[:i], dirName[i+1:]
	}
	return dirName, ""
}

// EntryFile is the fixed entry point of every module directory. Its public
// symbols, together with those of the directory's other .star files, form the
// module. The entry point is not configurable.
const EntryFile = "main.star"

// ModuleManifest describes a module. Every module is a directory containing a
// mod.yaml plus one or more .star files; the manifest is the single source of
// truth for the module's identity, independent of where the module lives.
type ModuleManifest struct {
	// Namespace and Name together identify the module as "namespace/name".
	// Namespace is optional in the manifest when it can be resolved from the
	// install source; Name is always required.
	Namespace   string `yaml:"namespace,omitempty"`
	Name        string `yaml:"name"`
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`

	// Dependencies declares the modules this module loads, mapping each
	// "namespace/name" identity to its source and version ("source@version").
	// The source lets the runtime resolve and fetch the dependency; the resolved
	// revision and content hash are recorded in mod.lock.
	Dependencies map[string]string `yaml:"dependencies,omitempty"`
}

// QualifiedName returns the module's "namespace/name" identity, or just "name"
// when no namespace is set.
func (m *ModuleManifest) QualifiedName() string {
	if m.Namespace == "" {
		return m.Name
	}
	return m.Namespace + "/" + m.Name
}

// LoadModuleManifest reads and validates the mod.yaml in dirPath. A module
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

	entry := filepath.Join(dirPath, EntryFile)
	if !fileExists(entry) {
		return nil, fmt.Errorf("module %q is missing its %s entry file", m.Name, EntryFile)
	}

	return &m, nil
}
