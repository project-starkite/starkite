package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// RuntimeType defines the execution environment type for language-specific profiles.
type RuntimeType string

const (
	RuntimePython RuntimeType = "python"
	RuntimeNode   RuntimeType = "node"
	RuntimeRuby   RuntimeType = "ruby"
)

var shieldedHomePaths = []string{
	".ssh",
	".aws",
	".kube",
	".gnupg",
	".config/gcloud",
	".docker",
	".gitconfig",
}

// isSensitiveHomeDir reports whether target is a protected sensitive directory or
// credential file within the user's home directory.
func isSensitiveHomeDir(target, home string) bool {
	if home == "" || target == "" {
		return false
	}
	cleanHome := filepath.Clean(home)
	if !filepath.IsAbs(cleanHome) {
		if abs, err := filepath.Abs(cleanHome); err == nil {
			cleanHome = abs
		}
	}
	resolvedHome := cleanHome
	if rHome, err := filepath.EvalSymlinks(cleanHome); err == nil && rHome != "" {
		resolvedHome = rHome
	}

	cleanTarget := filepath.Clean(target)
	if !filepath.IsAbs(cleanTarget) {
		if checkSensitivePath(filepath.Join(cleanHome, cleanTarget), cleanHome) ||
			checkSensitivePath(filepath.Join(resolvedHome, cleanTarget), resolvedHome) {
			return true
		}
		if abs, err := filepath.Abs(cleanTarget); err == nil {
			cleanTarget = abs
		}
	}

	if checkSensitivePath(cleanTarget, cleanHome) || checkSensitivePath(cleanTarget, resolvedHome) {
		return true
	}

	if resolved, err := filepath.EvalSymlinks(cleanTarget); err == nil && resolved != cleanTarget {
		if checkSensitivePath(resolved, cleanHome) || checkSensitivePath(resolved, resolvedHome) {
			return true
		}
	}

	return false
}

func checkSensitivePath(target, home string) bool {
	if home == "" || target == "" {
		return false
	}
	cleanTargetSlash := filepath.ToSlash(filepath.Clean(target))
	cleanHomeSlash := filepath.ToSlash(filepath.Clean(home))

	if strings.EqualFold(cleanTargetSlash, cleanHomeSlash) {
		return true
	}

	prefix := cleanHomeSlash
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	var relSlash string
	if len(cleanTargetSlash) >= len(prefix) && strings.EqualFold(cleanTargetSlash[:len(prefix)], prefix) {
		relSlash = cleanTargetSlash[len(prefix):]
	} else {
		rel, err := filepath.Rel(cleanHomeSlash, cleanTargetSlash)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return false
		}
		relSlash = filepath.ToSlash(rel)
	}

	for _, shielded := range shieldedHomePaths {
		shieldedSlash := filepath.ToSlash(shielded)
		if strings.EqualFold(relSlash, shieldedSlash) ||
			strings.HasPrefix(strings.ToLower(relSlash), strings.ToLower(shieldedSlash)+"/") {
			return true
		}
		if strings.HasPrefix(strings.ToLower(shieldedSlash), strings.ToLower(relSlash)+"/") {
			return true
		}
	}
	return false
}

