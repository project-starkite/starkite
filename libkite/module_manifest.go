package libkite

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	// Sandbox declares the execution capabilities required by this module.
	Sandbox *ManifestSandboxSpec `yaml:"sandbox,omitempty"`
}

// ManifestPathsSpec declares readable and writable host paths required by a module.
type ManifestPathsSpec struct {
	Read  []string `yaml:"read,omitempty"`
	Write []string `yaml:"write,omitempty"`
}

// ManifestExecSpec declares allowed and denied binaries for process execution.
type ManifestExecSpec struct {
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
}

// ManifestCallsSpec declares allowed and denied Starlark module.category(resource) calls.
type ManifestCallsSpec struct {
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
}

// ManifestSandboxSpec describes the sandbox capability declarations of a module in mod.yaml.
type ManifestSandboxSpec struct {
	Base             string            `yaml:"base,omitempty"`
	Driver           string            `yaml:"driver,omitempty"`
	Image            string            `yaml:"image,omitempty"`
	Network          any               `yaml:"network,omitempty"`
	MaxMemory        string            `yaml:"max_memory,omitempty"`
	Timeout          string            `yaml:"timeout,omitempty"`
	Paths            ManifestPathsSpec `yaml:"paths,omitempty"`
	Exec             ManifestExecSpec  `yaml:"exec,omitempty"`
	Calls            ManifestCallsSpec `yaml:"calls,omitempty"`
	Env              yaml.Node         `yaml:"env,omitempty"`
	NoExec           *bool             `yaml:"no_exec,omitempty"`
	AllowedExecPaths []string          `yaml:"allowed_exec_paths,omitempty"`
}

// Validate checks that the manifest sandbox specification has valid limits and network modes.
func (s *ManifestSandboxSpec) Validate() error {
	if s == nil {
		return nil
	}
	if s.Timeout != "" {
		if _, err := time.ParseDuration(s.Timeout); err != nil {
			return fmt.Errorf("invalid timeout %q: %w", s.Timeout, err)
		}
	}
	if s.MaxMemory != "" {
		sClean := strings.TrimRight(strings.ToUpper(strings.TrimSpace(s.MaxMemory)), "KMGIB")
		var val int64
		if _, err := fmt.Sscanf(sClean, "%d", &val); err != nil {
			return fmt.Errorf("invalid max_memory %q: %w", s.MaxMemory, err)
		}
	}
	if s.Network != nil {
		switch v := s.Network.(type) {
		case string:
			switch v {
			case "none", "host", "loopback":
			default:
				return fmt.Errorf("unknown network mode %q (want none, host, or loopback)", v)
			}
		case map[string]any:
			if mode, ok := v["mode"].(string); ok && mode != "" {
				switch mode {
				case "none", "host", "loopback":
				default:
					return fmt.Errorf("unknown network mode %q (want none, host, or loopback)", mode)
				}
			}
		}
	}
	return nil
}

// QualifiedName returns the module's "namespace/name" identity, or just "name"
// when no namespace is set.
func (m *ModuleManifest) QualifiedName() string {
	if m.Namespace == "" {
		return m.Name
	}
	return m.Namespace + "/" + m.Name
}

// ParseModuleManifest parses and validates manifest YAML bytes from a given source label.
func ParseModuleManifest(data []byte, source string) (*ModuleManifest, error) {
	var m ModuleManifest
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", source, err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("%s missing required field: name", source)
	}
	if m.Sandbox != nil {
		if err := m.Sandbox.Validate(); err != nil {
			return nil, fmt.Errorf("%s sandbox validation: %w", source, err)
		}
	}
	return &m, nil
}

// LoadModuleManifest reads and validates the mod.yaml in dirPath. A module
// directory without a valid manifest is not a module.
func LoadModuleManifest(dirPath string) (*ModuleManifest, error) {
	manifestPath := filepath.Join(dirPath, ManifestFile)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("missing %s", ManifestFile)
		}
		return nil, fmt.Errorf("reading %s: %w", ManifestFile, err)
	}

	m, err := ParseModuleManifest(data, ManifestFile)
	if err != nil {
		return nil, err
	}

	entry := filepath.Join(dirPath, EntryFile)
	if !fileExists(entry) {
		return nil, fmt.Errorf("module %q is missing its %s entry file", m.Name, EntryFile)
	}

	return m, nil
}
