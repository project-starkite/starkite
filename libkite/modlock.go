package libkite

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LockFile is the generated, committed lockfile at the root of a module or
// loose-script directory. It records the resolved dependency closure.
const LockFile = "mod.lock"

// Lock is the on-disk schema of mod.lock: the resolved transitive dependency
// closure, each identity pinned to a source, revision, and content hash.
type Lock struct {
	Version int                     `yaml:"version"`
	Modules map[string]LockedModule `yaml:"modules,omitempty"`
}

// LockedModule is a single resolved dependency in mod.lock.
type LockedModule struct {
	Source string `yaml:"source"`
	Rev    string `yaml:"rev"`
	Hash   string `yaml:"hash"`
}

// lockVersion is the current mod.lock format version.
const lockVersion = 1

// excludedFromHash names entries that never contribute to a module's content
// hash: VCS metadata and the manager-owned lock/receipt files (the receipt
// holds the hash, so including it would be circular).
var excludedFromHash = map[string]bool{
	".git":         true,
	LockFile:       true,
	".mod.receipt": true,
}

// HashModuleTree returns a deterministic content hash over the files in dir,
// in the style of a go.sum h1: digest: each file's SHA-256 is combined with its
// slash-relative path, sorted, and hashed. VCS metadata and the lock/receipt
// files are excluded. The result is prefixed "sha256:".
func HashModuleTree(dir string) (string, error) {
	var lines []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if excludedFromHash[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if excludedFromHash[d.Name()] {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		fh, err := hashFile(path)
		if err != nil {
			return err
		}
		lines = append(lines, fh+"  "+filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// FingerprintTree returns a machine-local fingerprint of dir computed from file
// metadata only — relative path, size, and modification time — without reading
// any file contents. It is a fast change-detector: an unchanged tree yields the
// same fingerprint, so a match lets a caller skip a full content re-hash. It is
// not portable across machines (mtimes differ) and is never recorded in
// mod.lock. The same VCS/lock/receipt entries excluded from the content hash are
// excluded here.
func FingerprintTree(dir string) (string, error) {
	var lines []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if excludedFromHash[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if excludedFromHash[d.Name()] {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("%s %d %d", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano()))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return "fp1:" + hex.EncodeToString(sum[:]), nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// LoadLock reads mod.lock from dir. It returns (nil, nil) when no lockfile is
// present.
func LoadLock(dir string) (*Lock, error) {
	data, err := os.ReadFile(filepath.Join(dir, LockFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var l Lock
	if err := yaml.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", LockFile, err)
	}
	return &l, nil
}

// Save writes the lock to dir/mod.lock.
func (l *Lock) Save(dir string) error {
	if l.Version == 0 {
		l.Version = lockVersion
	}
	data, err := yaml.Marshal(l)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, LockFile), data, 0o644)
}

// VerifyTree recomputes the content hash of dir and compares it to want. A
// mismatch means the directory's contents changed since it was locked.
func VerifyTree(dir, want string) error {
	got, err := HashModuleTree(dir)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("content hash mismatch for %s: locked %s, on disk %s", dir, want, got)
	}
	return nil
}