func pythonSearchPaths() []string {
	var paths []string
	switch runtime.GOOS {
	case "darwin":
		candidates := []string{
			"/opt/homebrew/Frameworks/Python.framework",
			"/Library/Frameworks/Python.framework",
			"/System/Library/Frameworks/Python.framework",
		}
		for _, c := range candidates {
			if fi, err := os.Stat(c); err == nil && fi.IsDir() {
				paths = append(paths, c)
			}
		}
		globs := []string{
			"/opt/homebrew/lib/python3*",
			"/opt/homebrew/opt/python@3*/libexec",
			"/usr/local/lib/python3*",
			"/Library/Python/3*",
		}
		for _, g := range globs {
			if matches, err := filepath.Glob(g); err == nil {
				for _, m := range matches {
					if fi, err := os.Stat(m); err == nil && fi.IsDir() {
						paths = append(paths, m)
					}
				}
			}
		}
	case "linux":
		globs := []string{
			"/usr/lib/python3*",
			"/usr/local/lib/python3*",
			"/usr/include/python3*",
		}
		for _, g := range globs {
			if matches, err := filepath.Glob(g); err == nil {
				for _, m := range matches {
					if fi, err := os.Stat(m); err == nil && fi.IsDir() {
						paths = append(paths, m)
					}
				}
			}
		}
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			progPython := filepath.Join(localAppData, "Programs", "Python")
			if fi, err := os.Stat(progPython); err == nil && fi.IsDir() {
				var versionDirs []string
				if entries, err := os.ReadDir(progPython); err == nil {
					for _, e := range entries {
						if e.IsDir() {
							versionDirs = append(versionDirs, filepath.Join(progPython, e.Name()))
						}
					}
				}
				if len(versionDirs) > 0 {
					paths = append(paths, versionDirs...)
				} else {
					paths = append(paths, progPython)
				}
			}
		}
		progFiles := os.Getenv("ProgramFiles")
		if progFiles != "" {
			if matches, err := filepath.Glob(filepath.Join(progFiles, "Python*")); err == nil {
				for _, m := range matches {
					if fi, err := os.Stat(m); err == nil && fi.IsDir() {
						paths = append(paths, m)
					}
				}
			}
		}
	}
	return paths
}

func nodeSearchPaths() []string {
	var paths []string
	switch runtime.GOOS {
	case "darwin", "linux":
		candidates := []string{
			"/usr/local/lib/node_modules",
			"/usr/lib/node_modules",
			"/opt/homebrew/lib/node_modules",
		}
		for _, c := range candidates {
			if fi, err := os.Stat(c); err == nil && fi.IsDir() {
				paths = append(paths, c)
			}
		}
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			npmModules := filepath.Join(appData, "npm", "node_modules")
			if fi, err := os.Stat(npmModules); err == nil && fi.IsDir() {
				paths = append(paths, npmModules)
			}
		}
		progFiles := os.Getenv("ProgramFiles")
		if progFiles != "" {
			nodeDir := filepath.Join(progFiles, "nodejs")
			if fi, err := os.Stat(nodeDir); err == nil && fi.IsDir() {
				paths = append(paths, nodeDir)
			}
		}
	}
	return paths
}

func rubySearchPaths() []string {
	var paths []string
	switch runtime.GOOS {
	case "darwin":
		candidates := []string{
			"/Library/Ruby/Gems",
			"/opt/homebrew/lib/ruby",
			"/System/Library/Frameworks/Ruby.framework",
		}
		for _, c := range candidates {
			if fi, err := os.Stat(c); err == nil && fi.IsDir() {
				paths = append(paths, c)
			}
		}
		if matches, err := filepath.Glob("/Library/Ruby/Gems/*"); err == nil {
			for _, m := range matches {
				if fi, err := os.Stat(m); err == nil && fi.IsDir() {
					paths = append(paths, m)
				}
			}
		}
	case "linux":
		candidates := []string{
			"/usr/lib/ruby",
			"/usr/local/lib/ruby",
			"/var/lib/gems",
		}
		for _, c := range candidates {
			if fi, err := os.Stat(c); err == nil && fi.IsDir() {
				paths = append(paths, c)
			}
		}
		globs := []string{
			"/usr/lib/ruby/gems/*",
			"/var/lib/gems/*",
		}
		for _, g := range globs {
			if matches, err := filepath.Glob(g); err == nil {
				for _, m := range matches {
					if fi, err := os.Stat(m); err == nil && fi.IsDir() {
						paths = append(paths, m)
					}
				}
			}
		}
	case "windows":
		progFiles := os.Getenv("ProgramFiles")
		if progFiles != "" {
			if matches, err := filepath.Glob(filepath.Join(progFiles, "Ruby*")); err == nil {
				for _, m := range matches {
					if fi, err := os.Stat(m); err == nil && fi.IsDir() {
						paths = append(paths, m)
					}
				}
			}
		}
		systemDrive := os.Getenv("SystemDrive")
		if systemDrive == "" {
			systemDrive = "C:"
		}
		if matches, err := filepath.Glob(systemDrive + `\Ruby*`); err == nil {
			for _, m := range matches {
				if fi, err := os.Stat(m); err == nil && fi.IsDir() {
					paths = append(paths, m)
				}
			}
		}
	}
	return paths
}

