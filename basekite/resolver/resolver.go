// Package resolver resolves a module's declared dependency closure into the
// version-addressed cache and records it in mod.lock. It bridges the manager
// (which fetches and stores modules) and libkite (which defines the lock format
// and verifies cached trees); libkite cannot import the manager, so the driver
// lives here.
package resolver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/project-starkite/starkite/basekite/manager"
	"github.com/project-starkite/starkite/libkite"
)

// Resolver resolves and fetches module dependencies into a manager-backed cache.
type Resolver struct {
	mgr *manager.Manager
}

// New returns a Resolver backed by mgr's module cache.
func New(mgr *manager.Manager) *Resolver {
	return &Resolver{mgr: mgr}
}

// EnsureClosure resolves the full transitive dependency closure of the module at
// dir into the cache and returns the resolved lock, without writing it. An
// existing dir/mod.lock pins revisions for incremental, cache-first resolution:
// a locked dependency whose cached tree still verifies is reused without
// re-fetching; anything else is fetched from its declared source.
func (r *Resolver) EnsureClosure(dir string) (*libkite.Lock, error) {
	manifest, err := libkite.LoadModuleManifest(dir)
	if err != nil {
		return nil, err
	}
	existing, err := libkite.LoadLock(dir)
	if err != nil {
		return nil, err
	}

	lock := &libkite.Lock{Modules: map[string]libkite.LockedModule{}}
	if err := r.resolveDeps(manifest.Dependencies, existing, lock); err != nil {
		return nil, err
	}
	return lock, nil
}

// Sync resolves the closure of the module at dir and writes dir/mod.lock. Use it
// for a working module directory the caller owns; for an immutable cache module,
// use EnsureClosure so the cached tree is not mutated.
func (r *Resolver) Sync(dir string) (*libkite.Lock, error) {
	lock, err := r.EnsureClosure(dir)
	if err != nil {
		return nil, err
	}
	// A module with no dependencies and no prior lock needs no lockfile; don't
	// create an empty one. A now-empty closure with a prior lock is still saved
	// so the removal is recorded.
	if len(lock.Modules) == 0 {
		if existing, _ := libkite.LoadLock(dir); existing == nil {
			return lock, nil
		}
	}
	if err := lock.Save(dir); err != nil {
		return nil, err
	}
	return lock, nil
}

// SyncLoose resolves the installed modules a loose script file load()s, using
// the cache only — nothing is fetched. It writes mod.lock beside the file and
// returns the resolved lock. An installed reference (namespace/name) that is not
// already in the cache is an error; relative loads, .star paths, and built-in
// modules are ignored.
func (r *Resolver) SyncLoose(filePath string) (*libkite.Lock, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	refs, err := libkite.LoadTargets(filePath, src)
	if err != nil {
		return nil, err
	}

	lock := &libkite.Lock{Modules: map[string]libkite.LockedModule{}}
	for _, ref := range refs {
		if !isInstalledRef(ref) {
			continue
		}
		if err := r.lockInstalled(ref, lock); err != nil {
			return nil, err
		}
	}

	dir := filepath.Dir(filePath)
	if len(lock.Modules) == 0 {
		if existing, _ := libkite.LoadLock(dir); existing == nil {
			return lock, nil
		}
	}
	if err := lock.Save(dir); err != nil {
		return nil, err
	}
	return lock, nil
}

// lockInstalled records an installed module and its transitive declared
// dependencies in lock, resolving each from the cache only. It errors if a
// module is not installed.
func (r *Resolver) lockInstalled(ref string, lock *libkite.Lock) error {
	if _, done := lock.Modules[ref]; done {
		return nil
	}
	info, err := r.mgr.Get(ref)
	if err != nil {
		return fmt.Errorf("module %q is not installed; install it with: kite module install <source>", ref)
	}

	source := ""
	if prov, pErr := manager.ReadProvenance(info.Path); pErr == nil && prov != nil {
		source = prov.InstalledFrom
	}
	lock.Modules[ref] = libkite.LockedModule{Source: source, Rev: info.Rev, Hash: info.Hash}

	if sub, mErr := libkite.LoadModuleManifest(info.Path); mErr == nil {
		for depID := range sub.Dependencies {
			if err := r.lockInstalled(depID, lock); err != nil {
				return err
			}
		}
	}
	return nil
}

