package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/M31-Labs/starlsp"

	"github.com/project-starkite/starkite/libkite"
)

// ResolveLoad maps a load() reference to a file on disk.
//
// This is the most dialect-specific answer Starkite gives the language server.
// The reference grammar is the runtime loader's: a path reference resolves
// relative to the caller, and a bare "namespace/name" is an installed identity
// resolved from the version-addressed module cache, pinned by mod.lock when one
// governs the caller.
func (h *Host) ResolveLoad(req starlsp.LoadRequest) (string, bool) {
	target := strings.TrimSpace(req.Target)
	if target == "" || req.From == "" {
		return "", false
	}
	callerDir := filepath.Dir(req.From)

	if isPathReference(target) {
		candidate := target
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(callerDir, candidate)
		}
		return resolveModuleDir(candidate)
	}

	// A bare .star name with no path prefix is an error in the runtime, so it
	// is not a link here either.
	if strings.HasSuffix(target, ".star") {
		return "", false
	}
	if !strings.Contains(target, "/") {
		return "", false // a built-in module has no file
	}
	return resolveInstalledModule(callerDir, target)
}

func isPathReference(target string) bool {
	return strings.HasPrefix(target, "./") ||
		strings.HasPrefix(target, "../") ||
		filepath.IsAbs(target)
}

// resolveModuleDir accepts a directory holding main.star, or a .star file.
func resolveModuleDir(candidate string) (string, bool) {
	if info, err := os.Stat(candidate); err == nil {
		if info.IsDir() {
			entry := filepath.Join(candidate, libkite.EntryFile)
			if _, err := os.Stat(entry); err == nil {
				return entry, true
			}
			return "", false
		}
		return candidate, true
	}
	if _, err := os.Stat(candidate + ".star"); err == nil {
		return candidate + ".star", true
	}
	return "", false
}

// resolveInstalledModule finds "namespace/name" in the module cache, honouring
// the revision mod.lock pins when the caller is governed by one.
func resolveInstalledModule(callerDir, identity string) (string, bool) {
	root := moduleCacheRoot()
	if root == "" {
		return "", false
	}
	name := identity
	if idx := strings.LastIndex(identity, "/"); idx >= 0 {
		name = identity[idx+1:]
	}

	if rev, ok := lockedRevision(callerDir, identity); ok {
		pinned := filepath.Join(root, fmt.Sprintf("%s@%s", name, rev))
		if entry, ok := resolveModuleDir(pinned); ok {
			return entry, true
		}
	}

	// No lock, or the pinned revision is absent: accept any installed revision.
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirName, _ := libkite.SplitModuleRev(e.Name())
		if dirName != name {
			continue
		}
		if entry, ok := resolveModuleDir(filepath.Join(root, e.Name())); ok {
			return entry, true
		}
	}
	return "", false
}

// lockedRevision reads the revision mod.lock pins for an identity, searching
// upward from the caller for the lockfile that governs it.
func lockedRevision(startDir, identity string) (string, bool) {
	dir := startDir
	for i := 0; i < 32; i++ {
		if lock, err := libkite.LoadLock(dir); err == nil && lock != nil {
			if m, ok := lock.Modules[identity]; ok && m.Rev != "" {
				return m.Rev, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// moduleCacheRoot is where installed modules live.
func moduleCacheRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".starkite", "modules")
}
