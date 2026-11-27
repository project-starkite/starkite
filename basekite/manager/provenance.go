package manager

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// receiptFile is the install receipt written for every installed module. It is
// a manager-owned record of where and how the module was installed, kept
// separate from the author-authored mod.yaml.
const receiptFile = ".mod.receipt"

const provenanceSchemaVersion = 1

// Provenance captures where and when a module was installed from.
type Provenance struct {
	SchemaVersion int    `json:"schema_version"`
	Namespace     string `json:"namespace,omitempty"`
	Name          string `json:"name,omitempty"`
	// Source is the resolved clone URL or local path.
	Source string `json:"source,omitempty"`
	// Version is the resolved commit or tag (human-facing).
	Version string `json:"version,omitempty"`
	// Rev is the immutable revision the cache directory is keyed by: the commit
	// SHA for a git source, or the content hash for a local source.
	Rev string `json:"rev,omitempty"`
	// Hash is the portable content hash of the installed tree ("sha256:..."),
	// the same value recorded in mod.lock.
	Hash string `json:"hash,omitempty"`
	// Fingerprint is a machine-local, stat-only fingerprint of the installed tree
	// ("fp1:..."). A later run that finds a matching fingerprint can trust Hash
	// without re-reading every file. It is not portable and is never written to
	// mod.lock.
	Fingerprint string `json:"fingerprint,omitempty"`
	// InstalledFrom is the original source string the user supplied.
	InstalledFrom string `json:"installed_from,omitempty"`
}

// WriteProvenance writes the JSON install receipt into a module directory.
func WriteProvenance(modulePath string, p *Provenance) error {
	if p == nil {
		return fmt.Errorf("provenance cannot be nil")
	}
	copyProv := *p
	copyProv.SchemaVersion = provenanceSchemaVersion
	data, err := json.MarshalIndent(copyProv, "", "  ")
	if err != nil {
		return fmt.Errorf("encode module receipt: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(modulePath, receiptFile), data, 0o644)
}

// ReadProvenance reads the JSON install receipt from a module directory.
// Returns (nil, nil) when no receipt is present.
func ReadProvenance(modulePath string) (*Provenance, error) {
	data, err := os.ReadFile(filepath.Join(modulePath, receiptFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var p Provenance
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("invalid module receipt: %w", err)
	}
	if p.SchemaVersion == 0 || p.SchemaVersion > provenanceSchemaVersion {
		return nil, fmt.Errorf("unsupported module receipt schema version: %d", p.SchemaVersion)
	}
	return &p, nil
}
