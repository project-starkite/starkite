package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/project-starkite/starkite/basekite/manager"
	"github.com/project-starkite/starkite/basekite/resolver"
	"github.com/project-starkite/starkite/libkite"
)

// resolveRunTarget maps a `kite run` argument to the entry file to execute and
// whether it is a module (a directory module or an installed module). A module
// run requires a `main()` entry point; a loose script file does not.
//
// Reference grammar (the Go model — bare references are identities, filesystem
// references require a path prefix):
//
//   - ./path or /abs/path        : a filesystem reference — a directory module
//     (mod.yaml + main.star) or a .star script file.
//   - namespace/name             : an installed module; the newest revision.
//   - namespace/name@rev         : a specific installed revision (id or prefix).
//   - a bare ".star" file or bare directory is an error: filesystem references
//     require the path prefix.
func resolveRunTarget(arg string) (entryPath string, isModule bool, err error) {
	if isPathArg(arg) {
		info, statErr := os.Stat(arg)
		if statErr != nil {
			return "", false, fmt.Errorf("not found: %s", arg)
		}
		// Directory module: the manifest check also verifies the main.star entry.
		if info.IsDir() {
			if _, mErr := libkite.LoadModuleManifest(arg); mErr != nil {
				return "", false, fmt.Errorf("%s is not a runnable module: %w", arg, mErr)
			}
			return filepath.Join(arg, libkite.EntryFile), true, nil
		}
		// Loose script file.
		return arg, false, nil
	}

	if strings.HasSuffix(arg, ".star") {
		return "", false, fmt.Errorf("file reference %q requires a path prefix: use %q", arg, "./"+arg)
	}

	// Installed module identity: namespace/name[@rev].
	if strings.Contains(arg, "/") {
		identity, rev := libkite.SplitModuleRev(arg)
		mgr, mErr := manager.New("")
		if mErr != nil {
			return "", false, mErr
		}
		info, gErr := mgr.Resolve(identity, rev)
		if gErr != nil {
			if rev == "" {
				return "", false, fmt.Errorf("module %q is not installed; install it with `kite module install <source>`, or use ./%s for a local path", identity, arg)
			}
			return "", false, gErr
		}
		entry := filepath.Join(info.Path, libkite.EntryFile)
		if st, sErr := os.Stat(entry); sErr != nil || st.IsDir() {
			return "", false, fmt.Errorf("module %q is a library (no %s) and cannot be run directly", identity, libkite.EntryFile)
		}
		return entry, true, nil
	}

	return "", false, fmt.Errorf("%q is not a runnable reference: use ./%s for a local path, or namespace/name for an installed module", arg, arg)
}

// isPathArg reports whether a run argument is a filesystem path reference.
func isPathArg(arg string) bool {
	return arg == "." || arg == ".." ||
		strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") ||
		filepath.IsAbs(arg)
}

// resolveModuleDeps resolves the dependency closure of a module directory before
// it runs, fetching any declared dependencies into the global cache. A working
// directory the user owns has its mod.lock written (Sync); an immutable cache
// module (an installed namespace/name) is resolved read-only (EnsureClosure).
func resolveModuleDeps(moduleDir string) error {
	mgr, err := manager.New("")
	if err != nil {
		return err
	}
	r := resolver.New(mgr)
	if withinDir(mgr.ModulesDir(), moduleDir) {
		_, err = r.EnsureClosure(moduleDir)
		return err
	}
	_, err = r.Sync(moduleDir)
	return err
}

// resolveLooseDeps resolves the installed modules a loose script file load()s,
// from the cache only, and writes mod.lock beside it. An uninstalled dependency
// is an error.
func resolveLooseDeps(scriptPath string) error {
	mgr, err := manager.New("")
	if err != nil {
		return err
	}
	_, err = resolver.New(mgr).SyncLoose(scriptPath)
	return err
}

// withinDir reports whether path is root or lies inside it.
func withinDir(root, path string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}
