package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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

		// Guardrail: Detect non-Starlark executable files
		if !isStarlarkFile(arg) && isExecutableFile(info, arg) {
			return "", false, fmt.Errorf("target '%s' is not a Starkite script or module.\n  Hint: Use 'kite exec %s' to launch external processes under the sandbox engine.", arg, arg)
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
		strings.HasPrefix(arg, ".\\") || strings.HasPrefix(arg, "..\\") ||
		strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "\\") ||
		filepath.IsAbs(arg) ||
		filepath.VolumeName(arg) != ""
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
	src, err := os.ReadFile(scriptPath)
	if err != nil {
		return err
	}
	refs, err := libkite.LoadTargets(scriptPath, src)
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}
	hasInstalledRef := slices.ContainsFunc(refs, resolver.IsInstalledRef)
	if !hasInstalledRef {
		return nil
	}

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
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

// resolveExecTarget resolves an executable target argument for 'kite exec':
// If it has a path prefix or path separator, it verifies the file exists,
// is not a directory, and has executable permission bits.
// Otherwise, it searches the system $PATH via exec.LookPath.
func resolveExecTarget(target string) (string, error) {
	if isPathArg(target) || strings.Contains(target, "/") || strings.Contains(target, string(filepath.Separator)) {
		info, err := os.Stat(target)
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("target file not found: %s", target)
			}
			return "", fmt.Errorf("failed to access target file %s: %w", target, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("target '%s' is a directory; use 'kite run' for directory modules", target)
		}

		if !isExecutableFile(info, target) {
			return "", fmt.Errorf("target file '%s' is not executable (missing chmod +x)", target)
		}

		cleanPath := filepath.Clean(target)
		if !filepath.IsAbs(cleanPath) {
			cwd, _ := os.Getwd()
			cleanPath = filepath.Join(cwd, cleanPath)
		}
		return cleanPath, nil
	}

	// Bare command lookup in $PATH
	path, err := exec.LookPath(target)
	if err == nil {
		clean := filepath.Clean(path)
		if !filepath.IsAbs(clean) {
			cwd, _ := os.Getwd()
			clean = filepath.Join(cwd, clean)
		}
		return clean, nil
	}

	return "", fmt.Errorf("executable '%s' not found in working directory or $PATH", target)
}

// isExecutableFile checks whether a file has execution permissions on the current OS.
func isExecutableFile(info os.FileInfo, path string) bool {
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		pathext := os.Getenv("PATHEXT")
		if pathext == "" {
			pathext = ".COM;.EXE;.BAT;.CMD;.PS1"
		}
		for _, e := range filepath.SplitList(pathext) {
			if strings.EqualFold(ext, e) {
				return true
			}
		}
		return ext == ".exe" || ext == ".bat" || ext == ".cmd"
	}

	// POSIX executable permission check: owner, group, or other
	return info.Mode()&0111 != 0
}

// isStarlarkFile checks whether a file is intended as a Starlark script.
func isStarlarkFile(path string) bool {
	if strings.HasSuffix(path, ".star") {
		return true
	}
	// Check shebang header for "kite"
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	header := make([]byte, 128)
	n, _ := f.Read(header)
	if n > 2 && header[0] == '#' && header[1] == '!' {
		firstLine := string(header[:n])
		if idx := strings.IndexByte(firstLine, '\n'); idx != -1 {
			firstLine = firstLine[:idx]
		}
		return strings.Contains(strings.ToLower(firstLine), "kite")
	}
	return false
}
