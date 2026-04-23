// Package edition provides edition management for starkite.
// Editions allow switching between different starkite builds (base, cloud)
// with automatic process handoff via syscall.Exec.
package edition

import (
	"os"
	"path/filepath"
)

const (
	// EditionBase is the default base edition.
	EditionBase = "base"

	// EditionCloud is the cloud edition with Kubernetes and cloud provider modules.
	EditionCloud = "cloud"

	// EditionAI is the ai edition with GenAI, MCP, and agent modules.
	EditionAI = "ai"
)

// KnownEditions lists all recognized edition names (excluding base).
var KnownEditions = []string{EditionCloud, EditionAI}

// EditionInfo holds information about an installed edition.
type EditionInfo struct {
	Name      string
	Path      string // absolute path to binary
	Installed bool
	Active    bool
	Size      int64
}

// StarkiteDir returns the starkite configuration directory (~/.starkite/),
// creating it if needed.
func StarkiteDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".starkite")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// EditionsDir returns the editions directory (~/.starkite/editions/),
// creating it if needed.
func EditionsDir() (string, error) {
	starkiteDir, err := StarkiteDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(starkiteDir, "editions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// EditionBinaryPath returns the path to an edition's binary.
// Format: ~/.starkite/editions/<name>/kite
func EditionBinaryPath(name string) (string, error) {
	editionsDir, err := EditionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(editionsDir, name, "kite"), nil
}

// IsKnownEdition returns true if the given name is a recognized edition.
func IsKnownEdition(name string) bool {
	for _, e := range KnownEditions {
		if e == name {
			return true
		}
	}
	return false
}