// isInstalledRef reports whether a load() reference names an installed
// "namespace/name" module (as opposed to a relative load, a .star path, or a
// bare built-in module name).
func isInstalledRef(ref string) bool {
	if strings.HasSuffix(ref, ".star") {
		return false
	}
	if ref == "." || ref == ".." ||
		strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../") ||
		strings.HasPrefix(ref, ".\\") || strings.HasPrefix(ref, "..\\") ||
		strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "\\") ||
		filepath.IsAbs(ref) ||
		filepath.VolumeName(ref) != "" {
		return false
	}
	return strings.Contains(ref, "/")
}

// resolveDeps walks declared dependencies, adding each to lock before recursing
// into its own dependencies. Adding before recursing both deduplicates the
// closure and breaks dependency cycles.
func (r *Resolver) resolveDeps(deps map[string]string, existing, lock *libkite.Lock) error {
	for identity, source := range deps {
		if _, done := lock.Modules[identity]; done {
			continue
		}
		locked, moduleDir, err := r.resolveOne(identity, source, existing)
		if err != nil {
			return fmt.Errorf("resolving dependency %q: %w", identity, err)
		}
		lock.Modules[identity] = locked

		sub, err := libkite.LoadModuleManifest(moduleDir)
		if err != nil {
			return fmt.Errorf("reading dependency %q manifest: %w", identity, err)
		}
		if err := r.resolveDeps(sub.Dependencies, existing, lock); err != nil {
			return err
		}
	}
	return nil
}

// resolveOne resolves a single declared dependency to a locked entry and its
// cache directory. A locked, present, and verified revision is reused; otherwise
// the dependency is fetched from source. The fetched module's identity must
// match the declared key.
func (r *Resolver) resolveOne(identity, source string, existing *libkite.Lock) (libkite.LockedModule, string, error) {
	if existing != nil {
		if prev, ok := existing.Modules[identity]; ok && prev.Source == source {
			dir := r.cacheDir(identity, prev.Rev)
			if isDir(dir) && verifyCached(dir, prev.Hash) == nil {
				return prev, dir, nil
			}
		}
	}

	info, err := r.mgr.Install(source, manager.InstallOptions{})
	if err != nil {
		return libkite.LockedModule{}, "", err
	}
	if got := info.Namespace + "/" + info.Name; got != identity {
		return libkite.LockedModule{}, "", fmt.Errorf("declared as %q but resolves to %q", identity, got)
	}
	hash := info.Hash
	if hash == "" {
		if hash, err = libkite.HashModuleTree(info.Path); err != nil {
			return libkite.LockedModule{}, "", err
		}
	}
	return libkite.LockedModule{Source: source, Rev: info.Rev, Hash: hash}, info.Path, nil
}

// verifyCached confirms that the cached tree at dir matches wantHash. It takes a
// fast stat-only path when the install receipt's fingerprint still matches the
// tree (the tree is unchanged, so the receipt's recorded hash is trusted),
// falling back to a full content re-hash otherwise.
func verifyCached(dir, wantHash string) error {
	if prov, err := manager.ReadProvenance(dir); err == nil && prov != nil &&
		prov.Hash == wantHash && prov.Fingerprint != "" {
		if fp, err := libkite.FingerprintTree(dir); err == nil && fp == prov.Fingerprint {
			return nil
		}
	}
	return libkite.VerifyTree(dir, wantHash)
}

// cacheDir returns the version-addressed cache path for an identity at a revision.
func (r *Resolver) cacheDir(identity, rev string) string {
	ns, name, _ := strings.Cut(identity, "/")
	return filepath.Join(r.mgr.ModulesDir(), ns, name+"@"+rev)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