// ResolveRuntimeEnvMounts discovers virtual environments and language module trees
// for the given RuntimeType and returns read-only bind mounts, shielding sensitive
// user paths under home.
func ResolveRuntimeEnvMounts(rtType RuntimeType, cwd, home string) ([]Mount, error) {
	if rtType == "" {
		return nil, nil
	}

	var candidates []string

	switch rtType {
	case RuntimePython:
		if venv := os.Getenv("VIRTUAL_ENV"); venv != "" {
			candidates = append(candidates, venv)
		}
		if pyPath := os.Getenv("PYTHONPATH"); pyPath != "" {
			for _, p := range filepath.SplitList(pyPath) {
				if p != "" {
					candidates = append(candidates, p)
				}
			}
		}
		if cwd != "" {
			candidates = append(candidates, filepath.Join(cwd, ".venv"))
			candidates = append(candidates, filepath.Join(cwd, "venv"))
		}
		candidates = append(candidates, pythonSearchPaths()...)

	case RuntimeNode:
		if cwd != "" {
			candidates = append(candidates, filepath.Join(cwd, "node_modules"))
		}
		if nodePath := os.Getenv("NODE_PATH"); nodePath != "" {
			for _, p := range filepath.SplitList(nodePath) {
				if p != "" {
					candidates = append(candidates, p)
				}
			}
		}
		if nvmBin := os.Getenv("NVM_BIN"); nvmBin != "" {
			candidates = append(candidates, nvmBin)
			nodeMod := filepath.Join(filepath.Dir(nvmBin), "lib", "node_modules")
			candidates = append(candidates, nodeMod)
		}
		candidates = append(candidates, nodeSearchPaths()...)

	case RuntimeRuby:
		if gemHome := os.Getenv("GEM_HOME"); gemHome != "" {
			candidates = append(candidates, gemHome)
		}
		if gemPath := os.Getenv("GEM_PATH"); gemPath != "" {
			for _, p := range filepath.SplitList(gemPath) {
				if p != "" {
					candidates = append(candidates, p)
				}
			}
		}
		candidates = append(candidates, rubySearchPaths()...)

	default:
		return nil, fmt.Errorf("sandbox: unsupported runtime environment %q", rtType)
	}

	realHome := home
	if home != "" {
		cleanHome := filepath.Clean(home)
		realHome = cleanHome
		if rHome, err := filepath.EvalSymlinks(cleanHome); err == nil && rHome != "" {
			realHome = rHome
		}
	}

	var mounts []Mount
	seen := make(map[string]bool)

	for _, cand := range candidates {
		if cand == "" {
			continue
		}
		clean := filepath.Clean(cand)
		if seen[clean] {
			continue
		}
		if _, err := os.Stat(clean); err != nil {
			continue
		}
		realPath := clean
		if rPath, err := filepath.EvalSymlinks(clean); err == nil && rPath != "" {
			realPath = rPath
		}
		if seen[realPath] {
			continue
		}
		if isSensitiveHomeDir(clean, home) || isSensitiveHomeDir(clean, realHome) ||
			isSensitiveHomeDir(realPath, home) || isSensitiveHomeDir(realPath, realHome) {
			continue
		}
		seen[clean] = true
		seen[realPath] = true
		mounts = append(mounts, Mount{
			Source:      clean,
			Destination: clean,
			Type:        MountBind,
			Mode:        MountRO,
		})
	}

	return mounts, nil
}
