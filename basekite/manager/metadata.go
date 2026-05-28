package manager

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const metadataFile = "module.yaml"

// Metadata holds module metadata.
type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
	Author      string `yaml:"author,omitempty"`
	License     string `yaml:"license,omitempty"`
	Repository  string `yaml:"repository,omitempty"`

	// Dependencies lists required modules
	Dependencies []string `yaml:"dependencies,omitempty"`

	// MinStarkiteVersion specifies the minimum starkite version required
	MinStarkiteVersion string `yaml:"min_starkite_version,omitempty"`
}

// ReadMetadata reads module metadata from a module directory.
// It reads module.yaml, falling back to git inference if not found.
func ReadMetadata(modulePath string) (*Metadata, error) {
	metaPath := filepath.Join(modulePath, metadataFile)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		// Try to infer from git
		return inferMetadataFromGit(modulePath)
	}

	var meta Metadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// WriteMetadata writes module metadata to the module directory.
func WriteMetadata(modulePath string, meta *Metadata) error {
	data, err := yaml.Marshal(meta)
	if err != nil {
		return err
	}

	metaPath := filepath.Join(modulePath, metadataFile)
	return os.WriteFile(metaPath, data, 0644)
}

// inferMetadataFromGit tries to get metadata from git repository info.
func inferMetadataFromGit(modulePath string) (*Metadata, error) {
	meta := &Metadata{
		Name: filepath.Base(modulePath),
	}

	// Try to get remote URL
	if GitIsRepo(modulePath) {
		if url, err := GitGetRemoteURL(modulePath); err == nil {
			meta.Repository = url
		}

		if commit, err := GitGetCurrentCommit(modulePath); err == nil {
			meta.Version = commit
		}
	}

	return meta, nil
}
