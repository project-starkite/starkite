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
// whether it is a module (a directory module or an installed @namespace/name).
// A module run requires a `main()` entry point; a loose script file does not.
//
// Forms:
//   - @namespace/name : an installed module, resolved from the global cache.
//   - ./dir or dir    : a directory module (mod.yaml + main.star).
//   - file.star       : a loose script file.
func resolveRunTarget(arg string) (entryPath string, isModule bool, err error) {
	// Installed module reference.
	if strings.HasPrefix(arg, "@") {
		ref := strings.TrimPrefix(arg, "@")
		mgr, mErr := manager.New("")
		if mErr != nil {
			return "", false, mErr
		}
		info, gErr := mgr.Get(ref)
		if gErr != nil {
			return "", false, fmt.Errorf("module %q is not installed; install it with: kite module install <source>", ref)
		}
		entry := filepath.Join(info.Path, libkite.EntryFile)
		if st, sErr := os.Stat(entry); sErr != nil || st.IsDir() {
			return "", false, fmt.Errorf("module %q is a library (no %s) and cannot be run directly", ref, libkite.EntryFile)
		}
		return entry, true, nil
	}

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

// resolveModuleDeps resolves the dependency closure of a module directory before
// it runs, fetching any declared dependencies into the global cache. A working
// directory the user owns has its mod.lock written (Sync); an immutable cache
// module (an installed @namespace/name) is resolved read-only (EnsureClosure).
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
